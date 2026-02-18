# Feature Specification: Log Aggregation in TUI Event Log

**Feature Branch**: `005-log-aggregation`
**Created**: 2026-02-17
**Status**: Draft
**Input**: User description: "Aggregate GUI subprocess logs (e.g., clipboard debug output from /tmp/nexus-gui.log) into the TUI's event log panel, with tab-cycling to view different log categories (audit events, clipboard/protocol debug, etc.) so users don't have to check separate log files."

## Clarifications

### Session 2026-02-18

- Q: Should the "Audit" tab in the Event Log coexist with the separate Audit Log view, or replace it? → A: Single unified "Logs" view replaces both the existing Event Log and the separate Audit Log view. Tab-cycle between log types (All, Events, Audit, Debug) within that one view.
- Q: How should GUI subprocess log messages be structured for display in the Logs view? → A: Structured IPC messages — GUI sends level + message fields over the existing Unix socket. The log file (`/tmp/nexus-gui.log`) continues to receive unstructured text as a fallback for external debugging.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - View All Logs in One Place (Priority: P1)

A user is troubleshooting an RDP clipboard issue. Instead of opening a separate terminal and tailing `/tmp/nexus-gui.log`, they open the Logs view (`Ctrl+L`) and see all log entries — TUI-level events (connection started, session ended), audit trail entries, and GUI subprocess messages (clipboard format changes, protocol errors) — interleaved chronologically in a single scrollable view.

**Why this priority**: This is the core value — eliminating the need to check a separate log file or navigate to a different view. Users currently have no visibility into GUI subprocess behavior from within the TUI, and audit data lives in a separate view. One unified Logs view solves both.

**Independent Test**: Can be fully tested by connecting to an RDP session, performing a clipboard operation (copy/paste), then opening the Logs view and verifying that GUI subprocess messages (clipboard format list, image detection) appear alongside TUI events and audit entries.

**Acceptance Scenarios**:

1. **Given** an active RDP session with clipboard activity, **When** the user opens the Logs view (`Ctrl+L`), **Then** they see TUI events, audit entries, and GUI subprocess log messages in chronological order.
2. **Given** the GUI subprocess emits a log message (e.g., clipboard format change), **When** the Logs view is already open, **Then** the new message appears automatically (live tail behavior).
3. **Given** no GUI subprocess is running, **When** the user opens the Logs view, **Then** only TUI events and audit entries appear — no errors or empty categories.

---

### User Story 2 - Filter Logs by Type (Priority: P2)

A user wants to focus on just clipboard-related messages without the noise of connection events. In the Logs view, they press Tab to cycle through log types: "All", "Events" (TUI-level), "Audit" (persistent audit trail), and "Debug" (GUI subprocess output). Each type shows only the relevant entries.

**Why this priority**: Once all logs are aggregated (US1), the volume can be overwhelming. Filtering by type lets users focus on what matters for their current task — e.g., only debug logs when troubleshooting clipboard issues, only audit entries when reviewing session history.

**Independent Test**: Can be tested by generating activity across multiple types (connect to a session, perform clipboard operations), then cycling through types and verifying each shows only its entries.

**Acceptance Scenarios**:

1. **Given** the Logs view is open with mixed log entries, **When** the user presses Tab, **Then** the view cycles to the next log type and shows only entries matching that type.
2. **Given** the user is viewing the "Debug" type, **When** they press Tab again, **Then** it cycles back to "All" (wrapping around).
3. **Given** a log type has no entries, **When** the user switches to that type, **Then** the view shows an empty state message (e.g., "No debug messages yet") rather than appearing broken.
4. **Given** the user is viewing a filtered type, **When** a new log entry arrives matching that type, **Then** it appears in the view in real time.

---

### User Story 3 - Active Log Type Indicator (Priority: P3)

A user glances at the Logs view and immediately knows which type they're viewing. The current type name is displayed in the panel header (e.g., "Logs [All]", "Logs [Debug]"). The indicator updates instantly when cycling.

**Why this priority**: Without a visible indicator, users lose context about which filter is active, especially after stepping away and returning. This is a small UX polish that prevents confusion.

**Independent Test**: Can be tested by opening the Logs view, noting the type indicator, pressing Tab, and verifying the indicator updates to reflect the new type.

**Acceptance Scenarios**:

