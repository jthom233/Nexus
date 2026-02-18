package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/launcher"
	"github.com/dr4zz/nexus/internal/session"
)

// PaneState represents the lifecycle state of a single pane.
type PaneState int

const (
	PaneEmpty        PaneState = iota // no session assigned
	PaneConnecting                    // session connecting
	PaneActive                        // session connected and running
	PaneDisconnected                  // session ended or failed
	PaneGUISession                    // non-SSH session delegated to GUI window (RDP/VNC)
)

// Direction is used for focus navigation and resize operations.
type Direction int

const (
	DirLeft  Direction = iota
	DirRight
	DirUp
	DirDown
)

// SplitOrientation indicates how two child nodes are arranged.
type SplitOrientation int

const (
	SplitVertical   SplitOrientation = iota // children are left / right
	SplitHorizontal                          // children are top / bottom
)

// Pane is a leaf node in the split tree — a single terminal pane.
type Pane struct {
	ID            string
	SessionID     string
	Width         int
	Height        int
	Focused       bool
	State         PaneState
	VTerm         *session.VTermBuffer    // nil when empty
	Session       *session.ManagedSession // nil when empty
	Picker        *PaneConnectionPicker   // non-nil while the connection picker is open
	DisconnectErr error                   // last disconnect reason; set when State == PaneDisconnected
	Connection    *config.Connection      // stored for reconnect when State == PaneDisconnected
}

// SplitNode is an interior node in the binary split tree.
type SplitNode struct {
	Orientation SplitOrientation
	Ratio       float64     // 0.0–1.0; default 0.5 (equal halves)
	First       interface{} // *SplitNode or *Pane (left/top child)
	Second      interface{} // *SplitNode or *Pane (right/bottom child)
}

// PaneLayoutModel is the top-level Bubbletea model that owns the split tree
// and routes messages to individual panes.
type PaneLayoutModel struct {
	Root               interface{} // *SplitNode or *Pane; nil when no panes exist
	ActivePaneID       string
	BroadcastMode      bool
	Panes              map[string]*Pane
	nextPaneID         int
	Width              int
	Height             int
	zoomed             bool
	zoomRestore        interface{} // saved tree root for zoom restore
	LastCreatedPaneID  string      // ID of the most recently created pane (set by newPane)

	// tickActive tracks whether a tickOutputs loop is running.
	tickActive bool
}

// PaneOutputMsg is sent when a pane's background session produces output.
type PaneOutputMsg struct {
	PaneID string
	Data   []byte
}

// PaneDisconnectedMsg is sent when a pane's session ends or errors.
type PaneDisconnectedMsg struct {
	PaneID    string
	SessionID string
	Err       error
}

// PaneConnectedMsg is sent when a pane's session becomes active.
type PaneConnectedMsg struct {
	PaneID    string
	SessionID string
}

// paneSessionStartedMsg is an internal message delivered after a background
// session has been successfully started for an empty pane. It carries the
// live ManagedSession so it can be attached to the pane.
type paneSessionStartedMsg struct {
	PaneID  string
	Session *session.ManagedSession
}

// NewPaneLayoutModel returns an empty PaneLayoutModel ready for use.
func NewPaneLayoutModel() *PaneLayoutModel {
	return &PaneLayoutModel{
		Panes: make(map[string]*Pane),
	}
}

