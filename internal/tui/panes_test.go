package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/session"
)

// ---- helpers ----------------------------------------------------------------

// newTestLayout returns a layout pre-sized to 200x50 with an initial pane
// already bootstrapped.
func newTestLayout(width, height int) *PaneLayoutModel {
	m := NewPaneLayoutModel()
	m.SetSize(width, height)
	// Bootstrap first pane.
	m.SplitVertical() // with no root, this creates the first pane.
	// The bootstrapped pane is the root.
	return m
}

// ---- split tree operations --------------------------------------------------

func TestSplitVertical_BasicSplit(t *testing.T) {
	m := newTestLayout(200, 50)

	if m.PaneCount() != 1 {
		t.Fatalf("expected 1 pane after bootstrap, got %d", m.PaneCount())
	}

	m.SplitVertical()

	if m.PaneCount() != 2 {
		t.Fatalf("expected 2 panes after vertical split, got %d", m.PaneCount())
	}

	root, ok := m.Root.(*SplitNode)
	if !ok {
		t.Fatal("expected root to be a SplitNode after split")
	}
	if root.Orientation != SplitVertical {
		t.Errorf("expected SplitVertical orientation, got %v", root.Orientation)
	}
	if root.Ratio != 0.5 {
		t.Errorf("expected default ratio 0.5, got %f", root.Ratio)
	}
}

func TestSplitHorizontal_BasicSplit(t *testing.T) {
	m := newTestLayout(200, 50)

	m.SplitHorizontal()

	if m.PaneCount() != 2 {
		t.Fatalf("expected 2 panes after horizontal split, got %d", m.PaneCount())
	}

	root, ok := m.Root.(*SplitNode)
	if !ok {
		t.Fatal("expected root to be a SplitNode after split")
	}
	if root.Orientation != SplitHorizontal {
		t.Errorf("expected SplitHorizontal orientation, got %v", root.Orientation)
	}
}

func TestSplitVertical_IDsAreUnique(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()
	m.SplitVertical()

	seen := make(map[string]bool)
	for id := range m.Panes {
		if seen[id] {
			t.Errorf("duplicate pane ID: %s", id)
		}
		seen[id] = true
	}
	if len(seen) != m.PaneCount() {
		t.Errorf("pane ID count mismatch: map has %d, PaneCount=%d", len(seen), m.PaneCount())
	}
}

func TestSplitTooSmall_ReturnsErrorCmd(t *testing.T) {
	// A 41-wide layout has one pane of 41 cols. Splitting it vertically gives
	// (41-1)/2 = 20 cols — exactly at the minimum. Let's go one smaller.
	m := newTestLayout(39, 50) // first pane will be 39 wide

	cmd := m.SplitVertical()
	if cmd == nil {
		t.Fatal("expected non-nil error command when pane too small to split")
	}

	result := cmd()
	if result == nil {
		t.Fatal("expected command to produce a non-nil result")
	}
	if _, ok := result.(error); !ok {
		t.Errorf("expected result to be an error, got %T", result)
	}
	// Pane count should not have changed.
	if m.PaneCount() != 1 {
		t.Errorf("expected 1 pane after failed split, got %d", m.PaneCount())
	}
}

func TestSplitMinimumSize_ExactlyAllowed(t *testing.T) {
	// 41-wide layout: (41-1)/2 = 20 — exactly the minimum.
	m := newTestLayout(41, 50)

	cmd := m.SplitVertical()
	if cmd != nil {
		// Could be an error command if the constraint is strict >=; check.
		result := cmd()
		if _, ok := result.(error); ok {
			t.Fatalf("split should be allowed at exactly minimum size (41 cols), got error")
		}
	}
}

// ---- SetSize recursive dimension calculation --------------------------------

func TestSetSize_DistributesWidth(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()

	// After a vertical split of 200 cols with ratio=0.5:
	// left = floor(0.5 * 199) = 99, right = 199 - 99 = 100
	root, ok := m.Root.(*SplitNode)
	if !ok {
		t.Fatal("expected SplitNode root")
	}
	left := root.First.(*Pane)
	right := root.Second.(*Pane)

	if left.Width+right.Width != 199 {
		t.Errorf("left.Width (%d) + right.Width (%d) should equal 199 (200-1 for border)", left.Width, right.Width)
	}
	if left.Height != 50 || right.Height != 50 {
		t.Errorf("expected both panes height=50, got left=%d right=%d", left.Height, right.Height)
	}
}

func TestSetSize_DistributesHeight(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitHorizontal()

	root, ok := m.Root.(*SplitNode)
	if !ok {
		t.Fatal("expected SplitNode root")
	}
	top := root.First.(*Pane)
	bot := root.Second.(*Pane)

	if top.Height+bot.Height != 49 {
		t.Errorf("top.Height (%d) + bot.Height (%d) should equal 49 (50-1 for border)", top.Height, bot.Height)
	}
	if top.Width != 200 || bot.Width != 200 {
		t.Errorf("expected both panes width=200, got top=%d bot=%d", top.Width, bot.Width)
	}
}

func TestSetSize_Resize(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()

	m.SetSize(100, 40)

	root := m.Root.(*SplitNode)
	left := root.First.(*Pane)
	right := root.Second.(*Pane)

	if left.Width+right.Width != 99 {
		t.Errorf("after resize, widths should sum to 99, got %d+%d", left.Width, right.Width)
	}
	if left.Height != 40 || right.Height != 40 {
		t.Errorf("after resize, height should be 40, got %d/%d", left.Height, right.Height)
	}
}

// ---- FocusDirection navigation ---------------------------------------------

func TestFocusDirection_VerticalSplit_LeftRight(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()

	// After split: active pane is the original (left), new pane is right.
	root := m.Root.(*SplitNode)
	leftPane := root.First.(*Pane)
	rightPane := root.Second.(*Pane)

	// Ensure active is left.
	m.ActivePaneID = leftPane.ID
	leftPane.Focused = true
	rightPane.Focused = false

	// Move right.
	m.FocusDirection(DirRight)

	if m.ActivePaneID != rightPane.ID {
		t.Errorf("expected focus on right pane %s, got %s", rightPane.ID, m.ActivePaneID)
	}
	if !rightPane.Focused {
		t.Error("right pane should be focused")
	}
	if leftPane.Focused {
		t.Error("left pane should not be focused")
	}

	// Move back left.
	m.FocusDirection(DirLeft)

	if m.ActivePaneID != leftPane.ID {
		t.Errorf("expected focus back on left pane %s, got %s", leftPane.ID, m.ActivePaneID)
	}
	if !leftPane.Focused {
		t.Error("left pane should be focused")
	}
	if rightPane.Focused {
		t.Error("right pane should not be focused")
	}
}

