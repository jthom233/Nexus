# Tasks: Log Aggregation in TUI

**Input**: Design documents from `/specs/005-log-aggregation/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md

**Tests**: No automated tests requested. Manual integration testing per quickstart.md.

**Organization**: Tasks grouped by user story. Each story is independently implementable and testable after foundational phase completes.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (IPC Protocol Extension)

**Purpose**: Add the IPC message type and payload struct that all subsequent phases depend on

- [ ] T001 Add `MsgLogLine = "log-line"` constant and `LogLineEvent` struct (Level, Message, Timestamp fields) to `internal/ipc/protocol.go`, following the existing `MsgTabOpened`/`TabOpenedEvent` pattern

---

## Phase 2: Foundational (GUI-Side Logger + IPC Wiring)

**Purpose**: Create the GUI logger wrapper and wire IPC broadcast so the GUI can send structured log messages. MUST complete before US1 can work end-to-end.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [ ] T002 Add `SendLogLine(level, message string)` method to `IPCManager` in `internal/gui/ipc.go` — calls `m.server.Broadcast(ipc.MsgLogLine, ipc.LogLineEvent{...})` with current timestamp
- [ ] T003 Create `internal/gui/logger.go` with `ipcLogger` struct: holds `*IPCManager` reference, exposes `Info(format, args...)`, `Warn(format, args...)`, `Error(format, args...)` methods. Each method dual-writes: (1) `log.Printf()` for stderr/file, (2) `ipcMgr.SendLogLine()` for IPC. If `ipcMgr` is nil, falls back to `log.Printf` only
- [ ] T004 [P] Replace `log.Printf` calls in `internal/gui/clipboard.go` with `ipcLogger.Info/Warn/Error()` calls. The `ipcLogger` instance should be accessible from the clipboard channel (pass via field or package-level var set during `App` init)
- [ ] T005 [P] Replace `log.Printf` calls in `internal/gui/rdp.go` with `ipcLogger.Info/Warn/Error()` calls, same pattern as T004
- [ ] T006 Add `GUILogLineMsg` tea.Msg type (Level string, Message string, Timestamp time.Time) to `internal/tui/app.go` or a dedicated messages file. Add `*tea.Program` field to `RDPLauncher` in `internal/launcher/rdp.go`. In the `Launch()` receive loop, add `case ipc.MsgLogLine:` that unmarshals `LogLineEvent`, calls `p.Send(GUILogLineMsg{...})`, and continues the loop (does NOT return)
- [ ] T007 Verify `go build ./...` and `go vet ./...` pass clean after foundational phase

**Checkpoint**: GUI subprocess can now broadcast structured log messages over IPC, and the TUI launcher can receive and forward them as tea.Msg. The unified Logs view is not yet built — messages arrive but are not displayed.

---

## Phase 3: User Story 1 - View All Logs in One Place (Priority: P1) 🎯 MVP

**Goal**: Replace the separate Event Log and Audit Log views with a single unified "Logs" view that shows TUI events, audit entries, and GUI debug messages interleaved chronologically.

**Independent Test**: Connect to an RDP session, perform a clipboard operation, open Logs (`Ctrl+L`), verify GUI subprocess messages appear alongside TUI events and audit entries in chronological order.

### Implementation for User Story 1

- [ ] T008 [US1] Rewrite `internal/tui/log.go`: Replace `logModel` with `logsModel`. Add `logType` enum constants (`logTypeEvent`, `logTypeAudit`, `logTypeDebug`). Extend `logEntry` struct with `logType` field. Add `logsModel` fields: `activeType int` (default 0 = All), `dirty bool`, `maxEntries int` (10000), `evictBatch int` (1000). Implement `add()` with buffer cap + batch eviction + dirty flag. Implement lazy `View()` that rebuilds viewport content only when dirty. Add convenience methods: `addEvent(level, format, args...)`, `addAudit(level, message)`, `addDebug(level, message)`. Keep existing `info()`, `warn()`, `error()` as aliases to `addEvent()`. Preserve `plainText()` for clipboard copy. Keep `setSize()`, `Update()` (viewport delegation) unchanged.
- [ ] T009 [US1] Delete `internal/tui/logview.go` entirely — its functionality is absorbed into the new `logsModel`
- [ ] T010 [US1] Update `internal/tui/app.go` — view consolidation: Remove `viewAuditLog` from `viewKind` enum. Remove `auditLogView *LogViewModel` field from `App` struct. Remove `handleAuditLogKey()` function (dead code). Remove `openAuditLogView()` function. Remove `auditLogView.SetSize()` call from `layout()`. Remove `viewAuditLog` cases from view dispatch in `Update()` and `View()`. Update all `a.log.info/warn/error()` call sites to use the new `logsModel` API (should be compatible if aliases kept). Rename the `log *logModel` field to `logs *logsModel` (or keep as `log` for minimal diff).
- [ ] T011 [US1] Update `internal/tui/app.go` — audit integration: In `logAuditEvent()`, after calling `a.auditLog.Log(event)`, also call `a.logs.addAudit(level, formattedMessage)` to feed audit entries into the unified model. Map audit event types to log levels: `EventConnect`/`EventDisconnect` → `logInfo`, `EventError` → `logError`, `EventCredentialUse`/`EventConfigChange` → `logWarn`.
- [ ] T012 [US1] Update `internal/tui/app.go` — GUI log reception: Add `case GUILogLineMsg:` in the main `Update()` switch. Map `msg.Level` string to `logLevel` enum ("info"→`logInfo`, "warn"→`logWarn`, "error"→`logError`, default→`logInfo`). Call `a.logs.addDebug(level, msg.Message)`.
- [ ] T013 [US1] Update `internal/tui/app.go` — header and commands: Rename "Event Log" tab to "Logs" in `syncHeaderView()`. Remove "Audit Log" header tab entry. Update `:audit` command to open the Logs view (same as `:logs` / `Ctrl+L`). Update `:logs` command if it doesn't already exist.
- [ ] T014 [US1] Verify `go build ./...` and `go vet ./...` pass clean. Manual test: connect to RDP, copy/paste image, open Logs (`Ctrl+L`), verify all three log sources appear interleaved.

**Checkpoint**: User Story 1 complete — unified Logs view shows all log sources. Tab cycling and type indicator not yet implemented.

---

## Phase 4: User Story 2 - Filter Logs by Type (Priority: P2)

**Goal**: Add Tab-cycling through log types (All, Events, Audit, Debug) within the Logs view.

**Independent Test**: Open the Logs view with mixed entries, press Tab repeatedly, verify each type shows only its entries and wraps around from Debug → All.

### Implementation for User Story 2

- [ ] T015 [US2] Add `nextType()` method to `logsModel` in `internal/tui/log.go`: increments `activeType` mod 4, sets `dirty = true`. Add `filterEntries()` helper that returns a subset of `entries` matching `activeType` (or all entries if activeType==0). Update `View()` to use `filterEntries()` when rebuilding viewport content. Include filtered entry count in scroll hint.
- [ ] T016 [US2] Update `handleLogKey()` in `internal/tui/app.go`: Add `case "tab": a.logs.nextType(); return a, nil` to handle Tab key within the Logs view. Ensure Tab key is NOT intercepted in other views (FR-009) — the case only triggers when `currentView() == viewLog`, which is already guarded by the `handleLogKey` dispatch.
- [ ] T017 [US2] Handle empty type state in `logsModel.View()` in `internal/tui/log.go`: When `filterEntries()` returns zero entries for the active type, render an empty state message (e.g., styled "No [type] messages yet") instead of a blank viewport.
- [ ] T018 [US2] Manual test: generate activity across all types (connect session = events + audit, copy/paste = debug), open Logs, Tab through All → Events → Audit → Debug → All. Verify each shows only matching entries, empty types show placeholder, new entries appear in real time.

**Checkpoint**: User Story 2 complete — Tab cycling works, filtering is accurate.

---

## Phase 5: User Story 3 - Active Log Type Indicator (Priority: P3)

**Goal**: Display the active log type name in the Logs view panel header.

**Independent Test**: Open the Logs view, verify header shows "Logs [All]". Press Tab, verify header changes to "Logs [Events]", then "Logs [Audit]", then "Logs [Debug]".

### Implementation for User Story 3

- [ ] T019 [US3] Update `View()` in `internal/tui/log.go`: Change the title render from `HelpTitleStyle.Render("Logs")` to `HelpTitleStyle.Render(fmt.Sprintf("Logs [%s]", typeName))` where `typeName` maps `activeType` to "All", "Events", "Audit", "Debug". Update the scroll hint to also include the type name or filtered count (e.g., "12 entries" when filtered vs "42 total").
- [ ] T020 [US3] Manual test: open Logs, verify "Logs [All]" in header. Tab through each type, verify header updates instantly to match.

**Checkpoint**: All user stories complete — unified Logs view with type filtering and header indicator.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Build verification, edge case hardening, cleanup

- [ ] T021 [P] Verify `go build ./...` and `go vet ./...` pass clean with all changes
- [ ] T022 [P] Review all error paths in IPC log forwarding: verify `SendLogLine()` silently handles broadcast failures (no panic, no user-visible error), verify `Launch()` loop handles malformed `LogLineEvent` payloads gracefully (unmarshal error → skip, continue loop)
- [ ] T023 Verify no regressions: existing text clipboard copy/paste still works, existing Ctrl+L keybinding opens Logs, existing `q`/`esc` close the Logs view, existing `y` copies log content to clipboard, existing `j`/`k` scroll the viewport
- [ ] T024 Verify buffer cap behavior: add 10,001+ entries programmatically or via rapid log generation, confirm oldest entries are evicted and viewport renders correctly with truncated history

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 (T001) — IPC protocol must exist before GUI logger can use it
- **User Story 1 (Phase 3)**: Depends on Phase 2 — GUI must be broadcasting + TUI must be receiving before the unified view can display debug messages
- **User Story 2 (Phase 4)**: Depends on Phase 3 — filtering requires the unified model from US1
- **User Story 3 (Phase 5)**: Depends on Phase 4 — header indicator shows the active type, which requires the type cycling from US2
- **Polish (Phase 6)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Depends on Foundational (Phase 2) only — core unified view
- **User Story 2 (P2)**: Depends on US1 — adds filtering on top of the unified model
- **User Story 3 (P3)**: Depends on US2 — adds header display of the active type

### Within Each Phase

- T004 and T005 can run in parallel (different files, same pattern)
- T008 and T009 are sequential (T009 deletes a file that T008 replaces)
- T010, T011, T012, T013 are sequential (all modify app.go)
- T015, T016, T017 are sequential (T015 adds the method, T016 wires it, T017 handles empty state)

### Parallel Opportunities

- Phase 2: T004 and T005 (clipboard.go and rdp.go log replacement) can run in parallel
- Phase 6: T021 and T022 can run in parallel

---

## Parallel Example: Phase 2

```
# These two tasks can run in parallel (different files, same pattern):
Task T004: "Replace log.Printf in internal/gui/clipboard.go with ipcLogger calls"
Task T005: "Replace log.Printf in internal/gui/rdp.go with ipcLogger calls"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001) — IPC protocol
2. Complete Phase 2: Foundational (T002-T007) — GUI logger + IPC wiring
3. Complete Phase 3: User Story 1 (T008-T014) — Unified Logs view
4. **STOP and VALIDATE**: Open Logs, verify all three sources appear
5. This delivers the core value: all logs in one place

### Incremental Delivery

1. Setup + Foundational → IPC log forwarding works
2. Add User Story 1 → Unified view shows all logs → Core feature complete
3. Add User Story 2 → Tab filtering works → Power users can focus
4. Add User Story 3 → Type indicator in header → Polish complete
5. Polish → Build clean, edge cases verified

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- US2 depends on US1, US3 depends on US2 (sequential story dependency — not parallelizable)
- The `logview.go` deletion (T009) should happen in the same commit as the `log.go` rewrite (T008) to avoid broken intermediate states
- All `app.go` modifications (T010-T013) should be done carefully as `app.go` is a large file (~3000 lines) with many interconnected view dispatch paths
- The `*tea.Program` reference for `RDPLauncher` (T006) may require changes to how the launcher is initialized in `app.go` — check where `RDPLauncher` is constructed
