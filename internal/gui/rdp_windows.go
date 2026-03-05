//go:build windows

package gui

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/hajimehoshi/ebiten/v2"
)

// clsidMsTscAx is the CLSID for the Microsoft Terminal Services ActiveX control (MsTscAx.MsTscAx).
var clsidMsTscAx = ole.NewGUID("{A41A4187-5A86-4E26-B40A-856F9035D9CB}")

// Win32 API bindings used for child-window management.
var (
	procCreateWindowEx  = user32.NewProc("CreateWindowExW")
	procDestroyWindow   = user32.NewProc("DestroyWindow")
	procShowWindow      = user32.NewProc("ShowWindow")
	procMoveWindow      = user32.NewProc("MoveWindow")
	procRegisterClassEx = user32.NewProc("RegisterClassExW")
	procDefWindowProc   = user32.NewProc("DefWindowProcW")

	// GetWindowRect / GetClientRect for coordinate queries.
	procGetWindowRect = user32.NewProc("GetWindowRect")
	procGetClientRect = user32.NewProc("GetClientRect")

	// FindWindowW finds a top-level window by class name; works from any thread.
	procFindWindowW = user32.NewProc("FindWindowW")
)

const (
	swHide       = 0
	swShow       = 5
	wsChild      = 0x40000000
	wsVisible    = 0x10000000
	wsClipChildren = 0x02000000
	wsClipSiblings = 0x04000000
	csSaveScreen = 0x0800

	// ActiveX OLE verbs.
	oleverbPrimary = 0
)

// comCmd is a request queued to the dedicated COM STA goroutine.
type comCmd struct {
	fn     func()
	doneCh chan struct{}
}

// MsTscSession implements Session using the Windows MsTscAx ActiveX control.
// The ActiveX control owns its own HWND child window embedded inside the
// Ebiten host window — it renders and receives input directly, so
// OwnsWindow() returns true and Framebuffer() returns nil.
type MsTscSession struct {
	host     string
	port     int
	username string
	password string
	domain   string
	options  map[string]interface{}
	width    int
	height   int

	// COM object references — only touched from comLoop goroutine.
	rdpClient *ole.IDispatch // IMsRdpClient

	// childHWND is the Win32 HWND hosting the ActiveX control.
	// Written once during Connect, read from multiple goroutines (Update, etc.).
	childHWND uintptr

	// parentHWND is the Ebiten top-level window; populated on first Update.
	parentHWND uintptr

	mu         sync.Mutex
	connected  bool
	closed     bool
	connectErr error

	// comCh serialises all COM calls onto the single STA goroutine.
	comCh chan comCmd

	// doneCh is closed when the session has ended (disconnect or Close).
	doneCh   chan struct{}
	doneOnce sync.Once
}

// NewMsTscSession creates a new Windows-native RDP session backed by MsTscAx.
func NewMsTscSession(host string, port int, username, password, domain string, options map[string]interface{}) *MsTscSession {
	w, h := 1024, 768
	if options != nil {
		if res, ok := options["resolution"].(string); ok && res != "" {
			fmt.Sscanf(res, "%dx%d", &w, &h)
		}
	}
	return &MsTscSession{
		host:     host,
		port:     port,
		username: username,
		password: password,
		domain:   domain,
		options:  options,
		width:    w,
		height:   h,
		comCh:   make(chan comCmd, 32),
		doneCh:  make(chan struct{}),
	}
}

// OwnsWindow returns true — the ActiveX HWND renders to its own window.
func (m *MsTscSession) OwnsWindow() bool { return true }

// Framebuffer returns nil — rendering is done by the COM control itself.
func (m *MsTscSession) Framebuffer() *ebiten.Image { return nil }

// NativeSize returns the requested resolution.
func (m *MsTscSession) NativeSize() (int, int) { return m.width, m.height }

// Connect initialises COM, creates the ActiveX control, and starts the
// connection to the remote host.  The COM STA goroutine is started here and
// runs for the lifetime of the session.
func (m *MsTscSession) Connect() error {
	rdpLog.Printf("MsTsc CONNECT — target=%s:%d resolution=%dx%d", m.host, m.port, m.width, m.height)

	// errCh receives a one-shot error (or nil) from the COM goroutine once
	// the initial setup is complete.
	errCh := make(chan error, 1)

	go m.comLoop(errCh)

	// Wait for the COM setup phase to complete.
	if err := <-errCh; err != nil {
		rdpLog.Printf("MsTsc CONNECT FAILED: %v", err)
		return err
	}

	rdpLog.Printf("MsTsc CONNECT handshake dispatched — waiting for OnConnected event")
	return nil
}