func TestFocusDirection_HorizontalSplit_UpDown(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitHorizontal()

	root := m.Root.(*SplitNode)
	topPane := root.First.(*Pane)
	botPane := root.Second.(*Pane)

	// Ensure active is top.
	m.ActivePaneID = topPane.ID
	topPane.Focused = true
	botPane.Focused = false

	// Move down.
	m.FocusDirection(DirDown)

	if m.ActivePaneID != botPane.ID {
		t.Errorf("expected focus on bottom pane %s, got %s", botPane.ID, m.ActivePaneID)
	}
	if !botPane.Focused {
		t.Error("bottom pane should be focused")
	}
	if topPane.Focused {
		t.Error("top pane should not be focused after moving down")
	}

	// Move back up.
	m.FocusDirection(DirUp)

	if m.ActivePaneID != topPane.ID {
		t.Errorf("expected focus back on top pane %s, got %s", topPane.ID, m.ActivePaneID)
	}
}

func TestFocusDirection_NoAdjacentPane_NoOp(t *testing.T) {
	m := newTestLayout(200, 50)
	// Single pane — no adjacent panes exist.

	activeID := m.ActivePaneID
	m.FocusDirection(DirRight)

	if m.ActivePaneID != activeID {
		t.Errorf("expected ActivePaneID unchanged, got %s", m.ActivePaneID)
	}
}

func TestFocusDirection_AtEdge_NoOp(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()

	root := m.Root.(*SplitNode)
	rightPane := root.Second.(*Pane)

	// Focus the right pane and try to go further right — should no-op.
	m.ActivePaneID = rightPane.ID
	rightPane.Focused = true
	root.First.(*Pane).Focused = false

	m.FocusDirection(DirRight)

	if m.ActivePaneID != rightPane.ID {
		t.Errorf("expected no change when at right edge, got active=%s", m.ActivePaneID)
	}
}

// ---- PaneCount / AllPanes --------------------------------------------------

func TestPaneCount(t *testing.T) {
	m := NewPaneLayoutModel()
	if m.PaneCount() != 0 {
		t.Errorf("expected 0 panes initially, got %d", m.PaneCount())
	}

	m.SetSize(200, 50)
	m.SplitVertical() // bootstrap first pane
	if m.PaneCount() != 1 {
		t.Errorf("expected 1 after bootstrap, got %d", m.PaneCount())
	}

	m.SplitVertical()
	if m.PaneCount() != 2 {
		t.Errorf("expected 2 after split, got %d", m.PaneCount())
	}

	m.SplitHorizontal()
	if m.PaneCount() != 3 {
		t.Errorf("expected 3 after second split, got %d", m.PaneCount())
	}
}

func TestAllPanes_MatchesPaneCount(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()

	all := m.AllPanes()
	if len(all) != m.PaneCount() {
		t.Errorf("AllPanes() returned %d, PaneCount()=%d", len(all), m.PaneCount())
	}
}

// ---- ActivePane ------------------------------------------------------------

func TestActivePane_ReturnsCorrectPane(t *testing.T) {
	m := newTestLayout(200, 50)

	active := m.ActivePane()
	if active == nil {
		t.Fatal("expected non-nil ActivePane")
	}
	if active.ID != m.ActivePaneID {
		t.Errorf("ActivePane().ID=%s, ActivePaneID=%s", active.ID, m.ActivePaneID)
	}
}

func TestActivePane_NilWhenEmpty(t *testing.T) {
	m := NewPaneLayoutModel()
	if m.ActivePane() != nil {
		t.Error("expected nil ActivePane on empty model")
	}
}

// ---- RouteInput ------------------------------------------------------------

func TestRouteInput_SkipsEmptyPane(t *testing.T) {
	m := newTestLayout(200, 50)

	// Active pane is in PaneEmpty state — RouteInput should not panic.
	active := m.ActivePane()
	if active.State != PaneEmpty {
		t.Skipf("pane state is not PaneEmpty, skipping")
	}
	// Should not panic.
	m.RouteInput([]byte("hello"))
}

// ---- EqualizeAll -----------------------------------------------------------

func TestEqualizeAll_ResetsRatios(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()

	root := m.Root.(*SplitNode)
	root.Ratio = 0.7

	m.EqualizeAll()

	if root.Ratio != 0.5 {
		t.Errorf("expected ratio reset to 0.5, got %f", root.Ratio)
	}
}

// ---- replaceLeaf -----------------------------------------------------------

func TestReplaceLeaf_ReplacesCorrectNode(t *testing.T) {
	paneA := &Pane{ID: "a"}
	paneB := &Pane{ID: "b"}
	replacement := &SplitNode{Orientation: SplitVertical, Ratio: 0.5, First: paneA, Second: paneB}

	result := replaceLeaf(paneA, "a", replacement)
	sn, ok := result.(*SplitNode)
	if !ok {
		t.Fatalf("expected SplitNode result, got %T", result)
	}
	if sn != replacement {
		t.Error("expected replacement to be returned")
	}
}

func TestReplaceLeaf_LeavesNonMatchingPanesAlone(t *testing.T) {
	paneA := &Pane{ID: "a"}
	paneC := &Pane{ID: "c"}

	result := replaceLeaf(paneA, "x", paneC)
	p, ok := result.(*Pane)
	if !ok {
		t.Fatalf("expected *Pane result, got %T", result)
	}
	if p.ID != "a" {
		t.Errorf("expected original pane, got ID=%s", p.ID)
	}
}

// ---- collectLeaves ---------------------------------------------------------

func TestCollectLeaves_FlatTree(t *testing.T) {
	paneA := &Pane{ID: "a"}
	paneB := &Pane{ID: "b"}
	root := &SplitNode{
		Orientation: SplitVertical,
		First:       paneA,
		Second:      paneB,
	}

	var leaves []*Pane
	collectLeaves(root, &leaves)

	if len(leaves) != 2 {
		t.Fatalf("expected 2 leaves, got %d", len(leaves))
	}
}

