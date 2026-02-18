package gui

import (
	"fmt"
	"image"
	"image/draw"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/tomatome/grdp/core"
	"github.com/tomatome/grdp/glog"
	"github.com/tomatome/grdp/plugin"
	"github.com/tomatome/grdp/protocol/nla"
	"github.com/tomatome/grdp/protocol/pdu"
	"github.com/tomatome/grdp/protocol/sec"
	"github.com/tomatome/grdp/protocol/t125"
	"github.com/tomatome/grdp/protocol/tpkt"
	"github.com/tomatome/grdp/protocol/x224"
)

// rdpLog is a dedicated logger that writes to %TEMP%/nexus-rdp-debug.log.
var rdpLog *log.Logger

func init() {
	logPath := os.TempDir() + "/nexus-rdp-debug.log"
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		rdpLog = log.New(os.Stderr, "[rdp] ", log.LstdFlags|log.Lmicroseconds)
	} else {
		rdpLog = log.New(f, "[rdp] ", log.LstdFlags|log.Lmicroseconds)
	}
	rdpLog.Printf("RDP diagnostics started — log: %s", logPath)

	// ERROR level only — avoids logging credentials that appear at DEBUG/INFO.
	glog.SetLogger(log.New(f, "[gordp] ", log.LstdFlags|log.Lmicroseconds))
	glog.SetLevel(glog.ERROR)
}

// RDPSession manages an RDP connection and renders its framebuffer.
type RDPSession struct {
	host     string
	port     int
	username string
	password string
	domain   string
	options  map[string]interface{}

	tpktLayer *tpkt.TPKT
	x224Layer *x224.X224
	mcsLayer  *t125.MCSClient
	secLayer  *sec.Client
	pduLayer  *pdu.Client
	channels  *plugin.Channels
	clipboard *clipboardChannel

	fb     *image.RGBA
	ebiImg *ebiten.Image
	mu        sync.Mutex
	closed    bool
	dirty     bool             // true if framebuffer has pending changes
	dirtyRect image.Rectangle  // bounding box of changed region
	width     int
	height int

	lastMouseX int
	lastMouseY int

	// asyncErr receives the first protocol-level error after Connect() returns
	// (e.g. NLA auth failure). Buffered so the sender never blocks.
	asyncErr chan error
	// doneCh is closed when the session ends (Close() or server-initiated close).
	doneCh   chan struct{}
	doneOnce sync.Once

	// diagnostics
	bitmapCallbacks atomic.Int64 // total "update" events received
	bitmapDecoded   atomic.Int64 // bitmaps successfully decoded
	bitmapDropped   atomic.Int64 // bitmaps dropped (nil data or decode failure)
}

// NewRDPSession creates a new RDP session.
func NewRDPSession(host string, port int, username, password, domain string, options map[string]interface{}) *RDPSession {
	w, h := 1024, 768
	if options != nil {
		if res, ok := options["resolution"].(string); ok && res != "" {
			fmt.Sscanf(res, "%dx%d", &w, &h)
		}
	}
	return &RDPSession{
		host:     host,
		port:     port,
		username: username,
		password: password,
		domain:   domain,
		options:  options,
		width:    w,
		height:   h,
		fb:       image.NewRGBA(image.Rect(0, 0, w, h)),
		asyncErr: make(chan error, 1),
		doneCh:   make(chan struct{}),
	}
}