1. **Given** the Logs view is open, **When** the user looks at the panel header, **Then** the current log type name is visible (e.g., "[All]", "[Events]", "[Audit]", "[Debug]").
2. **Given** the user presses Tab to change type, **When** the view updates, **Then** the header indicator changes to match the new type.

---

### Edge Cases

- What happens when the GUI subprocess crashes and restarts? The TUI should continue receiving log messages from the new GUI process without manual intervention; historical messages from the crashed process are lost (acceptable — they were ephemeral).
- What happens when the GUI subprocess produces a very high volume of log messages (e.g., rapid clipboard polling)? The log buffer should cap at a reasonable maximum entry count, discarding the oldest entries first to prevent memory growth.
- What happens when the TUI starts before the GUI subprocess? The Logs view shows only TUI events and audit entries until the GUI connects and starts sending messages.
- What happens when Tab is pressed outside the Logs view? Tab should retain its existing behavior in other views (no change to non-log-view keybindings).
- What happens when the user opens the Logs view for the first time in a session? It should show all accumulated messages from the session start, not just messages generated after opening the view.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST display GUI subprocess log messages (currently written to `/tmp/nexus-gui.log`) within the unified Logs view.
- **FR-002**: System MUST interleave all log sources (TUI events, audit trail, GUI subprocess) in chronological order within the Logs view.
- **FR-003**: System MUST support four log types: "All" (everything), "Events" (TUI-level info/warn/error), "Audit" (persistent audit trail events), and "Debug" (GUI subprocess output).
- **FR-004**: Users MUST be able to cycle through log types by pressing Tab while in the Logs view.
- **FR-005**: System MUST display the currently active log type name in the Logs view panel header.
- **FR-006**: System MUST update the Logs view in real time as new messages arrive, regardless of which log type is active.
- **FR-007**: System MUST cap the in-memory log buffer at a maximum number of entries to prevent unbounded memory growth. Oldest entries are discarded first.
- **FR-008**: System MUST preserve existing log behavior when no GUI subprocess is running — TUI events and audit entries display as they do today.
- **FR-009**: System MUST NOT change the behavior of the Tab key outside of the Logs view.
- **FR-010**: The GUI subprocess log forwarding MUST NOT interfere with existing IPC message types (open-tab, close-tab, focus-tab, tab-opened, tab-closed, tab-error, gui-ready).
- **FR-012**: GUI subprocess MUST send log messages as structured data (severity level and message text) over the existing IPC channel, not as raw unstructured text.
- **FR-011**: The unified Logs view MUST replace both the existing Event Log view and the separate Audit Log view. There should be a single "Logs" entry point, not two separate views.

### Key Entities

- **LogEntry**: A single log message with a timestamp, log type (events/audit/debug), severity level (info/warn/error), and message text. All log sources produce entries in this unified format for display and filtering. GUI subprocess entries arrive as structured IPC messages with explicit level and message fields.
- **LogType**: A named filter applied to the Logs view. Each type matches a subset of log entries by their source. "All" is the default and shows every entry.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can see GUI subprocess log messages in the Logs view without opening any external files or terminals.
- **SC-002**: Users can switch between log types in under 1 second (single keypress, instant visual update).
- **SC-003**: The Logs view displays the active log type name at all times when the view is open.
- **SC-004**: Existing log functionality works identically when no GUI subprocess is running — no regression.
- **SC-005**: The in-memory log buffer handles at least 10,000 entries without noticeable performance degradation in the TUI.
- **SC-006**: Users no longer need to navigate to a separate Audit Log view — all audit data is accessible within the unified Logs view under the "Audit" type.

## Assumptions

- The existing IPC channel (Unix domain socket with newline-delimited JSON envelopes) has sufficient bandwidth for log message forwarding. GUI subprocess log volume is low (tens of messages per minute at most during active clipboard operations).
- The maximum in-memory log buffer size of 10,000 entries is sufficient for typical sessions. Entries beyond this limit are silently discarded (oldest first).
- The "Audit" log type in the Logs view reads from the same persistent audit log source that the existing Audit Log view uses, but displays entries in the unified log format rather than the tabular audit format.
- Tab is not currently bound to any action within the Event Log view, so using it for type cycling introduces no keybinding conflict.
- The GUI subprocess log file (`/tmp/nexus-gui.log`) may continue to exist for external debugging purposes, but is no longer the primary way users access these logs.
- The existing separate Audit Log view and its keybinding are removed as part of this feature. The `Ctrl+L` keybinding opens the unified Logs view.