// Init implements tea.Model. Returns nil — no initial commands required.
func (m *PaneLayoutModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *PaneLayoutModel) Update(msg tea.Msg) (PaneLayoutModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return *m, nil

	case PaneOutputMsg:
		pane, ok := m.Panes[msg.PaneID]
		if !ok {
			return *m, nil
		}
		if pane.VTerm != nil {
			pane.VTerm.Write(msg.Data)
		}
		return *m, nil

	case PaneConnectedMsg:
		pane, ok := m.Panes[msg.PaneID]
		if !ok {
			return *m, nil
		}
		pane.State = PaneActive
		// Start the output tick loop if not already running.
		var cmd tea.Cmd
		if !m.tickActive {
			m.tickActive = true
			cmd = m.tickOutputs()
		}
		return *m, cmd

	case PaneDisconnectedMsg:
		pane, ok := m.Panes[msg.PaneID]
		if !ok {
			return *m, nil
		}
		pane.State = PaneDisconnected
		pane.DisconnectErr = msg.Err
		return *m, nil

	case panePickerMsg:
		pane, ok := m.Panes[msg.PaneID]
		if !ok {
			return *m, nil
		}
		// Close the picker and store the connection for potential reconnect.
		pane.Picker = nil
		connCopy := msg.Connection
		pane.Connection = &connCopy
		capturedPaneID := msg.PaneID
		capturedConn := msg.Connection

		switch capturedConn.Protocol {
		case config.ProtoRDP, config.ProtoVNC:
			// Delegate to the GUI process; show a placeholder in the pane region.
			pane.State = PaneGUISession
			l, _ := launcher.ForProtocol(capturedConn.Protocol)
			return *m, l.Launch(capturedConn)

		case config.ProtoTelnet:
			// Telnet uses tea.Exec which requires full TUI control — not compatible
			// with pane mode. Show a clear error instead of panicking.
			pane.State = PaneDisconnected
			pane.DisconnectErr = fmt.Errorf("Telnet is not supported in pane mode")
			return *m, nil

		default: // ProtoSSH and any future terminal protocols
			// Transition the pane to Connecting state and start a background session.
			pane.State = PaneConnecting
			// Allocate the VTerm buffer now so output can be written immediately.
			pane.VTerm = session.NewVTermBuffer(pane.Width, pane.Height)
			cmd := func() tea.Msg {
				sess, err := startBackgroundSession(capturedConn)
				if err != nil {
					return PaneDisconnectedMsg{PaneID: capturedPaneID, Err: err}
				}
				// Attach the session to the pane in a follow-up message.
				return paneSessionStartedMsg{PaneID: capturedPaneID, Session: sess}
			}
			return *m, cmd
		}

	case paneSessionStartedMsg:
		pane, ok := m.Panes[msg.PaneID]
		if !ok {
			return *m, nil
		}
		pane.Session = msg.Session
		pane.SessionID = msg.Session.ID
		// Transition to Active — mirrors the PaneConnectedMsg path.
		pane.State = PaneActive
		var tickCmd tea.Cmd
		if !m.tickActive {
			m.tickActive = true
			tickCmd = m.tickOutputs()
		}
		return *m, tickCmd

	case panePickerCancelMsg:
		pane, ok := m.Panes[msg.PaneID]
		if !ok {
			return *m, nil
		}
		// Deactivate the picker; pane stays in PaneEmpty state.
		if pane.Picker != nil {
			pane.Picker.Active = false
			pane.Picker = nil
		}
		return *m, nil

	case tickOutputsMsg:
		cmds := []tea.Cmd{}
		hasActive := false
		for _, pane := range m.Panes {
			if pane.State != PaneActive || pane.Session == nil {
				continue
			}
			hasActive = true
			// Drain all available output for this pane, concatenating into a
			// single buffer so we emit one PaneOutputMsg per pane per tick.
			var aggregated []byte
		drain:
			for {
				select {
				case data, ok := <-pane.Session.OutputChan():
					if !ok {
						// Channel closed — session ended.
						break drain
					}
					aggregated = append(aggregated, data...)
				default:
					break drain
				}
			}
			if len(aggregated) > 0 {
				capturedID := pane.ID
				capturedData := aggregated
				cmds = append(cmds, func() tea.Msg {
					return PaneOutputMsg{PaneID: capturedID, Data: capturedData}
				})
			}
		}
		if hasActive {
			cmds = append(cmds, m.tickOutputs())
		} else {
			m.tickActive = false
		}
		return *m, tea.Batch(cmds...)
	}

	return *m, nil
}

// tickOutputsMsg is an internal message produced by tickOutputs.
type tickOutputsMsg struct{}