func TestCollectLeaves_DeepTree(t *testing.T) {
	paneA := &Pane{ID: "a"}
	paneB := &Pane{ID: "b"}
	paneC := &Pane{ID: "c"}
	inner := &SplitNode{Orientation: SplitHorizontal, First: paneA, Second: paneB}
	root := &SplitNode{Orientation: SplitVertical, First: inner, Second: paneC}

	var leaves []*Pane
	collectLeaves(root, &leaves)

	if len(leaves) != 3 {
		t.Fatalf("expected 3 leaves, got %d", len(leaves))
	}
}

// ---- View ------------------------------------------------------------------

func TestView_EmptyModelReturnsEmpty(t *testing.T) {
	m := NewPaneLayoutModel()
	if m.View() != "" {
		t.Errorf("expected empty view for empty model, got %q", m.View())
	}
}

func TestView_SinglePane_ProducesOutput(t *testing.T) {
	m := newTestLayout(80, 24)
	out := m.View()
	if out == "" {
		t.Error("expected non-empty view for single pane")
	}
}

func TestView_SplitPane_ProducesOutput(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()
	out := m.View()
	if out == "" {
		t.Error("expected non-empty view after split")
	}
}

// ---- OpenPickerForPane / LastCreatedPaneID ---------------------------------

func TestLastCreatedPaneID_SetOnNewPane(t *testing.T) {
	m := newTestLayout(200, 50)
	// Bootstrap creates first pane; LastCreatedPaneID should be set.
	if m.LastCreatedPaneID == "" {
		t.Error("expected LastCreatedPaneID to be set after bootstrap")
	}
	firstID := m.LastCreatedPaneID

	m.SplitVertical()
	if m.LastCreatedPaneID == firstID {
		t.Errorf("expected LastCreatedPaneID to change after split, still %q", firstID)
	}
	if m.LastCreatedPaneID == "" {
		t.Error("LastCreatedPaneID must not be empty after split")
	}
}

func TestOpenPickerForPane_AttachesPicker(t *testing.T) {
	m := newTestLayout(200, 50)
	active := m.ActivePane()

	conns := []config.Connection{
		{ID: "c1", Name: "server-1", Host: "10.0.0.1"},
	}
	m.OpenPickerForPane(active.ID, conns)

	if active.Picker == nil {
		t.Fatal("expected picker to be attached after OpenPickerForPane")
	}
	if !active.Picker.Active {
		t.Error("expected picker to be active after attachment")
	}
	if len(active.Picker.Connections) != 1 {
		t.Errorf("expected 1 connection in picker, got %d", len(active.Picker.Connections))
	}
}

func TestOpenPickerForPane_NoopOnUnknownID(t *testing.T) {
	m := newTestLayout(200, 50)
	// Should not panic.
	m.OpenPickerForPane("no-such-pane", nil)
}

// ---- panePickerCancelMsg ---------------------------------------------------

func TestUpdate_PanePickerCancelMsg_ClearsPicker(t *testing.T) {
	m := newTestLayout(200, 50)
	active := m.ActivePane()

	conns := []config.Connection{{ID: "c1", Name: "server-1"}}
	m.OpenPickerForPane(active.ID, conns)

	if active.Picker == nil {
		t.Fatal("precondition: picker must be attached")
	}

	updated, _ := m.Update(panePickerCancelMsg{PaneID: active.ID})

	pane := updated.Panes[active.ID]
	if pane.Picker != nil {
		t.Error("expected picker to be nil after cancel")
	}
	if pane.State != PaneEmpty {
		t.Errorf("expected pane to remain PaneEmpty after cancel, got %v", pane.State)
	}
}

// ---- panePickerMsg — transition to Connecting -----------------------------

func TestUpdate_PanePickerMsg_TransitionsToConnecting(t *testing.T) {
	m := newTestLayout(80, 24)
	active := m.ActivePane()
	paneID := active.ID

	conn := config.Connection{
		ID:       "c1",
		Name:     "server-1",
		Host:     "127.0.0.1",
		Port:     22,
		Protocol: config.ProtoSSH,
	}

	// Feed a panePickerMsg directly. startBackgroundSession will fail because
	// there is no real SSH server, but the state transition to Connecting and
	// the VTerm allocation should happen before the async command fires.
	updated, cmd := m.Update(panePickerMsg{PaneID: paneID, Connection: conn})

	pane := updated.Panes[paneID]
	if pane.State != PaneConnecting {
		t.Errorf("expected PaneConnecting after picker confirm, got %v", pane.State)
	}
	if pane.VTerm == nil {
		t.Error("expected VTerm to be allocated after picker confirm")
	}
	if pane.Picker != nil {
		t.Error("expected picker to be cleared after confirm")
	}
	// There should be a command to start the background session.
	if cmd == nil {
		t.Error("expected a non-nil command to start the background session")
	}
}

// ---- Update messages -------------------------------------------------------

func TestUpdate_WindowSizeMsg_SetsSize(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	if updated.Width != 120 || updated.Height != 30 {
		t.Errorf("expected size 120x30, got %dx%d", updated.Width, updated.Height)
	}
}

func TestUpdate_PaneConnectedMsg_SetsStateActive(t *testing.T) {
	m := newTestLayout(200, 50)
	active := m.ActivePane()

	updated, cmd := m.Update(PaneConnectedMsg{PaneID: active.ID, SessionID: "sess-1"})

	pane := updated.Panes[active.ID]
	if pane.State != PaneActive {
		t.Errorf("expected PaneActive state, got %v", pane.State)
	}
	if cmd == nil {
		t.Error("expected tick command to be returned on first PaneConnectedMsg")
	}
}

func TestUpdate_PaneDisconnectedMsg_SetsStateDisconnected(t *testing.T) {
	m := newTestLayout(200, 50)
	active := m.ActivePane()

	updated, _ := m.Update(PaneDisconnectedMsg{PaneID: active.ID, SessionID: "sess-1"})

	pane := updated.Panes[active.ID]
	if pane.State != PaneDisconnected {
		t.Errorf("expected PaneDisconnected state, got %v", pane.State)
	}
}

func TestUpdate_PaneOutputMsg_WritesToVTerm(t *testing.T) {
	m := newTestLayout(80, 24)
	active := m.ActivePane()

	// Give the pane a VTerm buffer so output can be written.
	active.VTerm = session.NewVTermBuffer(80, 24)
	active.State = PaneActive

	data := []byte("hello")
	m.Update(PaneOutputMsg{PaneID: active.ID, Data: data})

	// VTerm should have consumed the write without panic.
	// The rendered output should contain the text.
	rendered := active.VTerm.Render()
	if len(rendered) == 0 {
		t.Error("expected non-empty VTerm render after write")
	}
}

