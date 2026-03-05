//go:build linux && cgo

package gui

/*
#cgo pkg-config: freerdp2 winpr2
#include <freerdp/freerdp.h>
#include <freerdp/gdi/gdi.h>
#include <freerdp/channels/channels.h>
#include <freerdp/client/cmdline.h>
#include <freerdp/input.h>
#include <freerdp/locale/keyboard.h>
#include <stdlib.h>
#include <string.h>

// Forward declarations for Go callbacks registered as C function pointers.
extern BOOL goFreeRDPPreConnect(freerdp* instance);
extern BOOL goFreeRDPPostConnect(freerdp* instance);
extern void goFreeRDPPostDisconnect(freerdp* instance);
extern void goFreeRDPEndPaint(rdpContext* context);

// cSetSettings configures the freerdp settings struct from Go strings.
// All char* fields are duplicated (owned by freerdp); caller must free the
// Go-side C strings it allocates before calling this function.
static void cSetSettings(freerdp* inst,
	const char* host, UINT32 port,
	const char* user, const char* pass, const char* domain,
	UINT32 width, UINT32 height) {
	rdpSettings* s = inst->settings;
	freerdp_settings_set_string(s, FreeRDP_ServerHostname, host);
	freerdp_settings_set_uint32(s, FreeRDP_ServerPort, port);
	freerdp_settings_set_string(s, FreeRDP_Username, user);
	freerdp_settings_set_string(s, FreeRDP_Password, pass);
	freerdp_settings_set_string(s, FreeRDP_Domain, domain);
	freerdp_settings_set_uint32(s, FreeRDP_DesktopWidth, width);
	freerdp_settings_set_uint32(s, FreeRDP_DesktopHeight, height);
	freerdp_settings_set_uint32(s, FreeRDP_ColorDepth, 32);

	// Enable modern codec pipeline.
	freerdp_settings_set_bool(s, FreeRDP_SupportGraphicsPipeline, TRUE);
	freerdp_settings_set_bool(s, FreeRDP_GfxH264, TRUE);
	freerdp_settings_set_bool(s, FreeRDP_GfxAVC444, FALSE);
	freerdp_settings_set_bool(s, FreeRDP_NSCodec, TRUE);
	freerdp_settings_set_bool(s, FreeRDP_RemoteFxCodec, TRUE);

	// Certificate / TLS: accept any server cert for now (same behaviour as
	// most RDP clients when no cert store is configured).
	freerdp_settings_set_bool(s, FreeRDP_IgnoreCertificate, TRUE);
}

// cSetSecurityNLA configures NLA-only security.
static void cSetSecurityNLA(freerdp* inst) {
	rdpSettings* s = inst->settings;
	freerdp_settings_set_bool(s, FreeRDP_NlaSecurity, TRUE);
	freerdp_settings_set_bool(s, FreeRDP_TlsSecurity, FALSE);
	freerdp_settings_set_bool(s, FreeRDP_RdpSecurity, FALSE);
}

// cSetSecurityTLS configures TLS-only security.
static void cSetSecurityTLS(freerdp* inst) {
	rdpSettings* s = inst->settings;
	freerdp_settings_set_bool(s, FreeRDP_NlaSecurity, FALSE);
	freerdp_settings_set_bool(s, FreeRDP_TlsSecurity, TRUE);
	freerdp_settings_set_bool(s, FreeRDP_RdpSecurity, FALSE);
}

// cSetSecurityRDP configures classic RDP security.
static void cSetSecurityRDP(freerdp* inst) {
	rdpSettings* s = inst->settings;
	freerdp_settings_set_bool(s, FreeRDP_NlaSecurity, FALSE);
	freerdp_settings_set_bool(s, FreeRDP_TlsSecurity, FALSE);
	freerdp_settings_set_bool(s, FreeRDP_RdpSecurity, TRUE);
}

// cRegisterCallbacks wires the Go callbacks into the instance.
static void cRegisterCallbacks(freerdp* inst) {
	inst->PreConnect     = goFreeRDPPreConnect;
	inst->PostConnect    = goFreeRDPPostConnect;
	inst->PostDisconnect = goFreeRDPPostDisconnect;
}

// cRegisterEndPaint wires the EndPaint callback on the update object.
static void cRegisterEndPaint(rdpContext* ctx) {
	ctx->update->EndPaint = goFreeRDPEndPaint;
}

// cGDIWidth/Height — safe accessors (avoids C struct navigation in Go).
static UINT32 cGDIWidth(rdpContext* ctx)  { return ctx->gdi->width; }
static UINT32 cGDIHeight(rdpContext* ctx) { return ctx->gdi->height; }

// cGDIBuffer returns a pointer to the primary GDI buffer.
static BYTE* cGDIBuffer(rdpContext* ctx) { return ctx->gdi->primary_buffer; }

// cGDIInvalidRect returns the dirty rectangle from the GDI primary surface.
static void cGDIInvalidRect(rdpContext* ctx,
		INT32* x, INT32* y, UINT32* w, UINT32* h) {
	HGDI_RGN inv = ctx->gdi->primary->hdc->hwnd->invalid;
	*x = inv->x;
	*y = inv->y;
	*w = (UINT32)inv->w;
	*h = (UINT32)inv->h;
}

// cResetInvalidRect marks the GDI invalid region as empty.
static void cResetInvalidRect(rdpContext* ctx) {
	ctx->gdi->primary->hdc->hwnd->invalid->null = 1;
}

// cInstanceContext returns the rdpContext for a freerdp instance.
static rdpContext* cInstanceContext(freerdp* inst) { return inst->context; }

// cCheckEventHandles drives the FreeRDP message loop (one iteration).
static BOOL cCheckEventHandles(freerdp* inst) {
	return freerdp_check_event_handles(inst->context);
}

// Input helpers — avoid direct struct access from Go.
static void cSendKeyEvent(freerdp* inst, UINT16 flags, UINT16 code) {
	freerdp_input_send_keyboard_event(inst->input, flags, code);
}

static void cSendMouseEvent(freerdp* inst, UINT16 flags, UINT16 x, UINT16 y) {
	freerdp_input_send_mouse_event(inst->input, flags, x, y);
}
*/
import "C"