// tickOutputs returns a command that fires after 16 ms and emits tickOutputsMsg.
func (m *PaneLayoutModel) tickOutputs() tea.Cmd {
	return tea.Tick(16*time.Millisecond, func(_ time.Time) tea.Msg {
		return tickOutputsMsg{}
	})
}

// View implements tea.Model.
func (m *PaneLayoutModel) View() string {
	if m.Root == nil {
		return ""
	}
	return m.renderNode(m.Root)
}

// IsZoomed returns true if a pane is currently zoomed to fill the layout.
func (m *PaneLayoutModel) IsZoomed() bool {
	return m.zoomed
}

// renderNode recursively renders the split tree.
func (m *PaneLayoutModel) renderNode(node interface{}) string {
	switch n := node.(type) {
	case *Pane:
		return renderPane(n, n.Focused, m.PaneCount() == 1, m.BroadcastMode, m.zoomed && n.ID == m.ActivePaneID)
	case *SplitNode:
		first := m.renderNode(n.First)
		second := m.renderNode(n.Second)
		if n.Orientation == SplitVertical {
			return lipgloss.JoinHorizontal(lipgloss.Top, first, second)
		}
		return lipgloss.JoinVertical(lipgloss.Left, first, second)
	}
	return ""
}

// SplitVertical splits the currently active pane into left/right halves.
func (m *PaneLayoutModel) SplitVertical() tea.Cmd {
	return m.splitActive(SplitVertical)
}

// SplitHorizontal splits the currently active pane into top/bottom halves.
func (m *PaneLayoutModel) SplitHorizontal() tea.Cmd {
	return m.splitActive(SplitHorizontal)
}

// splitActive is the shared implementation of SplitVertical and SplitHorizontal.
func (m *PaneLayoutModel) splitActive(orientation SplitOrientation) tea.Cmd {
	// Find the active pane. If none exists, bootstrap from scratch.
	if m.ActivePaneID == "" || m.Root == nil {
		return m.bootstrapFirstPane()
	}

	activePane, ok := m.Panes[m.ActivePaneID]
	if !ok {
		return nil
	}

	// Enforce minimum size constraint.
	var newWidth, newHeight int
	if orientation == SplitVertical {
		// Each child gets roughly half the width minus 1 for the border.
		newWidth = (activePane.Width - 1) / 2
		newHeight = activePane.Height
	} else {
		newWidth = activePane.Width
		newHeight = (activePane.Height - 1) / 2
	}

	const minCols = 20
	const minRows = 5
	if newWidth < minCols || newHeight < minRows {
		// Return a no-op command (caller can display the error if desired).
		return func() tea.Msg {
			return fmt.Errorf("pane too small to split (min %dx%d)", minCols, minRows)
		}
	}

	// Create the new empty pane.
	newPane := m.newPane()

	// Replace the active leaf node in the tree with a SplitNode.
	m.Root = replaceLeaf(m.Root, m.ActivePaneID, &SplitNode{
		Orientation: orientation,
		Ratio:       0.5,
		First:       activePane,
		Second:      newPane,
	})

	// Move focus to the new pane so the connection picker is immediately usable.
	activePane.Focused = false
	newPane.Focused = true
	m.ActivePaneID = newPane.ID

	// Recalculate dimensions.
	m.SetSize(m.Width, m.Height)

	return nil
}

// bootstrapFirstPane creates the initial single pane when the layout is empty.
func (m *PaneLayoutModel) bootstrapFirstPane() tea.Cmd {
	p := m.newPane()
	p.Width = m.Width
	p.Height = m.Height
	p.Focused = true
	m.ActivePaneID = p.ID
	m.Root = p
	return nil
}

// newPane allocates a new empty pane with a unique ID, adds it to Panes, and
// returns it. The caller is responsible for inserting it into the tree.
// LastCreatedPaneID is updated to the new pane's ID.
func (m *PaneLayoutModel) newPane() *Pane {
	m.nextPaneID++
	id := fmt.Sprintf("pane-%d", m.nextPaneID)
	p := &Pane{
		ID:    id,
		State: PaneEmpty,
	}
	m.Panes[id] = p
	m.LastCreatedPaneID = id
	if m.ActivePaneID == "" {
		m.ActivePaneID = id
		p.Focused = true
	}
	return p
}