// ---- IsBroadcasting / ToggleBroadcast --------------------------------------

func TestToggleBroadcast(t *testing.T) {
	m := NewPaneLayoutModel()

	if m.IsBroadcasting() {
		t.Error("expected broadcast off initially")
	}

	m.ToggleBroadcast()
	if !m.IsBroadcasting() {
		t.Error("expected broadcast on after toggle")
	}

	m.ToggleBroadcast()
	if m.IsBroadcasting() {
		t.Error("expected broadcast off after second toggle")
	}
}

func TestToggleBroadcast_ReturnsFlashCmd(t *testing.T) {
	m := newTestLayout(200, 50)

	// Toggle on — should produce a flash message.
	cmd := m.ToggleBroadcast()
	if cmd == nil {
		t.Fatal("expected non-nil cmd from ToggleBroadcast")
	}
	msg := cmd()
	flash, ok := msg.(broadcastFlashMsg)
	if !ok {
		t.Fatalf("expected broadcastFlashMsg, got %T", msg)
	}
	if flash.text == "" {
		t.Error("expected non-empty flash text when broadcast toggled on")
	}

	// Toggle off — should produce "Broadcast off" flash.
	cmd = m.ToggleBroadcast()
	if cmd == nil {
		t.Fatal("expected non-nil cmd from ToggleBroadcast (off)")
	}
	msg = cmd()
	flash, ok = msg.(broadcastFlashMsg)
	if !ok {
		t.Fatalf("expected broadcastFlashMsg on toggle off, got %T", msg)
	}
	if flash.text != "Broadcast off" {
		t.Errorf("expected 'Broadcast off', got %q", flash.text)
	}
}

func TestToggleBroadcast_CountsActivePanes(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical() // now 2 panes

	// Mark both panes as active.
	for _, p := range m.Panes {
		p.State = PaneActive
	}

	cmd := m.ToggleBroadcast()
	msg := cmd()
	flash := msg.(broadcastFlashMsg)

	// Should mention 2 sessions.
	if flash.text != "Broadcasting to 2 sessions" {
		t.Errorf("expected 'Broadcasting to 2 sessions', got %q", flash.text)
	}
}

// ---- RouteInput broadcast fan-out ------------------------------------------

func TestRouteInput_BroadcastOff_NoActiveSession_NoPanic(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical() // 2 panes

	root := m.Root.(*SplitNode)
	left := root.First.(*Pane)

	// Active pane is left with nil session — should not panic.
	left.State = PaneActive
	m.ActivePaneID = left.ID
	m.BroadcastMode = false

	m.RouteInput([]byte("hello"))
}

func TestRouteInput_BroadcastOn_SkipsNonActivePanes_NoPanic(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical() // 2 panes

	root := m.Root.(*SplitNode)
	left := root.First.(*Pane)
	right := root.Second.(*Pane)

	// Only one pane active; the other stays empty — should not panic.
	left.State = PaneActive
	right.State = PaneEmpty
	m.BroadcastMode = true

	m.RouteInput([]byte("data"))
}

// ---- renderPane broadcast styling ------------------------------------------

func TestRenderPane_BroadcastOn_ActivePane_ProducesOutput(t *testing.T) {
	pane := &Pane{
		ID:      "p1",
		Width:   80,
		Height:  24,
		State:   PaneActive,
		Focused: true,
	}
	out := renderPane(pane, true, false, true, false)
	if out == "" {
		t.Error("expected non-empty output for broadcast-active pane")
	}
}

func TestRenderPane_BroadcastOff_FocusedPane_ProducesOutput(t *testing.T) {
	pane := &Pane{
		ID:      "p1",
		Width:   80,
		Height:  24,
		State:   PaneActive,
		Focused: true,
	}
	out := renderPane(pane, true, false, false, false)
	if out == "" {
		t.Error("expected non-empty output for focused pane without broadcast")
	}
}

// ---- injectBorderTitle -----------------------------------------------------

func TestInjectBorderTitle_InjectsTitleIntoFirstLine(t *testing.T) {
	// NormalBorder top-left is "┌", horizontal filler is "─".
	input := "┌──────────┐\n│  content │\n└──────────┘"
	out := injectBorderTitle(input, "[B]")
	lines := strings.SplitN(out, "\n", 2)
	topLine := []rune(lines[0])
	// Rune at index 1 should be '[', 2 should be 'B', 3 should be ']'.
	if len(topLine) < 4 || topLine[1] != '[' || topLine[2] != 'B' || topLine[3] != ']' {
		t.Errorf("expected title at start of top border, got: %s", string(topLine))
	}
}

func TestInjectBorderTitle_EmptyInput_NoPanic(t *testing.T) {
	out := injectBorderTitle("", "[BROADCAST]")
	if out != "" {
		t.Errorf("expected empty output for empty input, got %q", out)
	}
}

func TestInjectBorderTitle_ShortLine_NoPanic(t *testing.T) {
	// Line too short for the title — should truncate, not panic.
	out := injectBorderTitle("┌┐\nrest", "[VERYLONGTITLE]")
	if out == "" {
		t.Error("expected non-empty output")
	}
}

// ---- ApplyPreset -----------------------------------------------------------

func TestApplyPreset_2h_ProducesCorrectTree(t *testing.T) {
	m := newTestLayout(200, 50)
	m.ApplyPreset("2h")

	if m.PaneCount() != 2 {
		t.Fatalf("expected 2 panes for '2h', got %d", m.PaneCount())
	}
	root, ok := m.Root.(*SplitNode)
	if !ok {
		t.Fatal("expected SplitNode root for '2h'")
	}
	if root.Orientation != SplitHorizontal {
		t.Errorf("expected SplitHorizontal orientation for '2h', got %v", root.Orientation)
	}
	if root.Ratio != 0.5 {
		t.Errorf("expected ratio 0.5 for '2h', got %f", root.Ratio)
	}
}

