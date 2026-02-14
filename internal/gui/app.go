package gui

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"sync"

	"github.com/dr4zz/nexus/internal/ipc"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const (
	tabBarHeight  = 28
	toolbarHeight = 28
	chromeHeight  = tabBarHeight + toolbarHeight
)

const (
	defaultWidth  = 1280
	defaultHeight = 800
	targetFPS     = 60
)

// Tab layout constants.
const (
	tabWidth       = 160
	tabCloseRegion = 20 // rightmost pixels of a tab = close button
)

// Toolbar button layout.
type toolbarButton struct {
	Label string
	Width int
}

var toolbarButtons = []toolbarButton{
	{Label: "Ctrl+Alt+Del", Width: 110},
	{Label: "Disconnect", Width: 90},
	{Label: "Detach [C-\\]", Width: 100},
}

// Session is the interface for RDP/VNC sessions rendering into framebuffers.
type Session interface {
	Connect() error
	Update()
	Framebuffer() *ebiten.Image
	NativeSize() (int, int) // Returns the session's native resolution
	HandleKeyPress(key ebiten.Key)
	HandleKeyRelease(key ebiten.Key)
	HandleHookKey(scancode uint16, extended bool, release bool)
	HandleMouseMove(x, y int)
	HandleMouseButton(button ebiten.MouseButton, pressed bool)
	HandleMouseWheel(dx, dy float64)
	SendCtrlAltDel()
	Close()
}

// App implements ebiten.Game and manages the graphical viewer.
type App struct {
	tabs   *TabManager
	ipcMgr *IPCManager

	mu     sync.RWMutex
	width  int
	height int

	// Window drag state (for borderless mode)
	dragging   bool
	dragStartX int
	dragStartY int
	dragWinX   int
	dragWinY   int
}

// NewApp creates a new GUI application.
func NewApp() *App {
	a := &App{
		tabs:   NewTabManager(),
		width:  defaultWidth,
		height: defaultHeight,
	}
	installKeyHook()
	return a
}

// SetIPCManager sets the IPC manager for handling commands.
func (a *App) SetIPCManager(mgr *IPCManager) {
	a.ipcMgr = mgr
}

// Update implements ebiten.Game. Called every tick.
func (a *App) Update() error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Handle tab bar clicks
	a.handleTabClicks()

	// Handle toolbar clicks
	a.handleToolbarClicks()

	// Handle window dragging from tab bar
	a.handleWindowDrag()

	// Check GUI-level hotkeys first (e.g., Ctrl+\ to detach)
	hotkeyConsumed := a.handleGUIHotkeys()

	// Route input to active session
	activeTab := a.tabs.Active()
	if activeTab != nil && activeTab.Session != nil {
		if !hotkeyConsumed {
			// Keyboard always forwarded regardless of framebuffer state
			forwardKeyboard(activeTab.Session)
		}

		// Process low-level keyboard hook events (Windows system keys)
		for _, ev := range drainHookEvents() {
			if ev.release {
				activeTab.Session.HandleHookKey(ev.scancode, ev.extended, true)
			} else {
				activeTab.Session.HandleHookKey(ev.scancode, ev.extended, false)
			}
		}

		// Mouse requires valid transform for coordinate mapping
		if scaleX, scaleY, offX, offY, ok := a.sessionTransform(); ok {
			forwardMouse(activeTab.Session, scaleX, scaleY, offX, offY)
		}
	}

	// Update all sessions (receive framebuffer updates)
	for _, tab := range a.tabs.All() {
		if tab.Session != nil {
			tab.Session.Update()
		}
	}

	return nil
}

