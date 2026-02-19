# Tasks: Picker-Style Multi-Select

**Input**: Design documents from `/specs/006-picker-multiselect/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md

**Tests**: Unit tests included — 16 existing tests in `visual_test.go` must be updated, plus new tests for Toggle, SelectAll, two-stage Esc behavior.

**Organization**: Tasks grouped by user story. US1 (toggle-select) is the MVP. US2 (bulk ops) builds on US1. US3 (select-all + visual mode) builds on both but is independently testable after US1.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: Preparation — no code changes, just validation of the starting state

- [ ] T001 Run `go test ./internal/tui/...` and confirm all 16 existing visual_test.go tests pass before refactoring in `internal/tui/visual_test.go`

---

## Phase 2: Foundational — SelectionSet Core (Blocking Prerequisites)

**Purpose**: Refactor `VisualState` into `SelectionSet` with the unified selected map. MUST complete before any user story work can begin — this is the data model that everything depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [ ] T002 Refactor `VisualState` struct to `SelectionSet` in `internal/tui/visual.go`: replace `selection map[int]bool` + `toggled map[int]bool` with single `selected map[int]bool` field. Keep `visualActive bool` and `anchor int`. Remove `rangeStart`/`rangeEnd` fields.
- [ ] T003 Implement `Toggle(idx int)` method on `SelectionSet` in `internal/tui/visual.go`: if `selected[idx]` is true, delete it; otherwise set it to true. Works regardless of `visualActive` state.
- [ ] T004 Implement `Deselect(idx int)` method on `SelectionSet` in `internal/tui/visual.go`: unconditionally delete `selected[idx]`. Idempotent — no error if already absent.
- [ ] T005 Implement `SelectAll(count int)` and `DeselectAll()` methods on `SelectionSet` in `internal/tui/visual.go`: `SelectAll` adds indices 0..count-1 to selected map. `DeselectAll` clears the map entirely but does NOT change `visualActive`.
- [ ] T006 Refactor `EnterVisual(cursor int)` in `internal/tui/visual.go`: set `visualActive=true`, `anchor=cursor`, add cursor to `selected` map. Do NOT clear existing selections — preserve any Tab-toggled items.
- [ ] T007 Refactor `ExitVisual()` in `internal/tui/visual.go`: set `visualActive=false` ONLY. Do NOT clear `selected` map. This is the key behavior change — selection persists after Visual mode exit.
- [ ] T008 Refactor `UpdateRange(cursor int)` in `internal/tui/visual.go`: compute range `[min(anchor,cursor)..max(anchor,cursor)]` and ADD all indices to `selected` map. This is now **additive** — does not remove items outside the range. Existing Tab-selected items outside the range are preserved.
- [ ] T009 Update read accessors in `internal/tui/visual.go`: `IsSelected(idx)` checks `selected[idx]`, `Count()` returns `len(selected)`, `HasSelection()` returns `Count() > 0`, `SelectedIndices()` returns sorted keys of `selected`, `SelectedIDs(rows)` maps indices to row IDs with bounds checking. Remove `Active()` method — replaced by `HasSelection()` and `VisualActive()`.
- [ ] T010 Update all 16 existing tests in `internal/tui/visual_test.go` to use the new `SelectionSet` API: replace `NewVisualState` with `SelectionSet{}`, update method calls from old to new signatures, verify all tests pass.
- [ ] T011 Add new unit tests in `internal/tui/visual_test.go`: test `Toggle` (add + remove), `Deselect` (idempotent), `SelectAll`/`DeselectAll`, `EnterVisual` preserves existing selection, `ExitVisual` preserves selected map, additive `UpdateRange` (Tab-select items 1,5,10 → Visual range 3-7 → result is {1,3,4,5,6,7,10}).
- [ ] T012 Run `go test ./internal/tui/...` and `go build ./...` to confirm all tests pass and build succeeds after foundational refactor

**Checkpoint**: SelectionSet core is complete. All existing tests pass with new API. New Toggle/SelectAll/DeselectAll/additive-range tests pass.

---

## Phase 3: User Story 1 — Toggle-Select Individual Items (Priority: P1) 🎯 MVP

**Goal**: Users can Tab-toggle individual items in the connection list. Selected items show a ✓ checkmark. Navigation does not change selection. Shift-Tab deselects.

**Independent Test**: Navigate a connection list with 5+ items, press Tab on 3 non-contiguous items, verify each shows a ✓ indicator, press Tab on one of them again to deselect it. Navigation with j/k does not change selection.

### Implementation for User Story 1

- [ ] T013 [US1] Remove the `case "tab": a.list.cycleGroup()` binding from Normal mode key handling in `internal/tui/app.go`. Group cycling is now only accessible via leader `g` `f`.
- [ ] T014 [US1] Rename `visualState` field to `selectionSet` (type `SelectionSet`) throughout `internal/tui/app.go`. Update all references from `a.visualState` to `a.selectionSet`. Replace all `a.visualState.Active()` checks with `a.selectionSet.HasSelection()` or `a.selectionSet.VisualActive()` as appropriate.
- [ ] T015 [US1] Add Tab key binding in Normal mode list view in `internal/tui/app.go`: on `"tab"`, call `a.selectionSet.Toggle(cursor)`, sync table's selectionSet pointer (`&a.selectionSet` if HasSelection, nil otherwise), sync status bar visual count.
- [ ] T016 [US1] Add Shift-Tab key binding in Normal mode list view in `internal/tui/app.go`: on `"shift+tab"`, call `a.selectionSet.Deselect(cursor)`, sync table and status bar.
- [ ] T017 [US1] Add Esc behavior for selection clearing in Normal mode in `internal/tui/app.go`: when Esc is pressed and `a.selectionSet.HasSelection()` is true (and NOT `VisualActive()`), call `DeselectAll()`, sync table and status bar, consume the keypress (return before other Esc handling).
- [ ] T018 [P] [US1] Rename `visualState *VisualState` to `selectionSet *SelectionSet` in `internal/tui/table.go`. Update the condition to set `selectionSet` pointer: set it whenever `HasSelection()` is true (not just when in Visual mode). In `renderRow()`, when `isSelected` is true, prepend `✓ ` to the first column content.
- [ ] T019 [P] [US1] Update `internal/tui/statusbar.go` to show selection count whenever `selectionSet.HasSelection()` is true, not just in `ModeVisual`. Format: `"N selected"` in Normal mode with selections, `"VISUAL (N selected)"` in Visual mode.
- [ ] T020 [P] [US1] Update `internal/tui/list.go`: remove or unbind the `cycleGroup()` key binding if it has a separate binding path. Keep the function itself accessible for leader key sequences if needed.
- [ ] T021 [US1] Run `go test ./internal/tui/...` and `go build ./...` to verify all tests pass and build succeeds after US1 implementation.

**Checkpoint**: Tab toggles items, ✓ appears, Shift-Tab deselects, Esc clears selection, status bar shows count. Navigation does not change selection. Group cycling removed from Tab.

---

## Phase 4: User Story 2 — Bulk Operations on Selected Items (Priority: P2)

**Goal**: All existing bulk operations (d, y, :tag, :move, :export) work on selected items when any are selected, and fall back to cursor item when none are selected. Selections clear after operations complete.

**Independent Test**: Select 3 items with Tab, run `:tag add test`, verify all 3 get tagged. Deselect all, run `:tag add test2` on cursor item, verify only that one gets tagged. Select 2 items, press `d`, verify confirmation shows both, confirm deletion.

### Implementation for User Story 2

- [ ] T022 [US2] Update `d` key handling in Normal mode in `internal/tui/app.go`: if `a.selectionSet.HasSelection()`, resolve selected items via `SelectedIDs(rows)` and show bulk delete confirmation listing all selected items. After deletion completes, call `DeselectAll()` and sync. If no selection, preserve existing single-item delete behavior.
- [ ] T023 [US2] Update `y` key handling in Normal mode in `internal/tui/app.go`: if `a.selectionSet.HasSelection()`, yank connection commands for all selected items. After yank completes, call `DeselectAll()` and sync. If no selection, preserve existing single-item yank behavior.
- [ ] T024 [US2] Standardize `:tag add/remove` command in `internal/tui/bulk.go` to use `SelectedIDs(rows)` instead of `SelectedIndices()` + `filtered[idx]` index-based resolution. Call `DeselectAll()` after operation completes.
- [ ] T025 [US2] Standardize `:move` command in `internal/tui/bulk.go` (or `internal/tui/app.go` where handled) to use `SelectedIDs(rows)` for item resolution if not already. Call `DeselectAll()` after operation completes.
- [ ] T026 [US2] Standardize `:export` command in `internal/tui/bulk.go` to use `SelectedIDs(rows)` instead of index-based resolution. Fix pre-existing bug: `:export` must call `DeselectAll()` after completion (currently doesn't clear selection).
- [ ] T027 [US2] Verify all bulk operations clear selection after completion: test `d`, `y`, `:tag add`, `:tag remove`, `:move`, `:export` each call `DeselectAll()` on success AND on cancellation. Run `go test ./internal/tui/...` and `go build ./...`.

**Checkpoint**: All bulk operations work on selected items. Single-item operations unchanged when nothing selected. Selection clears after every operation.

---

## Phase 5: User Story 3 — Select All and Visual Mode Range Select (Priority: P3)

**Goal**: Ctrl+A toggles select-all/deselect-all. Visual mode (V) retained for range selection with additive behavior. Visual mode Esc preserves selection (first Esc → exit visual, second Esc → clear selection).

**Independent Test**: Press Ctrl+A on a list of 10 items, verify all 10 selected. Ctrl+A again, verify all deselected. Press V, Ctrl+D to range-select, Esc to exit Visual mode, verify items stay selected. Press d to delete range-selected items.

### Implementation for User Story 3

- [ ] T028 [US3] Add Ctrl+A key binding in Normal mode in `internal/tui/app.go`: if `a.selectionSet.HasSelection()` → `DeselectAll()`; else → `SelectAll(rowCount)`. Sync table and status bar.
- [ ] T029 [US3] Update Visual mode entry (`V` key) in `internal/tui/app.go`: call `a.selectionSet.EnterVisual(cursor)` which preserves existing Tab-selected items and adds the cursor row. Set mode to `ModeVisual`.
- [ ] T030 [US3] Update `handleVisualKey` in `internal/tui/app.go` for two-stage Esc: on Esc in Visual mode, call `a.selectionSet.ExitVisual()` (preserves selection), switch to `ModeNormal`. The existing Normal mode Esc handler (T017) will handle clearing selection on the second press.
- [ ] T031 [US3] Update Visual mode movement keys in `handleVisualKey` in `internal/tui/app.go`: `j`/`k`/`G`/`Ctrl+D`/`Ctrl+U` move cursor and call `a.selectionSet.UpdateRange(cursor)` (additive — adds range to existing set). Sync table and status bar after each movement.
- [ ] T032 [US3] Add Tab key support within Visual mode in `handleVisualKey` in `internal/tui/app.go`: on Tab, call `a.selectionSet.Toggle(cursor)` to cherry-pick/deselect individual items while in Visual mode. Sync table and status bar.
- [ ] T033 [US3] Run `go test ./internal/tui/...` and `go build ./...` to verify all tests pass and build succeeds after US3 implementation.

**Checkpoint**: Ctrl+A toggles all. Visual mode range-select additive. Two-stage Esc works. Tab works within Visual mode. All selection methods contribute to same set.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final verification, edge cases, and cleanup

- [ ] T034 [P] Run `go vet ./...` and verify no new warnings introduced (pre-existing IPv6 warnings in rdp.go/vnc.go are acceptable)
- [ ] T035 [P] Verify edge cases: Tab on empty list (no-op), Shift-Tab on deselected item (no-op), Ctrl+A with 0 items (no-op), Visual UpdateRange where anchor==cursor (single item)
- [ ] T036 Verify selection state preserved on scroll/cursor off-screen: select items, scroll list with Ctrl+D/Ctrl+U in Normal mode, verify selection indicators remain on selected items when scrolled back into view
- [ ] T037 Full regression test: verify leader `g` `f` group filtering still works, existing keybindings (`:tag`, `:move`, `:export`, `d`, `y`) work in both selected and non-selected states, mode indicator displays correctly in all states

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 — BLOCKS all user stories
- **US1 (Phase 3)**: Depends on Phase 2 — this is the MVP
- **US2 (Phase 4)**: Depends on Phase 3 (needs Tab-toggle to exist for selection)
- **US3 (Phase 5)**: Depends on Phase 2 (SelectionSet core), logically after US1 but technically independent
- **Polish (Phase 6)**: Depends on all user stories being complete

### User Story Dependencies

- **US1 (P1)**: Depends on Foundational only. Delivers Tab-toggle, visual indicator, status bar count.
- **US2 (P2)**: Depends on US1 (needs selection mechanism to exist). Delivers bulk operation integration.
- **US3 (P3)**: Depends on Foundational. Could be done in parallel with US2 but recommended after US1 for cleaner integration.

### Within Each User Story

- Key binding changes before visual/rendering changes
- app.go changes before table.go/statusbar.go changes (where [P] allows parallel)
- Test verification at the end of each phase

### Parallel Opportunities

Within Phase 2:
- T003, T004, T005 can be done in parallel (independent methods on same struct — but same file, so sequential recommended)
- T010 and T011 can be parallelized (different test functions in same file)

Within Phase 3 (US1):
- T018, T019, T020 can be done in parallel (different files: table.go, statusbar.go, list.go)

Within Phase 6:
- T034, T035 can be done in parallel (independent verification)

---

## Parallel Example: User Story 1

```bash
# Sequential (same file — app.go):
T013 → T014 → T015 → T016 → T017