func TestApplyPreset_2v_ProducesCorrectTree(t *testing.T) {
	m := newTestLayout(200, 50)
	m.ApplyPreset("2v")

	if m.PaneCount() != 2 {
		t.Fatalf("expected 2 panes for '2v', got %d", m.PaneCount())
	}
	root, ok := m.Root.(*SplitNode)
	if !ok {
		t.Fatal("expected SplitNode root for '2v'")
	}
	if root.Orientation != SplitVertical {
		t.Errorf("expected SplitVertical orientation for '2v', got %v", root.Orientation)
	}
}

func TestApplyPreset_3v_ProducesThreeColumns(t *testing.T) {
	m := newTestLayout(200, 50)
	m.ApplyPreset("3v")

	if m.PaneCount() != 3 {
		t.Fatalf("expected 3 panes for '3v', got %d", m.PaneCount())
	}
	root, ok := m.Root.(*SplitNode)
	if !ok {
		t.Fatal("expected SplitNode root for '3v'")
	}
	if root.Orientation != SplitVertical {
		t.Errorf("expected SplitVertical root orientation for '3v', got %v", root.Orientation)
	}
	// Inner node (right child) should also be a vertical split.
	inner, ok := root.Second.(*SplitNode)
	if !ok {
		t.Fatal("expected SplitNode as right child of '3v' root")
	}
	if inner.Orientation != SplitVertical {
		t.Errorf("expected SplitVertical inner orientation for '3v', got %v", inner.Orientation)
	}
}

func TestApplyPreset_2x2_ProducesFourPanes(t *testing.T) {
	m := newTestLayout(200, 50)
	m.ApplyPreset("2x2")

	if m.PaneCount() != 4 {
		t.Fatalf("expected 4 panes for '2x2', got %d", m.PaneCount())
	}
	root, ok := m.Root.(*SplitNode)
	if !ok {
		t.Fatal("expected SplitNode root for '2x2'")
	}
	if root.Orientation != SplitVertical {
		t.Errorf("expected SplitVertical root orientation for '2x2', got %v", root.Orientation)
	}
	// Both children should be horizontal splits.
	leftCol, ok := root.First.(*SplitNode)
	if !ok {
		t.Fatal("expected left column to be a SplitNode for '2x2'")
	}
	if leftCol.Orientation != SplitHorizontal {
		t.Errorf("expected SplitHorizontal left column for '2x2', got %v", leftCol.Orientation)
	}
	rightCol, ok := root.Second.(*SplitNode)
	if !ok {
		t.Fatal("expected right column to be a SplitNode for '2x2'")
	}
	if rightCol.Orientation != SplitHorizontal {
		t.Errorf("expected SplitHorizontal right column for '2x2', got %v", rightCol.Orientation)
	}
}

func TestApplyPreset_MainSide_ProducesCorrectRatio(t *testing.T) {
	m := newTestLayout(200, 50)
	m.ApplyPreset("main-side")

	if m.PaneCount() != 2 {
		t.Fatalf("expected 2 panes for 'main-side', got %d", m.PaneCount())
	}
	root, ok := m.Root.(*SplitNode)
	if !ok {
		t.Fatal("expected SplitNode root for 'main-side'")
	}
	if root.Orientation != SplitVertical {
		t.Errorf("expected SplitVertical orientation for 'main-side', got %v", root.Orientation)
	}
	if root.Ratio != 0.70 {
		t.Errorf("expected ratio 0.70 for 'main-side', got %f", root.Ratio)
	}
}

func TestApplyPreset_Unknown_IsNoOp(t *testing.T) {
	m := newTestLayout(200, 50)
	initialCount := m.PaneCount()

	m.ApplyPreset("does-not-exist")

	// Unknown preset must not modify the layout — pane count must be unchanged.
	if m.PaneCount() != initialCount {
		t.Errorf("expected %d panes after unknown preset (no-op), got %d", initialCount, m.PaneCount())
	}
}

func TestApplyPreset_ClearsExistingTree(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()
	m.SplitHorizontal()

	// Three panes before applying preset.
	if m.PaneCount() != 3 {
		t.Fatalf("precondition: expected 3 panes, got %d", m.PaneCount())
	}

	m.ApplyPreset("2v")

	if m.PaneCount() != 2 {
		t.Errorf("expected 2 panes after applying '2v' preset to existing tree, got %d", m.PaneCount())
	}
}

func TestApplyPreset_SetsActivePaneToFirst(t *testing.T) {
	m := newTestLayout(200, 50)
	m.ApplyPreset("2v")

	if m.ActivePaneID == "" {
		t.Fatal("expected ActivePaneID to be set after applying preset")
	}
	activePane := m.ActivePane()
	if activePane == nil {
		t.Fatal("expected non-nil ActivePane after applying preset")
	}
	if !activePane.Focused {
		t.Error("expected active pane to have Focused=true")
	}

	// Only the first pane should be focused.
	focusCount := 0
	for _, p := range m.Panes {
		if p.Focused {
			focusCount++
		}
	}
	if focusCount != 1 {
		t.Errorf("expected exactly 1 focused pane, got %d", focusCount)
	}
}

func TestApplyPreset_AllPanesEmpty(t *testing.T) {
	m := newTestLayout(200, 50)
	m.ApplyPreset("2x2")

	for id, p := range m.Panes {
		if p.State != PaneEmpty {
			t.Errorf("pane %s: expected PaneEmpty state, got %v", id, p.State)
		}
	}
}

func TestApplyPreset_SetSizeCalledAfterBuild(t *testing.T) {
	m := NewPaneLayoutModel()
	m.SetSize(200, 50)
	m.ApplyPreset("2v")

	// After SetSize, panes should have non-zero dimensions.
	for id, p := range m.Panes {
		if p.Width == 0 || p.Height == 0 {
			t.Errorf("pane %s: expected non-zero dimensions after ApplyPreset, got %dx%d", id, p.Width, p.Height)
		}
	}
}

func TestIsValidPreset(t *testing.T) {
	valid := []string{"2h", "2v", "3v", "2x2", "main-side"}
	for _, name := range valid {
		if !IsValidPreset(name) {
			t.Errorf("expected %q to be a valid preset", name)
		}
	}
	invalid := []string{"", "quad", "single", "3h", "2x2x2"}
	for _, name := range invalid {
		if IsValidPreset(name) {
			t.Errorf("expected %q to be invalid, but IsValidPreset returned true", name)
		}
	}
}


// ---- ClosePane -------------------------------------------------------------

