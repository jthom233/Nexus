# Implementation Plan: Log Aggregation in TUI

**Branch**: `005-log-aggregation` | **Date**: 2026-02-18 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/005-log-aggregation/spec.md`

## Summary

Replace the separate Event Log and Audit Log views with a single unified "Logs" view that aggregates TUI events, audit trail entries, and GUI subprocess debug messages. Add structured IPC log forwarding from the GUI subprocess, Tab-cycling between log types (All/Events/Audit/Debug), and a header indicator showing the active type. Cap the in-memory buffer at 10,000 entries with batch eviction.

## Technical Context

**Language/Version**: Go 1.25 (per go.mod)
**Primary Dependencies**: charmbracelet/bubbletea v1.3.10, charmbracelet/bubbles v1.0.0, charmbracelet/lipgloss v1.1.0 (TUI framework), existing IPC via Unix socket (`internal/ipc/`)
**Storage**: Persistent audit JSONL file (unchanged), in-memory log buffer (new unified model)
**Testing**: Manual integration testing (no existing test framework for TUI)
**Target Platform**: Linux (primary), Windows (secondary)
**Project Type**: Single Go project — CLI/TUI + GUI subprocess
**Performance Goals**: Instant type switching (<100ms), no TUI stutter during log streaming
**Constraints**: 10,000 entry buffer cap, session-scoped (no historical data), lazy viewport rebuild
**Scale/Scope**: Single user, single TUI process, one GUI subprocess

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Constitution is an unfilled template — no project-specific gates defined. Proceeding without violations.

**Post-Phase 1 re-check**: No violations. The design modifies 6 existing files, creates 1 new file, and deletes 1 file. All changes within established `internal/tui/`, `internal/gui/`, `internal/ipc/`, and `internal/launcher/` packages. No new external dependencies. Follows existing Bubbletea patterns.

## Project Structure

### Documentation (this feature)

```text
specs/005-log-aggregation/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 output — research findings
├── data-model.md        # Phase 1 output — entity model
├── quickstart.md        # Phase 1 output — developer quickstart
├── checklists/          # Quality checklists
│   └── requirements.md
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code (repository root)

```text
internal/ipc/
├── protocol.go              # MODIFY — add MsgLogLine constant + LogLineEvent struct

internal/launcher/
├── rdp.go                   # MODIFY — add MsgLogLine case in Launch() receive loop,
│                            #   add *tea.Program reference for p.Send()

internal/gui/
├── ipc.go                   # MODIFY — add SendLogLine() broadcast helper
├── logger.go                # NEW — ipcLogger: dual-write to stderr + IPC
├── clipboard.go             # MODIFY — replace log.Printf with ipcLogger calls
├── rdp.go                   # MODIFY — replace log.Printf with ipcLogger calls

internal/tui/
├── log.go                   # MODIFY — replace logModel with logsModel:
│                            #   add logType to entries, tab cycling, type filtering,
│                            #   buffer cap with batch eviction, lazy viewport rebuild,
│                            #   header with type indicator
├── logview.go               # DELETE — functionality absorbed into logsModel
├── app.go                   # MODIFY — remove viewAuditLog, auditLogView,
│                            #   handleAuditLogKey(). Wire GUILogLineMsg in Update().
│                            #   Update audit event path to dual-write to logsModel.
│                            #   Remove openAuditLogView(). Update header tabs.
```

**Structure Decision**: Modifications within established packages following existing patterns. One new file (`logger.go`), one deleted file (`logview.go`). The `_other.go`/`_windows.go` build tag convention is not needed — IPC and logging are platform-independent.

## Implementation Design

### Component 1: IPC Log Protocol (`internal/ipc/protocol.go`)

Add a new message type for structured log forwarding:

- Constant: `MsgLogLine = "log-line"`
- Payload struct: `LogLineEvent{Level string, Message string, Timestamp time.Time}`
- Follows the existing `MsgTabOpened`/`TabOpenedEvent` pattern exactly

### Component 2: GUI Logger (`internal/gui/logger.go`)

A thin wrapper that dual-writes log messages:

- `ipcLogger` struct holds a reference to `*IPCManager`
- Methods: `Info(format, args...)`, `Warn(format, args...)`, `Error(format, args...)`
- Each method: (1) calls `log.Printf()` for stderr/file output, (2) calls `ipcMgr.SendLogLine()` for IPC broadcast
- If `ipcMgr` is nil (GUI not fully initialized), falls back to log.Printf only
- `SendLogLine()` on `IPCManager` calls `server.Broadcast(MsgLogLine, LogLineEvent{...})`