# Parallel (different files):
T018 (table.go) | T019 (statusbar.go) | T020 (list.go)

# Then verify:
T021 (go test + go build)
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001)
2. Complete Phase 2: Foundational (T002–T012) — SelectionSet refactor
3. Complete Phase 3: User Story 1 (T013–T021) — Tab-toggle, visual indicator, Esc clear
4. **STOP and VALIDATE**: Test Tab on items, verify ✓, verify j/k doesn't change selection
5. This alone delivers the core picker-style selection UX

### Incremental Delivery

1. Setup + Foundational → SelectionSet core ready, all old tests pass
2. Add US1 → Tab-toggle works → Validate independently (MVP!)
3. Add US2 → Bulk operations work on selected items → Validate
4. Add US3 → Ctrl+A, Visual mode range-select, two-stage Esc → Validate
5. Polish → Full regression, edge cases, vet clean

---

## Notes

- This is a **refactor** — no new files, no new dependencies. All changes in existing `internal/tui/` files.
- The key architectural change: `UpdateRange` is now **additive** — it adds to the set instead of replacing it. This is what enables combining Tab-select with Visual-range-select.
- `ExitVisual()` no longer clears selection — this is the biggest behavior change from the current code.
- All bulk operations must be audited for `DeselectAll()` calls — this is FR-009.
- The `Active()` method is replaced by `HasSelection()` (any items selected) and `VisualActive()` (in Visual mode). Code that used `Active()` to check "should I do bulk op?" should use `HasSelection()`. Code that used `Active()` to check "am I in visual mode?" should use `VisualActive()`.
