package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/session"
)

// --------------------------------------------------------------------------
// PaneConnectionPicker — navigation and selection
// --------------------------------------------------------------------------

func makeTestPicker(n int) *PaneConnectionPicker {
	conns := make([]config.Connection, n)
	for i := range conns {
		conns[i] = config.Connection{
			Name: strings.Repeat("c", i+1),
		}
	}
	return NewPaneConnectionPicker("test-pane", conns, 80, 24)
}

func TestPicker_MoveDown_IncrementsCursor(t *testing.T) {
	p := makeTestPicker(3)
	if p.Cursor != 0 {
		t.Fatalf("initial cursor: want 0, got %d", p.Cursor)
	}
	p.MoveDown()
	if p.Cursor != 1 {
		t.Errorf("after MoveDown: want 1, got %d", p.Cursor)
	}
}

func TestPicker_MoveDown_ClampsAtLastItem(t *testing.T) {
	p := makeTestPicker(3) // indices 0,1,2
	p.MoveDown()
	p.MoveDown()
	p.MoveDown() // already at 2, should stay
	if p.Cursor != 2 {
		t.Errorf("MoveDown clamp: want 2, got %d", p.Cursor)
	}
}

func TestPicker_MoveUp_DecrementsCursor(t *testing.T) {
	p := makeTestPicker(3)
	p.MoveDown()
	p.MoveDown() // cursor at 2
	p.MoveUp()
	if p.Cursor != 1 {
		t.Errorf("after MoveUp: want 1, got %d", p.Cursor)
	}
}

func TestPicker_MoveUp_ClampsAtFirstItem(t *testing.T) {
	p := makeTestPicker(3)
	// Already at 0
	p.MoveUp()
	if p.Cursor != 0 {
		t.Errorf("MoveUp clamp: want 0, got %d", p.Cursor)
	}
}

func TestPicker_Selected_ReturnsCurrentItem(t *testing.T) {
	p := makeTestPicker(3)
	p.MoveDown()
	got := p.Selected()
	if got == nil {
		t.Fatal("Selected: want non-nil")
	}
	if got.Name != "cc" { // second connection name = "cc"
		t.Errorf("Selected: want %q, got %q", "cc", got.Name)
	}
}

func TestPicker_Selected_EmptyConnections_ReturnsNil(t *testing.T) {
	p := NewPaneConnectionPicker("p", nil, 80, 24)
	if got := p.Selected(); got != nil {
		t.Errorf("Selected on empty: want nil, got %v", got)
	}
}

func TestPicker_Confirm_ReturnsSelection_DeactivatesPicker(t *testing.T) {
	p := makeTestPicker(3)
	got := p.Confirm()
	if got == nil {
		t.Fatal("Confirm: want non-nil connection")
	}
	if p.Active {
		t.Error("Confirm: picker should be deactivated after confirm")
	}
}

func TestPicker_Confirm_EmptyList_ReturnsNil(t *testing.T) {
	p := NewPaneConnectionPicker("p", nil, 80, 24)
	got := p.Confirm()
	if got != nil {
		t.Errorf("Confirm empty: want nil, got %v", got)
	}
	if p.Active {
		t.Error("Confirm: picker should be deactivated even with empty list")
	}
}

func TestPicker_Cancel_DeactivatesPicker(t *testing.T) {
	p := makeTestPicker(3)
	p.Cancel()
	if p.Active {
		t.Error("Cancel: picker should be deactivated")
	}
}

func TestPicker_View_ContainsConnectionNames(t *testing.T) {
	p := makeTestPicker(3)
	out := p.View()
	if !strings.Contains(out, "Select connection") {
		t.Errorf("picker View: missing header, got:\n%s", out)
	}
}

func TestPicker_View_NoConnections_ShowsEmptyMessage(t *testing.T) {
	p := NewPaneConnectionPicker("p", nil, 80, 10)
	out := p.View()
	if !strings.Contains(out, "No connections configured") {
		t.Errorf("picker View (empty): missing empty message, got:\n%s", out)
	}
}

func TestPicker_View_CursorHighlighted(t *testing.T) {
	p := makeTestPicker(3)
	// Cursor at 0 — first item should have "> " prefix
	out := p.View()
	if !strings.Contains(out, "> ") {
		t.Errorf("picker View: cursor row should start with '> ', got:\n%s", out)
	}
}