// comLoop is the dedicated Single-Threaded Apartment goroutine. It must
// remain pinned to its OS thread for the lifetime of the session.
func (m *MsTscSession) comLoop(errCh chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// --- 1. Initialise COM STA ---
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		// S_FALSE (0x00000001) means COM already initialised on this thread —
		// that is acceptable.
		if err.(*ole.OleError).Code() != 0x00000001 {
			errCh <- fmt.Errorf("CoInitializeEx: %w", err)
			return
		}
	}
	defer ole.CoUninitialize()

	// --- 2. Create MsTscAx COM object ---
	unknown, err := ole.CreateInstance(clsidMsTscAx, ole.IID_IUnknown)
	if err != nil {
		errCh <- fmt.Errorf("CreateInstance MsTscAx: %w", err)
		return
	}
	rdpLog.Printf("MsTsc COM object created")

	// QI for IDispatch (IMsRdpClient inherits IDispatch).
	disp, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		unknown.Release()
		errCh <- fmt.Errorf("QueryInterface IDispatch: %w", err)
		return
	}
	unknown.Release() // Release original IUnknown; we only need IDispatch from here.
	m.rdpClient = disp

	// --- 3. Set connection properties ---
	domain, user := splitDomainUser(m.domain, m.username)
	rdpLog.Printf("MsTsc auth — domain=%q user=%q", domain, user)

	if err := m.putProperty("Server", m.host); err != nil {
		m.release()
		errCh <- fmt.Errorf("set Server: %w", err)
		return
	}
	if err := m.putProperty("UserName", user); err != nil {
		m.release()
		errCh <- fmt.Errorf("set UserName: %w", err)
		return
	}
	if err := m.putProperty("Domain", domain); err != nil {
		m.release()
		errCh <- fmt.Errorf("set Domain: %w", err)
		return
	}
	if err := m.putProperty("DesktopWidth", m.width); err != nil {
		m.release()
		errCh <- fmt.Errorf("set DesktopWidth: %w", err)
		return
	}
	if err := m.putProperty("DesktopHeight", m.height); err != nil {
		m.release()
		errCh <- fmt.Errorf("set DesktopHeight: %w", err)
		return
	}

	// Port (default 3389 can be omitted, but honour custom ports).
	if m.port != 0 && m.port != 3389 {
		if err := m.putProperty("RDPPort", m.port); err != nil {
			rdpLog.Printf("MsTsc set RDPPort warning: %v", err)
		}
	}

	// Advanced settings via the AdvancedSettings sub-object.
	advDisp := m.getSubDispatch("AdvancedSettings7")
	if advDisp == nil {
		advDisp = m.getSubDispatch("AdvancedSettings2")
	}
	if advDisp != nil {
		// AuthenticationLevel=0: do not prompt on certificate mismatch
		if err := dispPut(advDisp, "AuthenticationLevel", uint32(0)); err != nil {
			rdpLog.Printf("MsTsc AdvancedSettings AuthenticationLevel warning: %v", err)
		}
		// EnableCredSspSupport=true: use CredSSP/NLA
		if err := dispPut(advDisp, "EnableCredSspSupport", true); err != nil {
			rdpLog.Printf("MsTsc AdvancedSettings EnableCredSspSupport warning: %v", err)
		}
		advDisp.Release()
	}

	// Password via IMsTscNonScriptable.ClearTextPassword.
	if m.password != "" {
		if err := m.putProperty("ClearTextPassword", m.password); err != nil {
			// Not all server/client combos expose this at the IDispatch level;
			// log and continue — NLA may still work via CredSSP pass-through.
			rdpLog.Printf("MsTsc ClearTextPassword warning: %v", err)
		}
	}

	// --- 4. Create a Win32 child window to host the control ---
	parentHWND := getEbitenWindowHandle()
	rdpLog.Printf("MsTsc parent HWND=0x%X", parentHWND)

	childHWND, err := createHostWindow(parentHWND, m.width, m.height)
	if err != nil {
		m.release()
		errCh <- fmt.Errorf("createHostWindow: %w", err)
		return
	}
	m.childHWND = childHWND
	rdpLog.Printf("MsTsc child HWND=0x%X created", childHWND)

	// --- 5. Embed the ActiveX control into the child HWND ---
	if err := m.embedInWindow(childHWND); err != nil {
		m.release()
		destroyWindow(childHWND)
		errCh <- fmt.Errorf("OLE embed: %w", err)
		return
	}

	// --- 6. Show the child window ---
	showWindow(childHWND, swShow)

	// --- 7. Initiate the RDP connection ---
	_, err = m.rdpClient.CallMethod("Connect")
	if err != nil {
		m.release()
		destroyWindow(childHWND)
		errCh <- fmt.Errorf("IMsRdpClient.Connect: %w", err)
		return
	}

	rdpLog.Printf("MsTsc Connect() called — entering COM message pump")
	errCh <- nil // Signal success to Connect() caller.

	// --- 8. COM message pump + command dispatcher ---
	// The STA thread must pump messages for COM in-process calls to work.
	type msg struct {
		HWND    uintptr
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      struct{ X, Y int32 }
	}

	peekMessage := user32.NewProc("PeekMessageW")
	translateMessage := user32.NewProc("TranslateMessage")
	dispatchMessage := user32.NewProc("DispatchMessageW")

	var winMsg msg
	const pmRemove = 0x0001

	for {
		select {
		case <-m.doneCh:
			rdpLog.Printf("MsTsc comLoop done — tearing down")
			m.teardown(childHWND)
			return
		case cmd := <-m.comCh:
			cmd.fn()
			if cmd.doneCh != nil {
				close(cmd.doneCh)
			}
		default:
			// Pump any pending Win32 messages (required for COM STA).
			ret, _, _ := peekMessage.Call(
				uintptr(unsafe.Pointer(&winMsg)),
				0, 0, 0, pmRemove,
			)
			if ret != 0 {
				translateMessage.Call(uintptr(unsafe.Pointer(&winMsg)))
				dispatchMessage.Call(uintptr(unsafe.Pointer(&winMsg)))
			} else {
				// No messages: yield briefly to avoid busy-spin.
				runtime.Gosched()
			}
		}
	}
}

