# Developer Quickstart: Log Aggregation

**Feature Branch**: `005-log-aggregation`

## Prerequisites

- Go 1.21+
- Existing Nexus codebase with IPC infrastructure (`internal/ipc/`)
- Understanding of Bubbletea `Update()`/`View()` cycle

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│  TUI Process (internal/tui/)                            │
│                                                         │
│  ┌─────────────────────────────────────────────────┐    │
│  │  logsModel (replaces logModel + LogViewModel)   │    │
│  │  ┌─────────────────────────────────────┐        │    │
│  │  │ entries []logEntry (max 10,000)     │        │    │
│  │  │  - logTypeEvent  (TUI events)      │        │    │
│  │  │  - logTypeAudit  (audit trail)     │        │    │
│  │  │  - logTypeDebug  (GUI subprocess)  │        │    │
│  │  └─────────────────────────────────────┘        │    │
│  │  activeType: All | Events | Audit | Debug       │    │
│  │  viewport: scrollable filtered view             │    │
│  └─────────────────────────────────────────────────┘    │
│       ▲              ▲              ▲                    │
│       │              │              │                    │
│  a.log.info()   a.logAudit()   GUILogLineMsg            │
│  (direct call)  (direct call)  (tea.Msg from IPC)       │
│                                     ▲                    │
└─────────────────────────────────────│────────────────────┘
                                      │ p.Send()
                              ┌───────┴────────┐
                              │ RDPLauncher    │
                              │ Launch() loop  │
                              │ client.Recv()  │
                              └───────┬────────┘
                                      │ Unix socket
┌─────────────────────────────────────│────────────────────┐
│  GUI Process (internal/gui/)        │                    │
│                                     ▼                    │
│  ┌──────────────────────────────────────────┐            │
│  │  ipcLogger                               │            │
│  │  ├─→ log.Printf()  → /tmp/nexus-gui.log │            │
│  │  └─→ ipcMgr.SendLogLine() → IPC socket  │            │
│  └──────────────────────────────────────────┘            │
│       ▲                                                  │
│  clipboard.go, rdp.go, etc.                              │
└──────────────────────────────────────────────────────────┘
```

## Key Files to Modify

| File | Change |
|------|--------|
| `internal/tui/log.go` | Replace `logModel` with `logsModel` — add `logType` to entries, tab cycling, type filtering, buffer cap, lazy viewport rebuild |
| `internal/tui/logview.go` | **DELETE** — functionality absorbed into `logsModel` |
| `internal/tui/app.go` | Remove `viewAuditLog`, `auditLogView`, `handleAuditLogKey()`. Update `viewLog` key handler for Tab cycling. Update `openAuditLogView()` → remove. Wire `GUILogLineMsg` in `Update()`. |
| `internal/ipc/protocol.go` | Add `MsgLogLine = "log-line"` constant and `LogLineEvent` struct |
| `internal/launcher/rdp.go` | Add `MsgLogLine` case in `Launch()` receive loop. Add `*tea.Program` reference for `p.Send()` |
| `internal/gui/ipc.go` | Add `SendLogLine()` broadcast helper |
| `internal/gui/logger.go` | **NEW** — `ipcLogger` wrapper: dual-write to stderr + IPC |
| `internal/gui/clipboard.go` | Replace `log.Printf` with `ipcLogger` calls |
| `internal/gui/rdp.go` | Replace `log.Printf` with `ipcLogger` calls |

## Build & Test

```bash
# Build
go build ./...

# Vet
go vet ./...

# Manual test: connect to RDP, copy image, open Logs (Ctrl+L), verify debug messages appear
# Manual test: Tab through All → Events → Audit → Debug, verify filtering
# Manual test: verify header shows "Logs [All]", "Logs [Events]", etc.
```

## Key Patterns

**Adding a log entry (TUI-side)**:
```go
// Event log (direct call in Update())
a.logs.addEvent(logInfo, "Connected to %s", connName)

// Audit log (dual path: persistent file + in-memory)
a.logAuditEvent(event)  // writes to file AND calls a.logs.addAudit(...)
```

**Receiving GUI log (tea.Msg path)**:
```go
// In App.Update():
case GUILogLineMsg:
    a.logs.addDebug(msg.Level, msg.Message)
```

**Tab cycling (in handleLogKey)**:
```go
case "tab":
    a.logs.nextType()  // cycles activeType, marks viewport dirty
```