// Connect establishes the RDP connection using grdp protocol layers.
func (r *RDPSession) Connect() error {
	addr := fmt.Sprintf("%s:%d", r.host, r.port)
	rdpLog.Printf("CONNECT start — target=%s resolution=%dx%d", addr, r.width, r.height)

	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		rdpLog.Printf("CONNECT tcp-dial FAILED after %v: %v", time.Since(start), err)
		return fmt.Errorf("rdp dial: %w", err)
	}
	rdpLog.Printf("CONNECT tcp-dial OK in %v — local=%s remote=%s", time.Since(start), conn.LocalAddr(), conn.RemoteAddr())

	domain, user := splitDomainUser(r.domain, r.username)
	rdpLog.Printf("CONNECT auth — domain=%q user=%q hasPassword=%v", domain, user, r.password != "")

	r.tpktLayer = tpkt.New(core.NewSocketLayer(conn), nla.NewNTLMv2(domain, user, r.password))
	r.x224Layer = x224.New(r.tpktLayer)
	r.mcsLayer = t125.NewMCSClient(r.x224Layer)
	r.secLayer = sec.NewClient(r.mcsLayer)
	r.pduLayer = pdu.NewClient(r.secLayer)
	r.channels = plugin.NewChannels(r.secLayer)

	r.mcsLayer.SetClientCoreData(uint16(r.width), uint16(r.height))

	r.secLayer.SetUser(user)
	r.secLayer.SetPwd(r.password)
	r.secLayer.SetDomain(domain)

	r.tpktLayer.SetFastPathListener(r.secLayer)
	r.secLayer.SetFastPathListener(r.pduLayer)
	r.secLayer.SetChannelSender(r.mcsLayer)
	r.channels.SetChannelSender(r.secLayer)
	r.clipboard = newClipboardChannel()
	r.channels.Register(r.clipboard)

	// "update" is emitted with one arg: []pdu.BitmapData
	r.pduLayer.On("update", func(data interface{}) {
		n := r.bitmapCallbacks.Add(1)
		bitmaps, ok := data.([]pdu.BitmapData)
		if !ok {
			rdpLog.Printf("UPDATE event #%d — unexpected type %T", n, data)
			return
		}
		rdpLog.Printf("UPDATE event #%d — %d bitmap(s)", n, len(bitmaps))
		r.handleBitmapUpdate(bitmaps)
	})

	// "close" is emitted with zero args — must use func() not func(interface{})
	r.pduLayer.On("close", func() {
		rdpLog.Printf("SESSION closed by server — callbacks=%d decoded=%d dropped=%d",
			r.bitmapCallbacks.Load(), r.bitmapDecoded.Load(), r.bitmapDropped.Load())
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()
		r.doneOnce.Do(func() { close(r.doneCh) })
	})

	// "error" is emitted with one arg: the error value
	r.pduLayer.On("error", func(data interface{}) {
		var err error
		switch v := data.(type) {
		case error:
			err = v
		case string:
			err = fmt.Errorf("%s", v)
		default:
			err = fmt.Errorf("rdp protocol error: %v", data)
		}
		rdpLog.Printf("PDU error event — %v (callbacks=%d decoded=%d dropped=%d)",
			err, r.bitmapCallbacks.Load(), r.bitmapDecoded.Load(), r.bitmapDropped.Load())
		select {
		case r.asyncErr <- err:
		default:
		}
	})

	rdpLog.Printf("CONNECT handshake starting...")
	hsStart := time.Now()
	if err := r.x224Layer.Connect(); err != nil {
		rdpLog.Printf("CONNECT handshake FAILED after %v: %v", time.Since(hsStart), err)
		return fmt.Errorf("rdp connect: %w", err)
	}
	rdpLog.Printf("CONNECT handshake OK in %v — NLA auth completes asynchronously", time.Since(hsStart))

	return nil
}

