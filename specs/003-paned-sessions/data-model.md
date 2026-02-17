# Data Model: Paned Sessions with Broadcast

**Feature**: 003-paned-sessions
**Date**: 2026-02-16

## Entities

### Pane

A rectangular terminal region displaying a single connection session.

| Field | Type | Description |
|-------|------|-------------|
| ID | string | Unique pane identifier (e.g., "p1", "p2") |
| SessionID | string | Reference to ManagedSession.ID (e.g., "s1"), empty if no connection |
| Width | int | Current width in columns |
| Height | int | Current height in rows |
| Focused | bool | Whether this pane has input focus |
| State | PaneState | Current state (Empty, Connecting, Active, Disconnected) |

**State transitions**:
```
Empty → Connecting → Active → Disconnected
  ↑                    │          │
  └────────────────────┘──────────┘  (close pane or reconnect)
```

- **Empty**: Pane created, showing connection list picker. No session attached.
- **Connecting**: Connection initiated, waiting for SSH handshake.
- **Active**: Session running, output rendering in pane.
- **Disconnected**: Session lost or ended. Shows status message. User can reconnect or close.

### SplitNode (PaneLayout tree)

Internal node in the binary split tree.

| Field | Type | Description |
|-------|------|-------------|
| Orientation | SplitOrientation | Horizontal or Vertical |
| Ratio | float64 | Split position (0.0-1.0), default 0.5 |
| Left/Top | *SplitNode or *Pane | First child |
| Right/Bottom | *SplitNode or *Pane | Second child |

**Invariants**:
- Every internal node has exactly two children
- Every leaf node is a Pane
- Ratio is clamped to [0.1, 0.9] to enforce minimum pane size
- Total pane count = number of leaf nodes

### PaneLayoutModel

Top-level model managing the split tree and broadcast state.

| Field | Type | Description |
|-------|------|-------------|
| Root | *SplitNode | Root of the split tree (nil = no panes) |
| ActivePaneID | string | ID of the currently focused pane |
| BroadcastMode | bool | Whether broadcast is active |
| Panes | map[string]*Pane | Flat lookup of all panes by ID |
| NextPaneID | int | Auto-incrementing pane ID counter |
| Width | int | Total available width |
| Height | int | Total available height |

### VTermBuffer

Virtual terminal buffer interpreting ANSI sequences for a single session.

| Field | Type | Description |
|-------|------|-------------|
| Cells | [][]Cell | 2D grid of cells [row][col] |
| CursorRow | int | Current cursor row |
| CursorCol | int | Current cursor column |
| Width | int | Buffer width in columns |
| Height | int | Buffer height in rows |
| ScrollRegionTop | int | Top of scroll region |
| ScrollRegionBottom | int | Bottom of scroll region |
| DefaultStyle | CellStyle | Default foreground/background |

### Cell

Single character cell in the VTerm grid.

| Field | Type | Description |
|-------|------|-------------|
| Char | rune | Character at this position |
| Style | CellStyle | Foreground, background, bold, underline, etc. |

### CellStyle

| Field | Type | Description |
|-------|------|-------------|
| FG | lipgloss.Color | Foreground color |
| BG | lipgloss.Color | Background color |
| Bold | bool | Bold attribute |
| Underline | bool | Underline attribute |
| Reverse | bool | Reverse video attribute |

## Relationships

```
PaneLayoutModel
  └── Root: SplitNode (tree)
        ├── Left: SplitNode or Pane
        └── Right: SplitNode or Pane

Pane ──references──→ ManagedSession (via SessionID)
Pane ──owns──→ VTermBuffer (one per pane with active session)

ManagedSession (existing, modified)
  ├── InputCh: chan []byte   (NEW: replaces direct stdin read)
  ├── OutputCh: chan []byte  (NEW: replaces direct stdout write)
  └── existing fields preserved
```

## Modified Existing Entities

### ManagedSession (internal/session/managed.go)

**New fields**:

| Field | Type | Description |
|-------|------|-------------|
| InputCh | chan []byte | Channel for receiving keyboard input (replaces stdin reader goroutine) |
| OutputCh | chan []byte | Channel for sending session output (replaces stdout writer) |
| BackgroundMode | bool | If true, runs without terminal attachment |
| PaneWidth | int | Current pane width (for SSH WindowChange) |
| PaneHeight | int | Current pane height (for SSH WindowChange) |

**New methods**:
- `StartBackground() error` — Connect and start I/O goroutines without entering attachLoop
- `SetPaneSize(width, height int)` — Update pane dimensions and send SSH WindowChange
- `WriteInput(data []byte)` — Send input to session (via InputCh or sshStdin)

**Preserved behavior**:
- `Run()` still works for single-session `tea.Exec()` mode (backward compat)
- Background goroutines (`readOutput`, `readStderr`, `waitDone`) unchanged
- Replay buffer and ring buffer unchanged
- Detach/reattach lifecycle unchanged for non-pane sessions
