# Feature Specification: TUI Command & Keybinding Cleanup

**Feature Branch**: `001-command-cleanup`
**Created**: 2026-02-14
**Status**: Draft
**Input**: Clean up the Nexus TUI command and keybinding system: remove dead stubs, implement group management, ensure all commands are available from leader menu

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Remove Dead Commands and Keybindings (Priority: P1)

A user navigating the TUI should never encounter "not yet implemented" messages or commands that silently do nothing. All registered commands and keybindings must either work or not exist. Dead code (tree folding, marks, stub commands) must be removed entirely so the codebase is clean and the command registry accurately reflects available functionality.

**Why this priority**: Dead stubs create confusion and erode user trust. Cleaning them up is prerequisite to adding new functionality — we need to know the real baseline before building on it.

**Independent Test**: After cleanup, running every registered command and pressing every bound key produces either a working result or a clear error — never silence or "not yet implemented" flash messages.

**Acceptance Scenarios**:

1. **Given** the command registry, **When** a user types any registered command (e.g., `:theme`, `:recordings`), **Then** every command either executes successfully or does not exist in the registry
2. **Given** the TUI in normal mode, **When** a user presses `z` followed by any letter, **Then** nothing happens (tree folding removed)
3. **Given** the TUI in normal mode, **When** a user presses `m` followed by a letter, **Then** nothing happens (marks removed)
4. **Given** the leader menu, **When** a user opens any group, **Then** every listed action works — no "not yet implemented" messages appear
5. **Given** the motion system, **When** a user types `cc` or `c{motion}`, **Then** nothing happens (change operator removed)

---

### User Story 2 - Group Management (Priority: P2)

A user needs to organize connections into groups. They should be able to create empty groups ahead of time, delete groups that are no longer needed, and move connections between groups — all from both the command bar and the leader menu.

**Why this priority**: Groups are a core organizational concept. Users currently have no way to create or delete groups from the TUI, and moving connections between groups requires editing each connection individually.

**Independent Test**: A user can create a group via `:mkdir`, move connections into it via `:mv`, and delete it via `:rmdir` — all round-tripping through config persistence.

**Acceptance Scenarios**:

1. **Given** the command bar, **When** a user types `:mkdir production`, **Then** a group named "production" is created and persisted to config
2. **Given** a group "staging" with no connections, **When** a user types `:rmdir staging`, **Then** the group is removed from config
3. **Given** a group "staging" with connections assigned to it, **When** a user types `:rmdir staging`, **Then** the command is rejected with an error message indicating the group has active connections
4. **Given** a connection "webserver" in group "dev", **When** a user types `:mv webserver production`, **Then** the connection's group is updated to "production" and persisted
5. **Given** the leader menu Groups group, **When** a user navigates to create/delete/move actions, **Then** each action works identically to its command-bar equivalent

---

### User Story 3 - Leader Menu Completeness (Priority: P3)

A user who prefers the leader menu (space key) over command-bar should be able to access every feature from the leader menu, similar to neovim's which-key experience. Every working command should have a corresponding leader menu path.

**Why this priority**: Power users expect consistent access patterns. If a command exists, it should be reachable from both `:` commands and the leader menu.

**Independent Test**: For every registered `:` command, there exists a corresponding leader menu action that triggers the same behavior.

**Acceptance Scenarios**:

1. **Given** the leader menu, **When** a user opens the Connections group, **Then** actions for add, edit, delete, connect, and quick-connect are available
2. **Given** the leader menu, **When** a user opens the Groups group, **Then** actions for create group, delete group, move connection, and filter by group are available and functional
3. **Given** the leader menu, **When** a user opens the Sessions group, **Then** the kill action works directly — either kills the current/selected session or prompts for selection — without requiring the sessions view to be open first
4. **Given** the leader menu, **When** a user opens the View group, **Then** only implemented views are listed (no tree view option)
5. **Given** the leader menu Options group, **When** a user opens it, **Then** only implemented options are listed (no theme option)
6. **Given** the leader menu, **When** a user looks for any working `:` command, **Then** there is a corresponding leader action for: import, import-ssh, export, recent, frequent, all, pulse, log/audit, note, and field management