func TestClosePane_LastPane_ReturnsAllPanesClosedMsg(t *testing.T) {
	m := newTestLayout(200, 50)
	// Single pane after bootstrap.
	if m.PaneCount() != 1 {
		t.Fatalf("expected 1 pane, got %d", m.PaneCount())
	}

	cmd := m.ClosePane()
	if cmd == nil {
		t.Fatal("expected non-nil cmd when closing last pane")
	}

	msg := cmd()
	if _, ok := msg.(AllPanesClosedMsg); !ok {
		t.Errorf("expected AllPanesClosedMsg, got %T", msg)
	}

	if m.PaneCount() != 0 {
		t.Errorf("expected 0 panes after closing last, got %d", m.PaneCount())
	}
	if m.Root != nil {
		t.Error("expected nil Root after closing last pane")
	}
	if m.ActivePaneID != "" {
		t.Errorf("expected empty ActivePaneID after closing last pane, got %q", m.ActivePaneID)
	}
}

func TestClosePane_TwoPanes_CollapsesToSibling(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()

	root := m.Root.(*SplitNode)
	leftPane := root.First.(*Pane)
	rightPane := root.Second.(*Pane)

	// Active pane is the left (original) one.
	m.ActivePaneID = leftPane.ID
	leftPane.Focused = true
	rightPane.Focused = false

	cmd := m.ClosePane()
	if cmd != nil {
		t.Error("expected nil cmd when closing non-last pane")
	}

	if m.PaneCount() != 1 {
		t.Errorf("expected 1 pane after close, got %d", m.PaneCount())
	}

	// Root should now be the right pane (the sibling).
	remaining, ok := m.Root.(*Pane)
	if !ok {
		t.Fatalf("expected root to be a *Pane after collapse, got %T", m.Root)
	}
	if remaining.ID != rightPane.ID {
		t.Errorf("expected remaining pane to be right pane %s, got %s", rightPane.ID, remaining.ID)
	}

	// ActivePaneID should have moved to the remaining pane.
	if m.ActivePaneID != rightPane.ID {
		t.Errorf("expected ActivePaneID=%s after close, got %s", rightPane.ID, m.ActivePaneID)
	}
	if !remaining.Focused {
		t.Error("expected remaining pane to be focused")
	}

	// The closed pane should no longer be in Panes map.
	if _, exists := m.Panes[leftPane.ID]; exists {
		t.Error("expected closed pane to be removed from Panes map")
	}
}

func TestClosePane_ThreePanes_TreeCollapsesCorrectly(t *testing.T) {
	// Build: left | (top / bottom)
	m := newTestLayout(200, 50)
	m.SplitVertical() // left | right
	// Split the right pane horizontally.
	root := m.Root.(*SplitNode)
	rightPane := root.Second.(*Pane)
	m.ActivePaneID = rightPane.ID
	rightPane.Focused = true
	root.First.(*Pane).Focused = false
	m.SplitHorizontal() // right → top/bot

	if m.PaneCount() != 3 {
		t.Fatalf("expected 3 panes, got %d", m.PaneCount())
	}

	// Close the active pane (top of the right split).
	m.ClosePane()

	if m.PaneCount() != 2 {
		t.Errorf("expected 2 panes after close, got %d", m.PaneCount())
	}
}

func TestClosePane_Noop_WhenEmpty(t *testing.T) {
	m := NewPaneLayoutModel()
	cmd := m.ClosePane()
	if cmd != nil {
		t.Error("expected nil cmd on empty layout")
	}
}

func TestClosePane_DimensionsRecalculated(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()

	root := m.Root.(*SplitNode)
	leftPane := root.First.(*Pane)
	rightPane := root.Second.(*Pane)

	m.ActivePaneID = leftPane.ID
	leftPane.Focused = true
	rightPane.Focused = false

	m.ClosePane()

	// The remaining pane should have been resized to the full layout width.
	remaining := m.Root.(*Pane)
	if remaining.Width != 200 {
		t.Errorf("expected remaining pane width=200 after close, got %d", remaining.Width)
	}
	if remaining.Height != 50 {
		t.Errorf("expected remaining pane height=50 after close, got %d", remaining.Height)
	}
}

// ---- ResizeActive ----------------------------------------------------------

func TestResizeActive_IncreasesRatioRight(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()

	root := m.Root.(*SplitNode)
	initial := root.Ratio // 0.5

	// Active pane is the left one (First).
	leftPane := root.First.(*Pane)
	m.ActivePaneID = leftPane.ID

	m.ResizeActive(DirRight, 0.05)

	if root.Ratio != initial+0.05 {
		t.Errorf("expected ratio %.2f, got %.2f", initial+0.05, root.Ratio)
	}
}

func TestResizeActive_DecreasesRatioLeft(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()

	root := m.Root.(*SplitNode)
	initial := root.Ratio // 0.5

	leftPane := root.First.(*Pane)
	m.ActivePaneID = leftPane.ID

	m.ResizeActive(DirLeft, 0.05)

	if root.Ratio != initial-0.05 {
		t.Errorf("expected ratio %.2f, got %.2f", initial-0.05, root.Ratio)
	}
}

func TestResizeActive_ClampsToMin(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()

	root := m.Root.(*SplitNode)
	root.Ratio = 0.1

	leftPane := root.First.(*Pane)
	m.ActivePaneID = leftPane.ID

	// Decreasing below 0.1 should clamp to 0.1.
	m.ResizeActive(DirLeft, 0.05)

	if root.Ratio < 0.1 {
		t.Errorf("expected ratio clamped to 0.1, got %.2f", root.Ratio)
	}
	if root.Ratio != 0.1 {
		t.Errorf("expected ratio == 0.1, got %.2f", root.Ratio)
	}
}

func TestResizeActive_ClampsToMax(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()

	root := m.Root.(*SplitNode)
	root.Ratio = 0.9

	leftPane := root.First.(*Pane)
	m.ActivePaneID = leftPane.ID

	// Increasing above 0.9 should clamp to 0.9.
	m.ResizeActive(DirRight, 0.05)

	if root.Ratio > 0.9 {
		t.Errorf("expected ratio clamped to 0.9, got %.2f", root.Ratio)
	}
	if root.Ratio != 0.9 {
		t.Errorf("expected ratio == 0.9, got %.2f", root.Ratio)
	}
}

func TestResizeActive_NoOp_SinglePane(t *testing.T) {
	m := newTestLayout(200, 50)
	// Single pane — no parent SplitNode exists.
	// Should not panic.
	m.ResizeActive(DirRight, 0.05)
}