// replaceLeaf walks node, finds the leaf *Pane with matchID, and replaces it
// with replacement. Returns the (possibly new) root node.
func replaceLeaf(node interface{}, matchID string, replacement interface{}) interface{} {
	switch n := node.(type) {
	case *Pane:
		if n.ID == matchID {
			return replacement
		}
		return n
	case *SplitNode:
		n.First = replaceLeaf(n.First, matchID, replacement)
		n.Second = replaceLeaf(n.Second, matchID, replacement)
		return n
	}
	return node
}

// AllPanesClosedMsg is sent by ClosePane when the final pane is removed,
// signalling the app to pop the pane layout view and return to the connection list.
type AllPanesClosedMsg struct{}

// ClosePane closes the currently active pane and collapses the split tree.
// If the closed pane had a session, cleanup channels are closed.
// If it was the last pane, a tea.Cmd emitting AllPanesClosedMsg is returned.
func (m *PaneLayoutModel) ClosePane() tea.Cmd {
	if m.Root == nil || m.ActivePaneID == "" {
		return nil
	}

	activePane, ok := m.Panes[m.ActivePaneID]
	if !ok {
		return nil
	}

	// Clean up the session if one is attached.
	if activePane.Session != nil {
		activePane.Session.Kill()
	}

	// Remove from Panes map.
	delete(m.Panes, m.ActivePaneID)

	// If this was the only pane, reset the model and signal the app.
	if len(m.Panes) == 0 {
		m.Root = nil
		m.ActivePaneID = ""
		m.zoomed = false
		m.zoomRestore = nil
		return func() tea.Msg { return AllPanesClosedMsg{} }
	}

	// If we were zoomed, discard zoom state before restructuring.
	if m.zoomed {
		m.Root = m.zoomRestore
		m.zoomRestore = nil
		m.zoomed = false
	}

	// Remove the active leaf from the tree, promoting the sibling subtree.
	m.Root = removeLeaf(m.Root, m.ActivePaneID)

	// Pick a new active pane: first leaf in the remaining tree.
	var remaining []*Pane
	collectLeaves(m.Root, &remaining)
	if len(remaining) > 0 {
		// Clear old focus flags and set the new one.
		for _, p := range m.Panes {
			p.Focused = false
		}
		remaining[0].Focused = true
		m.ActivePaneID = remaining[0].ID
	}

	// Recalculate dimensions.
	m.SetSize(m.Width, m.Height)
	return nil
}

// removeLeaf removes the leaf *Pane with matchID from the tree and returns the
// new root. When a SplitNode's child is the target leaf, the SplitNode is
// replaced by the sibling subtree (the sibling expands to fill the space).
func removeLeaf(node interface{}, matchID string) interface{} {
	switch n := node.(type) {
	case *Pane:
		// If this leaf is the target, return nil to signal removal.
		if n.ID == matchID {
			return nil
		}
		return n
	case *SplitNode:
		// Check if either child is the target leaf directly.
		if p, ok := n.First.(*Pane); ok && p.ID == matchID {
			return n.Second
		}
		if p, ok := n.Second.(*Pane); ok && p.ID == matchID {
			return n.First
		}
		// Recurse into children.
		newFirst := removeLeaf(n.First, matchID)
		newSecond := removeLeaf(n.Second, matchID)
		// If a child was collapsed away (returned nil), promote the other.
		if newFirst == nil {
			return newSecond
		}
		if newSecond == nil {
			return newFirst
		}
		n.First = newFirst
		n.Second = newSecond
		return n
	}
	return node
}

// FocusDirection moves keyboard focus to the pane in the given direction.
func (m *PaneLayoutModel) FocusDirection(dir Direction) {
	if m.Root == nil {
		return
	}
	activePane, ok := m.Panes[m.ActivePaneID]
	if !ok {
		return
	}

	candidate := findAdjacentPane(m.Root, activePane, dir)
	if candidate == nil || candidate.ID == m.ActivePaneID {
		return
	}

	activePane.Focused = false
	candidate.Focused = true
	m.ActivePaneID = candidate.ID
}

