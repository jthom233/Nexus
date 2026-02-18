package gui

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"sync"
	"sync/atomic"

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

// Fullscreen overlay constants.
const (
	overlayHeight  = 32 // Height of the expanded overlay bar
	overlayHandleH = 4  // Height of the thin grab handle at the top
	overlayWidthPc = 40 // Percentage of screen width for the overlay bar
	overlayTriggerY = 8 // How many pixels from top trigger overlay show
)

const (
	defaultWidth  = 1280
	defaultHeight = 800
	targetFPS     = 60
)

// isWayland returns true if the display server is Wayland.
// This guards against Wayland's broken GLFW iconify state detection.
func isWayland() bool {
	return os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland"
}

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
	{Label: "Task Manager", Width: 100},
	{Label: "Disconnect", Width: 90},
}

// Window control button layout (top-right of tab bar).
const (
	winBtnWidth  = 30
	winBtnHeight = 22
	winBtnGap    = 2
)

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
	SendCtrlShiftEsc()
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

	// Pending actions queued from IPC goroutines for the main thread.
	// Ebiten window calls must happen on the Update/Draw goroutine.
	// ALL Ebiten window-state calls (SetFullscreen, MinimizeWindow, RestoreWindow)
	// must be deferred here and executed at the TOP of Update(), before any RLock.
	pendingRestore         atomic.Bool // restore minimized/maximized → normal
	pendingFullscreen      atomic.Bool // enter fullscreen
	pendingExitFullscreen  atomic.Bool // exit fullscreen (windowed restore)
	pendingMinimize        atomic.Bool // minimize (only fires when not in fullscreen)
	pendingExitThenMinimize atomic.Bool // two-tick: exit fullscreen this tick, minimize next tick

	// Fullscreen overlay state.
	overlayVisible bool    // whether the expanded overlay bar is showing
	overlayPinned  bool    // whether the overlay is pinned open
	overlayAlpha   float64 // fade animation target (0.0 = hidden, 1.0 = visible)
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
	// Process pending window actions BEFORE acquiring any lock.
	// All Ebiten window-state functions must run here, never inside a locked section
	// or directly inside a click handler, to avoid Ebiten frame-cycle deadlocks.
	if a.pendingRestore.CompareAndSwap(true, false) {
		if ebiten.IsWindowMinimized() || ebiten.IsWindowMaximized() {
			ebiten.RestoreWindow()
		}
	}
	if a.pendingFullscreen.CompareAndSwap(true, false) {
		ebiten.SetFullscreen(true)
	}
	if a.pendingExitFullscreen.CompareAndSwap(true, false) {
		ebiten.SetFullscreen(false)
	}
	// pendingMinimize is checked BEFORE pendingExitThenMinimize so that the
	// two-tick sequence works correctly: tick 1 (pendingExitThenMinimize) exits
	// fullscreen and stores pendingMinimize=true; tick 2 (pendingMinimize) runs
	// MinimizeWindow() after the fullscreen state has settled in Ebiten's
	// internal frame pipeline.
	if a.pendingMinimize.CompareAndSwap(true, false) {
		if !ebiten.IsFullscreen() && !isWayland() {
			ebiten.MinimizeWindow()
		}
	}
	if a.pendingExitThenMinimize.CompareAndSwap(true, false) {
		ebiten.SetFullscreen(false)
		a.pendingMinimize.Store(true)
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	if ebiten.IsFullscreen() {
		// In fullscreen: handle overlay interactions only
		a.handleOverlay()
	} else {
		// In windowed: handle normal chrome interactions
		a.handleTabClicks()
		a.handleToolbarClicks()
		a.handleWindowDrag()
	}

	// Route input to active session
	activeTab := a.tabs.Active()
	if activeTab != nil && activeTab.Session != nil {
		// Keyboard always forwarded regardless of framebuffer state
		forwardKeyboard(activeTab.Session)

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

	if ebiten.IsFullscreen() {
		a.drawFullscreen(screen)
		return
	}

	// Windowed mode: draw normal chrome
	a.drawTabBar(screen)
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

// drawFullscreen handles drawing when in fullscreen mode.
// The session framebuffer fills the entire screen from y=0.
// The overlay bar is drawn on top when visible.
func (a *App) drawFullscreen(screen *ebiten.Image) {
	activeTab := a.tabs.Active()
	if activeTab == nil {
		return
	}

	// Show status messages for non-connected states (centered on full screen)
	switch activeTab.Status {
	case "connecting":
		msg := "Connecting..."
		cx := a.width/2 - len(msg)*3
		cy := a.height / 2
		ebitenutil.DebugPrintAt(screen, msg, cx, cy)
		a.drawOverlay(screen)
		return
	case "error":
		errMsg := "Connection failed"
		if activeTab.Error != nil {
			errMsg = activeTab.Error.Error()
		}
		if len(errMsg) > 80 {
			errMsg = errMsg[:80] + "..."
		}
		label := "[!] " + errMsg
		cx := a.width/2 - len(label)*3
		cy := a.height / 2
		ebitenutil.DebugPrintAt(screen, label, cx, cy)
		a.drawOverlay(screen)
		return
	}

	// Draw session framebuffer filling the entire screen (y=0)
	if activeTab.Session != nil {
		fb := activeTab.Session.Framebuffer()
		if fb != nil {
			nw, nh := activeTab.Session.NativeSize()
			if nw > 0 && nh > 0 {
				scaleX := float64(a.width) / float64(nw)
				scaleY := float64(a.height) / float64(nh)
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Scale(scaleX, scaleY)
				// No Y offset — session fills from y=0
				op.Filter = ebiten.FilterLinear
				screen.DrawImage(fb, op)
			}
		}
	}

	// Draw overlay on top of the session
	a.drawOverlay(screen)
}

// sessionTransform returns the X/Y scale and offset used to draw the active session.
// Mouse coordinates need to be reverse-mapped through this transform.
// In fullscreen mode the session fills the entire screen so offsetY is 0.
func (a *App) sessionTransform() (scaleX, scaleY, offsetX, offsetY float64, ok bool) {
	activeTab := a.tabs.Active()
	if activeTab == nil || activeTab.Session == nil {
		return 0, 0, 0, 0, false
	}
	nw, nh := activeTab.Session.NativeSize()
	if nw <= 0 || nh <= 0 {
		return 0, 0, 0, 0, false
	}

	if ebiten.IsFullscreen() {
		// Session fills the entire screen — no chrome offset
		scaleX = float64(a.width) / float64(nw)
		scaleY = float64(a.height) / float64(nh)
		offsetX = 0
		offsetY = 0
	} else {
		availW := float64(a.width)
		availH := float64(a.height - chromeHeight)
		scaleX = availW / float64(nw)
		scaleY = availH / float64(nh)
		offsetX = 0
		offsetY = float64(chromeHeight)
	}
	return scaleX, scaleY, offsetX, offsetY, true
}

// overlayRect returns the bounding box of the overlay bar in its current state.
// When the overlay is visible (expanded), it is overlayHeight tall.
// The thin handle is always overlayHandleH tall (used for trigger detection).
func (a *App) overlayRect() (x, y, w, h int) {
	w = a.width * overlayWidthPc / 100
	x = (a.width - w) / 2
	y = 0
	if a.overlayVisible || a.overlayPinned {
		h = overlayHeight
	} else {
		h = overlayHandleH
	}
	return x, y, w, h
}

// handleOverlay processes overlay-related mouse interactions each Update() tick.
// Must only be called when ebiten.IsFullscreen() is true.
func (a *App) handleOverlay() {
	mx, my := ebiten.CursorPosition()

	// Advance alpha toward target (simple linear interpolation at 60fps).
	targetAlpha := 0.0
	if a.overlayVisible || a.overlayPinned {
		targetAlpha = 1.0
	}
	const alphaStep = 0.12
	if a.overlayAlpha < targetAlpha {
		a.overlayAlpha += alphaStep
		if a.overlayAlpha > 1.0 {
			a.overlayAlpha = 1.0
		}
	} else if a.overlayAlpha > targetAlpha {
		a.overlayAlpha -= alphaStep
		if a.overlayAlpha < 0.0 {
			a.overlayAlpha = 0.0
		}
	}

	// Trigger overlay when mouse is near the top edge.
	if my >= 0 && my < overlayTriggerY {
		a.overlayVisible = true
	}

	// Hide overlay when mouse leaves the expanded bar (unless pinned).
	if !a.overlayPinned && a.overlayVisible {
		_, _, ow, oh := a.overlayRect()
		ox := (a.width - ow) / 2
		if mx < ox || mx >= ox+ow || my < 0 || my >= oh {
			a.overlayVisible = false
		}
	}

	// Handle clicks on overlay buttons.
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	if !a.overlayVisible && !a.overlayPinned {
		return
	}

	ox, _, ow, oh := a.overlayRect()
	if mx < ox || mx >= ox+ow || my < 0 || my >= oh {
		return
	}

	// Button layout within the overlay bar (from left and right edges).
	// Left side: [Pin] [Ctrl+Alt+Del] [TaskMgr]
	// Center: connection label (no click)
	// Right side: [Minimize] [Restore] [X]
	const btnW = 80
	const btnPad = 4

	// Left buttons
	pinBtnX := ox + btnPad
	cadBtnX := pinBtnX + btnW + btnPad
	tmBtnX := cadBtnX + btnW + btnPad

	// Right buttons
	closeBtnX := ox + ow - btnW - btnPad
	restoreBtnX := closeBtnX - btnW - btnPad
	minBtnX := restoreBtnX - btnW - btnPad

	btnY := 0
	btnH := oh

	activeTab := a.tabs.Active()

	// Pin/Unpin
	if mx >= pinBtnX && mx < pinBtnX+btnW && my >= btnY && my < btnY+btnH {
		a.overlayPinned = !a.overlayPinned
		return
	}
	// Ctrl+Alt+Del
	if mx >= cadBtnX && mx < cadBtnX+btnW && my >= btnY && my < btnY+btnH {
		if activeTab != nil && activeTab.Session != nil {
			activeTab.Session.SendCtrlAltDel()
		}
		return
	}
	// Task Manager
	if mx >= tmBtnX && mx < tmBtnX+btnW && my >= btnY && my < btnY+btnH {
		if activeTab != nil && activeTab.Session != nil {
			activeTab.Session.SendCtrlShiftEsc()
		}
		return
	}
	// Minimize — defer all window-state changes; never call ebiten window
	// functions directly inside a click handler while mu.RLock is held.
	if mx >= minBtnX && mx < minBtnX+btnW && my >= btnY && my < btnY+btnH {
		a.overlayVisible = false
		a.overlayPinned = false
		a.pendingExitThenMinimize.Store(true)
		return
	}
	// Restore (exit fullscreen) — same deferral rule.
	if mx >= restoreBtnX && mx < restoreBtnX+btnW && my >= btnY && my < btnY+btnH {
		a.overlayVisible = false
		a.overlayPinned = false
		a.pendingExitFullscreen.Store(true)
		return
	}
	// Close/Disconnect
	if mx >= closeBtnX && mx < closeBtnX+btnW && my >= btnY && my < btnY+btnH {
		if activeTab != nil {
			a.closeTabFromGUI(activeTab.ConnID)
		}
		return
	}
}

// drawOverlay draws the fullscreen overlay bar on top of the session.
// The overlay shows a thin grab handle always, and expands when the mouse
// is near the top edge or when pinned.
func (a *App) drawOverlay(screen *ebiten.Image) {
	ox, oy, ow, _ := a.overlayRect()

	// Always draw the thin grab handle as a subtle hint strip.
	handleImg := ebiten.NewImage(ow, overlayHandleH)
	handleImg.Fill(color.RGBA{R: 100, G: 100, B: 140, A: 120})
	hop := &ebiten.DrawImageOptions{}
	hop.GeoM.Translate(float64(ox), float64(oy))
	screen.DrawImage(handleImg, hop)

	// Draw the expanded bar only when visible/pinned (fades with alpha).
	if a.overlayAlpha <= 0 {
		return
	}

	alpha := uint8(a.overlayAlpha * 220) // max ~220/255 opacity

	// Background: semi-transparent dark bar
	barImg := ebiten.NewImage(ow, overlayHeight)
	barImg.Fill(color.RGBA{R: 20, G: 20, B: 35, A: alpha})
	bop := &ebiten.DrawImageOptions{}
	bop.GeoM.Translate(float64(ox), float64(oy))
	screen.DrawImage(barImg, bop)

	// Only draw text/buttons if sufficiently visible to avoid clutter.
	if a.overlayAlpha < 0.3 {
		return
	}

	const btnW = 80
	const btnPad = 4
	const btnH = overlayHeight - 4
	btnTop := oy + 2

	// Helper to draw a button background + label.
	drawBtn := func(bx int, label string, bg color.RGBA) {
		btnImg := ebiten.NewImage(btnW, btnH)
		btnImg.Fill(bg)
		bop2 := &ebiten.DrawImageOptions{}
		bop2.GeoM.Translate(float64(bx), float64(btnTop))
		screen.DrawImage(btnImg, bop2)
		// Center text horizontally (approx 6px per char)
		textX := bx + (btnW-len(label)*6)/2
		ebitenutil.DebugPrintAt(screen, label, textX, btnTop+6)
	}

	normalBg := color.RGBA{R: 50, G: 50, B: 75, A: alpha}
	pinBg := color.RGBA{R: 70, G: 90, B: 50, A: alpha}
	dangerBg := color.RGBA{R: 150, G: 40, B: 40, A: alpha}

	// Left buttons
	pinBtnX := ox + btnPad
	cadBtnX := pinBtnX + btnW + btnPad
	tmBtnX := cadBtnX + btnW + btnPad

	if a.overlayPinned {
		drawBtn(pinBtnX, "[Unpin]", pinBg)
	} else {
		drawBtn(pinBtnX, "[Pin]", normalBg)
	}
	drawBtn(cadBtnX, "CAD", normalBg)
	drawBtn(tmBtnX, "TaskMgr", normalBg)

	// Right buttons
	closeBtnX := ox + ow - btnW - btnPad
	restoreBtnX := closeBtnX - btnW - btnPad
	minBtnX := restoreBtnX - btnW - btnPad

	drawBtn(closeBtnX, "[X]", dangerBg)
	drawBtn(restoreBtnX, "Restore", normalBg)
	drawBtn(minBtnX, "Min", normalBg)

	// Connection label in the center
	activeTab := a.tabs.Active()
	if activeTab != nil {
		label := activeTab.Label
		maxChars := (ow - 6*(btnW+btnPad)) / 6
		if maxChars < 1 {
			maxChars = 1
		}
		if len(label) > maxChars {
			label = label[:maxChars-2] + ".."
		}
		labelX := ox + ow/2 - len(label)*3
		ebitenutil.DebugPrintAt(screen, label, labelX, btnTop+6)
	}
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

	activeID := ""
	if active := a.tabs.Active(); active != nil {
		activeID = active.ConnID
	}

	// Reserve space for window control buttons on the right
	winBtnsWidth := 2*(winBtnWidth+winBtnGap) + winBtnGap

	for i, tab := range tabs {
		x := i * tabWidth
		if x+tabWidth > a.width-winBtnsWidth {
			break
		}

		// Tab background
		var bgColor color.RGBA
		if tab.Status == "error" {
			if tab.ConnID == activeID {
				bgColor = color.RGBA{R: 120, G: 50, B: 50, A: 255}
			} else {
				bgColor = color.RGBA{R: 90, G: 40, B: 40, A: 255}
			}
		} else if tab.ConnID == activeID {
			bgColor = color.RGBA{R: 69, G: 71, B: 90, A: 255}
		} else {
			bgColor = color.RGBA{R: 49, G: 50, B: 68, A: 255}
		}

		tabImg := ebiten.NewImage(tabWidth-2, tabBarHeight-2)
		tabImg.Fill(bgColor)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(x+1), 1)
		screen.DrawImage(tabImg, op)

		// Tab label text
		label := tab.Label
		if tab.Status == "error" {
			label = "[!] " + label
		}
		maxChars := (tabWidth - tabCloseRegion - 10) / 6
		if len(label) > maxChars {
			label = label[:maxChars-1] + ".."
		}
		ebitenutil.DebugPrintAt(screen, label, x+4, 6)

		// Close button "x"
		closeX := x + tabWidth - tabCloseRegion
		ebitenutil.DebugPrintAt(screen, "x", closeX+6, 6)
	}

	// Draw window control buttons (top-right)
	btnY := (tabBarHeight - winBtnHeight) / 2

	// Maximize/Restore button
	maxBtnX := a.width - 2*(winBtnWidth+winBtnGap)
	maxImg := ebiten.NewImage(winBtnWidth, winBtnHeight)
	maxImg.Fill(color.RGBA{R: 59, G: 60, B: 78, A: 255})
	mop := &ebiten.DrawImageOptions{}
	mop.GeoM.Translate(float64(maxBtnX), float64(btnY))
	screen.DrawImage(maxImg, mop)
	if ebiten.IsFullscreen() {
		ebitenutil.DebugPrintAt(screen, "[ ]", maxBtnX+3, btnY+4)
	} else {
		ebitenutil.DebugPrintAt(screen, "[+]", maxBtnX+3, btnY+4)
	}

	// Close button
	closeBtnX := a.width - winBtnWidth - winBtnGap
	closeImg := ebiten.NewImage(winBtnWidth, winBtnHeight)
	closeImg.Fill(color.RGBA{R: 180, G: 50, B: 50, A: 255})
	cop := &ebiten.DrawImageOptions{}
	cop.GeoM.Translate(float64(closeBtnX), float64(btnY))
	screen.DrawImage(closeImg, cop)
	ebitenutil.DebugPrintAt(screen, " X", closeBtnX+5, btnY+4)
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

	// Check window control buttons (top-right)
	btnY := (tabBarHeight - winBtnHeight) / 2
	if my >= btnY && my < btnY+winBtnHeight {
		// Maximize/Restore button — defer window-state change to avoid
		// calling Ebiten window functions inside mu.RLock during click handling.
		maxBtnX := a.width - 2*(winBtnWidth+winBtnGap)
		if mx >= maxBtnX && mx < maxBtnX+winBtnWidth {
			if ebiten.IsFullscreen() {
				a.pendingExitFullscreen.Store(true)
			} else {
				a.pendingFullscreen.Store(true)
			}
			return
		}
		// Close button
		closeBtnX := a.width - winBtnWidth - winBtnGap
		if mx >= closeBtnX && mx < closeBtnX+winBtnWidth {
			os.Exit(0)
		}
	}

	tabs := a.tabs.All()
	for i, tab := range tabs {
		x := i * tabWidth
		if x+tabWidth > a.width {
			break
		}
		if mx >= x && mx < x+tabWidth {
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
			case 1: // Task Manager
				if activeTab != nil && activeTab.Session != nil {
					activeTab.Session.SendCtrlShiftEsc()
				}
			case 2: // Disconnect
				if activeTab != nil {
					a.closeTabFromGUI(activeTab.ConnID)
				}
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

// isTabHit returns true if the given x coordinate hits an existing tab
// or the window control buttons area.
func (a *App) isTabHit(mx int) bool {
	// Window control buttons area
	winBtnsStart := a.width - 2*(winBtnWidth+winBtnGap) - winBtnGap
	if mx >= winBtnsStart {
		return true
	}

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

	// Handle fullscreen option (queue for main thread)
	if options != nil {
		if fs, ok := options["fullscreen"].(bool); ok && fs {
			a.pendingFullscreen.Store(true)
			// Don't override user's resolution — it will be used as-is.
			// The window will go fullscreen on the next Update tick.
		}
	}

	// Auto-resize: set RDP resolution to match current window area
	if protocol == "rdp" {
		if options == nil {
			options = make(map[string]interface{})
		}
		// Only override if not already set by fullscreen or user
		if res, _ := options["resolution"].(string); res == "" {
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

// RestoreWindow queues a window restore for the next Update() tick.
// Safe to call from any goroutine (IPC handlers, etc.).
func (a *App) RestoreWindow() {
	a.pendingRestore.Store(true)
}

// SessionSize returns available session area (below chrome).
func (a *App) SessionSize() image.Point {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return image.Point{X: a.width, Y: a.height - chromeHeight}
}
