# Research: Bulk Edit Operations

## Decision 1: Batch Undo Storage Model

**Decision**: Add `UndoOpBatch` constant to `OpType` and a `Children []Operation` field to the `Operation` struct. When `Type == UndoOpBatch`, the `Children` slice contains the individual operations. The existing `ConnID`, `Before`, `After` fields are unused for batch entries.

**Rationale**: Minimal change — adds one constant and one field. The `performUndo`/`performRedo` functions gain a single new case that iterates children. Existing single-item operations are completely unaffected (Children is nil). A batch counts as one slot in the undo stack (max 50).

**Alternatives considered**:
- Separate `BatchOperation` struct with its own stack — rejected: doubles the API surface, complicates redo
- Marker operations (begin/end batch) — rejected: fragile if stack is trimmed mid-batch
- Wrapper type with interface — rejected: over-engineered for this use case

## Decision 2: Bulk Move Activation in Visual Mode

**Decision**: Check `a.visualState.Active()` at the top of the `:mv`/`:move` case in `handleCommand()`. If active, extract IDs via `ResolveVisualIDs()`, call `ExecuteBulkMove()` from bulk.go, build batch undo, exit visual mode. If not active, fall through to existing single-connection logic.

**Rationale**: Follows the exact pattern used by `:tag add/remove` which already checks visual mode. The `ExecuteBulkMove` function in bulk.go handles skipping connections already in the target group and returns the count.

**Alternatives considered**:
- Separate `:bmv` command for bulk move — rejected: inconsistent with tag behavior (`:tag` handles both modes)
- Leader-only bulk move — rejected: command bar should also work

## Decision 3: Snapshot Strategy for Batch Undo

**Decision**: Before executing the bulk operation, snapshot each affected connection. After the operation, snapshot again. Store both in child `Operation` entries within the batch. Undo iterates children in reverse, restoring each `Before` snapshot. Redo iterates forward, restoring each `After` snapshot.

**Rationale**: Per-connection snapshots correctly handle mixed source groups (different connections coming from different original groups). The storage cost is proportional to the number of modified connections, which is bounded by the selection size.

**Alternatives considered**:
- Full config snapshot (before/after) — rejected: wasteful for large configs with many connections
- Delta-only storage (just the changed field) — rejected: fragile, doesn't generalize to tag changes

## Decision 4: Batch Undo Flash Message

**Decision**: For batch undo, show "Undo: reverted N connections" (or "Redo: re-applied N connections"). The `Name` field of the batch Operation stores a descriptive label like "bulk move to Production" for contextual messages.

**Rationale**: Individual connection names are too long to display for bulk operations. A count + operation description is sufficient.

## Decision 5: Visual Mode Exit After Bulk Operation

**Decision**: All bulk operations (move, tag, delete) exit visual mode after completion. This is consistent — the selection has been acted upon and is no longer meaningful.

**Rationale**: Matches vim behavior where operators clear visual selection. The tag handlers already do this implicitly (they don't exit, but the user expects it). We'll add explicit `a.visualState.Exit()` calls.