---

### Edge Cases

- What happens when a user tries to `:mkdir` a group name that already exists? Command reports the group already exists.
- What happens when a user tries to `:mv` a connection to a group that does not exist? Auto-create the target group.
- What happens when a user types `:rmdir` on a group with nested sub-groups (e.g., "prod/web")? Only the exact group name is matched — sub-groups are independent.
- What happens when the last connection is removed from a group via `:mv`? The group remains (groups are explicitly managed, not implicit from connections).
- What happens when a user tries to `:mv` with an ambiguous connection name? Show error listing matches, or use selected connection if no name given.
- What happens when a user tries to `:rmdir` a group that doesn't exist? Show error "group not found".

## Requirements *(mandatory)*

### Functional Requirements

**Removals:**

- **FR-001**: System MUST remove these commands from the registry: `disconnect`/`dc`, `marks`, `theme`, `settings`/`set`, `recordings`/`rec`, `health`, `vault`, `version`/`ver`, `template`/`tpl`
- **FR-002**: System MUST remove tree folding keybindings (`z` prefix: `za`, `zo`, `zc`, `zR`, `zM`) and the TreeModel code
- **FR-003**: System MUST remove the marks system entirely: `marks.go` file, mark keybindings (`m{a-z}`, `'{a-z}`, backtick, `Ctrl+o`, `Ctrl+i`), and all mark-related state in the App struct
- **FR-004**: System MUST remove the `cc`/`c{motion}` change operator from the motion engine
- **FR-005**: System MUST remove "view-tree" and "theme" options from the leader menu
- **FR-006**: System MUST remove "list-groups" and "filter-by-group" stubs from the leader Groups menu

**Group Management:**

- **FR-007**: System MUST implement `:mkdir <name>` to create a named group, persisted to config
- **FR-008**: System MUST implement `:rmdir <name>` to delete a group, only if no connections are assigned to it
- **FR-009**: System MUST implement `:mv <connection> <group>` to move a connection to a different group, updating the connection's group field and persisting
- **FR-010**: System MUST support `:mv` without a connection name to move the currently selected connection
- **FR-011**: Group operations MUST integrate with the undo system (undo move, undo group deletion)

**Leader Menu Completeness:**

- **FR-012**: Leader menu Groups group MUST include: create group, delete group, move connection to group, filter by group
- **FR-013**: Leader menu Sessions kill action MUST work directly without requiring the sessions view to be open
- **FR-014**: Leader menu MUST have paths for all working commands: add, edit, delete, connect, quick-connect, import, import-ssh, export, recent, frequent, all, pulse, log/audit, note, field, sort, filter, group, tag, favorites, sessions, help, quit
- **FR-015**: Leader menu Options group MUST only contain implemented options (keybindings only)
- **FR-016**: Leader menu View group MUST only contain implemented views (table, detail, wide, event log)

### Key Entities

- **Group**: A named organizational category for connections. Stored in config as a list of group names. Connections reference groups by name string.
- **Connection**: Existing entity — gains ability to be moved between groups via `:mv` command.
- **Command Registry**: The map of command names to handlers — must accurately reflect only available commands.
- **Leader Menu**: Hierarchical key-action menu — must provide access to all available commands.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Zero "not yet implemented" flash messages appear for any leader menu action or registered command
- **SC-002**: Command registry contains only commands with working handlers (count of stub commands = 0)
- **SC-003**: Every registered `:` command has a corresponding leader menu path
- **SC-004**: Users can create, delete, and manage groups entirely from the TUI without editing config files
- **SC-005**: Users can move connections between groups in under 5 seconds via either `:mv` or leader menu
- **SC-006**: Application compiles with zero new warnings and no references to removed code
- **SC-007**: All group operations persist across application restarts
