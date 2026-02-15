# Data Model: Bulk Edit Operations

## Modified Entities

### OpType (undo.go)

Add one new constant:

| Constant | Value | Description |
|----------|-------|-------------|
| UndoOpAdd | 0 | Connection was added (existing) |
| UndoOpDelete | 1 | Connection was deleted (existing) |
| UndoOpEdit | 2 | Connection was edited (existing) |
| UndoOpTagChange | 3 | Tags were changed (existing) |
| **UndoOpBatch** | **4** | **Batch operation containing multiple child operations (NEW)** |

### Operation (undo.go)

Add one new field:

| Field | Type | Description |
|-------|------|-------------|
| Type | OpType | Operation type (existing) |
| ConnID | string | Connection ID (existing, unused for batch) |
| Name | string | Human-readable label (existing, used for batch description) |
| Index | int | Original index for delete reinsertion (existing) |
| Before | config.Connection | Snapshot before operation (existing) |
| After | config.Connection | Snapshot after operation (existing) |
| **Children** | **[]Operation** | **Child operations for batch (NEW, nil for non-batch)** |

### Behavior Changes

**UndoStack.Push**: No change — a batch Operation is pushed like any other. It counts as 1 slot.

**performUndo**: New `UndoOpBatch` case iterates `op.Children` in reverse order, applying each child's undo logic (same switch as existing single-item cases).

**performRedo**: New `UndoOpBatch` case iterates `op.Children` in forward order, applying each child's redo logic.

## Unchanged Entities

- **Connection** (config/connection.go) — no changes
- **Config** (config/config.go) — no changes
- **VisualState** (tui/visual.go) — no changes, just consumed differently
- **BulkOperation** (tui/bulk.go) — no changes, existing functions reused

## State Flow

```
Visual Select → :mv <group> → Check visual active
  → YES: ResolveVisualIDs → snapshot each → ExecuteBulkMove → snapshot after
       → Build Operation{Type: UndoOpBatch, Children: [...]}
       → Push to undoStack → Exit visual mode → Flash "Moved N to <group>"
  → NO:  Existing single-connection logic (unchanged)
```