import (
	"fmt"
	"image"
	"image/draw"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/hajimehoshi/ebiten/v2"
)

// instanceRegistry maps a freerdp pointer (as uintptr) to its Go wrapper so
// that C callbacks can reach the right FreeRDPSession without storing Go
// pointers inside C memory.
var (
	instanceMu       sync.RWMutex
	instanceRegistry = map[uintptr]*FreeRDPSession{}
)

func registerInstance(inst *C.freerdp, s *FreeRDPSession) {
	instanceMu.Lock()
	defer instanceMu.Unlock()
	instanceRegistry[uintptr(unsafe.Pointer(inst))] = s
}

func unregisterInstance(inst *C.freerdp) {
	instanceMu.Lock()
	defer instanceMu.Unlock()
	delete(instanceRegistry, uintptr(unsafe.Pointer(inst)))
}

func lookupInstance(inst *C.freerdp) (*FreeRDPSession, bool) {
	instanceMu.RLock()
	defer instanceMu.RUnlock()
	s, ok := instanceRegistry[uintptr(unsafe.Pointer(inst))]
	return s, ok
}

// contextRegistry indexes sessions by their rdpContext pointer so EndPaint
// (which only receives an rdpContext*) can find the session.
var (
	contextMu       sync.RWMutex
	contextRegistry = map[uintptr]*FreeRDPSession{}
)

func registerContext(ctx *C.rdpContext, s *FreeRDPSession) {
	contextMu.Lock()
	defer contextMu.Unlock()
	contextRegistry[uintptr(unsafe.Pointer(ctx))] = s
}

func unregisterContext(ctx *C.rdpContext) {
	contextMu.Lock()
	defer contextMu.Unlock()
	delete(contextRegistry, uintptr(unsafe.Pointer(ctx)))
}

func lookupContext(ctx *C.rdpContext) (*FreeRDPSession, bool) {
	contextMu.RLock()
	defer contextMu.RUnlock()
	s, ok := contextRegistry[uintptr(unsafe.Pointer(ctx))]
	return s, ok
}

// FreeRDPSession is a Linux/CGo RDP session backed by the FreeRDP library.
// It implements the Session interface and renders into an Ebiten framebuffer.
type FreeRDPSession struct {
	host     string
	port     int
	username string
	password string
	domain   string
	options  map[string]interface{}
	security string

	width  int
	height int

	inst *C.freerdp // owned; freed in Close()

	fb        *image.RGBA    // CPU-side framebuffer (written by callback, read by Update)
	ebiImg    *ebiten.Image  // GPU-side image (written by Update, drawn by Framebuffer)
	mu        sync.Mutex     // guards fb, ebiImg, dirty, dirtyRect, closed
	dirty     bool
	dirtyRect image.Rectangle
	closed    bool

	lastMouseX int
	lastMouseY int

	doneCh   chan struct{}
	doneOnce sync.Once
}

