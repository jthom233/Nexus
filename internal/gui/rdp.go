package gui

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"log"
	"net"
	"strings"
	"sync"
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

func init() {
	glog.SetLogger(log.New(io.Discard, "", 0))
	glog.SetLevel(glog.NONE)
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
	mu     sync.Mutex
	closed bool
	width  int
	height int

	lastMouseX int
	lastMouseY int
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
	}
}

// Connect establishes the RDP connection using grdp protocol layers.
func (r *RDPSession) Connect() error {
	addr := fmt.Sprintf("%s:%d", r.host, r.port)
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("rdp dial: %w", err)
	}

	domain, user := splitDomainUser(r.domain, r.username)

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

	// Register bitmap update callback
	r.pduLayer.On("update", func(data interface{}) {
		bitmaps, ok := data.([]pdu.BitmapData)
		if !ok {
			return
		}
		r.handleBitmapUpdate(bitmaps)
	})

	r.pduLayer.On("close", func(data interface{}) {
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()
	})

	if err := r.x224Layer.Connect(); err != nil {
		return fmt.Errorf("rdp connect: %w", err)
	}

	return nil
}

// handleBitmapUpdate processes bitmap rectangles from the RDP server.
func (r *RDPSession) handleBitmapUpdate(bitmaps []pdu.BitmapData) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, bmp := range bitmaps {
		data := bmp.BitmapDataStream
		compressed := bmp.IsCompress()
		if compressed {
			data = core.Decompress(data, int(bmp.Width), int(bmp.Height), bppToBytes(bmp.BitsPerPixel))
		}
		if data == nil {
			continue
		}
		// Decompressed data is top-down; uncompressed RDP data is bottom-up.
		// Decompressed 16-bit data uses big-endian byte order.
		img := decodeBitmapData(data, int(bmp.Width), int(bmp.Height), int(bmp.BitsPerPixel), !compressed)
		if img == nil {
			continue
		}
		destRect := image.Rect(int(bmp.DestLeft), int(bmp.DestTop), int(bmp.DestRight)+1, int(bmp.DestBottom)+1)
		draw.Draw(r.fb, destRect, img, image.Point{}, draw.Src)
	}
}

// Update processes pending framebuffer changes.
func (r *RDPSession) Update() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ebiImg == nil {
		r.ebiImg = ebiten.NewImage(r.width, r.height)
	}
	r.ebiImg.WritePixels(r.fb.Pix)
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
	switch button {
	case ebiten.MouseButtonLeft:
		p.PointerFlags |= pdu.PTRFLAGS_BUTTON1
	case ebiten.MouseButtonRight:
		p.PointerFlags |= pdu.PTRFLAGS_BUTTON2
	case ebiten.MouseButtonMiddle:
		p.PointerFlags |= pdu.PTRFLAGS_BUTTON3
	}
	p.XPos = uint16(r.lastMouseX)
	p.YPos = uint16(r.lastMouseY)
	r.pduLayer.SendInputEvents(pdu.INPUT_EVENT_MOUSE, []pdu.InputEventsInterface{p})
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

// Close terminates the RDP session.
func (r *RDPSession) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	if r.clipboard != nil {
		r.clipboard.stop()
	}
	if r.tpktLayer != nil {
		r.tpktLayer.Close()
	}
}

func splitDomainUser(domain, user string) (string, string) {
	if domain != "" {
		return domain, user
	}
	if strings.Contains(user, "\\") {
		parts := strings.SplitN(user, "\\", 2)
		return parts[0], parts[1]
	}
	if strings.Contains(user, "/") {
		parts := strings.SplitN(user, "/", 2)
		return parts[0], parts[1]
	}
	return "", user
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
func decodeBitmapData(data []byte, w, h, bpp int, bottomUp bool) image.Image {
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

		for x := 0; x < w; x++ {
			switch bpp {
			case 32:
				offset := rowOffset + x*4
				if offset+3 >= len(data) {
					return img
				}
				img.SetRGBA(x, destY, color.RGBA{
					B: data[offset],
					G: data[offset+1],
					R: data[offset+2],
					A: 255,
				})
			case 24:
				offset := rowOffset + x*3
				if offset+2 >= len(data) {
					return img
				}
				img.SetRGBA(x, destY, color.RGBA{
					B: data[offset],
					G: data[offset+1],
					R: data[offset+2],
					A: 255,
				})
			case 16:
				offset := rowOffset + x*2
				if offset+1 >= len(data) {
					return img
				}
				var pixel uint16
				if bottomUp {
					pixel = uint16(data[offset]) | uint16(data[offset+1])<<8 // LE
				} else {
					pixel = uint16(data[offset])<<8 | uint16(data[offset+1]) // BE
				}
				r := uint8((pixel >> 11) & 0x1F)
				g := uint8((pixel >> 5) & 0x3F)
				b := uint8(pixel & 0x1F)
				img.SetRGBA(x, destY, color.RGBA{R: r << 3, G: g << 2, B: b << 3, A: 255})
			case 15:
				offset := rowOffset + x*2
				if offset+1 >= len(data) {
					return img
				}
				var pixel uint16
				if bottomUp {
					pixel = uint16(data[offset]) | uint16(data[offset+1])<<8
				} else {
					pixel = uint16(data[offset])<<8 | uint16(data[offset+1])
				}
				r := uint8((pixel >> 10) & 0x1F)
				g := uint8((pixel >> 5) & 0x1F)
				b := uint8(pixel & 0x1F)
				img.SetRGBA(x, destY, color.RGBA{R: r << 3, G: g << 3, B: b << 3, A: 255})
			}
		}
	}

	return img
}