func TestResizeActive_NoOp_EmptyLayout(t *testing.T) {
	m := NewPaneLayoutModel()
	// Should not panic.
	m.ResizeActive(DirDown, 0.05)
}

func TestResizeActive_HorizontalSplit_Down(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitHorizontal()

	root := m.Root.(*SplitNode)
	topPane := root.First.(*Pane)
	m.ActivePaneID = topPane.ID
	topPane.Focused = true
	root.Second.(*Pane).Focused = false

	initial := root.Ratio

	m.ResizeActive(DirDown, 0.05)

	if root.Ratio != initial+0.05 {
		t.Errorf("expected ratio %.2f, got %.2f", initial+0.05, root.Ratio)
	}
}

func TestResizeActive_RecalculatesDimensions(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()

	root := m.Root.(*SplitNode)
	leftPane := root.First.(*Pane)
	rightPane := root.Second.(*Pane)
	m.ActivePaneID = leftPane.ID

	widthBefore := leftPane.Width

	m.ResizeActive(DirRight, 0.1)

	if leftPane.Width <= widthBefore {
		t.Errorf("expected left pane to grow after resize-right, before=%d after=%d", widthBefore, leftPane.Width)
	}
	// Total should still equal 199 (200 - 1 border).
	if leftPane.Width+rightPane.Width != 199 {
		t.Errorf("expected widths to sum to 199, got %d+%d", leftPane.Width, rightPane.Width)
	}
}

// ---- removeLeaf ------------------------------------------------------------

func TestRemoveLeaf_DirectChild_First(t *testing.T) {
	paneA := &Pane{ID: "a"}
	paneB := &Pane{ID: "b"}
	root := &SplitNode{Orientation: SplitVertical, First: paneA, Second: paneB}

	result := removeLeaf(root, "a")
	p, ok := result.(*Pane)
	if !ok {
		t.Fatalf("expected *Pane result, got %T", result)
	}
	if p.ID != "b" {
		t.Errorf("expected sibling b, got %s", p.ID)
	}
}

func TestRemoveLeaf_DirectChild_Second(t *testing.T) {
	paneA := &Pane{ID: "a"}
	paneB := &Pane{ID: "b"}
	root := &SplitNode{Orientation: SplitVertical, First: paneA, Second: paneB}

	result := removeLeaf(root, "b")
	p, ok := result.(*Pane)
	if !ok {
		t.Fatalf("expected *Pane result, got %T", result)
	}
	if p.ID != "a" {
		t.Errorf("expected sibling a, got %s", p.ID)
	}
}

func TestRemoveLeaf_NestedTarget(t *testing.T) {
	paneA := &Pane{ID: "a"}
	paneB := &Pane{ID: "b"}
	paneC := &Pane{ID: "c"}
	inner := &SplitNode{Orientation: SplitHorizontal, First: paneA, Second: paneB}
	root := &SplitNode{Orientation: SplitVertical, First: inner, Second: paneC}

	// Remove "a" — inner should collapse to paneB.
	result := removeLeaf(root, "a")
	sn, ok := result.(*SplitNode)
	if !ok {
		t.Fatalf("expected outer SplitNode, got %T", result)
	}
	// First child of outer should now be paneB.
	if p, ok := sn.First.(*Pane); !ok || p.ID != "b" {
		t.Errorf("expected sn.First to be pane-b, got %v", sn.First)
	}
	if p, ok := sn.Second.(*Pane); !ok || p.ID != "c" {
		t.Errorf("expected sn.Second to be pane-c, got %v", sn.Second)
	}
}

func TestRemoveLeaf_NoMatch_ReturnsOriginal(t *testing.T) {
	paneA := &Pane{ID: "a"}
	paneB := &Pane{ID: "b"}
	root := &SplitNode{Orientation: SplitVertical, First: paneA, Second: paneB}

	result := removeLeaf(root, "x")
	sn, ok := result.(*SplitNode)
	if !ok {
		t.Fatalf("expected SplitNode, got %T", result)
	}
	if sn != root {
		t.Error("expected original root when no match found")
	}
}

// ---- ZoomToggle ------------------------------------------------------------

func TestZoomToggle_ZoomsActivePane(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()

	root := m.Root.(*SplitNode)
	leftPane := root.First.(*Pane)
	m.ActivePaneID = leftPane.ID
	leftPane.Focused = true
	root.Second.(*Pane).Focused = false

	m.ZoomToggle()

	if !m.IsZoomed() {
		t.Error("expected zoomed=true after ZoomToggle")
	}
	// Root should now be the active pane.
	if m.Root != leftPane {
		t.Errorf("expected Root to be left pane, got %T", m.Root)
	}
	// Pane should have full layout dimensions.
	if leftPane.Width != 200 || leftPane.Height != 50 {
		t.Errorf("expected full size 200x50, got %dx%d", leftPane.Width, leftPane.Height)
	}
}

func TestZoomToggle_RestoresPreviousTree(t *testing.T) {
	m := newTestLayout(200, 50)
	m.SplitVertical()

	originalRoot := m.Root
	leftPane := m.Root.(*SplitNode).First.(*Pane)
	m.ActivePaneID = leftPane.ID
	leftPane.Focused = true
	m.Root.(*SplitNode).Second.(*Pane).Focused = false

	m.ZoomToggle() // zoom in

	if !m.IsZoomed() {
		t.Fatal("expected zoomed after first toggle")
	}

	m.ZoomToggle() // zoom out

	if m.IsZoomed() {
		t.Error("expected not zoomed after second toggle")
	}
	if m.Root != originalRoot {
		t.Error("expected original root restored after unzoom")
	}
}

func TestZoomToggle_NoOp_WhenNoActivePane(t *testing.T) {
	m := NewPaneLayoutModel()
	// Should not panic.
	m.ZoomToggle()
	if m.IsZoomed() {
		t.Error("expected not zoomed on empty model")
	}
}

// ---- T045: non-SSH protocol handling in panes --------------------------------

