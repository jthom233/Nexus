package gui

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	vnc "github.com/amitbet/vnc2video"
	"github.com/hajimehoshi/ebiten/v2"
)

// VNCSession manages a VNC connection and renders its framebuffer.
type VNCSession struct {
	host     string
	port     int
	password string

	conn   *vnc.ClientConn
	ebiImg *ebiten.Image
	mu     sync.Mutex
	closed bool
	width  int
	height int
	cancel context.CancelFunc

	serverMsgCh chan vnc.ServerMessage
	clientMsgCh chan vnc.ClientMessage
	errorCh     chan error

	lastMouseX int
	lastMouseY int
}

// NewVNCSession creates a new VNC session.
func NewVNCSession(host string, port int, password string) *VNCSession {
	return &VNCSession{
		host:        host,
		port:        port,
		password:    password,
		width:       1024,
		height:      768,
		serverMsgCh: make(chan vnc.ServerMessage, 50),
		clientMsgCh: make(chan vnc.ClientMessage, 50),
		errorCh:     make(chan error, 5),
	}
}

// Connect establishes the VNC connection.
func (v *VNCSession) Connect() error {
	addr := fmt.Sprintf("%s:%d", v.host, v.port)

	nc, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("vnc dial: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel

	secHandlers := []vnc.SecurityHandler{
		&vnc.ClientAuthNone{},
	}
	if v.password != "" {
		secHandlers = []vnc.SecurityHandler{
			&vnc.ClientAuthVNC{Password: []byte(v.password)},
			&vnc.ClientAuthNone{},
		}
	}

	cfg := &vnc.ClientConfig{
		SecurityHandlers: secHandlers,
		DrawCursor:       true,
		PixelFormat:      vnc.PixelFormat32bit,
		ClientMessageCh:  v.clientMsgCh,
		ServerMessageCh:  v.serverMsgCh,
		ErrorCh:          v.errorCh,
		Messages:         vnc.DefaultServerMessages,
		Encodings: []vnc.Encoding{
			&vnc.RawEncoding{},
			&vnc.TightEncoding{},
			&vnc.CopyRectEncoding{},
			&vnc.ZRLEEncoding{},
			&vnc.ZLibEncoding{},
			&vnc.HextileEncoding{},
			&vnc.RREEncoding{},
		},
	}

	conn, err := vnc.Connect(ctx, nc, cfg)
	if err != nil {
		cancel()
		nc.Close()
		return fmt.Errorf("vnc connect: %w", err)
	}

	v.conn = conn
	v.width = int(conn.Width())
	v.height = int(conn.Height())

	// Set encoding target to the canvas
	canvas := conn.Canvas
	if canvas != nil {
		for _, enc := range cfg.Encodings {
			if renderer, ok := enc.(vnc.Renderer); ok {
				renderer.SetTargetImage(canvas)
			}
		}
	}

	// Set preferred encodings
	conn.SetEncodings([]vnc.EncodingType{
		vnc.EncCursorPseudo,
		vnc.EncPointerPosPseudo,
		vnc.EncCopyRect,
		vnc.EncTight,
		vnc.EncZRLE,
		vnc.EncRaw,
	})

	// Request initial framebuffer
	reqMsg := vnc.FramebufferUpdateRequest{Inc: 0, X: 0, Y: 0, Width: conn.Width(), Height: conn.Height()}
	reqMsg.Write(conn)

	// Start message processing loop
	go v.messageLoop(ctx)

	return nil
}

// messageLoop processes server messages and requests framebuffer updates.
func (v *VNCSession) messageLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-v.errorCh:
			if err != nil {
				v.mu.Lock()
				v.closed = true
				v.mu.Unlock()
				return
			}
		case msg := <-v.serverMsgCh:
			if msg.Type() == vnc.FramebufferUpdateMsgType {
				// Request next incremental update
				reqMsg := vnc.FramebufferUpdateRequest{
					Inc: 1, X: 0, Y: 0,
					Width: v.conn.Width(), Height: v.conn.Height(),
				}
				reqMsg.Write(v.conn)
			}
		}
	}
}

// Update refreshes the ebiten image from the VNC canvas.
func (v *VNCSession) Update() {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.conn == nil || v.conn.Canvas == nil {
		return
	}

	canvas := v.conn.Canvas
	if canvas == nil {
		return
	}

	if v.ebiImg == nil {
		v.ebiImg = ebiten.NewImage(v.width, v.height)
	}

	// Convert the canvas image to RGBA pixels
	bounds := canvas.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	pixels := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := canvas.At(x, y).RGBA()
			off := (y*w + x) * 4
			pixels[off] = byte(r >> 8)
			pixels[off+1] = byte(g >> 8)
			pixels[off+2] = byte(b >> 8)
			pixels[off+3] = byte(a >> 8)
		}
	}
	v.ebiImg.WritePixels(pixels)
}