// NewFreeRDPSession creates a new FreeRDP-backed RDP session.
func NewFreeRDPSession(host string, port int, username, password, domain string, options map[string]interface{}) *FreeRDPSession {
	w, h := 1024, 768
	security := ""
	if options != nil {
		if res, ok := options["resolution"].(string); ok && res != "" {
			fmt.Sscanf(res, "%dx%d", &w, &h)
		}
		if s, ok := options["security"].(string); ok {
			security = strings.ToLower(strings.TrimSpace(s))
		}
	}
	return &FreeRDPSession{
		host:     host,
		port:     port,
		username: username,
		password: password,
		domain:   domain,
		options:  options,
		security: security,
		width:    w,
		height:   h,
		fb:       image.NewRGBA(image.Rect(0, 0, w, h)),
		doneCh:   make(chan struct{}),
	}
}

// Connect establishes the FreeRDP connection and starts the event loop goroutine.
func (f *FreeRDPSession) Connect() error {
	addr := fmt.Sprintf("%s:%d", f.host, f.port)
	rdpLog.Printf("FREERDP CONNECT start — target=%s resolution=%dx%d security=%s",
		addr, f.width, f.height, f.security)

	inst := C.freerdp_new()
	if inst == nil {
		return fmt.Errorf("freerdp_new failed")
	}
	f.inst = inst

	// Register in global maps BEFORE wiring callbacks (callbacks may fire during connect).
	registerInstance(inst, f)

	// Allocate and populate settings using safe C helpers.
	cHost := C.CString(f.host)
	defer C.free(unsafe.Pointer(cHost))
	cUser := C.CString(f.username)
	defer C.free(unsafe.Pointer(cUser))
	cPass := C.CString(f.password)
	defer C.free(unsafe.Pointer(cPass))
	cDomain := C.CString(f.domain)
	defer C.free(unsafe.Pointer(cDomain))

	C.cSetSettings(inst,
		cHost, C.UINT32(f.port),
		cUser, cPass, cDomain,
		C.UINT32(f.width), C.UINT32(f.height),
	)

	switch f.security {
	case "nla":
		rdpLog.Printf("FREERDP security=nla")
		C.cSetSecurityNLA(inst)
	case "tls":
		rdpLog.Printf("FREERDP security=tls")
		C.cSetSecurityTLS(inst)
	case "rdp":
		rdpLog.Printf("FREERDP security=rdp")
		C.cSetSecurityRDP(inst)
	default:
		rdpLog.Printf("FREERDP security=auto (negotiate)")
	}

	// Wire Go callbacks into the C struct.
	C.cRegisterCallbacks(inst)

	rdpLog.Printf("FREERDP calling freerdp_connect...")
	if C.freerdp_connect(inst) == 0 {
		code := C.freerdp_get_last_error(C.cInstanceContext(inst))
		unregisterInstance(inst)
		C.freerdp_free(inst)
		f.inst = nil
		return fmt.Errorf("freerdp_connect failed (error 0x%08X)", uint32(code))
	}
	rdpLog.Printf("FREERDP connected successfully")

	// Start the event-loop goroutine.
	go f.eventLoop()
	return nil
}

// eventLoop drives the FreeRDP message pump. It must run on a locked OS thread
// because FreeRDP uses thread-local SSL/OpenSSL state.
func (f *FreeRDPSession) eventLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	rdpLog.Printf("FREERDP event loop started")
	for {
		select {
		case <-f.doneCh:
			rdpLog.Printf("FREERDP event loop: done signal received, exiting")
			return
		default:
		}

		f.mu.Lock()
		closed := f.closed
		inst := f.inst
		f.mu.Unlock()
		if closed || inst == nil {
			rdpLog.Printf("FREERDP event loop: session closed, exiting")
			return
		}

		if C.cCheckEventHandles(inst) == 0 {
			code := C.freerdp_get_last_error(C.cInstanceContext(inst))
			if code != 0 {
				rdpLog.Printf("FREERDP event loop: freerdp_check_event_handles failed (error 0x%08X)", uint32(code))
			}
			// Connection dropped — mark closed and exit.
			f.mu.Lock()
			f.closed = true
			f.mu.Unlock()
			f.doneOnce.Do(func() { close(f.doneCh) })
			return
		}
	}
}

