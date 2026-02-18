# Feature Specification: Paned Sessions with Broadcast

**Feature Branch**: `003-paned-sessions`
**Created**: 2026-02-16
**Status**: Draft
**Input**: User description: "Add paned sessions to Nexus TUI with send-to-all broadcast mode"

## Clarifications

### Session 2026-02-16

- Q: Maximum concurrent panes? → A: No hard limit — only constrained by minimum pane size.
- Q: What appears in a new pane after splitting? → A: Connection list picker — user selects a connection to open in the pane.
- Q: Keybinding prefix for pane management? → A: `Space w` (Leader > window) — extends existing leader key system.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Split Terminal into Panes (Priority: P1)

A user managing multiple servers needs to monitor or work on several connections simultaneously. They open a connection to Server A, then split the view horizontally to open a connection to Server B below it. They can continue splitting to create additional panes, each holding an independent session. They navigate between panes using vim-style keybindings.

**Why this priority**: This is the foundational capability. Without pane splitting, no other pane features are possible. A single split with two independent sessions delivers immediate value for side-by-side server work.

**Independent Test**: Can be fully tested by opening two connections, splitting the view, and verifying both sessions operate independently. Delivers the core value of simultaneous multi-session visibility.

**Acceptance Scenarios**:

1. **Given** a user has one active connection, **When** they invoke the horizontal split command (`Space w s`), **Then** the terminal divides into two panes — the original session remains in one pane and the other pane opens the connection list picker for the user to select a new connection.
2. **Given** a user has one active connection, **When** they invoke the vertical split command (`Space w v`), **Then** the terminal divides into two side-by-side panes with the new pane showing the connection list picker.
3. **Given** a user has multiple panes open, **When** they use pane navigation keybindings, **Then** focus moves to the target pane and a visual indicator shows which pane is active.
4. **Given** a user is in a pane, **When** they type commands, **Then** only the focused pane receives the input.

---

### User Story 2 - Broadcast Input to All Panes (Priority: P2)

A user needs to run the same command across multiple servers (e.g., applying a security patch, checking service status). They toggle broadcast mode, and every keystroke they type is sent to all open panes simultaneously. A clear visual indicator shows that broadcast mode is active.

**Why this priority**: Broadcast is the key differentiator that makes paned sessions a productivity multiplier, not just a display convenience. It depends on pane splitting being functional first.

**Independent Test**: Can be tested by opening two or more panes with active sessions, enabling broadcast mode, typing a command, and verifying the command appears in all panes. Delivers the high-value "run everywhere at once" workflow.

**Acceptance Scenarios**:

1. **Given** a user has multiple panes with active sessions, **When** they toggle broadcast mode on, **Then** a visual indicator appears showing broadcast is active, and all subsequent keystrokes are sent to every pane.
2. **Given** broadcast mode is active, **When** the user types a command and presses Enter, **Then** the command executes in all panes.
3. **Given** broadcast mode is active, **When** the user toggles broadcast mode off, **Then** the visual indicator disappears and input returns to the focused pane only.

---

### User Story 3 - Resize and Close Panes (Priority: P2)

A user wants to give more screen space to one pane while keeping others visible. They resize a pane larger using keybindings, pushing adjacent panes smaller. They can also close a pane when done with that session, and the remaining panes redistribute the freed space.

**Why this priority**: Resize and close are essential for a usable pane workflow — without them, users are stuck with equal-size panes they cannot dismiss, making the feature frustrating for real use.

**Independent Test**: Can be tested by creating panes, resizing them, and closing individual panes while verifying remaining panes adjust to fill the available space.

**Acceptance Scenarios**:

1. **Given** a user has multiple panes, **When** they invoke the resize keybinding, **Then** the active pane grows or shrinks in the specified direction and adjacent panes adjust accordingly.
2. **Given** a user has multiple panes, **When** they close one pane, **Then** the pane is removed and adjacent panes expand to fill the freed space.
3. **Given** a user has only one pane remaining, **When** they close it, **Then** they return to the connection list view.

---

### User Story 4 - Pane Layouts and Presets (Priority: P3)

A user frequently works with the same set of servers in the same arrangement. They can select from preset layouts (e.g., 2x2 grid, 3-column, main+sidebar) and can assign connections to panes from the connection list.

**Why this priority**: Quality-of-life enhancement that reduces repetitive setup. The core split/broadcast functionality must work first. This builds on top of it for power users.