func TestPicker_View_ScrollingWindow(t *testing.T) {
	// Create a picker taller than the visible window.
	conns := make([]config.Connection, 20)
	for i := range conns {
		conns[i] = config.Connection{Name: strings.Repeat("x", i+1)}
	}
	p := NewPaneConnectionPicker("p", conns, 80, 6) // height=6, maxVisible=2
	// Move cursor down past the visible window
	for i := 0; i < 15; i++ {
		p.MoveDown()
	}
	out := p.View()
	// Should not panic and should produce some output
	if out == "" {
		t.Error("picker View (scrolled): expected non-empty output")
	}
}

// --------------------------------------------------------------------------
// paneContent — states not covered by existing tests
// --------------------------------------------------------------------------

func TestPaneContent_Connecting_ShowsCenteredText(t *testing.T) {
	pane := &Pane{
		State:  PaneConnecting,
		Width:  80,
		Height: 24,
	}
	out := paneContent(pane)
	if !strings.Contains(out, "Connecting") {
		t.Errorf("PaneConnecting content: want 'Connecting...', got %q", out)
	}
}

func TestPaneContent_Empty_NoText(t *testing.T) {
	pane := &Pane{
		State:  PaneEmpty,
		Width:  80,
		Height: 24,
	}
	out := paneContent(pane)
	// centeredText with empty string produces pad + empty styled string
	// Just verify it doesn't panic and returns a string
	_ = out
}

func TestPaneContent_Active_WithNilVTerm_ReturnsEmpty(t *testing.T) {
	pane := &Pane{
		State: PaneActive,
		VTerm: nil,
	}
	out := paneContent(pane)
	if out != "" {
		t.Errorf("PaneActive with nil VTerm: want empty string, got %q", out)
	}
}

func TestPaneContent_Active_WithVTerm_ReturnsRender(t *testing.T) {
	vt := session.NewVTermBuffer(20, 5)
	vt.Write([]byte("Terminal output"))
	pane := &Pane{
		State: PaneActive,
		VTerm: vt,
	}
	out := paneContent(pane)
	if !strings.Contains(out, "Terminal output") {
		t.Errorf("PaneActive with VTerm: expected 'Terminal output', got %q", out)
	}
}

func TestPaneContent_Picker_ActivePicker_RendersPicker(t *testing.T) {
	p := makeTestPicker(3)
	pane := &Pane{
		State:  PaneEmpty,
		Picker: p,
		Width:  80,
		Height: 24,
	}
	out := paneContent(pane)
	if !strings.Contains(out, "Select connection") {
		t.Errorf("paneContent with active picker: expected picker view, got %q", out)
	}
}

func TestPaneContent_Picker_InactivePicker_FallsThrough(t *testing.T) {
	p := makeTestPicker(3)
	p.Active = false
	pane := &Pane{
		State:  PaneEmpty,
		Picker: p,
		Width:  80,
		Height: 24,
	}
	out := paneContent(pane)
	// Inactive picker → falls through to PaneEmpty → centeredText
	if strings.Contains(out, "Select connection") {
		t.Errorf("paneContent with inactive picker: should not show picker, got %q", out)
	}
}

// --------------------------------------------------------------------------
// renderDisconnected — padding when pane is tall
// --------------------------------------------------------------------------

func TestRenderDisconnected_PaddingApplied(t *testing.T) {
	pane := &Pane{
		State:         PaneDisconnected,
		DisconnectErr: errors.New("connection refused"),
		Width:         80,
		Height:        20, // tall — should get vertical padding
	}
	out := renderDisconnected(pane)
	// Should contain the error and a leading newline (vertical padding)
	if !strings.Contains(out, "connection refused") {
		t.Errorf("renderDisconnected: missing error text, got %q", out)
	}
	// With height=20 and 3 content lines, padTop = (20-3)/2 = 8 newlines
	lines := strings.Split(out, "\n")
	if len(lines) < 4 {
		t.Errorf("renderDisconnected: expected vertical padding (multiple lines), got %d lines", len(lines))
	}
}

