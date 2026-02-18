# Contract: PaneLayoutModel

**Module**: `internal/tui/panes.go`
**Role**: Manages pane split tree, focus, broadcast, and rendering.

## Interface

### Bubbletea Model Interface

```go
// Init returns initial command (nil for pane layout)
func (m PaneLayoutModel) Init() tea.Cmd

// Update handles messages routed from App
func (m PaneLayoutModel) Update(msg tea.Msg) (PaneLayoutModel, tea.Cmd)

// View renders the full pane layout as a string
func (m PaneLayoutModel) View() string
```

### Public Methods

```go
// NewPaneLayoutModel creates a new empty layout
func NewPaneLayoutModel() *PaneLayoutModel

// SplitVertical splits the active pane into left/right
func (m *PaneLayoutModel) SplitVertical() tea.Cmd

// SplitHorizontal splits the active pane into top/bottom
func (m *PaneLayoutModel) SplitHorizontal() tea.Cmd

// ClosePane closes the active pane and redistributes space
func (m *PaneLayoutModel) ClosePane() tea.Cmd

// FocusDirection moves focus to the pane in the given direction
func (m *PaneLayoutModel) FocusDirection(dir Direction) // Left, Right, Up, Down

// ToggleBroadcast toggles broadcast mode on/off
func (m *PaneLayoutModel) ToggleBroadcast()

// IsBroadcasting returns whether broadcast mode is active
func (m *PaneLayoutModel) IsBroadcasting() bool

// SetSize updates total available dimensions and recalculates pane sizes
func (m *PaneLayoutModel) SetSize(width, height int)

// ActivePane returns the currently focused pane
func (m *PaneLayoutModel) ActivePane() *Pane

// AllPanes returns all leaf panes
func (m *PaneLayoutModel) AllPanes() []*Pane

// PaneCount returns the number of active panes
func (m *PaneLayoutModel) PaneCount() int

// ResizeActive adjusts the split ratio of the parent of the active pane
func (m *PaneLayoutModel) ResizeActive(direction Direction, amount int)

// EqualizeAll sets all split ratios to 0.5
func (m *PaneLayoutModel) EqualizeAll()

// ZoomToggle maximizes/restores the active pane
func (m *PaneLayoutModel) ZoomToggle()

// ApplyPreset applies a named layout preset (e.g., "2x2", "3-col")
func (m *PaneLayoutModel) ApplyPreset(name string) tea.Cmd

// RouteInput sends input bytes to the active pane (or all panes if broadcasting)
func (m *PaneLayoutModel) RouteInput(data []byte)
```

### Messages

```go
// PaneOutputMsg carries output from a session to its pane
type PaneOutputMsg struct {
    PaneID string
    Data   []byte
}

// PaneDisconnectedMsg signals a session in a pane has ended
type PaneDisconnectedMsg struct {
    PaneID    string
    SessionID string
    Err       error
}

// PaneConnectedMsg signals a session has successfully connected
type PaneConnectedMsg struct {
    PaneID    string
    SessionID string
}
```

## Behavioral Contracts

1. **Split preserves active session**: When splitting, the existing session stays in the original pane. The new pane opens empty (connection picker).
2. **Close redistributes space**: Closing a pane merges its space into its sibling. If the sibling is a split node, the entire subtree expands.
3. **Focus is always on exactly one pane**: `ActivePaneID` is never empty when panes exist.
4. **Broadcast skips empty panes**: `RouteInput()` in broadcast mode only writes to panes with `State == Active`.
5. **Minimum size enforced**: `SplitVertical`/`SplitHorizontal` return an error command if the resulting panes would be smaller than 20x5.
6. **Single pane = no borders**: When only one pane exists, no pane chrome is rendered.