// Draw implements ebiten.Game. Called every frame.
func (a *App) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{R: 30, G: 30, B: 46, A: 255}) // Dark background

	// Draw tab bar
	a.drawTabBar(screen)

	// Draw toolbar
	a.drawToolbar(screen)

	activeTab := a.tabs.Active()
	if activeTab == nil {
		return
	}

	// Show status messages for non-connected states
	switch activeTab.Status {
	case "connecting":
		msg := "Connecting..."
		cx := a.width/2 - len(msg)*3
		cy := chromeHeight + (a.height-chromeHeight)/2
		ebitenutil.DebugPrintAt(screen, msg, cx, cy)
		return
	case "error":
		errMsg := "Connection failed"
		if activeTab.Error != nil {
			errMsg = activeTab.Error.Error()
		}
		// Truncate long error messages
		if len(errMsg) > 80 {
			errMsg = errMsg[:80] + "..."
		}
		label := "[!] " + errMsg
		cx := a.width/2 - len(label)*3
		cy := chromeHeight + (a.height-chromeHeight)/2
		ebitenutil.DebugPrintAt(screen, label, cx, cy)
		return
	}

	// Draw active session framebuffer stretched to fill available area
	if activeTab.Session != nil {
		fb := activeTab.Session.Framebuffer()
		if fb != nil {
			nw, nh := activeTab.Session.NativeSize()
			if nw > 0 && nh > 0 {
				availW := float64(a.width)
				availH := float64(a.height - chromeHeight)
				scaleX := availW / float64(nw)
				scaleY := availH / float64(nh)

				op := &ebiten.DrawImageOptions{}
				op.GeoM.Scale(scaleX, scaleY)
				op.GeoM.Translate(0, float64(chromeHeight))
				op.Filter = ebiten.FilterLinear
				screen.DrawImage(fb, op)
			}
		}
	}
}

// sessionTransform returns the X/Y scale and offset used to draw the active session.
// Mouse coordinates need to be reverse-mapped through this transform.
func (a *App) sessionTransform() (scaleX, scaleY, offsetX, offsetY float64, ok bool) {
	activeTab := a.tabs.Active()
	if activeTab == nil || activeTab.Session == nil {
		return 0, 0, 0, 0, false
	}
	nw, nh := activeTab.Session.NativeSize()
	if nw <= 0 || nh <= 0 {
		return 0, 0, 0, 0, false
	}
	availW := float64(a.width)
	availH := float64(a.height - chromeHeight)
	scaleX = availW / float64(nw)
	scaleY = availH / float64(nh)
	offsetX = 0
	offsetY = float64(chromeHeight)
	return scaleX, scaleY, offsetX, offsetY, true
}

// Layout implements ebiten.Game. Returns the logical screen size.
func (a *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	a.mu.Lock()
	a.width = outsideWidth
	a.height = outsideHeight
	a.mu.Unlock()
	return outsideWidth, outsideHeight
}

func (a *App) drawTabBar(screen *ebiten.Image) {
	tabs := a.tabs.All()
	if len(tabs) == 0 {
		return
	}

	activeID := ""
	if active := a.tabs.Active(); active != nil {
		activeID = active.ConnID
	}

	for i, tab := range tabs {
		x := i * tabWidth
		if x+tabWidth > a.width {
			break
		}

		// Tab background
		var bgColor color.RGBA
		if tab.Status == "error" {
			if tab.ConnID == activeID {
				bgColor = color.RGBA{R: 120, G: 50, B: 50, A: 255} // red-tinted active
			} else {
				bgColor = color.RGBA{R: 90, G: 40, B: 40, A: 255} // red-tinted inactive
			}
		} else if tab.ConnID == activeID {
			bgColor = color.RGBA{R: 69, G: 71, B: 90, A: 255} // #45475A
		} else {
			bgColor = color.RGBA{R: 49, G: 50, B: 68, A: 255} // #313244
		}

		tabImg := ebiten.NewImage(tabWidth-2, tabBarHeight-2)
		tabImg.Fill(bgColor)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(x+1), 1)
		screen.DrawImage(tabImg, op)

		// Tab label text — prefix with [!] for error tabs
		label := tab.Label
		if tab.Status == "error" {
			label = "[!] " + label
		}
		maxChars := (tabWidth - tabCloseRegion - 10) / 6 // ~6px per char
		if len(label) > maxChars {
			label = label[:maxChars-1] + ".."
		}
		ebitenutil.DebugPrintAt(screen, label, x+4, 6)

		// Close button "x"
		closeX := x + tabWidth - tabCloseRegion
		ebitenutil.DebugPrintAt(screen, "x", closeX+6, 6)
	}
}