// endPaintCallback is called from Go (exported to C via cgo export) when the
// FreeRDP GDI EndPaint fires. It copies the dirty region from the C GDI
// primary buffer into the Go framebuffer.
//
//export goFreeRDPEndPaint
func goFreeRDPEndPaint(ctx *C.rdpContext) {
	sess, ok := lookupContext(ctx)
	if !ok {
		return
	}

	// Read the dirty rectangle.
	var cx, cy C.INT32
	var cw, ch C.UINT32
	C.cGDIInvalidRect(ctx, &cx, &cy, &cw, &ch)
	if cw == 0 || ch == 0 {
		return
	}

	x := int(cx)
	y := int(cy)
	w := int(cw)
	h := int(ch)

	// Clamp to framebuffer bounds.
	fbW := int(C.cGDIWidth(ctx))
	fbH := int(C.cGDIHeight(ctx))
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > fbW {
		w = fbW - x
	}
	if y+h > fbH {
		h = fbH - y
	}
	if w <= 0 || h <= 0 {
		return
	}

	// Get the C GDI primary buffer (BGRA32 format).
	cBuf := C.cGDIBuffer(ctx)
	if cBuf == nil {
		return
	}

	// Phase 1 (lock-free): copy dirty rectangle from C buffer into a temporary
	// Go slice, converting BGRA → RGBA.
	srcStride := fbW * 4
	tmp := make([]byte, w*h*4)
	cBufSlice := unsafe.Slice((*byte)(unsafe.Pointer(cBuf)), fbW*fbH*4)

	for row := 0; row < h; row++ {
		srcOff := (y+row)*srcStride + x*4
		dstOff := row * w * 4
		for col := 0; col < w; col++ {
			b := cBufSlice[srcOff+col*4+0]
			g := cBufSlice[srcOff+col*4+1]
			r := cBufSlice[srcOff+col*4+2]
			// alpha from C buffer; set opaque if zero (pre-multiplied / unused)
			a := cBufSlice[srcOff+col*4+3]
			if a == 0 {
				a = 255
			}
			tmp[dstOff+col*4+0] = r
			tmp[dstOff+col*4+1] = g
			tmp[dstOff+col*4+2] = b
			tmp[dstOff+col*4+3] = a
		}
	}

	// Reset the invalid region in C so FreeRDP doesn't re-fire for the same area.
	C.cResetInvalidRect(ctx)

	// Phase 2 (brief lock): blit into the Go framebuffer and mark dirty.
	destRect := image.Rect(x, y, x+w, y+h)
	srcImg := &image.RGBA{
		Pix:    tmp,
		Stride: w * 4,
		Rect:   image.Rect(0, 0, w, h),
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if sess.fb == nil {
		return
	}
	draw.Draw(sess.fb, destRect, srcImg, image.Point{}, draw.Src)
	if sess.dirtyRect.Empty() {
		sess.dirtyRect = destRect
	} else {
		sess.dirtyRect = sess.dirtyRect.Union(destRect)
	}
	sess.dirty = true
	rdpLog.Printf("FREERDP EndPaint — rect=%v", destRect)
}

// preConnectCallback is called by FreeRDP before the connection handshake.
//
//export goFreeRDPPreConnect
func goFreeRDPPreConnect(inst *C.freerdp) C.BOOL {
	rdpLog.Printf("FREERDP PreConnect")
	return C.TRUE
}

// postConnectCallback is called by FreeRDP after the connection is established.
// It initialises the GDI subsystem and registers the EndPaint hook.
//
//export goFreeRDPPostConnect
func goFreeRDPPostConnect(inst *C.freerdp) C.BOOL {
	rdpLog.Printf("FREERDP PostConnect — initialising GDI")
	ctx := C.cInstanceContext(inst)
	if C.gdi_init(inst, C.PIXEL_FORMAT_BGRA32) == 0 {
		rdpLog.Printf("FREERDP PostConnect: gdi_init failed")
		return C.FALSE
	}
	// Register context → session mapping so EndPaint can find us.
	sess, ok := lookupInstance(inst)
	if !ok {
		rdpLog.Printf("FREERDP PostConnect: no session for instance")
		return C.FALSE
	}
	registerContext(ctx, sess)
	// Wire EndPaint after GDI is ready.
	C.cRegisterEndPaint(ctx)
	rdpLog.Printf("FREERDP PostConnect OK — GDI %dx%d", C.cGDIWidth(ctx), C.cGDIHeight(ctx))
	return C.TRUE
}

// postDisconnectCallback is called by FreeRDP after the session closes.
//
//export goFreeRDPPostDisconnect
func goFreeRDPPostDisconnect(inst *C.freerdp) {
	rdpLog.Printf("FREERDP PostDisconnect")
	ctx := C.cInstanceContext(inst)
	if ctx != nil {
		unregisterContext(ctx)
		C.gdi_free(inst)
	}
	sess, ok := lookupInstance(inst)
	if ok {
		sess.mu.Lock()
		sess.closed = true
		sess.mu.Unlock()
		sess.doneOnce.Do(func() { close(sess.doneCh) })
	}
}

// Update copies the dirty framebuffer region to the Ebiten GPU image.
// Must be called from the Ebiten game loop thread.
func (f *FreeRDPSession) Update() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ebiImg == nil {
		rdpLog.Printf("FREERDP UPDATE ebiImg created — %dx%d", f.width, f.height)
		f.ebiImg = ebiten.NewImage(f.width, f.height)
	}

	if !f.dirty {
		return
	}

	rect := f.dirtyRect.Intersect(image.Rect(0, 0, f.width, f.height))
	rdpLog.Printf("FREERDP UPDATE WritePixels — dirtyRect=%v clampedRect=%v", f.dirtyRect, rect)
	if !rect.Empty() {
		w := rect.Dx()
		h := rect.Dy()
		buf := make([]byte, 4*w*h)
		for y := 0; y < h; y++ {
			srcOff := (rect.Min.Y+y)*f.fb.Stride + rect.Min.X*4
			dstOff := y * w * 4
			copy(buf[dstOff:dstOff+w*4], f.fb.Pix[srcOff:srcOff+w*4])
		}
		sub := f.ebiImg.SubImage(rect).(*ebiten.Image)
		sub.WritePixels(buf)
		rdpLog.Printf("FREERDP UPDATE WritePixels OK — %d bytes to rect=%v", len(buf), rect)
	}

	f.dirty = false
	f.dirtyRect = image.Rectangle{}
}