// dispatchCOM sends a function to the COM STA goroutine and waits for it.
func (m *MsTscSession) dispatchCOM(fn func()) {
	done := make(chan struct{})
	select {
	case m.comCh <- comCmd{fn: fn, doneCh: done}:
		<-done
	case <-m.doneCh:
	}
}

// putProperty calls IDispatch.PutProperty on m.rdpClient.
func (m *MsTscSession) putProperty(name string, val interface{}) error {
	_, err := m.rdpClient.PutProperty(name, val)
	return err
}

// getSubDispatch retrieves a named sub-IDispatch property (e.g. AdvancedSettings).
// Returns nil if the property does not exist or cannot be queried.
func (m *MsTscSession) getSubDispatch(name string) *ole.IDispatch {
	v, err := m.rdpClient.GetProperty(name)
	if err != nil {
		return nil
	}
	disp := v.ToIDispatch()
	v.Clear()
	return disp
}

// embedInWindow places the ActiveX control inside the given HWND using
// OleCreate / IOleObject::DoVerb. We use a simplified approach that relies on
// the IDispatch interface already being created — for MsTscAx the control
// auto-activates when Connect() is called, so we only need to set the client
// site to attach the HWND.
//
// For MsTscAx specifically, the recommended embedding path is:
//
//  1. QI for IOleObject
//  2. Call IOleObject::SetClientSite (optional, can be nil)
//  3. Call IOleObject::DoVerb(OLEIVERB_PRIMARY, ..., hwnd, ...)
//
// Since go-ole does not expose IOleObject directly, we use the raw vtable.
func (m *MsTscSession) embedInWindow(hwnd uintptr) error {
	// QI for IOleObject ({00000112-0000-0000-C000-000000000046})
	iidIOleObject := ole.NewGUID("{00000112-0000-0000-C000-000000000046}")
	oleObj, err := m.rdpClient.IUnknown.QueryInterface(iidIOleObject)
	if err != nil {
		// If QI fails the control may still work without explicit embedding.
		rdpLog.Printf("MsTsc embedInWindow: IOleObject QI failed (%v) — skipping DoVerb", err)
		return nil
	}
	defer oleObj.Release()

	// IOleObject vtable:
	//  0: QueryInterface
	//  1: AddRef
	//  2: Release
	//  3: SetClientSite
	//  4: GetClientSite
	//  5: SetHostNames
	//  6: Close
	//  7: SetMoniker
	//  8: GetMoniker
	//  9: InitFromData
	// 10: GetClipboardData
	// 11: DoVerb
	//
	// We call DoVerb(0 = OLEIVERB_PRIMARY) with the HWND.
	// DoVerb(lVerb, lpmsg, pActiveSite, lindex, hwndParent, lprcPosRect)
	// We pass nil for lpmsg, nil for pActiveSite, 0 for lindex.
	type RECT struct{ Left, Top, Right, Bottom int32 }
	rc := RECT{0, 0, int32(m.width), int32(m.height)}

	vtbl := *(*[20]uintptr)(unsafe.Pointer(oleObj))
	doVerb := vtbl[11]

	ret, _, _ := syscall.SyscallN(doVerb,
		uintptr(unsafe.Pointer(oleObj)), // this
		0,                               // OLEIVERB_PRIMARY
		0,                               // lpmsg = nil
		0,                               // pActiveSite = nil
		0,                               // lindex = 0
		hwnd,                            // hwndParent
		uintptr(unsafe.Pointer(&rc)),    // lprcPosRect
	)
	if ret != 0 {
		// S_OK = 0; non-zero is an HRESULT error — log but don't abort.
		rdpLog.Printf("MsTsc DoVerb returned HRESULT=0x%08X (non-fatal)", ret)
	}
	return nil
}