func TestUpdate_PanePickerMsg_RDP_TransitionsToGUISession(t *testing.T) {
	m := newTestLayout(80, 24)
	active := m.ActivePane()
	paneID := active.ID

	conn := config.Connection{
		ID:       "rdp-1",
		Name:     "windows-server",
		Host:     "10.0.0.1",
		Port:     3389,
		Protocol: config.ProtoRDP,
	}

	updated, cmd := m.Update(panePickerMsg{PaneID: paneID, Connection: conn})

	pane := updated.Panes[paneID]
	if pane.State != PaneGUISession {
		t.Errorf("expected PaneGUISession for RDP connection, got %v", pane.State)
	}
	if pane.Picker != nil {
		t.Error("expected picker to be cleared after confirm")
	}
	// A GUI launch command must be returned so the launcher IPC fires.
	if cmd == nil {
		t.Error("expected a non-nil command to launch the GUI session")
	}
	// Connection should be stored on the pane for reconnect.
	if pane.Connection == nil {
		t.Error("expected Connection to be stored on pane after RDP selection")
	}
}

func TestUpdate_PanePickerMsg_VNC_TransitionsToGUISession(t *testing.T) {
	m := newTestLayout(80, 24)
	active := m.ActivePane()
	paneID := active.ID

	conn := config.Connection{
		ID:       "vnc-1",
		Name:     "linux-desktop",
		Host:     "10.0.0.2",
		Port:     5900,
		Protocol: config.ProtoVNC,
	}

	updated, cmd := m.Update(panePickerMsg{PaneID: paneID, Connection: conn})

	pane := updated.Panes[paneID]
	if pane.State != PaneGUISession {
		t.Errorf("expected PaneGUISession for VNC connection, got %v", pane.State)
	}
	if cmd == nil {
		t.Error("expected a non-nil launch command for VNC")
	}
}

func TestUpdate_PanePickerMsg_Telnet_TransitionsToDisconnectedWithError(t *testing.T) {
	m := newTestLayout(80, 24)
	active := m.ActivePane()
	paneID := active.ID

	conn := config.Connection{
		ID:       "telnet-1",
		Name:     "old-router",
		Host:     "192.168.1.1",
		Port:     23,
		Protocol: config.ProtoTelnet,
	}

	updated, cmd := m.Update(panePickerMsg{PaneID: paneID, Connection: conn})

	pane := updated.Panes[paneID]
	if pane.State != PaneDisconnected {
		t.Errorf("expected PaneDisconnected for Telnet (unsupported in pane mode), got %v", pane.State)
	}
	if pane.DisconnectErr == nil {
		t.Error("expected a DisconnectErr explaining Telnet is unsupported")
	}
	// No async command needed for Telnet — it's a synchronous rejection.
	if cmd != nil {
		// It's acceptable to have nil or a no-op; the important thing is the state.
		_ = cmd
	}
}

// ---- T046: pane disconnection stores error and enables reconnect ---------------

func TestUpdate_PaneDisconnectedMsg_StoresError(t *testing.T) {
	m := newTestLayout(200, 50)
	active := m.ActivePane()
	expectedErr := errors.New("connection reset by peer")

	updated, _ := m.Update(PaneDisconnectedMsg{
		PaneID:    active.ID,
		SessionID: "sess-1",
		Err:       expectedErr,
	})

	pane := updated.Panes[active.ID]
	if pane.State != PaneDisconnected {
		t.Errorf("expected PaneDisconnected state, got %v", pane.State)
	}
	if pane.DisconnectErr == nil {
		t.Fatal("expected DisconnectErr to be set after disconnect")
	}
	if pane.DisconnectErr.Error() != expectedErr.Error() {
		t.Errorf("expected err %q, got %q", expectedErr.Error(), pane.DisconnectErr.Error())
	}
}

func TestUpdate_PaneDisconnectedMsg_NilError_NoDisconnectErr(t *testing.T) {
	m := newTestLayout(200, 50)
	active := m.ActivePane()

	updated, _ := m.Update(PaneDisconnectedMsg{
		PaneID:    active.ID,
		SessionID: "sess-1",
		Err:       nil,
	})

	pane := updated.Panes[active.ID]
	if pane.State != PaneDisconnected {
		t.Errorf("expected PaneDisconnected state, got %v", pane.State)
	}
	if pane.DisconnectErr != nil {
		t.Errorf("expected nil DisconnectErr for clean disconnect, got %v", pane.DisconnectErr)
	}
}

func TestPanePickerMsg_StoresConnectionForReconnect(t *testing.T) {
	m := newTestLayout(80, 24)
	active := m.ActivePane()
	paneID := active.ID

	conn := config.Connection{
		ID:       "ssh-1",
		Name:     "webserver",
		Host:     "10.0.0.10",
		Port:     22,
		Protocol: config.ProtoSSH,
	}

	updated, _ := m.Update(panePickerMsg{PaneID: paneID, Connection: conn})

	pane := updated.Panes[paneID]
	if pane.Connection == nil {
		t.Fatal("expected Connection to be stored on pane after picker confirm")
	}
	if pane.Connection.ID != conn.ID {
		t.Errorf("expected stored connection ID %q, got %q", conn.ID, pane.Connection.ID)
	}
	if pane.Connection.Host != conn.Host {
		t.Errorf("expected stored connection host %q, got %q", conn.Host, pane.Connection.Host)
	}
}

func TestRenderDisconnected_ShowsErrorAndHints(t *testing.T) {
	pane := &Pane{
		ID:            "p1",
		Width:         80,
		Height:        24,
		State:         PaneDisconnected,
		DisconnectErr: errors.New("connection refused"),
	}

	content := renderDisconnected(pane)
	if !strings.Contains(content, "connection refused") {
		t.Errorf("expected disconnect error in rendered content, got: %q", content)
	}
	if !strings.Contains(content, "reconnect") {
		t.Errorf("expected reconnect hint in rendered content, got: %q", content)
	}
}

func TestRenderDisconnected_NoError_ShowsGenericMessage(t *testing.T) {
	pane := &Pane{
		ID:     "p1",
		Width:  80,
		Height: 24,
		State:  PaneDisconnected,
	}

	content := renderDisconnected(pane)
	if !strings.Contains(content, "Disconnected") {
		t.Errorf("expected 'Disconnected' in rendered content, got: %q", content)
	}
}

func TestPaneContent_GUISession_ShowsPlaceholder(t *testing.T) {
	pane := &Pane{
		ID:     "p1",
		Width:  80,
		Height: 24,
		State:  PaneGUISession,
	}

	content := paneContent(pane)
	if !strings.Contains(content, "GUI window") {
		t.Errorf("expected 'GUI window' text for PaneGUISession, got: %q", content)
	}
}