// handleBitmapUpdate processes bitmap rectangles from the RDP server.
func (r *RDPSession) handleBitmapUpdate(bitmaps []pdu.BitmapData) {
	// Phase 1: Decompress and decode WITHOUT holding the mutex.
	// This is the expensive work — do it lock-free.
	type decodedBmp struct {
		img  *image.RGBA
		rect image.Rectangle
	}
	decoded := make([]decodedBmp, 0, len(bitmaps))

	for i, bmp := range bitmaps {
		data := bmp.BitmapDataStream
		if bmp.IsCompress() {
			bpp := bppToBytes(bmp.BitsPerPixel)
			if bpp == 0 {
				rdpLog.Printf("  DECODE[%d] SKIP — unsupported bpp=%d (bppToBytes=0)", i, bmp.BitsPerPixel)
				r.bitmapDropped.Add(1)
				continue
			}
			data = core.Decompress(data, int(bmp.Width), int(bmp.Height), bpp)
			if data == nil {
				rdpLog.Printf("  DECODE[%d] SKIP — Decompress returned nil (bpp=%d w=%d h=%d srcLen=%d)",
					i, bmp.BitsPerPixel, bmp.Width, bmp.Height, len(bmp.BitmapDataStream))
				r.bitmapDropped.Add(1)
				continue
			}
			rdpLog.Printf("  DECODE[%d] decompress OK — bpp=%d w=%d h=%d srcLen=%d dstLen=%d",
				i, bmp.BitsPerPixel, bmp.Width, bmp.Height, len(bmp.BitmapDataStream), len(data))
		}
		if data == nil {
			rdpLog.Printf("  DECODE[%d] SKIP — nil data (uncompressed, bpp=%d)", i, bmp.BitsPerPixel)
			r.bitmapDropped.Add(1)
			continue
		}
		img := decodeBitmapData(data, int(bmp.Width), int(bmp.Height), int(bmp.BitsPerPixel), !bmp.IsCompress())
		if img == nil {
			rdpLog.Printf("  DECODE[%d] SKIP — decodeBitmapData returned nil (bpp=%d w=%d h=%d)",
				i, bmp.BitsPerPixel, bmp.Width, bmp.Height)
			r.bitmapDropped.Add(1)
			continue
		}
		destRect := image.Rect(int(bmp.DestLeft), int(bmp.DestTop), int(bmp.DestRight)+1, int(bmp.DestBottom)+1)
		rdpLog.Printf("  DECODE[%d] OK — blit to %v", i, destRect)
		r.bitmapDecoded.Add(1)
		decoded = append(decoded, decodedBmp{img: img, rect: destRect})
	}

	if len(decoded) == 0 {
		rdpLog.Printf("  handleBitmapUpdate: all %d bitmap(s) dropped (total dropped=%d)",
			len(bitmaps), r.bitmapDropped.Load())
		return
	}

	// Phase 2: Brief lock to blit onto framebuffer and mark dirty.
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, d := range decoded {
		draw.Draw(r.fb, d.rect, d.img, image.Point{}, draw.Src)
		if r.dirtyRect.Empty() {
			r.dirtyRect = d.rect
		} else {
			r.dirtyRect = r.dirtyRect.Union(d.rect)
		}
	}
	r.dirty = true
	rdpLog.Printf("  framebuffer updated — %d rect(s) blitted, dirtyRect=%v", len(decoded), r.dirtyRect)
}

// Update processes pending framebuffer changes.
func (r *RDPSession) Update() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ebiImg == nil {
		rdpLog.Printf("UPDATE ebiImg created — %dx%d", r.width, r.height)
		r.ebiImg = ebiten.NewImage(r.width, r.height)
	}

	if !r.dirty {
		return
	}

	// Clamp dirty rect to framebuffer bounds.
	rect := r.dirtyRect.Intersect(image.Rect(0, 0, r.width, r.height))
	rdpLog.Printf("UPDATE WritePixels — dirtyRect=%v clampedRect=%v", r.dirtyRect, rect)
	if !rect.Empty() {
		w := rect.Dx()
		h := rect.Dy()
		buf := make([]byte, 4*w*h)
		for y := 0; y < h; y++ {
			srcOff := (rect.Min.Y+y)*r.fb.Stride + rect.Min.X*4
			dstOff := y * w * 4
			copy(buf[dstOff:dstOff+w*4], r.fb.Pix[srcOff:srcOff+w*4])
		}
		sub := r.ebiImg.SubImage(rect).(*ebiten.Image)
		sub.WritePixels(buf)
		rdpLog.Printf("UPDATE WritePixels OK — %d bytes to rect=%v", len(buf), rect)
	} else {
		rdpLog.Printf("UPDATE WritePixels SKIP — clamped rect is empty (fb=%dx%d dirtyRect=%v)", r.width, r.height, r.dirtyRect)
	}

	r.dirty = false
	r.dirtyRect = image.Rectangle{}
}

// Framebuffer returns the current rendered image.
func (r *RDPSession) Framebuffer() *ebiten.Image {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ebiImg
}

// NativeSize returns the RDP session's native resolution.
func (r *RDPSession) NativeSize() (int, int) {
	return r.width, r.height
}