// findAdjacentPane walks the split tree to find the nearest pane in the given
// direction relative to current. Returns nil if no such pane exists.
//
// Strategy: derive absolute positions from the split tree dimensions, then
// pick the best candidate by proximity in the requested direction.
func findAdjacentPane(root interface{}, current *Pane, dir Direction) *Pane {
	// Collect all leaf panes.
	var leaves []*Pane
	collectLeaves(root, &leaves)

	if len(leaves) <= 1 {
		return nil
	}

	// Derive absolute positions by traversing the tree.
	positions := collectPositions(root, 0, 0)

	// Find current pane's position.
	var curPos panePosition
	for _, p := range positions {
		if p.pane.ID == current.ID {
			curPos = p
			break
		}
	}

	curCenterX := curPos.x + current.Width/2
	curCenterY := curPos.y + current.Height/2

	var best *Pane
	bestDist := -1

	for _, p := range positions {
		if p.pane.ID == current.ID {
			continue
		}
		cx := p.x + p.pane.Width/2
		cy := p.y + p.pane.Height/2

		// Check that the candidate is in the requested direction.
		var inDir bool
		switch dir {
		case DirLeft:
			inDir = cx < curCenterX && p.x+p.pane.Width <= curPos.x+1
		case DirRight:
			inDir = cx > curCenterX && p.x >= curPos.x+current.Width-1
		case DirUp:
			inDir = cy < curCenterY && p.y+p.pane.Height <= curPos.y+1
		case DirDown:
			inDir = cy > curCenterY && p.y >= curPos.y+current.Height-1
		}
		if !inDir {
			continue
		}

		// Manhattan distance from the edge we are moving toward.
		var dist int
		switch dir {
		case DirLeft:
			dist = curPos.x - (p.x + p.pane.Width)
		case DirRight:
			dist = p.x - (curPos.x + current.Width)
		case DirUp:
			dist = curPos.y - (p.y + p.pane.Height)
		case DirDown:
			dist = p.y - (curPos.y + current.Height)
		}
		if dist < 0 {
			dist = -dist
		}

		if bestDist < 0 || dist < bestDist {
			bestDist = dist
			best = p.pane
		}
	}

	return best
}

type panePosition struct {
	pane *Pane
	x, y int
}

// collectPositions recursively traverses the tree and records absolute
// top-left (x, y) pixel positions for each leaf pane.
func collectPositions(node interface{}, x, y int) []panePosition {
	switch n := node.(type) {
	case *Pane:
		return []panePosition{{pane: n, x: x, y: y}}
	case *SplitNode:
		var results []panePosition
		if n.Orientation == SplitVertical {
			// First child is left, second is right.
			// We need the widths to compute the split. Get them from the panes.
			firstWidth := leafTotalWidth(n.First)
			results = append(results, collectPositions(n.First, x, y)...)
			results = append(results, collectPositions(n.Second, x+firstWidth+1, y)...)
		} else {
			// First child is top, second is bottom.
			firstHeight := leafTotalHeight(n.First)
			results = append(results, collectPositions(n.First, x, y)...)
			results = append(results, collectPositions(n.Second, x, y+firstHeight+1)...)
		}
		return results
	}
	return nil
}

// leafTotalWidth returns the sum of widths of all leaves under node.
// For a single leaf, this is just its Width.
// For a SplitVertical node, it's First width + 1 (border) + Second width.
func leafTotalWidth(node interface{}) int {
	switch n := node.(type) {
	case *Pane:
		return n.Width
	case *SplitNode:
		if n.Orientation == SplitVertical {
			return leafTotalWidth(n.First) + 1 + leafTotalWidth(n.Second)
		}
		// Horizontal split — both children have the same width; just pick First.
		return leafTotalWidth(n.First)
	}
	return 0
}

