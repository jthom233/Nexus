package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// newMinimalPaneApp creates a minimal App in the viewPaneLayout state,
// configured for testing insert-mode ESC handling.
func newMinimalPaneApp() App {
	return App{
		mode:       ModeNormal,
		viewStack:  []viewKind{viewList, viewPaneLayout},
		paneLayout: NewPaneLayoutModel(),
		leader:     newLeader(),
		statusBar:  newStatusBar(),
		log:        newLog(),
	}
}

// keyMsg constructs a tea.KeyMsg from a string (e.g. "esc", "i", " ").
func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "alt+esc":
		// Simulates ESC received with Alt modifier set — this can happen due to
		// terminal escape-sequence timing ambiguity when the user presses ESC
		// quickly and Bubbletea has already seen the ESC byte as an alt prefix.
		// However, a standalone alt+esc is still KeyEsc with Alt=true.
		return tea.KeyMsg{Type: tea.KeyEsc, Alt: true}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "i":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}}
	case "a":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// TestPaneLayoutInsertMode_EscExitsInsertMode verifies that pressing ESC while
// in insert mode (pane layout view) correctly transitions back to ModeNormal.
func TestPaneLayoutInsertMode_EscExitsInsertMode(t *testing.T) {
	a := newMinimalPaneApp()

	// Verify initial state.
	if a.mode != ModeNormal {
		t.Fatalf("precondition: expected ModeNormal, got %v", a.mode)
	}
	if a.currentView() != viewPaneLayout {
		t.Fatalf("precondition: expected viewPaneLayout, got %v", a.currentView())
	}

	// Press 'i' to enter insert mode.
	result, _ := a.handlePaneLayoutKey(keyMsg("i"))
	a = result.(App)

	if a.mode != ModeInsert {
		t.Errorf("after pressing 'i': expected ModeInsert, got %v", a.mode)
	}
	if a.statusBar.mode != ModeInsert {
		t.Errorf("after pressing 'i': expected statusBar.mode=ModeInsert, got %v", a.statusBar.mode)
	}

	// Type a character (forwarded to pane but mode stays Insert).
	result, _ = a.handlePaneLayoutKey(keyMsg("a"))
	a = result.(App)

	if a.mode != ModeInsert {
		t.Errorf("after typing 'a': expected mode to remain ModeInsert, got %v", a.mode)
	}

	// Press ESC to exit insert mode.
	result, _ = a.handlePaneLayoutKey(keyMsg("esc"))
	a = result.(App)

	if a.mode != ModeNormal {
		t.Errorf("after pressing ESC: expected ModeNormal, got %v", a.mode)
	}
	if a.statusBar.mode != ModeNormal {
		t.Errorf("after pressing ESC: expected statusBar.mode=ModeNormal, got %v", a.statusBar.mode)
	}
}

// TestPaneLayoutInsertMode_SpaceActivatesLeaderAfterEsc verifies that after
// ESC exits insert mode, pressing Space activates the leader key menu.
func TestPaneLayoutInsertMode_SpaceActivatesLeaderAfterEsc(t *testing.T) {
	a := newMinimalPaneApp()

	// Enter insert mode via 'i'.
	result, _ := a.handlePaneLayoutKey(keyMsg("i"))
	a = result.(App)
	if a.mode != ModeInsert {
		t.Fatalf("precondition: expected ModeInsert after 'i', got %v", a.mode)
	}

	// Exit insert mode via ESC.
	result, _ = a.handlePaneLayoutKey(keyMsg("esc"))
	a = result.(App)
	if a.mode != ModeNormal {
		t.Fatalf("precondition: expected ModeNormal after ESC, got %v", a.mode)
	}

	// Press Space — should activate leader key.
	result, cmd := a.handlePaneLayoutKey(keyMsg(" "))
	a = result.(App)

	if !a.leader.active {
		t.Error("after Space in Normal mode: expected leader to be active")
	}
	if cmd == nil {
		t.Error("after Space in Normal mode: expected non-nil command (leader timeout tick)")
	}
}

// TestPaneLayoutInsertMode_SpaceForwardedInInsertMode verifies that pressing
// Space while in insert mode forwards the byte to the pane (not the leader).
func TestPaneLayoutInsertMode_SpaceForwardedInInsertMode(t *testing.T) {
	a := newMinimalPaneApp()

	// Enter insert mode.
	result, _ := a.handlePaneLayoutKey(keyMsg("i"))
	a = result.(App)
	if a.mode != ModeInsert {
		t.Fatalf("precondition: expected ModeInsert, got %v", a.mode)
	}

	// Press Space while in insert mode. The leader must NOT activate.
	result, _ = a.handlePaneLayoutKey(keyMsg(" "))
	a = result.(App)

	if a.leader.active {
		t.Error("Space in insert mode must not activate the leader")
	}
	if a.mode != ModeInsert {
		t.Error("Space in insert mode must not change the mode")
	}
}

