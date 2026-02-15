# Tasks: Bulk Edit Operations

**Input**: Design documents from `/specs/002-bulk-edit/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/

**Tests**: Not requested — no test tasks generated.

**Organization**: Tasks grouped by user story for independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: User Story 2 - Batch Undo Infrastructure (Priority: P2, implemented first as foundation)

**Goal**: The undo system supports batch operations — a single undo/redo reverses an entire bulk action.

**Independent Test**: Push a batch of 3 operations, call Undo() once, verify all 3 are reversed. Call Redo() once, verify all 3 are re-applied.

**Note**: US2 is implemented first because US1 (bulk move) and US3 (retrofit) both depend on batch undo existing.

### Undo System Changes

- [ ] T001 [US2] Add `UndoOpBatch` constant to `OpType` enum in `internal/tui/undo.go` (after `UndoOpTagChange`, value 4)
- [ ] T002 [US2] Add `Children []Operation` field to `Operation` struct in `internal/tui/undo.go` (after the `After` field)
- [ ] T003 [US2] Add `PushBatch(name string, children []Operation)` method to `UndoStack` in `internal/tui/undo.go` — creates `Operation{Type: UndoOpBatch, Name: name, Children: children}` and calls `Push()`

### Undo/Redo Handler Changes

- [ ] T004 [US2] Add `UndoOpBatch` case to `performUndo()` in `internal/tui/app.go` — iterate `op.Children` in reverse order, apply each child's undo logic (reuse existing switch cases for UndoOpAdd/Delete/Edit/TagChange), flash "Undo: <name> (N items)"
- [ ] T005 [US2] Add `UndoOpBatch` case to `performRedo()` in `internal/tui/app.go` — iterate `op.Children` in forward order, apply each child's redo logic (reuse existing switch cases), flash "Redo: <name> (N items)"

### Verification

- [ ] T006 [US2] Verify `go build ./...` and `go test ./internal/tui/...` pass after batch undo infrastructure changes

**Checkpoint**: Batch undo infrastructure is ready. Existing single-item undo/redo unchanged.

---

## Phase 2: User Story 1 - Bulk Group Assignment (Priority: P1)

**Goal**: Users can select multiple connections and run `:mv <group>` to move them all at once, with a single batch undo entry.

**Independent Test**: Select 5 connections with V, run `:mv Production`, verify all 5 have group "Production". Press `u` once, verify all 5 revert.

### Implementation

- [ ] T007 [US1] Add visual mode check at the top of the `case "mv", "move":` handler in `internal/tui/app.go` — if `a.visualState.Active()`, branch to new bulk move logic instead of existing single-connection logic
- [ ] T008 [US1] Implement bulk move logic in the visual mode branch of `:mv`/`:move` handler in `internal/tui/app.go`: extract IDs via `ResolveVisualIDs(a.visualState, a.list.table.rows)`, validate non-empty selection, treat entire `strings.TrimSpace(msg.Args)` as target group name (no connection name parsing in visual mode)
- [ ] T009 [US1] Add snapshot and batch undo logic to bulk move in `internal/tui/app.go`: before calling `ExecuteBulkMove`, snapshot each connection (`before := *conn`), after the move snapshot again (`after := *conn`), build `[]Operation` children with `Type: UndoOpEdit`, call `a.undoStack.PushBatch("bulk move to "+targetGroup, children)`
- [ ] T010 [US1] Add auto-create group logic for bulk move in `internal/tui/app.go`: if `a.cfg.FindGroup(targetGroup) == nil && targetGroup != ""`, call `a.cfg.AddGroup(targetGroup)` before executing the bulk move
- [ ] T011 [US1] Add post-move cleanup in bulk move handler in `internal/tui/app.go`: call `a.visualState.Exit()`, save config via `config.Save(a.cfg)`, refresh list (`a.list.filtered = a.list.groupFilteredConns()`, `a.list.rebuildTable()`, `a.syncHeaderView()`, `a.syncCursorPosition()`), flash "Moved X of Y connections to <group>"

### Verification

- [ ] T012 [US1] Verify `go build ./...` and `go test ./internal/tui/...` pass after bulk move implementation

**Checkpoint**: Bulk group assignment works via `:mv`/`:move` with batch undo.

---

## Phase 3: User Story 3 - Bulk Tag/Delete Batch Undo Retrofit (Priority: P3)

**Goal**: Existing bulk tag and delete operations use batch undo entries instead of individual entries.

**Independent Test**: Select 5 connections, run `:tag add critical`, press `u` once, verify tag removed from all 5.

### Tag Retrofit

- [ ] T013 [US3] Retrofit `tagAddVisual()` in `internal/tui/app.go` to use batch undo: collect individual `Operation{Type: UndoOpTagChange, ...}` entries into a `[]Operation` slice instead of pushing each individually, then call `a.undoStack.PushBatch("bulk tag add "+tag, ops)` after the loop, add `a.visualState.Exit()` call
- [ ] T014 [US3] Retrofit `tagRemoveVisual()` in `internal/tui/app.go` to use batch undo: same pattern as T013 — collect operations, call `PushBatch("bulk tag remove "+tag, ops)`, add `a.visualState.Exit()` call

### Delete Retrofit

- [ ] T015 [US3] Retrofit `delete-visual` handler in `internal/tui/app.go` confirm action to use batch undo: collect individual `Operation{Type: UndoOpDelete, ...}` entries into a `[]Operation` slice instead of pushing each individually, then call `a.undoStack.PushBatch("bulk delete", ops)` after the loop

### Verification

- [ ] T016 [US3] Verify `go build ./...` and `go test ./internal/tui/...` pass after retrofit changes

**Checkpoint**: All bulk operations (move, tag, delete) use batch undo.

---

## Phase 4: Polish & Cross-Cutting Concerns

**Purpose**: Final verification across all stories

- [ ] T017 Verify full compilation: `go build ./...` and `go vet ./...` pass with zero new warnings
- [ ] T018 Verify existing single-item undo/redo still works correctly (not broken by batch changes)
- [ ] T019 Verify visual mode exits after all bulk operations (move, tag add, tag remove, delete)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (US2 — Batch Undo)**: No dependencies — start immediately. BLOCKS US1 and US3.
- **Phase 2 (US1 — Bulk Move)**: Depends on Phase 1 completion (needs PushBatch method)
- **Phase 3 (US3 — Retrofit)**: Depends on Phase 1 completion (needs PushBatch method). Can run in parallel with Phase 2.
- **Phase 4 (Polish)**: Depends on all phases complete

### User Story Dependencies

- **US2 (P2, Batch Undo)**: Foundation — must complete first despite being P2 priority
- **US1 (P1, Bulk Move)**: Depends on US2 for PushBatch
- **US3 (P3, Retrofit)**: Depends on US2 for PushBatch. Independent of US1.

### Parallel Opportunities

**After Phase 1 completes:**
```
Phase 2 (US1 — Bulk Move):     T007 → T008 → T009 → T010 → T011 → T012
Phase 3 (US3 — Retrofit):      T013, T014 parallel with T015 → T016
```

US1 and US3 can run in parallel since they modify different sections of app.go (mv handler vs tag/delete handlers).

---

## Implementation Strategy

### MVP First (US2 + US1)

1. Complete Phase 1: Batch undo infrastructure
2. Complete Phase 2: Bulk move
3. **STOP and VALIDATE**: Select connections, `:mv` to group, undo once
4. Commit: "Add batch undo and bulk group assignment"

### Incremental Delivery

1. Phase 1 (US2) → Commit → Batch undo ready
2. Phase 2 (US1) → Commit → Bulk move working
3. Phase 3 (US3) → Commit → All bulk ops use batch undo
4. Phase 4 → Final verification → Merge-ready

---

## Notes

- US2 (batch undo) is implemented before US1 (bulk move) because it's the foundation
- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- After each phase: `go build ./...` must pass