// leafTotalHeight returns the total height occupied by node.
func leafTotalHeight(node interface{}) int {
	switch n := node.(type) {
	case *Pane:
		return n.Height
	case *SplitNode:
		if n.Orientation == SplitHorizontal {
			return leafTotalHeight(n.First) + 1 + leafTotalHeight(n.Second)
		}
		// Vertical split — both children have the same height; just pick First.
		return leafTotalHeight(n.First)
	}
	return 0
}

// collectLeaves appends all leaf *Pane nodes under root to out.
func collectLeaves(node interface{}, out *[]*Pane) {
	switch n := node.(type) {
	case *Pane:
		*out = append(*out, n)
	case *SplitNode:
		collectLeaves(n.First, out)
		collectLeaves(n.Second, out)
	}
}

// ToggleBroadcast toggles broadcast mode — when on, input is sent to all panes.
// It returns a tea.Cmd that delivers a flash message indicating the new state.
func (m *PaneLayoutModel) ToggleBroadcast() tea.Cmd {
	m.BroadcastMode = !m.BroadcastMode
	var msg string
	if m.BroadcastMode {
		count := 0
		for _, p := range m.Panes {
			if p.State == PaneActive {
				count++
			}
		}
		msg = fmt.Sprintf("Broadcasting to %d sessions", count)
	} else {
		msg = "Broadcast off"
	}
	return func() tea.Msg {
		return broadcastFlashMsg{text: msg}
	}
}

// broadcastFlashMsg carries a flash notification from a broadcast toggle.
type broadcastFlashMsg struct {
	text string
}

// IsBroadcasting returns true when broadcast mode is active.
func (m *PaneLayoutModel) IsBroadcasting() bool {
	return m.BroadcastMode
}

// SetSize updates the total available width and height for the layout and
// propagates dimensions down the split tree.
func (m *PaneLayoutModel) SetSize(width, height int) {
	m.Width = width
	m.Height = height
	if m.Root != nil {
		applySize(m.Root, width, height)
	}
}

// applySize recursively sets Width/Height on leaf Pane nodes and notifies
// sessions of the new dimensions.
func applySize(node interface{}, width, height int) {
	switch n := node.(type) {
	case *Pane:
		n.Width = width
		n.Height = height
		if n.Session != nil {
			n.Session.SetPaneSize(width, height)
		}
		if n.VTerm != nil {
			n.VTerm.Resize(width, height)
		}
	case *SplitNode:
		if n.Orientation == SplitVertical {
			// Split width: left gets floor(ratio * (width-1)), right gets the rest.
			leftWidth := int(n.Ratio * float64(width-1))
			rightWidth := width - 1 - leftWidth
			if leftWidth < 1 {
				leftWidth = 1
			}
			if rightWidth < 1 {
				rightWidth = 1
			}
			applySize(n.First, leftWidth, height)
			applySize(n.Second, rightWidth, height)
		} else {
			// Split height: top gets floor(ratio * (height-1)), bottom gets the rest.
			topHeight := int(n.Ratio * float64(height-1))
			botHeight := height - 1 - topHeight
			if topHeight < 1 {
				topHeight = 1
			}
			if botHeight < 1 {
				botHeight = 1
			}
			applySize(n.First, width, topHeight)
			applySize(n.Second, width, botHeight)
		}
	}
}

// ActivePane returns the currently focused pane, or nil if none.
func (m *PaneLayoutModel) ActivePane() *Pane {
	return m.Panes[m.ActivePaneID]
}

// AllPanes returns all panes in the layout in an unspecified order.
func (m *PaneLayoutModel) AllPanes() []*Pane {
	out := make([]*Pane, 0, len(m.Panes))
	for _, p := range m.Panes {
		out = append(out, p)
	}
	return out
}

// PaneCount returns the number of panes currently in the layout.
func (m *PaneLayoutModel) PaneCount() int {
	return len(m.Panes)
}