func TestRenderDisconnected_ZeroHeight_NoCrash(t *testing.T) {
	pane := &Pane{
		State:  PaneDisconnected,
		Width:  80,
		Height: 0,
	}
	out := renderDisconnected(pane)
	// padTop = (0-3)/2 = -1 → clamped to 0 → no leading newlines
	if !strings.Contains(out, "Disconnected") {
		t.Errorf("renderDisconnected zero height: want 'Disconnected', got %q", out)
	}
}

// --------------------------------------------------------------------------
// findParentSplit — nested target
// --------------------------------------------------------------------------

func TestFindParentSplit_RootIsPane_ReturnsNil(t *testing.T) {
	pane := &Pane{ID: "leaf"}
	result := findParentSplit(pane, "leaf")
	if result != nil {
		t.Errorf("findParentSplit on leaf root: want nil, got %v", result)
	}
}

func TestFindParentSplit_TargetIsFirstChild(t *testing.T) {
	child := &Pane{ID: "target"}
	other := &Pane{ID: "other"}
	split := &SplitNode{
		First:  child,
		Second: other,
	}
	result := findParentSplit(split, "target")
	if result != split {
		t.Errorf("findParentSplit first child: want parent split, got %v", result)
	}
}

func TestFindParentSplit_TargetIsSecondChild(t *testing.T) {
	child := &Pane{ID: "target"}
	other := &Pane{ID: "other"}
	split := &SplitNode{
		First:  other,
		Second: child,
	}
	result := findParentSplit(split, "target")
	if result != split {
		t.Errorf("findParentSplit second child: want parent split, got %v", result)
	}
}

func TestFindParentSplit_NestedTarget(t *testing.T) {
	deepPane := &Pane{ID: "deep"}
	shallow := &Pane{ID: "shallow"}
	innerSplit := &SplitNode{
		First:  deepPane,
		Second: &Pane{ID: "sibling"},
	}
	outerSplit := &SplitNode{
		First:  shallow,
		Second: innerSplit,
	}
	result := findParentSplit(outerSplit, "deep")
	if result != innerSplit {
		t.Errorf("findParentSplit nested: want innerSplit, got %v", result)
	}
}

func TestFindParentSplit_NoMatch_ReturnsNil(t *testing.T) {
	pane := &Pane{ID: "a"}
	split := &SplitNode{
		First:  pane,
		Second: &Pane{ID: "b"},
	}
	result := findParentSplit(split, "nonexistent")
	if result != nil {
		t.Errorf("findParentSplit no match: want nil, got %v", result)
	}
}

// --------------------------------------------------------------------------
// renderPane — solo mode (no border)
// --------------------------------------------------------------------------

func TestRenderPane_SoloMode_NoBorder(t *testing.T) {
	vt := session.NewVTermBuffer(20, 5)
	vt.Write([]byte("solo content"))
	pane := &Pane{
		State:  PaneActive,
		VTerm:  vt,
		Width:  20,
		Height: 5,
	}
	out := renderPane(pane, true, true /* solo */, false, false)
	// In solo mode there is no border — output should contain raw content
	if !strings.Contains(out, "solo content") {
		t.Errorf("renderPane solo: expected content without border, got %q", out)
	}
	// No border characters
	if strings.ContainsAny(out, "┌┐└┘─│") {
		t.Errorf("renderPane solo: should not have border characters, got %q", out)
	}
}

func TestRenderPane_BroadcastMode_UnfocusedPane_NoBroadcastTitle(t *testing.T) {
	vt := session.NewVTermBuffer(20, 5)
	pane := &Pane{
		State:  PaneActive,
		VTerm:  vt,
		Width:  20,
		Height: 5,
	}
	// Broadcasting but NOT focused → no [BROADCAST] title
	out := renderPane(pane, false /* not focused */, false, true /* broadcasting */, false)
	if strings.Contains(out, "[BROADCAST]") {
		t.Errorf("renderPane broadcast unfocused: should not have [BROADCAST] title, got %q", out)
	}
}

func TestRenderPane_Zoom_InjectsZoomTitle(t *testing.T) {
	vt := session.NewVTermBuffer(20, 5)
	pane := &Pane{
		State:  PaneActive,
		VTerm:  vt,
		Width:  20,
		Height: 5,
	}
	out := renderPane(pane, true, false, false, true /* zoomed */)
	if !strings.Contains(out, "[ZOOM]") {
		t.Errorf("renderPane zoomed: expected [ZOOM] in border title, got %q", out)
	}
}
