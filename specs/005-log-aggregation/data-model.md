# Data Model: Log Aggregation in TUI

**Feature Branch**: `005-log-aggregation`
**Date**: 2026-02-18

## Entities

### LogEntry (unified)

Represents a single log message from any source, displayed in the Logs view.

| Field | Type | Description |
|-------|------|-------------|
| time | timestamp | When the entry was generated (local clock) |
| logType | enum | Source category: events, audit, debug |
| level | enum | Severity: info, warn, error |
| message | string | Human-readable log message text |

**Log types**:
- `logTypeEvent` (0) — TUI-level events (connection started, session ended, config changes)
- `logTypeAudit` (1) — Persistent audit trail entries (connect, disconnect, error, credential_use, config_change)
- `logTypeDebug` (2) — GUI subprocess messages (clipboard protocol, RDP internals)

**Severity levels** (reuse existing `logLevel` enum):
- `logInfo` (0) — Informational
- `logWarn` (1) — Warning
- `logError` (2) — Error

**Storage**: In-memory slice in `logsModel`, capped at 10,000 entries. Oldest 1,000 discarded in batch when cap exceeded. Session-scoped — not persisted (audit entries are separately persisted to the existing JSONL file).

### LogType (filter)

Represents a named filter for the Logs view.

| Value | Label | Matches |
|-------|-------|---------|
| 0 | "All" | All entries regardless of type |
| 1 | "Events" | Only entries with logType=logTypeEvent |
| 2 | "Audit" | Only entries with logType=logTypeAudit |
| 3 | "Debug" | Only entries with logType=logTypeDebug |

**Cycling order**: All → Events → Audit → Debug → All (wrapping)

### GUILogLineMsg (tea.Msg)

Delivered from the IPC receive loop to the Bubbletea program when the GUI subprocess sends a structured log message.

| Field | Type | Description |
|-------|------|-------------|
| Level | string | "info", "warn", or "error" |
| Message | string | Log message text |
| Timestamp | timestamp | When the GUI generated the message |

### LogLineEvent (IPC payload)

Wire format for log messages sent over the IPC socket from GUI to TUI.

| Field | JSON key | Type | Description |
|-------|----------|------|-------------|
| Level | "level" | string | "info", "warn", "error" |
| Message | "message" | string | Log message text |
| Timestamp | "timestamp" | ISO 8601 | When generated |

**IPC message type**: `"log-line"` (new constant `MsgLogLine` in protocol.go)

## Data Flows

### TUI Event → Logs View
```
a.log.info("message")  →  logsModel.add(logTypeEvent, logInfo, "message")
                            →  entries = append(entries, logEntry{...})
                            →  mark dirty (defer viewport rebuild to View())
```

### Audit Event → Logs View + Persistent File
```
a.logAuditEvent(event)  →  audit.AuditLog.Log(event)           [JSONL file]
                         →  logsModel.add(logTypeAudit, level, formatted)  [in-memory]
```

### GUI Debug Log → IPC → Logs View
```
GUI: ipcLogger.Info("message")
  → log.Printf("message")                      [stderr → /tmp/nexus-gui.log]
  → ipcMgr.SendLogLine("info", "message")      [IPC broadcast]
  → ipc.Server.Broadcast("log-line", payload)   [socket write]

TUI: client.Recv() in Launch() goroutine
  → case "log-line": p.Send(GUILogLineMsg{...}) [inject into Bubbletea]

Bubbletea: Update(GUILogLineMsg)
  → logsModel.add(logTypeDebug, level, message) [in-memory]
```

### View Rendering (lazy rebuild)
```
logsModel.View() called by Bubbletea
  → if dirty: filterEntries(activeType) → rebuild viewport content
  → render header "Logs [ActiveType]" + viewport
```

## Constraints

- Maximum 10,000 entries in memory. Batch eviction of oldest 1,000 when exceeded.
- Audit entries in the Logs view are session-scoped. Historical audit data (from previous sessions) is not loaded into the Logs view.
- GUI log messages arriving after the IPC receive loop terminates (post tab-close) are silently dropped.
- The persistent audit JSONL file is unaffected — writes continue independently of the Logs view.