// ResizeActive adjusts the split ratio of the active pane's parent SplitNode by
// amount (0.0–1.0 per step, e.g. 0.05). The direction determines whether the
// ratio increases (DirRight/DirDown) or decreases (DirLeft/DirUp). The ratio
// is clamped to [0.1, 0.9] and dimensions are recalculated via SetSize.
func (m *PaneLayoutModel) ResizeActive(direction Direction, amount float64) {
	if m.Root == nil || m.ActivePaneID == "" {
		return
	}

	// Find the SplitNode that is the direct parent of the active pane.
	parent := findParentSplit(m.Root, m.ActivePaneID)
	if parent == nil {
		return
	}

	// Determine sign: growing toward DirRight/DirDown increases the ratio of
	// the First child; DirLeft/DirUp decreases it (shrinks First, grows Second).
	switch direction {
	case DirRight, DirDown:
		parent.Ratio += amount
	case DirLeft, DirUp:
		parent.Ratio -= amount
	}

	// Clamp to [0.1, 0.9].
	if parent.Ratio < 0.1 {
		parent.Ratio = 0.1
	}
	if parent.Ratio > 0.9 {
		parent.Ratio = 0.9
	}

	m.SetSize(m.Width, m.Height)
}

// findParentSplit returns the SplitNode whose First or Second child is the leaf
// pane with the given ID, or nil if no such parent exists (e.g. root is a Pane).
func findParentSplit(node interface{}, paneID string) *SplitNode {
	sn, ok := node.(*SplitNode)
	if !ok {
		return nil
	}
	// Check if either child is the target leaf.
	if p, ok := sn.First.(*Pane); ok && p.ID == paneID {
		return sn
	}
	if p, ok := sn.Second.(*Pane); ok && p.ID == paneID {
		return sn
	}
	// Recurse.
	if found := findParentSplit(sn.First, paneID); found != nil {
		return found
	}
	return findParentSplit(sn.Second, paneID)
}

// EqualizeAll resets all split ratios to 0.5 (equal halves).
func (m *PaneLayoutModel) EqualizeAll() {
	equalizeRatios(m.Root)
	m.SetSize(m.Width, m.Height)
}

func equalizeRatios(node interface{}) {
	if n, ok := node.(*SplitNode); ok {
		n.Ratio = 0.5
		equalizeRatios(n.First)
		equalizeRatios(n.Second)
	}
}

// ZoomToggle zooms the active pane to fill the full layout area, or restores
// the previous split tree if already zoomed.
func (m *PaneLayoutModel) ZoomToggle() {
	if m.zoomed {
		m.Root = m.zoomRestore
		m.zoomRestore = nil
		m.zoomed = false
		m.SetSize(m.Width, m.Height)
		return
	}
	activePane := m.ActivePane()
	if activePane == nil {
		return
	}
	m.zoomRestore = m.Root
	m.Root = activePane
	m.zoomed = true
	m.SetSize(m.Width, m.Height)
}

// validPresets lists all supported preset names.
var validPresets = []string{"2h", "2v", "3v", "2x2", "main-side"}

// IsValidPreset reports whether name is a supported layout preset name.
func IsValidPreset(name string) bool {
	for _, p := range validPresets {
		if p == name {
			return true
		}
	}
	return false
}