// Framebuffer returns the current GPU image.
func (f *FreeRDPSession) Framebuffer() *ebiten.Image {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ebiImg
}

// NativeSize returns the session's configured resolution.
func (f *FreeRDPSession) NativeSize() (int, int) {
	return f.width, f.height
}

// OwnsWindow always returns false — Ebiten renders the framebuffer.
func (f *FreeRDPSession) OwnsWindow() bool { return false }

// Show is a no-op for framebuffer-based sessions.
func (f *FreeRDPSession) Show() {}

// Hide is a no-op for framebuffer-based sessions.
func (f *FreeRDPSession) Hide() {}

// HandleKeyPress sends a key-down scancode event to the RDP server.
func (f *FreeRDPSession) HandleKeyPress(key ebiten.Key) {
	if f.inst == nil {
		return
	}
	sc := ebitenKeyToScancode(key)
	if sc == 0 {
		return
	}
	var flags C.UINT16 = 0
	if isExtendedScancode(key) {
		flags |= C.KBD_FLAGS_EXTENDED
	}
	C.cSendKeyEvent(f.inst, flags, C.UINT16(sc))
}

// HandleKeyRelease sends a key-up scancode event to the RDP server.
func (f *FreeRDPSession) HandleKeyRelease(key ebiten.Key) {
	if f.inst == nil {
		return
	}
	sc := ebitenKeyToScancode(key)
	if sc == 0 {
		return
	}
	flags := C.UINT16(C.KBD_FLAGS_RELEASE)
	if isExtendedScancode(key) {
		flags |= C.KBD_FLAGS_EXTENDED
	}
	C.cSendKeyEvent(f.inst, flags, C.UINT16(sc))
}

// HandleHookKey sends a raw scancode from the keyboard hook to the RDP server.
func (f *FreeRDPSession) HandleHookKey(scancode uint16, extended bool, release bool) {
	if f.inst == nil {
		return
	}
	var flags C.UINT16 = 0
	if extended {
		flags |= C.KBD_FLAGS_EXTENDED
	}
	if release {
		flags |= C.KBD_FLAGS_RELEASE
	}
	C.cSendKeyEvent(f.inst, flags, C.UINT16(scancode))
}