// Framebuffer returns the current rendered image.
func (v *VNCSession) Framebuffer() *ebiten.Image {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.ebiImg
}

// NativeSize returns the VNC session's native resolution.
func (v *VNCSession) NativeSize() (int, int) {
	return v.width, v.height
}

// HandleKeyPress sends a key press event to the VNC server.
func (v *VNCSession) HandleKeyPress(key ebiten.Key) {
	if v.conn == nil {
		return
	}
	keysym := ebitenKeyToVNCKeysym(key)
	if keysym != 0 {
		msg := &vnc.KeyEvent{Down: 1, Key: vnc.Key(keysym)}
		msg.Write(v.conn)
	}
}

// HandleKeyRelease sends a key release event to the VNC server.
func (v *VNCSession) HandleKeyRelease(key ebiten.Key) {
	if v.conn == nil {
		return
	}
	keysym := ebitenKeyToVNCKeysym(key)
	if keysym != 0 {
		msg := &vnc.KeyEvent{Down: 0, Key: vnc.Key(keysym)}
		msg.Write(v.conn)
	}
}

// HandleHookKey is a no-op for VNC — the keyboard hook only captures raw scancodes
// which don't apply to the VNC keysym protocol.
func (v *VNCSession) HandleHookKey(scancode uint16, extended bool, release bool) {}

// HandleMouseMove sends a mouse move event to the VNC server.
func (v *VNCSession) HandleMouseMove(x, y int) {
	if v.conn == nil {
		return
	}
	v.lastMouseX = x
	v.lastMouseY = y
	msg := &vnc.PointerEvent{Mask: 0, X: uint16(x), Y: uint16(y)}
	msg.Write(v.conn)
}

// HandleMouseButton sends a mouse button event to the VNC server.
func (v *VNCSession) HandleMouseButton(button ebiten.MouseButton, pressed bool) {
	if v.conn == nil {
		return
	}
	mask := mouseButtonToVNC(button)
	if !pressed {
		mask = 0
	}
	msg := &vnc.PointerEvent{Mask: mask, X: uint16(v.lastMouseX), Y: uint16(v.lastMouseY)}
	msg.Write(v.conn)
}

// HandleMouseWheel sends a mouse wheel event to the VNC server.
func (v *VNCSession) HandleMouseWheel(dx, dy float64) {
	if v.conn == nil {
		return
	}
	if dy > 0 {
		msg := &vnc.PointerEvent{Mask: 8, X: uint16(v.lastMouseX), Y: uint16(v.lastMouseY)}
		msg.Write(v.conn)
		msg2 := &vnc.PointerEvent{Mask: 0, X: uint16(v.lastMouseX), Y: uint16(v.lastMouseY)}
		msg2.Write(v.conn)
	} else if dy < 0 {
		msg := &vnc.PointerEvent{Mask: 16, X: uint16(v.lastMouseX), Y: uint16(v.lastMouseY)}
		msg.Write(v.conn)
		msg2 := &vnc.PointerEvent{Mask: 0, X: uint16(v.lastMouseX), Y: uint16(v.lastMouseY)}
		msg2.Write(v.conn)
	}
}

// SendCtrlAltDel sends the Ctrl+Alt+Delete key sequence to the VNC server.
func (v *VNCSession) SendCtrlAltDel() {
	if v.conn == nil {
		return
	}
	// X11 keysyms: Control_L=0xFFE3, Alt_L=0xFFE9, Delete=0xFFFF
	keys := []vnc.Key{0xFFE3, 0xFFE9, 0xFFFF}
	// Press all keys
	for _, k := range keys {
		msg := &vnc.KeyEvent{Down: 1, Key: k}
		msg.Write(v.conn)
	}
	// Release all keys in reverse order
	for i := len(keys) - 1; i >= 0; i-- {
		msg := &vnc.KeyEvent{Down: 0, Key: keys[i]}
		msg.Write(v.conn)
	}
}

// Close terminates the VNC session.
func (v *VNCSession) Close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.closed = true
	if v.cancel != nil {
		v.cancel()
	}
	if v.conn != nil {
		v.conn.Close()
	}
}

func mouseButtonToVNC(button ebiten.MouseButton) uint8 {
	switch button {
	case ebiten.MouseButtonLeft:
		return 1
	case ebiten.MouseButtonMiddle:
		return 2
	case ebiten.MouseButtonRight:
		return 4
	default:
		return 0
	}
}