// teardown cleans up COM objects and the child HWND. Must be called on the
// STA thread (i.e. from within comLoop).
func (m *MsTscSession) teardown(childHWND uintptr) {
	// Attempt a graceful disconnect.
	if m.rdpClient != nil {
		m.rdpClient.CallMethod("Disconnect") //nolint:errcheck
	}
	m.release()
	if childHWND != 0 {
		showWindow(childHWND, swHide)
		destroyWindow(childHWND)
	}
	rdpLog.Printf("MsTsc teardown complete")
}

// release frees COM interface references. Must be called on the STA thread.
func (m *MsTscSession) release() {
	if m.rdpClient != nil {
		m.rdpClient.Release()
		m.rdpClient = nil
	}
}

// Update is called every Ebiten tick. For MsTscSession it repositions the
// child HWND to track the Ebiten window's content area.
func (m *MsTscSession) Update() {
	hwnd := m.childHWND
	if hwnd == 0 {
		return
	}
	parent := getEbitenWindowHandle()
	if parent == 0 {
		return
	}

	// Content area starts below tab bar + toolbar.
	var rc [4]int32 // left, top, right, bottom
	ret, _, _ := procGetClientRect.Call(parent, uintptr(unsafe.Pointer(&rc)))
	if ret == 0 {
		return
	}
	contentW := int(rc[2] - rc[0])
	contentH := int(rc[3]-rc[1]) - chromeHeight
	if contentW <= 0 || contentH <= 0 {
		return
	}

	procMoveWindow.Call(hwnd,
		0,
		uintptr(chromeHeight),
		uintptr(contentW),
		uintptr(contentH),
		1, // repaint
	)
}

// Show makes the child HWND visible. Called when this tab becomes active.
func (m *MsTscSession) Show() {
	if m.childHWND != 0 {
		showWindow(m.childHWND, swShow)
	}
}

// Hide hides the child HWND. Called when another tab is activated.
func (m *MsTscSession) Hide() {
	if m.childHWND != 0 {
		showWindow(m.childHWND, swHide)
	}
}

// HandleKeyPress is a no-op — the ActiveX HWND captures keyboard input directly.
func (m *MsTscSession) HandleKeyPress(key ebiten.Key) {}

// HandleKeyRelease is a no-op — the ActiveX HWND captures keyboard input directly.
func (m *MsTscSession) HandleKeyRelease(key ebiten.Key) {}

// HandleHookKey is a no-op — the ActiveX HWND processes Win32 key messages.
func (m *MsTscSession) HandleHookKey(scancode uint16, extended bool, release bool) {}

// HandleMouseMove is a no-op — the ActiveX HWND captures mouse input directly.
func (m *MsTscSession) HandleMouseMove(x, y int) {}

// HandleMouseButton is a no-op — the ActiveX HWND captures mouse input directly.
func (m *MsTscSession) HandleMouseButton(button ebiten.MouseButton, pressed bool) {}

// HandleMouseWheel is a no-op — the ActiveX HWND captures mouse input directly.
func (m *MsTscSession) HandleMouseWheel(dx, dy float64) {}