// HandleKeyPress sends a key press event to the RDP server.
func (r *RDPSession) HandleKeyPress(key ebiten.Key) {
	if r.pduLayer == nil {
		return
	}
	sc := ebitenKeyToScancode(key)
	if sc != 0 {
		p := &pdu.ScancodeKeyEvent{}
		p.KeyCode = sc
		if isExtendedScancode(key) {
			p.KeyboardFlags |= pdu.KBDFLAGS_EXTENDED
		}
		r.pduLayer.SendInputEvents(pdu.INPUT_EVENT_SCANCODE, []pdu.InputEventsInterface{p})
	}
}

// HandleKeyRelease sends a key release event to the RDP server.
func (r *RDPSession) HandleKeyRelease(key ebiten.Key) {
	if r.pduLayer == nil {
		return
	}
	sc := ebitenKeyToScancode(key)
	if sc != 0 {
		p := &pdu.ScancodeKeyEvent{}
		p.KeyCode = sc
		p.KeyboardFlags |= pdu.KBDFLAGS_RELEASE
		if isExtendedScancode(key) {
			p.KeyboardFlags |= pdu.KBDFLAGS_EXTENDED
		}
		r.pduLayer.SendInputEvents(pdu.INPUT_EVENT_SCANCODE, []pdu.InputEventsInterface{p})
	}
}

// HandleHookKey sends a raw scancode from the keyboard hook to the RDP server.
func (r *RDPSession) HandleHookKey(scancode uint16, extended bool, release bool) {
	if r.pduLayer == nil {
		return
	}
	p := &pdu.ScancodeKeyEvent{}
	p.KeyCode = scancode
	if extended {
		p.KeyboardFlags |= pdu.KBDFLAGS_EXTENDED
	}
	if release {
		p.KeyboardFlags |= pdu.KBDFLAGS_RELEASE
	}
	r.pduLayer.SendInputEvents(pdu.INPUT_EVENT_SCANCODE, []pdu.InputEventsInterface{p})
}

// HandleMouseMove sends a mouse move event to the RDP server.
func (r *RDPSession) HandleMouseMove(x, y int) {
	if r.pduLayer == nil {
		return
	}
	r.lastMouseX = x
	r.lastMouseY = y
	p := &pdu.PointerEvent{}
	p.PointerFlags |= pdu.PTRFLAGS_MOVE
	p.XPos = uint16(x)
	p.YPos = uint16(y)
	r.pduLayer.SendInputEvents(pdu.INPUT_EVENT_MOUSE, []pdu.InputEventsInterface{p})
}

// HandleMouseButton sends a mouse button event to the RDP server.
func (r *RDPSession) HandleMouseButton(button ebiten.MouseButton, pressed bool) {
	if r.pduLayer == nil {
		return
	}
	p := &pdu.PointerEvent{}
	if pressed {
		p.PointerFlags |= pdu.PTRFLAGS_DOWN
	}
	p.XPos = uint16(r.lastMouseX)
	p.YPos = uint16(r.lastMouseY)

	// Back/forward buttons use extended mouse event type
	switch button {
	case ebiten.MouseButtonLeft:
		p.PointerFlags |= pdu.PTRFLAGS_BUTTON1
		r.pduLayer.SendInputEvents(pdu.INPUT_EVENT_MOUSE, []pdu.InputEventsInterface{p})
	case ebiten.MouseButtonRight:
		p.PointerFlags |= pdu.PTRFLAGS_BUTTON2
		r.pduLayer.SendInputEvents(pdu.INPUT_EVENT_MOUSE, []pdu.InputEventsInterface{p})
	case ebiten.MouseButtonMiddle:
		p.PointerFlags |= pdu.PTRFLAGS_BUTTON3
		r.pduLayer.SendInputEvents(pdu.INPUT_EVENT_MOUSE, []pdu.InputEventsInterface{p})
	case ebiten.MouseButton3: // Back
		p.PointerFlags |= pdu.PTRFLAGS_BUTTON1
		r.pduLayer.SendInputEvents(pdu.INPUT_EVENT_MOUSEX, []pdu.InputEventsInterface{p})
	case ebiten.MouseButton4: // Forward
		p.PointerFlags |= pdu.PTRFLAGS_BUTTON2
		r.pduLayer.SendInputEvents(pdu.INPUT_EVENT_MOUSEX, []pdu.InputEventsInterface{p})
	}
}

