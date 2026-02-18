# Research: Log Aggregation in TUI

**Feature Branch**: `005-log-aggregation`
**Date**: 2026-02-18

## Decision 1: Log Message Delivery Mechanism

**Decision**: Add a new `MsgLogLine` IPC message type. GUI broadcasts structured log lines over the existing Unix socket. TUI receives them in the existing `client.Recv()` loop inside `RDPLauncher.Launch()` and delivers them as a new `GUILogLineMsg` tea.Msg type to the Bubbletea program.

**Rationale**: The IPC infrastructure already exists and handles all GUI→TUI communication. Adding a new message type follows the established pattern (`MsgTabOpened`, `MsgTabClosed`, etc.) with minimal new code. The `PaneOutputMsg` precedent shows that streaming data from background goroutines to the TUI model via tea.Msg is the idiomatic Bubbletea pattern.

**Alternatives considered**:
- File tailing (`/tmp/nexus-gui.log`): Would require polling, text parsing, and has no structured severity. Rejected — fragile and doesn't leverage existing IPC.
- Custom log writer with channel: Would bypass IPC and require a new communication path. Rejected — unnecessary complexity when IPC works.

## Decision 2: Unified Logs View Architecture

**Decision**: Replace both `logModel` (`viewLog`) and `LogViewModel` (`viewAuditLog`) with a single new `logsModel` that stores all log entries (events, audit, debug) in a unified `[]logEntry` slice with a `logType` field on each entry. Tab cycles through type filters. The existing `viewLog` constant is reused; `viewAuditLog` is removed.

**Rationale**: The two existing models are structurally identical (viewport + entries + render). Unifying them reduces code duplication, eliminates the dead-code `handleAuditLogKey()` bug, and delivers the user-requested single Logs view. The audit log's tabular format is replaced by the `logModel`-style timestamped line format — the persistent JSONL file remains unchanged for external tooling.

**Alternatives considered**:
- Keep both views, add a third for debug: Would contradict the clarification (single unified view) and increase complexity. Rejected.
- Embed `LogViewModel` inside a wrapper: The tabular audit format is too different from the line-based event format. A unified entry model is cleaner. Rejected.

## Decision 3: IPC Receive Loop Enhancement

**Decision**: Modify the `RDPLauncher.Launch()` blocking receive loop to handle `MsgLogLine` messages without terminating the loop. Log lines produce a `GUILogLineMsg` tea.Msg delivered via `p.Send()` (the Bubbletea program reference), while the loop continues waiting for `MsgTabClosed`/`MsgTabError`.

**Rationale**: The current loop only returns (terminates) on tab-closed or tab-error. Log lines must be delivered without ending the connection session. The `PaneOutputMsg` pattern in `internal/tui/panes.go` shows that background goroutines can call `p.Send(msg)` to inject messages into the Bubbletea event loop while continuing to run. The launcher needs a reference to `*tea.Program` (passed via the `Launch()` call or a field on `RDPLauncher`).

**Alternatives considered**:
- Separate goroutine for log reception: Would require a second IPC client or splitting the socket, adding complexity. Rejected.
- Return log lines as part of `LaunchFinishedMsg`: Would only deliver logs at session end — no live streaming. Rejected.

## Decision 4: Entry Buffer Cap Strategy

**Decision**: Cap the unified `[]logEntry` slice at 10,000 entries. When the cap is exceeded, discard the oldest 1,000 entries at once (batch eviction) rather than evicting one-by-one. Rebuild the viewport content string only when the Logs view is actively displayed, not on every `add()`.

**Rationale**: The current `logModel.updateContent()` does a full O(n) rebuild on every entry addition. With streaming debug logs, this becomes expensive. Batch eviction amortizes the cost of slice compaction. Lazy content rebuild (only in `View()` or on explicit refresh) eliminates per-entry viewport rebuilds for log entries arriving while the user is in a different view.

**Alternatives considered**:
- Ring buffer: More complex data structure for marginal benefit over slice + batch eviction. Rejected — Go slice append + copy is fast enough for 10K entries.
- No cap: Memory could grow unbounded in long sessions. Rejected per FR-007.

## Decision 5: Audit Event Integration

**Decision**: When the TUI calls `a.logAuditEvent()`, it also adds a `logEntry` with `logType=logTypeAudit` to the unified log model. The persistent JSONL audit log file continues to be written independently. The `logsModel` does NOT read from the audit file — audit entries arrive via the same in-memory path as events.

**Rationale**: This avoids file I/O in the display path and keeps the unified model purely in-memory. The existing `openAuditLogView()` pattern of querying 200 entries from disk is replaced by filtering the in-memory entries by type. Historical audit data (from before the current session) is not shown in the Logs view — this is acceptable because the current `logModel` is also session-scoped.

**Alternatives considered**:
- Query audit file on "Audit" tab switch: Would introduce file I/O on every tab cycle and mix historical data with session-scoped events. Rejected.
- Stream audit events via IPC: Audit events are TUI-local — no IPC needed. Rejected as unnecessary complexity.

## Decision 6: GUI-Side Log Wrapper

**Decision**: Create a thin `ipcLogger` in `internal/gui/` that wraps the standard `log` package. It writes to both stderr (for `/tmp/nexus-gui.log` fallback) and broadcasts structured `MsgLogLine` messages via `IPCManager.SendLogLine()`. Replace `log.Printf` calls in `clipboard.go`, `rdp.go`, etc. with `ipcLogger.Info/Warn/Error()`.

**Rationale**: The GUI currently uses `log.Printf()` directly, which only writes to stderr (redirected to the log file). A wrapper that also broadcasts over IPC is the minimal change — no need to restructure the GUI's logging philosophy. The dual-write ensures the log file continues working for external debugging even if IPC is unavailable.

**Alternatives considered**:
- Replace `log` with a structured logging library (zerolog, slog): Over-engineering for this use case. The GUI has ~15 log sites. Rejected.
- Only IPC, drop the log file: Loses the external debugging fallback. Rejected per spec assumptions.
