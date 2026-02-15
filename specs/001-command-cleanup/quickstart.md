# Quickstart: TUI Command & Keybinding Cleanup

## Overview

This feature cleans up the Nexus TUI by removing dead code (marks, tree, stub commands) and adding group management. After implementation, every registered command works and every command is reachable from the leader menu.

## Implementation Order

### Phase 1: Removals (safe, no new behavior)
1. Delete `marks.go` and `tree.go`
2. Remove all references in `app.go` (marks fields, tree fields, handlers)
3. Remove stub commands from `command.go` registry
4. Remove `OpChange` from `motion.go`
5. Verify: `go build ./...` succeeds

### Phase 2: Config Layer (foundation for commands)
1. Add `AddGroup`, `DeleteGroup`, `FindGroup`, `HasConnectionsInGroup`, `MoveConnection` to config.go
2. Verify: `go build ./...` succeeds

### Phase 3: Command Handlers (wire up new commands)
1. Add `:mkdir` handler in app.go command dispatch
2. Add `:rmdir` handler in app.go command dispatch
3. Add `:mv`/`:move` handler in app.go command dispatch (merge into single handler)
4. Wire `:mv` to undo system via existing `UndoOpEdit`
5. Verify: `go build ./...` succeeds

### Phase 4: Leader Menu (final polish)
1. Restructure `leaderGroups()` in leader.go — remove stubs, add missing items
2. Update `executeLeaderAction()` in app.go — wire new actions to command handlers
3. Fix sessions kill action to work without sessions view
4. Verify: `go build ./...` succeeds, no "not yet implemented" messages

## Key Files

| File | Action | Lines affected |
|------|--------|---------------|
| `internal/tui/marks.go` | DELETE | ~115 lines removed |
| `internal/tui/tree.go` | DELETE | ~440 lines removed |
| `internal/tui/app.go` | MODIFY | ~80 lines removed, ~60 lines added |
| `internal/tui/command.go` | MODIFY | ~10 lines removed (registry entries) |
| `internal/tui/leader.go` | MODIFY | ~30 lines removed, ~40 lines added |
| `internal/tui/motion.go` | MODIFY | ~10 lines removed |
| `internal/config/config.go` | MODIFY | ~50 lines added |

## Verification

After each phase: `go build ./...` and `go vet ./...` must pass with no new warnings.
Final check: grep for "not yet implemented" — should return zero results in TUI code.