// HandleMouseWheel sends a mouse wheel event to the RDP server.
func (r *RDPSession) HandleMouseWheel(dx, dy float64) {
	if r.pduLayer == nil {
		return
	}
	// Vertical wheel
	if dy != 0 {
		p := &pdu.PointerEvent{}
		p.PointerFlags |= pdu.PTRFLAGS_WHEEL
		if dy < 0 {
			p.PointerFlags |= pdu.PTRFLAGS_WHEEL_NEGATIVE
		}
		// Encode rotation magnitude into the lower 9 bits.
		// Ebiten reports fractional deltas; map to RDP click units (120 per notch).
		step := int(dy)
		if step == 0 {
			step = 1
		}
		if step < 0 {
			step = -step
		}
		p.PointerFlags |= uint16(step*120) & pdu.WheelRotationMask
		p.XPos = uint16(r.lastMouseX)
		p.YPos = uint16(r.lastMouseY)
		r.pduLayer.SendInputEvents(pdu.INPUT_EVENT_MOUSE, []pdu.InputEventsInterface{p})
	}
	// Horizontal wheel
	if dx != 0 {
		p := &pdu.PointerEvent{}
		p.PointerFlags |= pdu.PTRFLAGS_HWHEEL
		if dx < 0 {
			p.PointerFlags |= pdu.PTRFLAGS_WHEEL_NEGATIVE
		}
		step := int(dx)
		if step == 0 {
			step = 1
		}
		if step < 0 {
			step = -step
		}
		p.PointerFlags |= uint16(step*120) & pdu.WheelRotationMask
		p.XPos = uint16(r.lastMouseX)
		p.YPos = uint16(r.lastMouseY)
		r.pduLayer.SendInputEvents(pdu.INPUT_EVENT_MOUSE, []pdu.InputEventsInterface{p})
	}
}

// SendCtrlAltDel sends the Ctrl+Alt+Delete key sequence to the RDP server.
func (r *RDPSession) SendCtrlAltDel() {
	if r.pduLayer == nil {
		return
	}
	// Scancodes: Ctrl=0x1D, Alt=0x38, Delete=0x53
	keys := []uint16{0x1D, 0x38, 0x53}
	// Press all keys
	for _, sc := range keys {
		p := &pdu.ScancodeKeyEvent{}
		p.KeyCode = sc
		r.pduLayer.SendInputEvents(pdu.INPUT_EVENT_SCANCODE, []pdu.InputEventsInterface{p})
	}
	// Release all keys in reverse order
	for i := len(keys) - 1; i >= 0; i-- {
		p := &pdu.ScancodeKeyEvent{}
		p.KeyCode = keys[i]
		p.KeyboardFlags |= pdu.KBDFLAGS_RELEASE
		r.pduLayer.SendInputEvents(pdu.INPUT_EVENT_SCANCODE, []pdu.InputEventsInterface{p})
	}
}

// SendCtrlShiftEsc sends the Ctrl+Shift+Escape key sequence to open Task Manager.
func (r *RDPSession) SendCtrlShiftEsc() {
	if r.pduLayer == nil {
		return
	}
	// Scancodes: Ctrl=0x1D, Shift=0x2A, Escape=0x01
	keys := []uint16{0x1D, 0x2A, 0x01}
	for _, sc := range keys {
		p := &pdu.ScancodeKeyEvent{}
		p.KeyCode = sc
		r.pduLayer.SendInputEvents(pdu.INPUT_EVENT_SCANCODE, []pdu.InputEventsInterface{p})
	}
	for i := len(keys) - 1; i >= 0; i-- {
		p := &pdu.ScancodeKeyEvent{}
		p.KeyCode = keys[i]
		p.KeyboardFlags |= pdu.KBDFLAGS_RELEASE
		r.pduLayer.SendInputEvents(pdu.INPUT_EVENT_SCANCODE, []pdu.InputEventsInterface{p})
	}
}

// Close terminates the RDP session.
func (r *RDPSession) Close() {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	r.doneOnce.Do(func() { close(r.doneCh) })
	if r.clipboard != nil {
		r.clipboard.stop()
	}
	if r.tpktLayer != nil {
		r.tpktLayer.Close()
	}
}