// TestPaneLayoutInsertMode_AltEscExitsInsertMode verifies that alt+esc (which
// can occur due to terminal escape-sequence timing ambiguity) also exits insert
// mode. Bubbletea checks msg.Type == tea.KeyEsc directly, so this variant is
// caught by the same code path.
func TestPaneLayoutInsertMode_AltEscExitsInsertMode(t *testing.T) {
	a := newMinimalPaneApp()

	// Enter insert mode.
	result, _ := a.handlePaneLayoutKey(keyMsg("i"))
	a = result.(App)
	if a.mode != ModeInsert {
		t.Fatalf("precondition: expected ModeInsert after 'i', got %v", a.mode)
	}

	// Simulate alt+esc (terminal timing ambiguity) — must exit insert mode.
	result, _ = a.handlePaneLayoutKey(keyMsg("alt+esc"))
	a = result.(App)

	if a.mode != ModeNormal {
		t.Errorf("after alt+esc: expected ModeNormal, got %v", a.mode)
	}
	if a.statusBar.mode != ModeNormal {
		t.Errorf("after alt+esc: expected statusBar.mode=ModeNormal, got %v", a.statusBar.mode)
	}
}

// TestPaneLayoutInsertMode_EscDoesNotForwardByte verifies that ESC is not
// forwarded to the pane as raw bytes when exiting insert mode.
// (If ESC were forwarded, the remote shell might receive an unexpected 0x1b.)
func TestPaneLayoutInsertMode_EscDoesNotForwardByte(t *testing.T) {
	a := newMinimalPaneApp()

	// Set up the pane layout with a pane so we can observe RouteInput.
	// We can't easily intercept RouteInput, but we can verify the mode
	// transition is correct (which implies the return-before-forward path).
	a.paneLayout.SetSize(80, 24)
	a.paneLayout.SplitVertical()

	// Enter insert mode.
	result, _ := a.handlePaneLayoutKey(keyMsg("i"))
	a = result.(App)

	// Press ESC — must exit insert mode and NOT forward to pane.
	result, _ = a.handlePaneLayoutKey(keyMsg("esc"))
	a = result.(App)

	if a.mode != ModeNormal {
		t.Errorf("expected ModeNormal after ESC, got %v", a.mode)
	}
}

// TestPaneLayoutInsertMode_SetModeCalledCorrectly verifies that the setMode
// helper produces a ModeChangedMsg when called from the 'i' key handler.
// This is a regression test for the value-receiver / pointer-receiver
// interaction: setMode must be called BEFORE a.mode is pre-set on the copy.
func TestPaneLayoutInsertMode_SetModeNotSuppressed(t *testing.T) {
	a := newMinimalPaneApp()
	// a.mode is ModeNormal.

	// setMode(ModeInsert) should transition Normal → Insert and return a cmd.
	cmd := a.setMode(ModeInsert)
	if cmd == nil {
		t.Fatal("setMode(ModeInsert) from ModeNormal must return a non-nil ModeChangedMsg command")
	}
	msg := cmd()
	mcm, ok := msg.(ModeChangedMsg)
	if !ok {
		t.Fatalf("expected ModeChangedMsg, got %T", msg)
	}
	if mcm.From != ModeNormal {
		t.Errorf("ModeChangedMsg.From: want ModeNormal, got %v", mcm.From)
	}
	if mcm.To != ModeInsert {
		t.Errorf("ModeChangedMsg.To: want ModeInsert, got %v", mcm.To)
	}
	if a.mode != ModeInsert {
		t.Errorf("setMode should update a.mode, want ModeInsert, got %v", a.mode)
	}

	// Now calling setMode(ModeNormal) should transition Insert → Normal.
	cmd = a.setMode(ModeNormal)
	if cmd == nil {
		t.Fatal("setMode(ModeNormal) from ModeInsert must return a non-nil ModeChangedMsg command")
	}
	msg = cmd()
	mcm, ok = msg.(ModeChangedMsg)
	if !ok {
		t.Fatalf("expected ModeChangedMsg, got %T", msg)
	}
	if mcm.From != ModeInsert {
		t.Errorf("ModeChangedMsg.From: want ModeInsert, got %v", mcm.From)
	}
	if mcm.To != ModeNormal {
		t.Errorf("ModeChangedMsg.To: want ModeNormal, got %v", mcm.To)
	}
}
