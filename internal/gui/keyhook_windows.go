//go:build windows

package gui

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	procSetWindowsHook  = user32.NewProc("SetWindowsHookExW")
	procCallNextHook    = user32.NewProc("CallNextHookEx")
	procUnhookWindows   = user32.NewProc("UnhookWindowsHookEx")
	procGetForegroundWin = user32.NewProc("GetForegroundWindow")
	procGetMessage      = user32.NewProc("GetMessageW")

	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procGetModuleHandle = kernel32.NewProc("GetModuleHandleW")
)

const (
	whKeyboardLL = 13
	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmSysKeyDown = 0x0104
	wmSysKeyUp   = 0x0105
)

// KBDLLHOOKSTRUCT is the structure passed to the low-level keyboard hook.
type kbdllHookStruct struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

// hookEvent represents a captured keyboard event.
type hookEvent struct {
	scancode uint16
	extended bool
	release  bool
}

var (
	hookHandle  uintptr
	hookChan    chan hookEvent
	hookOnce    sync.Once
	hookWinHwnd uintptr // the Nexus GUI window handle
	hookMu      sync.Mutex
)

// keyHookCallback is the low-level keyboard hook procedure.
func keyHookCallback(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 {
		kb := (*kbdllHookStruct)(unsafe.Pointer(lParam))

		// Only intercept when our window has focus
		fgWin, _, _ := procGetForegroundWin.Call()

		hookMu.Lock()
		ourWin := hookWinHwnd
		hookMu.Unlock()

		if fgWin == ourWin && ourWin != 0 {
			// Determine if this is a key we should capture
			// System keys: Win (VK_LWIN=0x5B, VK_RWIN=0x5C), Apps (VK_APPS=0x5D)
			// Also capture all keys to prevent OS from stealing them
			isRelease := wParam == wmKeyUp || wParam == wmSysKeyUp
			isExtended := kb.Flags&0x01 != 0 // LLKHF_EXTENDED

			sc := uint16(kb.ScanCode)
			if sc > 0 {
				select {
				case hookChan <- hookEvent{
					scancode: sc,
					extended: isExtended,
					release:  isRelease,
				}:
				default:
					// Channel full, drop the event
				}
				// Return 1 to suppress the key from reaching the OS
				return 1
			}
		}
	}

	ret, _, _ := procCallNextHook.Call(hookHandle, uintptr(nCode), wParam, lParam)
	return ret
}

// installKeyHook installs the low-level keyboard hook. Must be called from
// a dedicated goroutine that runs a message pump.
func installKeyHook() {
	hookOnce.Do(func() {
		hookChan = make(chan hookEvent, 64)

		go func() {
			modHandle, _, _ := procGetModuleHandle.Call(0)

			hookHandle, _, _ = procSetWindowsHook.Call(
				whKeyboardLL,
				syscall.NewCallback(keyHookCallback),
				modHandle,
				0,
			)

			if hookHandle == 0 {
				return
			}

			// Message pump — required for the hook to work
			type msg struct {
				Hwnd    uintptr
				Message uint32
				WParam  uintptr
				LParam  uintptr
				Time    uint32
				Pt      struct{ X, Y int32 }
			}
			var m msg
			for {
				ret, _, _ := procGetMessage.Call(
					uintptr(unsafe.Pointer(&m)),
					0, 0, 0,
				)
				if ret == 0 {
					break
				}
			}
		}()
	})
}

// setHookWindow sets the window handle that the hook should protect.
func setHookWindow(hwnd uintptr) {
	hookMu.Lock()
	hookWinHwnd = hwnd
	hookMu.Unlock()
}

// drainHookEvents returns all pending hook events, clearing the channel.
func drainHookEvents() []hookEvent {
	var events []hookEvent
	for {
		select {
		case ev := <-hookChan:
			events = append(events, ev)
		default:
			return events
		}
	}
}

// stopKeyHook removes the keyboard hook.
func stopKeyHook() {
	if hookHandle != 0 {
		procUnhookWindows.Call(hookHandle)
		hookHandle = 0
	}
}