// done returns a channel that is closed when the session ends.
func (r *RDPSession) done() <-chan struct{} {
	return r.doneCh
}

func splitDomainUser(domain, user string) (string, string) {
	// Strip any embedded domain prefix from the username regardless of source,
	// since NTLMv2 wants domain and bare username as separate fields.
	if idx := strings.Index(user, "\\"); idx >= 0 {
		if domain == "" {
			domain = user[:idx]
		}
		user = user[idx+1:]
	} else if idx := strings.Index(user, "/"); idx >= 0 {
		if domain == "" {
			domain = user[:idx]
		}
		user = user[idx+1:]
	}
	return domain, user
}

func bppToBytes(bpp uint16) int {
	switch bpp {
	case 15:
		return 1
	case 16:
		return 2
	case 24:
		return 3
	case 32:
		return 4
	default:
		return 0
	}
}

// decodeBitmapData converts raw pixel data to a Go image.
// bottomUp indicates the data is in bottom-up row order (uncompressed RDP data).
// When false, data is top-down (decompressed data).
func decodeBitmapData(data []byte, w, h, bpp int, bottomUp bool) *image.RGBA {
	if w <= 0 || h <= 0 {
		return nil
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bytesPerPixel := bppToBytes(uint16(bpp))

	// Stride: decompressed data is tightly packed (w * bytesPerPixel).
	// Uncompressed RDP data has scanlines padded to 4-byte boundaries.
	stride := w * bytesPerPixel
	if bottomUp {
		stride = ((w*bytesPerPixel + 3) / 4) * 4
	}

	for y := 0; y < h; y++ {
		destY := y
		if bottomUp {
			destY = h - 1 - y
		}
		rowOffset := y * stride
		dstRowOffset := destY * img.Stride

		switch bpp {
		case 32:
			for x := 0; x < w; x++ {
				src := rowOffset + x*4
				if src+3 >= len(data) {
					return img
				}
				dst := dstRowOffset + x*4
				img.Pix[dst+0] = data[src+2] // R <- B
				img.Pix[dst+1] = data[src+1] // G
				img.Pix[dst+2] = data[src+0] // B <- R
				img.Pix[dst+3] = 255         // A
			}
		case 24:
			for x := 0; x < w; x++ {
				src := rowOffset + x*3
				if src+2 >= len(data) {
					return img
				}
				dst := dstRowOffset + x*4
				img.Pix[dst+0] = data[src+2] // R <- B
				img.Pix[dst+1] = data[src+1] // G
				img.Pix[dst+2] = data[src+0] // B <- R
				img.Pix[dst+3] = 255         // A
			}
		case 16:
			for x := 0; x < w; x++ {
				src := rowOffset + x*2
				if src+1 >= len(data) {
					return img
				}
				var pixel uint16
				if bottomUp {
					pixel = uint16(data[src]) | uint16(data[src+1])<<8
				} else {
					pixel = uint16(data[src])<<8 | uint16(data[src+1])
				}
				r := uint8((pixel >> 11) & 0x1F)
				g := uint8((pixel >> 5) & 0x3F)
				b := uint8(pixel & 0x1F)
				dst := dstRowOffset + x*4
				img.Pix[dst+0] = r << 3
				img.Pix[dst+1] = g << 2
				img.Pix[dst+2] = b << 3
				img.Pix[dst+3] = 255
			}
		case 15:
			for x := 0; x < w; x++ {
				src := rowOffset + x*2
				if src+1 >= len(data) {
					return img
				}
				var pixel uint16
				if bottomUp {
					pixel = uint16(data[src]) | uint16(data[src+1])<<8
				} else {
					pixel = uint16(data[src])<<8 | uint16(data[src+1])
				}
				r := uint8((pixel >> 10) & 0x1F)
				g := uint8((pixel >> 5) & 0x1F)
				b := uint8(pixel & 0x1F)
				dst := dstRowOffset + x*4
				img.Pix[dst+0] = r << 3
				img.Pix[dst+1] = g << 3
				img.Pix[dst+2] = b << 3
				img.Pix[dst+3] = 255
			}
		}
	}

	return img
}
