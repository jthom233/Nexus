# Quickstart: Paned Sessions Development

**Feature**: 003-paned-sessions
**Branch**: `003-paned-sessions`

## Prerequisites

- Go 1.25+
- Working Nexus build: `go build ./cmd/nexus/`
- SSH server accessible for testing (localhost or remote)

## Build & Run

```bash
# Build TUI
go build ./cmd/nexus/

# Run TUI
./nexus

# Run tests
go test ./...

# Run specific package tests
go test ./internal/session/
go test ./internal/tui/
```

## Key Files

| File | Purpose |
|------|---------|
| `internal/tui/panes.go` | NEW — Pane layout model, split tree, rendering |
| `internal/tui/pane_render.go` | NEW — Per-pane VTerm output rendering |
| `internal/session/vterm.go` | NEW — Virtual terminal ANSI interpreter |
| `internal/session/managed.go` | MODIFIED — Channel-based I/O for background mode |
| `internal/tui/app.go` | MODIFIED — viewPaneLayout, routing, leader actions |
| `internal/tui/leader.go` | MODIFIED — Space w group |

## Development Order

1. **VTermBuffer** (`internal/session/vterm.go`) — Can be developed and tested in isolation. Write unit tests with raw ANSI sequences and verify cell grid output.

2. **ManagedSession extensions** (`internal/session/managed.go`) — Add `StartBackground()`, `WriteInput()`, `OutputChan()`, `SetPaneSize()`. Test with a real SSH connection.

3. **PaneLayoutModel** (`internal/tui/panes.go`) — Split tree, focus management, resize. Can be unit tested with mock panes (no real sessions needed).

4. **Pane rendering** (`internal/tui/pane_render.go`) — Wire VTermBuffer output into lipgloss layout. Integration test with real sessions.

5. **App integration** (`internal/tui/app.go`, `leader.go`) — Add view, leader keys, message routing. Full integration test.

6. **Broadcast mode** — Add to PaneLayoutModel. Test with multiple sessions.

7. **Preset layouts** — Sugar on top of split tree. Add after core works.

## Testing Strategy

```bash
# Unit tests for VTerm
go test ./internal/session/ -run TestVTerm

# Unit tests for pane layout
go test ./internal/tui/ -run TestPaneLayout

# Integration test (requires SSH)
go test ./internal/tui/ -run TestPaneIntegration -tags integration
```

## Key Design Decisions

- **Space w prefix** for all pane commands (see research.md R5)
- **Binary split tree** for layout (see research.md R4)
- **Channel-based I/O** replaces direct stdin/stdout (see research.md R1)
- **VTerm buffer** interprets ANSI for pane rendering (see research.md R2)
- **tea.Exec preserved** for single-session non-pane connections (see research.md R6)
- **Broadcast is all-or-nothing** per spec clarification