// HandleMouseMove sends a mouse movement event to the RDP server.
func (f *FreeRDPSession) HandleMouseMove(x, y int) {
	if f.inst == nil {
		return
	}
	f.lastMouseX = x
	f.lastMouseY = y
	C.cSendMouseEvent(f.inst, C.PTR_FLAGS_MOVE, C.UINT16(x), C.UINT16(y))
}

// HandleMouseButton sends a mouse button press/release to the RDP server.
func (f *FreeRDPSession) HandleMouseButton(button ebiten.MouseButton, pressed bool) {
	if f.inst == nil {
		return
	}
	var flags C.UINT16
	if pressed {
		flags |= C.PTR_FLAGS_DOWN
	}
	switch button {
	case ebiten.MouseButtonLeft:
		flags |= C.PTR_FLAGS_BUTTON1
	case ebiten.MouseButtonRight:
		flags |= C.PTR_FLAGS_BUTTON2
	case ebiten.MouseButtonMiddle:
		flags |= C.PTR_FLAGS_BUTTON3
	default:
		return
	}
	C.cSendMouseEvent(f.inst, flags, C.UINT16(f.lastMouseX), C.UINT16(f.lastMouseY))
}

// HandleMouseWheel sends a mouse wheel event to the RDP server.
func (f *FreeRDPSession) HandleMouseWheel(dx, dy float64) {
	if f.inst == nil {
		return
	}
	// Vertical wheel.
	if dy != 0 {
		flags := C.UINT16(C.PTR_FLAGS_WHEEL)
		if dy < 0 {
			flags |= C.PTR_FLAGS_WHEEL_NEGATIVE
		}
		step := int(dy)
		if step < 0 {
			step = -step
		}
		if step == 0 {
			step = 1
		}
		rotation := C.UINT16(step*120) & C.UINT16(C.WheelRotationMask)
		flags |= rotation
		C.cSendMouseEvent(f.inst, flags, C.UINT16(f.lastMouseX), C.UINT16(f.lastMouseY))
	}
	// Horizontal wheel.
	if dx != 0 {
		flags := C.UINT16(C.PTR_FLAGS_HWHEEL)
		if dx < 0 {
			flags |= C.PTR_FLAGS_WHEEL_NEGATIVE
		}
		step := int(dx)
		if step < 0 {
			step = -step
		}
		if step == 0 {
			step = 1
		}
		rotation := C.UINT16(step*120) & C.UINT16(C.WheelRotationMask)
		flags |= rotation
		C.cSendMouseEvent(f.inst, flags, C.UINT16(f.lastMouseX), C.UINT16(f.lastMouseY))
	}
}

// SendCtrlAltDel sends the Ctrl+Alt+Delete key sequence to the RDP server.
func (f *FreeRDPSession) SendCtrlAltDel() {
	if f.inst == nil {
		return
	}
	// Scancodes: Ctrl=0x1D, Alt=0x38, Delete=0x53
	keys := []C.UINT16{0x1D, 0x38, 0x53}
	for _, sc := range keys {
		C.cSendKeyEvent(f.inst, 0, sc)
	}
	for i := len(keys) - 1; i >= 0; i-- {
		C.cSendKeyEvent(f.inst, C.KBD_FLAGS_RELEASE, keys[i])
	}
}

// SendCtrlShiftEsc sends the Ctrl+Shift+Escape key sequence to the RDP server.
func (f *FreeRDPSession) SendCtrlShiftEsc() {
	if f.inst == nil {
		return
	}
	// Scancodes: Ctrl=0x1D, Shift=0x2A, Escape=0x01
	keys := []C.UINT16{0x1D, 0x2A, 0x01}
	for _, sc := range keys {
		C.cSendKeyEvent(f.inst, 0, sc)
	}
	for i := len(keys) - 1; i >= 0; i-- {
		C.cSendKeyEvent(f.inst, C.KBD_FLAGS_RELEASE, keys[i])
	}
}

// Close disconnects the FreeRDP session and frees resources.
func (f *FreeRDPSession) Close() {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	f.closed = true
	f.mu.Unlock()

	f.doneOnce.Do(func() { close(f.doneCh) })

	if f.inst != nil {
		rdpLog.Printf("FREERDP Close — disconnecting")
		C.freerdp_disconnect(f.inst)
		unregisterInstance(f.inst)
		C.freerdp_free(f.inst)
		f.inst = nil
		rdpLog.Printf("FREERDP Close — done")
	}
}
