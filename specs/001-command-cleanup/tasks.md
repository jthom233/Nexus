# Tasks: TUI Command & Keybinding Cleanup

**Input**: Design documents from `/specs/001-command-cleanup/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/

**Tests**: Not requested — no test tasks generated.

**Organization**: Tasks grouped by user story for independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: User Story 1 - Remove Dead Commands and Keybindings (Priority: P1)

**Goal**: Eliminate all dead/stub code so every registered command and keybinding works. Zero "not yet implemented" messages.

**Independent Test**: `go build ./...` succeeds, `go vet ./...` passes, grep for "not yet implemented" in `internal/tui/` returns zero matches (except keybindings editor which remains as a real planned feature).

### Removals — File Deletions

- [X] T001 [P] [US1] Delete marks system file `internal/tui/marks.go` (entire file — Mark, MarkManager structs and all methods)
- [X] T002 [P] [US1] Delete tree model file `internal/tui/tree.go` (entire file — TreeNode, TreeEntry, TreeModel structs and all methods)

### Removals — app.go Cleanup

- [X] T003 [US1] Remove marks-related fields from App struct in `internal/tui/app.go`: remove `marks *MarkManager` (line 70), `markPending rune` (line 71), and `NewMarkManager()` initialization (line 140)
- [X] T004 [US1] Remove marks keybinding handlers from `internal/tui/app.go`: remove `case "m":` mark-set handler (line 724-725), `case "'"`: mark-jump handler (line 727-728), backtick handler (line 730-731), mark completion logic (lines 681-717), `ctrl+o` jump-back handler (lines 733-739), `ctrl+i` jump-forward handler (lines 740-746)
- [X] T005 [US1] Remove all `marks.PushJump()` calls scattered through `internal/tui/app.go` (lines 854, 875, 2245, 2472)
- [X] T006 [US1] Remove tree-related fields from App struct in `internal/tui/app.go`: remove `tree *TreeModel` (line 57), `viewMode string` (line 84), `treeFoldPending bool` (line 85), and `NewTreeModel()` initialization (line 132)
- [X] T007 [US1] Remove `handleTreeKey()` function and its call site from `internal/tui/app.go` (lines 592-656 function, line 674 call site)
- [X] T008 [US1] Remove `case "z":` fold-pending handler (line 629-630) and fold completion logic from `internal/tui/app.go`

### Removals — Command Registry

- [X] T009 [US1] Remove 9 stub commands from `defaultCommands()` in `internal/tui/command.go` (lines 39-72): disconnect/dc, theme, version/ver, marks, template/tpl, settings/set, recordings/rec, health, vault

### Removals — Motion Engine

- [X] T010 [P] [US1] Remove `OpChange` operator constant and all change-operator handling from `internal/tui/motion.go` (remove the constant from OpType, remove `cc`/`c{motion}` dispatch logic)

### Removals — Leader Menu Stubs

- [X] T011 [P] [US1] Remove stub leader items from `leaderGroups()` in `internal/tui/leader.go`: remove list-groups and filter-by-group from Groups group (lines 186-192), remove view-tree from View group (line 225), remove check-selected from Health group, remove theme from Options group (line 265)
- [X] T012 [US1] Remove "not yet implemented" flash message handlers from `executeLeaderAction()` in `internal/tui/app.go`: remove cases for list-groups (line 2016), filter-by-group (line 2019), view-tree (line 2088), check-selected (line 2177), theme (line 2202)

### Verification

- [X] T013 [US1] Verify `go build ./...` and `go vet ./...` pass with no new warnings after all removals in `internal/tui/` and `internal/config/`

**Checkpoint**: All dead code removed. Every registered command has a working handler. No "not yet implemented" messages for removed features.

---

## Phase 2: User Story 2 - Group Management (Priority: P2)

**Goal**: Users can create, delete, and move connections between groups via `:mkdir`, `:rmdir`, and `:mv` commands.

**Independent Test**: Create a group with `:mkdir test`, verify it appears in group filter. Move a connection with `:mv`. Delete the group with `:rmdir test`. All operations persist after restart.

### Config Layer (foundation)

- [ ] T014 [US2] Add `FindGroup(name string) (*Group, bool)` method to Config in `internal/config/config.go` — iterate `Config.Groups`, return pointer and bool
- [ ] T015 [US2] Add `HasConnectionsInGroup(name string) bool` method to Config in `internal/config/config.go` — iterate `Config.Connections`, return true if any has `Group == name`
- [ ] T016 [US2] Add `AddGroup(name string) error` method to Config in `internal/config/config.go` — validate non-empty, check duplicates, append `Group{Name: name}` to `Config.Groups`
- [ ] T017 [US2] Add `DeleteGroup(name string) error` method to Config in `internal/config/config.go` — check group exists, check no connections reference it, remove from `Config.Groups`
- [ ] T018 [US2] Add `MoveConnection(connID string, targetGroup string) error` method to Config in `internal/config/config.go` — find connection by ID, update Group field, auto-create target group if missing via `AddGroup`

### Command Handlers

- [ ] T019 [US2] Add `:mkdir` command handler in `internal/tui/app.go` command dispatch switch — parse group name from args, call `cfg.AddGroup()`, save config, flash success/error message
- [ ] T020 [US2] Add `:rmdir` command handler in `internal/tui/app.go` command dispatch switch — parse group name from args, call `cfg.DeleteGroup()`, save config, flash success/error message
- [ ] T021 [US2] Add `:mv`/`:move` command handler in `internal/tui/app.go` command dispatch switch — parse optional connection name + required group, use selected connection if no name given, call `cfg.MoveConnection()`, register with undo system as `UndoOpEdit` (snapshot before/after), save config, flash success/error
- [ ] T022 [US2] Merge `:mv` and `:move` into single handler in `internal/tui/app.go` — both commands should route to the same logic (`:move <group>` moves selected connection, `:mv <conn> <group>` moves named connection)

### Verification

- [ ] T023 [US2] Verify `go build ./...` passes and group commands work: `:mkdir`, `:rmdir`, `:mv` all produce correct flash messages and persist to config

**Checkpoint**: Group management fully functional via command bar. All operations persist.

---

## Phase 3: User Story 3 - Leader Menu Completeness (Priority: P3)

**Goal**: Every working `:` command has a corresponding leader menu path. No stub actions remain.

**Independent Test**: Open leader menu (space), verify every group has only working actions. Cross-reference against command registry — every command reachable.

### Leader Menu Restructure

- [ ] T024 [US3] Restructure Connections group in `leaderGroups()` in `internal/tui/leader.go` — add items: add-connection (a), edit-connection (e), delete-connection (d) alongside existing connect-selected (c) and quick-connect (q)
- [ ] T025 [US3] Restructure Find group in `leaderGroups()` in `internal/tui/leader.go` — add items: show-recent (r→keep), show-frequent (q), show-favorites (b), show-all (a) alongside existing fuzzy-find, find-sessions, find-by-tag, find-by-group, toggle-favorite
- [ ] T026 [US3] Restructure Groups group in `leaderGroups()` in `internal/tui/leader.go` — replace old stubs with: create-group (c), delete-group (d), move-to-group (m), filter-by-group (f)
- [ ] T027 [US3] Add new Data group to `leaderGroups()` in `internal/tui/leader.go` — key "d", items: set-note (n), manage-fields (f)
- [ ] T028 [US3] Trim Health group in `leaderGroups()` in `internal/tui/leader.go` — keep only check-all (a) and pulse-view (p), remove check-selected
- [ ] T029 [US3] Trim Options group in `leaderGroups()` in `internal/tui/leader.go` — keep only keybindings (k), remove theme
- [ ] T030 [US3] Trim View group in `leaderGroups()` in `internal/tui/leader.go` — keep table (t), detail (d), wide (w), event-log (e), remove tree

### Leader Action Handlers

- [ ] T031 [US3] Add `executeLeaderAction` cases in `internal/tui/app.go` for new Connections actions: add-connection (trigger add form), edit-connection (trigger edit on selected), delete-connection (trigger delete confirmation on selected)
- [ ] T032 [US3] Add `executeLeaderAction` cases in `internal/tui/app.go` for new Find actions: show-recent (set listViewRecent), show-frequent (set listViewFrequent), show-favorites (set listViewFavorites), show-all (clear filters)
- [ ] T033 [US3] Add `executeLeaderAction` cases in `internal/tui/app.go` for Groups actions: create-group (prompt for name → `:mkdir`), delete-group (prompt for name → `:rmdir`), move-to-group (prompt for group → `:mv`), filter-by-group (trigger group filter picker)
- [ ] T034 [US3] Add `executeLeaderAction` cases in `internal/tui/app.go` for Data actions: set-note (enter command mode with `:note ` prefilled), manage-fields (enter command mode with `:field ` prefilled)
- [ ] T035 [US3] Fix kill-session leader action in `internal/tui/app.go` — instead of "open sessions view first" message, directly kill the session associated with the currently selected connection (if any), or switch to sessions view for selection
- [ ] T036 [US3] Implement copy-password leader action in `internal/tui/app.go` — copy selected connection's password to clipboard using `atotto/clipboard` (already a project dependency)

### Verification

- [ ] T037 [US3] Verify `go build ./...` passes and every leader menu item triggers a working action — no "not yet implemented" flash messages remain in `internal/tui/app.go`

**Checkpoint**: Leader menu is complete. Every `:` command has a leader path. No stubs remain.

---

## Phase 4: Polish & Cross-Cutting Concerns

**Purpose**: Final verification across all stories

- [ ] T038 Verify full compilation: `go build ./...` and `go vet ./...` pass with zero new warnings
- [ ] T039 Grep `internal/tui/` for "not yet implemented" — confirm zero matches (except keybinding editor if intentionally kept)
- [ ] T040 Verify no dangling references to deleted types (MarkManager, TreeModel, TreeNode, TreeEntry, Mark) anywhere in `internal/tui/`
- [ ] T041 Verify command registry count matches leader menu action count — every command reachable from both `:` and space leader

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (US1 — Removals)**: No dependencies — start immediately
- **Phase 2 (US2 — Group Management)**: Depends on T009 (command registry cleanup) and T013 (compilation check) from Phase 1
- **Phase 3 (US3 — Leader Menu)**: Depends on Phase 2 completion (groups leader actions need working group commands)
- **Phase 4 (Polish)**: Depends on all phases complete

### User Story Dependencies

- **US1 (P1)**: Independent — can start immediately, blocks US2 and US3
- **US2 (P2)**: Depends on US1 completion (clean registry baseline)
- **US3 (P3)**: Depends on US2 completion (group commands must work before wiring leader)

### Within Each User Story

- File deletions (T001, T002) can run in parallel
- app.go changes (T003-T008) are sequential (same file, dependent edits)
- Config methods (T014-T018) are sequential (same file, methods build on each other)
- Leader restructure (T024-T030) can run in parallel (different sections of same function, but logically independent edits)

### Parallel Opportunities

**Phase 1 parallel groups:**
```
Group A (file deletions):  T001, T002 — parallel
Group B (motion.go):       T010 — parallel with Group A
Group C (leader stubs):    T011 — parallel with Groups A, B
Group D (app.go cleanup):  T003 → T004 → T005 → T006 → T007 → T008 — sequential
Group E (command.go):      T009 — parallel with Groups A, B, C
Group F (leader handlers): T012 — after T011
```

**Phase 2 sequential:**
```
T014 → T015 → T016 → T017 → T018 (config methods, same file)
T019 → T020 → T021 → T022 (command handlers, same file)
```

**Phase 3 parallel groups:**
```
Group A (leader.go restructure): T024-T030 — logically parallel
Group B (app.go handlers):       T031-T036 — sequential (same file)
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Remove all dead code
2. **STOP and VALIDATE**: `go build`, `go vet`, grep for stubs
3. Commit: "Remove dead commands, marks, tree, and change operator"

### Incremental Delivery

1. Phase 1 (US1) → Commit → Clean baseline
2. Phase 2 (US2) → Commit → Group management working
3. Phase 3 (US3) → Commit → Leader menu complete
4. Phase 4 → Final verification → Merge-ready

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- US1 is prerequisite for US2 and US3 (sequential dependency)
- Commit after each phase for clean git history
- After each phase: `go build ./...` must pass