func (a *App) drawToolbar(screen *ebiten.Image) {
	// Toolbar background
	tbImg := ebiten.NewImage(a.width, toolbarHeight)
	tbImg.Fill(color.RGBA{R: 39, G: 39, B: 55, A: 255})
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(0, float64(tabBarHeight))
	screen.DrawImage(tbImg, op)

	// Draw buttons
	btnX := 4
	for _, btn := range toolbarButtons {
		btnImg := ebiten.NewImage(btn.Width, toolbarHeight-4)
		btnImg.Fill(color.RGBA{R: 59, G: 60, B: 78, A: 255})
		bop := &ebiten.DrawImageOptions{}
		bop.GeoM.Translate(float64(btnX), float64(tabBarHeight+2))
		screen.DrawImage(btnImg, bop)

		ebitenutil.DebugPrintAt(screen, btn.Label, btnX+4, tabBarHeight+6)
		btnX += btn.Width + 4
	}
}

func (a *App) handleTabClicks() {
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}

	mx, my := ebiten.CursorPosition()

	// Check if click is in the tab bar area
	if my < 0 || my >= tabBarHeight {
		return
	}

	tabs := a.tabs.All()
	for i, tab := range tabs {
		x := i * tabWidth
		if x+tabWidth > a.width {
			break
		}
		if mx >= x && mx < x+tabWidth {
			// Check if clicking the close region
			closeX := x + tabWidth - tabCloseRegion
			if mx >= closeX {
				a.closeTabFromGUI(tab.ConnID)
			} else {
				a.tabs.Focus(tab.ConnID)
			}
			return
		}
	}
}

func (a *App) handleToolbarClicks() {
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}

	mx, my := ebiten.CursorPosition()

	// Check if click is in the toolbar area
	if my < tabBarHeight || my >= chromeHeight {
		return
	}

	btnX := 4
	for i, btn := range toolbarButtons {
		if mx >= btnX && mx < btnX+btn.Width {
			activeTab := a.tabs.Active()
			switch i {
			case 0: // Ctrl+Alt+Del
				if activeTab != nil && activeTab.Session != nil {
					activeTab.Session.SendCtrlAltDel()
				}
			case 1: // Disconnect
				if activeTab != nil {
					a.closeTabFromGUI(activeTab.ConnID)
				}
			case 2: // Detach
				ebiten.MinimizeWindow()
			}
			return
		}
		btnX += btn.Width + 4
	}
}

// handleWindowDrag allows dragging the borderless window by clicking on
// empty tab bar space (areas not occupied by a tab).
func (a *App) handleWindowDrag() {
	mx, my := ebiten.CursorPosition()

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		// Only start drag if clicking in tab bar area on empty space
		if my >= 0 && my < tabBarHeight && !a.isTabHit(mx) {
			a.dragging = true
			// Get absolute cursor position on screen
			wx, wy := ebiten.WindowPosition()
			a.dragStartX = wx + mx
			a.dragStartY = wy + my
			a.dragWinX = wx
			a.dragWinY = wy
		}
	}

	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		a.dragging = false
		return
	}

	if a.dragging {
		wx, wy := ebiten.WindowPosition()
		curAbsX := wx + mx
		curAbsY := wy + my
		newX := a.dragWinX + (curAbsX - a.dragStartX)
		newY := a.dragWinY + (curAbsY - a.dragStartY)
		ebiten.SetWindowPosition(newX, newY)
	}
}

// isTabHit returns true if the given x coordinate hits an existing tab.
func (a *App) isTabHit(mx int) bool {
	tabCount := a.tabs.Count()
	for i := 0; i < tabCount; i++ {
		x := i * tabWidth
		if x+tabWidth > a.width {
			break
		}
		if mx >= x && mx < x+tabWidth {
			return true
		}
	}
	return false
}