// ApplyPreset arranges panes according to a named layout preset.
//
// Supported presets:
//   - "2h"        — 2 panes stacked horizontally (top / bottom)
//   - "2v"        — 2 panes side-by-side (left / right)
//   - "3v"        — 3 equal columns
//   - "2x2"       — 4-pane grid (2 columns, each column has 2 rows)
//   - "main-side" — large left pane (70%) + small right pane (30%)
//
// Any existing tree is discarded before building the new layout.
// All new panes start in PaneEmpty state so the connection picker
// auto-shows for each of them. Focus is placed on the first pane.
func (m *PaneLayoutModel) ApplyPreset(name string) tea.Cmd {
	// Validate before modifying any state so an unknown name is a no-op.
	if !IsValidPreset(name) {
		return nil
	}

	// Discard existing tree and pane map.
	m.Root = nil
	m.Panes = make(map[string]*Pane)
	m.ActivePaneID = ""
	m.zoomed = false
	m.zoomRestore = nil

	switch name {
	case "2h":
		// Two panes stacked horizontally (top / bottom).
		top := m.newPane()
		bot := m.newPane()
		m.Root = &SplitNode{
			Orientation: SplitHorizontal,
			Ratio:       0.5,
			First:       top,
			Second:      bot,
		}

	case "2v":
		// Two panes side-by-side (left / right).
		left := m.newPane()
		right := m.newPane()
		m.Root = &SplitNode{
			Orientation: SplitVertical,
			Ratio:       0.5,
			First:       left,
			Second:      right,
		}

	case "3v":
		// Three equal columns: col1 | (col2 | col3).
		// Outer ratio 1/3 gives col1 one-third; inner ratio 0.5 splits the
		// remaining two-thirds equally between col2 and col3.
		col1 := m.newPane()
		col2 := m.newPane()
		col3 := m.newPane()
		inner := &SplitNode{
			Orientation: SplitVertical,
			Ratio:       0.5,
			First:       col2,
			Second:      col3,
		}
		m.Root = &SplitNode{
			Orientation: SplitVertical,
			Ratio:       1.0 / 3.0,
			First:       col1,
			Second:      inner,
		}

	case "2x2":
		// 4-pane grid: 2 columns, each column has 2 rows.
		topLeft := m.newPane()
		botLeft := m.newPane()
		topRight := m.newPane()
		botRight := m.newPane()
		leftCol := &SplitNode{
			Orientation: SplitHorizontal,
			Ratio:       0.5,
			First:       topLeft,
			Second:      botLeft,
		}
		rightCol := &SplitNode{
			Orientation: SplitHorizontal,
			Ratio:       0.5,
			First:       topRight,
			Second:      botRight,
		}
		m.Root = &SplitNode{
			Orientation: SplitVertical,
			Ratio:       0.5,
			First:       leftCol,
			Second:      rightCol,
		}

	case "main-side":
		// Large left pane (70%) + small right pane (30%).
		main := m.newPane()
		side := m.newPane()
		m.Root = &SplitNode{
			Orientation: SplitVertical,
			Ratio:       0.70,
			First:       main,
			Second:      side,
		}

	default:
		// Unknown preset — no-op; callers should validate via IsValidPreset.
		return nil
	}

	m.setFocusFirst()
	m.SetSize(m.Width, m.Height)
	return nil
}

// setFocusFirst sets focus on the first (top-left) leaf pane in the tree.
func (m *PaneLayoutModel) setFocusFirst() {
	first := firstLeaf(m.Root)
	if first == nil {
		return
	}
	for _, p := range m.Panes {
		p.Focused = false
	}
	first.Focused = true
	m.ActivePaneID = first.ID
}

// firstLeaf returns the leftmost/topmost leaf *Pane in the subtree.
func firstLeaf(node interface{}) *Pane {
	switch n := node.(type) {
	case *Pane:
		return n
	case *SplitNode:
		return firstLeaf(n.First)
	}
	return nil
}

// OpenPickerForPane attaches a connection picker to the given pane, replacing
// any existing picker. If paneID is empty or not found, it is a no-op.
func (m *PaneLayoutModel) OpenPickerForPane(paneID string, conns []config.Connection) {
	pane, ok := m.Panes[paneID]
	if !ok {
		return
	}
	pane.Picker = NewPaneConnectionPicker(paneID, conns, pane.Width, pane.Height)
}

// RouteInput sends input data to the active pane (or all panes in broadcast mode).
func (m *PaneLayoutModel) RouteInput(data []byte) {
	if m.BroadcastMode {
		for _, pane := range m.Panes {
			if pane.State != PaneActive || pane.Session == nil {
				continue
			}
			pane.Session.WriteInput(data)
		}
		return
	}

	activePane := m.ActivePane()
	if activePane == nil || activePane.State != PaneActive || activePane.Session == nil {
		return
	}

	activePane.Session.WriteInput(data)
}
