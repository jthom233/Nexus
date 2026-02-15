# Quickstart: Bulk Edit Operations

## Overview

This feature adds batch undo support and wires up bulk group moves. After implementation, selecting multiple connections and running `:mv <group>` moves them all, and pressing `u` once undoes the entire operation.

## Implementation Order

### Phase 1: Batch Undo Infrastructure (undo.go)
1. Add `UndoOpBatch` constant to `OpType`
2. Add `Children []Operation` field to `Operation` struct
3. Add `PushBatch` helper method to `UndoStack`
4. Update `performUndo` with `UndoOpBatch` case
5. Update `performRedo` with `UndoOpBatch` case

### Phase 2: Bulk Move (app.go)
1. Add visual mode check at top of `:mv`/`:move` handler
2. Extract IDs via `ResolveVisualIDs`
3. Snapshot before, call `ExecuteBulkMove`, snapshot after
4. Build batch Operation with children, push to undo stack
5. Exit visual mode, refresh list, flash message

### Phase 3: Retrofit Existing Bulk Operations (app.go)
1. Update `tagAddVisual` to collect operations into batch
2. Update `tagRemoveVisual` to collect operations into batch
3. Update `delete-visual` handler to collect operations into batch

## Key Files

| File | Changes |
|------|---------|
| `internal/tui/undo.go` | Add UndoOpBatch, Children field, PushBatch method |
| `internal/tui/undo_test.go` | Tests for batch undo/redo |
| `internal/tui/app.go` | Wire bulk move, retrofit tag/delete handlers |

## Testing

```bash
go build ./...        # Must compile cleanly
go test ./internal/tui/...  # All tests pass
go vet ./...          # No new warnings
```