// closeTabFromGUI closes a tab initiated from the GUI (click X or Disconnect).
// Notifies the TUI via the per-tab reply callback and broadcasts via IPC manager.
func (a *App) closeTabFromGUI(connID string) {
	tab := a.tabs.Get(connID)
	if tab == nil {
		return
	}

	if tab.Session != nil {
		tab.Session.Close()
	}

	// Notify TUI via per-tab reply callback
	if tab.ReplyFunc != nil {
		tab.ReplyFunc(ipc.MsgTabClosed, &ipc.TabClosedEvent{ConnID: connID})
	}

	// Also broadcast so any other listeners know
	if a.ipcMgr != nil {
		a.ipcMgr.SendTabClosed(connID)
	}

	a.tabs.Remove(connID)

	if a.tabs.Count() == 0 {
		os.Exit(0)
	}
}

// OpenTab adds a new tab for a graphical session.
func (a *App) OpenTab(connID, protocol, host string, port int, username, password, domain string, options map[string]interface{}, reply func(string, interface{}) error) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Handle fullscreen option
	if options != nil {
		if fs, ok := options["fullscreen"].(bool); ok && fs {
			ebiten.SetFullscreen(true)
			// Set resolution to monitor size
			mw, mh := ebiten.Monitor().Size()
			if mw > 0 && mh > 0 {
				if options == nil {
					options = make(map[string]interface{})
				}
				options["resolution"] = fmt.Sprintf("%dx%d", mw, mh)
			}
		}
	}

	// Auto-resize: set RDP resolution to match current window area
	if protocol == "rdp" {
		if options == nil {
			options = make(map[string]interface{})
		}
		// Only override if not already set by fullscreen or user
		if _, hasRes := options["resolution"]; !hasRes {
			availW := a.width
			availH := a.height - chromeHeight
			if availW > 0 && availH > 0 {
				options["resolution"] = fmt.Sprintf("%dx%d", availW, availH)
			}
		}
	}

	var sess Session
	switch protocol {
	case "rdp":
		sess = NewRDPSession(host, port, username, password, domain, options)
	case "vnc":
		sess = NewVNCSession(host, port, password)
	default:
		return nil
	}

	tab := &Tab{
		ConnID:    connID,
		Protocol:  protocol,
		Label:     connID + " (" + protocol + ")",
		Session:   sess,
		Status:    "connecting",
		ReplyFunc: reply,
	}
	a.tabs.Add(tab)

	// Connect in background
	go func() {
		if err := sess.Connect(); err != nil {
			tab.Error = err
			tab.Status = "error"
			if reply != nil {
				reply(ipc.MsgTabError, &ipc.TabErrorEvent{
					ConnID: connID,
					Error:  err.Error(),
				})
			}
			return
		}
		tab.Status = "connected"
		if reply != nil {
			reply(ipc.MsgTabOpened, &ipc.TabOpenedEvent{ConnID: connID})
		}
	}()

	return nil
}

// CloseTab closes a tab by connection ID.
func (a *App) CloseTab(connID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	tab := a.tabs.Get(connID)
	if tab != nil && tab.Session != nil {
		tab.Session.Close()
	}
	a.tabs.Remove(connID)

	if a.tabs.Count() == 0 {
		os.Exit(0)
	}
}

// handleGUIHotkeys checks for GUI-level key combos before forwarding to sessions.
// Returns true if a hotkey was consumed.
func (a *App) handleGUIHotkeys() bool {
	ctrl := ebiten.IsKeyPressed(ebiten.KeyControl) ||
		ebiten.IsKeyPressed(ebiten.KeyControlLeft) ||
		ebiten.IsKeyPressed(ebiten.KeyControlRight)
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyBackslash) {
		ebiten.MinimizeWindow()
		return true
	}
	return false
}

// RestoreWindow restores the GUI window from minimized state.
func (a *App) RestoreWindow() {
	ebiten.RestoreWindow()
}

// SessionSize returns available session area (below chrome).
func (a *App) SessionSize() image.Point {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return image.Point{X: a.width, Y: a.height - chromeHeight}
}