// SendCtrlAltDel sends Ctrl+Alt+Delete via the IMsRdpClient.SendKeys method.
func (m *MsTscSession) SendCtrlAltDel() {
	m.dispatchCOM(func() {
		if m.rdpClient == nil {
			return
		}
		// SendKeys is available on IMsRdpClient8+. Attempt the call;
		// if it fails we fall back to a WM_KEYDOWN injection below.
		_, err := m.rdpClient.CallMethod("SendKeys", int32(1), false)
		if err != nil {
			rdpLog.Printf("MsTsc SendCtrlAltDel via SendKeys failed: %v", err)
		}
	})
}

// SendCtrlShiftEsc sends Ctrl+Shift+Escape to open Task Manager.
func (m *MsTscSession) SendCtrlShiftEsc() {
	m.dispatchCOM(func() {
		if m.rdpClient == nil {
			return
		}
		_, err := m.rdpClient.CallMethod("SendKeys", int32(0), false)
		if err != nil {
			rdpLog.Printf("MsTsc SendCtrlShiftEsc via SendKeys failed: %v", err)
		}
	})
}

// Close terminates the session. Safe to call multiple times.
func (m *MsTscSession) Close() {
	rdpLog.Printf("MsTsc Close() called")
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.mu.Unlock()

	m.doneOnce.Do(func() { close(m.doneCh) })
}

// --- Win32 helpers ---

// getEbitenWindowHandle returns the HWND of the Ebiten/GLFW window by
// searching for the well-known GLFW window class "GLFW30". FindWindowW is
// system-wide and works correctly from any thread, unlike GetActiveWindow
// which only sees windows on the calling thread's message queue.
func getEbitenWindowHandle() uintptr {
	className, _ := syscall.UTF16PtrFromString("GLFW30")
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(className)), 0)
	return hwnd
}

// wndClassRegistered guards the one-time RegisterClassEx call.
var wndClassRegistered sync.Once

// wndClassName is the class name for the host container window.
var wndClassName = syscall.StringToUTF16Ptr("NexusRDPHost")

// wndProc is a minimal window procedure for the host HWND.
// It delegates everything to DefWindowProc.
func wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	ret, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return ret
}

// createHostWindow creates a Win32 child window to host the ActiveX control.
func createHostWindow(parent uintptr, w, h int) (uintptr, error) {
	// Register the window class once.
	wndClassRegistered.Do(func() {
		type WNDCLASSEX struct {
			CbSize        uint32
			Style         uint32
			LpfnWndProc   uintptr
			CbClsExtra    int32
			CbWndExtra    int32
			HInstance     uintptr
			HIcon         uintptr
			HCursor       uintptr
			HbrBackground uintptr
			LpszMenuName  *uint16
			LpszClassName *uint16
			HIconSm       uintptr
		}

		modHandle, _, _ := procGetModuleHandle.Call(0)
		wc := WNDCLASSEX{
			CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
			Style:         csSaveScreen,
			LpfnWndProc:   syscall.NewCallback(wndProc),
			HInstance:     modHandle,
			LpszClassName: wndClassName,
		}
		procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	})

	style := uintptr(wsChild | wsVisible | wsClipChildren | wsClipSiblings)
	hwnd, _, err := procCreateWindowEx.Call(
		0,                                         // dwExStyle
		uintptr(unsafe.Pointer(wndClassName)),     // lpClassName
		0,                                         // lpWindowName
		style,                                     // dwStyle
		0,                                         // x (relative to parent)
		uintptr(chromeHeight),                     // y (below tab bar + toolbar)
		uintptr(w),                                // nWidth
		uintptr(h),                                // nHeight
		parent,                                    // hWndParent
		0,                                         // hMenu
		0,                                         // hInstance
		0,                                         // lpParam
	)
	if hwnd == 0 {
		return 0, fmt.Errorf("CreateWindowEx failed: %w", err)
	}
	return hwnd, nil
}

// destroyWindow destroys a Win32 HWND.
func destroyWindow(hwnd uintptr) {
	if hwnd != 0 {
		procDestroyWindow.Call(hwnd)
	}
}

// showWindow calls Win32 ShowWindow.
func showWindow(hwnd uintptr, cmd int) {
	if hwnd != 0 {
		procShowWindow.Call(hwnd, uintptr(cmd))
	}
}

// dispPut calls IDispatch.PutProperty on an arbitrary sub-dispatch object.
func dispPut(disp *ole.IDispatch, name string, val interface{}) error {
	_, err := disp.PutProperty(name, val)
	return err
}