### Component 3: IPC Receive Loop Enhancement (`internal/launcher/rdp.go`)

Modify `Launch()` to handle log messages without terminating:

- Add `*tea.Program` field to `RDPLauncher` (set during initialization)
- In the `client.Recv()` loop, add case `MsgLogLine`:
  - Unmarshal `LogLineEvent` from payload
  - Call `p.Send(GUILogLineMsg{Level, Message, Timestamp})` to inject into Bubbletea
  - Continue loop (don't return)
- Unknown message types continue to be silently dropped

### Component 4: Unified Logs Model (`internal/tui/log.go`)

Replace `logModel` with `logsModel`:

**New entry struct**:
- Extends existing `logEntry` with `logType` field (logTypeEvent/logTypeAudit/logTypeDebug)

**New model fields**:
- `activeType int` — current filter (0=All, 1=Events, 2=Audit, 3=Debug)
- `dirty bool` — tracks whether viewport content needs rebuild
- `maxEntries int` — buffer cap (10,000)
- `evictBatch int` — entries to evict at once (1,000)

**Entry addition** (`add()`):
- Append to `entries` slice
- If `len(entries) > maxEntries`: evict oldest `evictBatch` entries
- Set `dirty = true` (defer viewport rebuild to `View()`)

**Type cycling** (`nextType()`):
- Increment `activeType` mod 4
- Set `dirty = true`

**View rendering** (`View()`):
- If `dirty`: filter entries by `activeType`, rebuild viewport content string, clear `dirty`
- Render header: `"Logs [TypeName]"` + scroll hint with entry count
- Render viewport

**Convenience methods**:
- `addEvent(level, format, args...)` — adds with `logTypeEvent`
- `addAudit(level, message)` — adds with `logTypeAudit`
- `addDebug(level, message)` — adds with `logTypeDebug`

### Component 5: App Integration (`internal/tui/app.go`)

Wire everything together:

1. **Remove `viewAuditLog`** from `viewKind` enum
2. **Remove `auditLogView *LogViewModel`** field from `App`
3. **Remove `handleAuditLogKey()`** function (was dead code anyway)
4. **Remove `openAuditLogView()`** function
5. **Update `logAuditEvent()`**: After writing to persistent file, also call `a.logs.addAudit(level, formatted)`
6. **Add `GUILogLineMsg` handler** in `Update()`: `a.logs.addDebug(msg.Level, msg.Message)`
7. **Update `handleLogKey()`**: Add `case "tab": a.logs.nextType()` for type cycling
8. **Update header tabs**: Remove "Audit Log" tab, rename "Event Log" tab to "Logs"
9. **Update `:audit` command**: Redirect to open Logs view with Audit type pre-selected (or remove command)
10. **Update layout()**: Remove `auditLogView.SetSize()` call

### Data Flow

```
LOCAL TUI EVENTS:
  a.logs.addEvent(logInfo, "Connected to %s", name)
    │
    └─→ entries = append(entries, {logTypeEvent, logInfo, msg, time.Now()})
        → dirty = true

AUDIT EVENTS:
  a.logAuditEvent(event)
    ├─→ audit.AuditLog.Log(event)               [persistent JSONL file]
    └─→ a.logs.addAudit(logInfo, formatted)      [in-memory]
        → dirty = true

GUI DEBUG LOGS:
  GUI: ipcLogger.Info("cliprdr: format list sent")
    ├─→ log.Printf(...)                          [stderr → /tmp/nexus-gui.log]
    └─→ ipcMgr.SendLogLine("info", "cliprdr: format list sent")
        → ipc.Server.Broadcast("log-line", {...})
        → Unix socket → TUI client.Recv()
        → p.Send(GUILogLineMsg{...})

  TUI: Update(GUILogLineMsg)
    → a.logs.addDebug(msg.Level, msg.Message)
    → dirty = true

VIEW RENDERING:
  logsModel.View()
    → if dirty: filter entries by activeType → rebuild viewport string
    → render "Logs [All/Events/Audit/Debug]" header + viewport
```

### Error Handling

All errors handled silently — no user-facing error messages for log infrastructure:

- IPC broadcast failure (GUI side) → log.Printf to file, skip IPC delivery
- IPC receive failure (TUI side) → silently drop the message
- Buffer overflow → batch evict oldest entries, no notification
- Invalid log level from IPC → treat as "info"
- Viewport rebuild failure → should not happen; viewport.SetContent is infallible

## Complexity Tracking

No constitution violations to justify — design stays within existing patterns.