**Independent Test**: Can be tested by selecting a layout preset, verifying the correct number of panes appears in the expected arrangement, and assigning connections to each.

**Acceptance Scenarios**:

1. **Given** a user is in the connection list view, **When** they select a layout preset, **Then** the view splits into the preset arrangement with empty panes ready for connections.
2. **Given** a user has a layout with empty panes, **When** they assign a connection to a pane, **Then** that pane initiates the chosen connection.

---

### Edge Cases

- What happens when the terminal is too small to split further? The system should refuse the split and display a message indicating minimum pane size has been reached.
- What happens when a connection in a pane disconnects? The pane should display the disconnection status and allow the user to reconnect or close the pane.
- What happens when broadcast mode is on and one pane has no active session? That pane should be skipped for broadcast input (no error, input just goes to panes with active sessions).
- How does the pane system interact with the existing view stack? Pane management operates within the session view — the connection list, command bar, and other views remain accessible and overlay the paned layout when invoked.
- What happens when the terminal window is resized externally? All panes should proportionally adjust to the new terminal dimensions.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST allow users to split the active pane horizontally (top/bottom) or vertically (left/right), creating two independent panes from one. The new pane MUST display the connection list picker for the user to select a connection.
- **FR-002**: System MUST support pane navigation using vim-directional keybindings to move focus between panes.
- **FR-003**: System MUST visually indicate which pane currently has focus (e.g., highlighted border, distinct border color).
- **FR-004**: System MUST deliver keystrokes only to the focused pane when broadcast mode is off.
- **FR-005**: System MUST provide a broadcast mode toggle that, when active, sends all keystrokes to every pane with an active session.
- **FR-006**: System MUST display a persistent visual indicator when broadcast mode is active.
- **FR-007**: System MUST allow users to resize panes using keybindings, adjusting adjacent panes proportionally.
- **FR-008**: System MUST allow users to close individual panes, redistributing space to adjacent panes.
- **FR-009**: System MUST support all connection types (SSH, RDP, VNC, Telnet) within any pane.
- **FR-010**: System MUST enforce a minimum pane size and refuse further splits when the minimum would be violated. There is no hard upper limit on pane count — the minimum size constraint is the sole limiter.
- **FR-011**: System MUST handle pane disconnections gracefully, showing status in the affected pane without disrupting other panes.
- **FR-012**: System MUST integrate pane keybindings under the `Space w` (Leader > window) prefix, extending the existing leader key system without conflicting with existing Normal, Insert, Visual, or Command mode bindings.
- **FR-013**: System MUST provide preset layout options (e.g., even horizontal, even vertical, grid) accessible via command bar or keybindings.
- **FR-014**: System MUST proportionally resize all panes when the terminal window dimensions change.

### Key Entities

- **Pane**: A rectangular region of the terminal displaying a single connection session. Has dimensions, position, focus state, and an optional active connection.
- **PaneLayout**: A tree structure of splits (horizontal/vertical) that defines how panes are arranged. Each leaf node is a Pane.
- **BroadcastGroup**: All panes with active sessions. Broadcast mode is all-or-nothing — when enabled, input goes to every pane with an active connection. Granular per-pane targeting is out of scope for the initial release.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can split, navigate, and manage panes entirely through keyboard commands with no mouse required.
- **SC-002**: A broadcast command typed once executes across all targeted panes within the same input cycle (no perceptible delay between panes).
- **SC-003**: Users can set up a 4-pane layout with active connections in under 30 seconds.
- **SC-004**: Pane operations (split, close, resize, navigate) complete with no visible rendering delay.
- **SC-005**: Terminal resize events redistribute pane space proportionally with no content loss or rendering artifacts.
- **SC-006**: All existing single-session workflows continue to function unchanged when only one pane is open.

## Assumptions

- Pane keybindings use the `Space w` prefix (Leader > window), extending the existing leader key system. This avoids Ctrl-w conflicts with terminal emulators that intercept it.
- RDP and VNC panes render their graphical content via the existing Ebiten GUI subsystem — pane splitting in the TUI controls layout, but graphical sessions may launch in separate GUI windows rather than inline terminal panes.
- The minimum pane size is determined by the minimum usable dimensions for displaying a connection session (assumed ~20 columns x 5 rows).
- Broadcast mode applies to the Insert mode (active typing) context — in Normal mode, pane navigation and management keys are not broadcast.
