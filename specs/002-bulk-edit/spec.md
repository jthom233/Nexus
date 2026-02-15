# Feature Specification: Bulk Edit Operations

**Feature Branch**: `002-bulk-edit`
**Created**: 2026-02-14
**Status**: Draft
**Input**: Implement bulk edit operations for the Nexus TUI — bulk group assignment via `:mv`/`:move`, and batch undo so one `u` reverses an entire bulk operation.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Bulk Group Assignment (Priority: P1)

A user selects multiple connections via visual mode (V for contiguous range, v for individual toggle) and then uses `:mv <group>` or `:move <group>` to move all selected connections to a target group in one action. The operation respects the existing group management system — auto-creating the target group if it doesn't exist — and exits visual mode after completion.

**Why this priority**: Bulk group assignment is the primary missing bulk operation. Users currently must edit each connection individually to change groups, which is tedious for reorganizing large connection lists. The `ExecuteBulkMove` function already exists but isn't wired up.

**Independent Test**: Select 5 connections with V, run `:mv Production`, verify all 5 connections now have group "Production" and the change persists after restart.

**Acceptance Scenarios**:

1. **Given** 10 connections with no group, **When** the user selects 5 via visual mode and runs `:mv Staging`, **Then** all 5 connections have group "Staging", config is saved, and visual mode exits
2. **Given** visual mode with 3 connections selected, **When** the user runs `:move Production Servers`, **Then** all 3 connections are moved to the group "Production Servers" (space-aware parsing)
3. **Given** visual mode with connections selected, **When** the user runs `:mv` with no arguments, **Then** an error flash "Usage: :mv <group>" is shown and no changes are made
4. **Given** visual mode with connections selected, **When** the user runs `:mv NewGroup` where "NewGroup" doesn't exist, **Then** the group is auto-created and all connections are moved to it
5. **Given** normal mode (no visual selection), **When** the user runs `:mv connname Production`, **Then** the single-connection move behavior is preserved (backward compatible)

---

### User Story 2 - Batch Undo (Priority: P2)

When a user performs a bulk operation (move, tag, delete) on N connections, pressing `u` (undo) reverses the entire batch in one step — all N connections revert to their previous state simultaneously. Pressing `Ctrl+r` (redo) re-applies the entire batch.

**Why this priority**: Without batch undo, bulk operations are dangerous — a user who accidentally bulk-moves 20 connections must press undo 20 times to recover. Batch undo makes bulk operations safe and trustworthy.

**Independent Test**: Select 5 connections, bulk-move to a group, press `u` once, verify all 5 connections revert to their original groups in one undo step.

**Acceptance Scenarios**:

1. **Given** 5 connections in group "Dev" that were just bulk-moved to "Prod", **When** the user presses `u`, **Then** all 5 revert to group "Dev" in a single undo step
2. **Given** a batch undo was just performed, **When** the user presses `Ctrl+r`, **Then** all connections re-apply the bulk move in a single redo step
3. **Given** 3 connections that just had tag "critical" bulk-added, **When** the user presses `u`, **Then** the tag is removed from all 3 in a single undo step
4. **Given** a mix of individual and batch operations in the undo stack, **When** the user presses `u` multiple times, **Then** each press correctly handles either a single-item or batch operation based on what's on the stack

---

### User Story 3 - Bulk Tag Batch Undo Retrofit (Priority: P3)

The existing bulk tag operations (`:tag add/remove` in visual mode) currently create individual undo entries per connection. These should be retrofitted to use the new batch undo system so that bulk tagging also benefits from single-step undo.

**Why this priority**: Consistency — once batch undo exists, all bulk operations should use it. The existing bulk tag code paths need minimal changes to adopt the new pattern.

**Independent Test**: Select 5 connections, run `:tag add critical`, press `u` once, verify the tag is removed from all 5 in one step.

**Acceptance Scenarios**:

1. **Given** visual mode with 5 connections selected, **When** the user runs `:tag add urgent`, **Then** the tag is added to all 5 and a single batch undo entry is created
2. **Given** visual mode with 3 tagged connections selected, **When** the user runs `:tag remove urgent`, **Then** the tag is removed from all 3 and a single batch undo entry is created
3. **Given** the above tag removal was just undone, **When** the user presses `Ctrl+r`, **Then** the tag removal is re-applied to all 3 connections

---

### Edge Cases

- What happens when a bulk move targets the same group some connections are already in? Connections already in the target group are skipped (no-op for those items); flash message shows "Moved X of Y connections to <group>".
- What happens when visual selection contains 0 items (toggled all off)? Show error flash "No connections selected".
- What happens when the undo stack is full (50 items) and a batch operation is pushed? The oldest entry is evicted as normal — a batch counts as one entry.
- What happens when a bulk operation partially fails (e.g., one connection was deleted mid-operation)? Skip the missing connection, continue with the rest, report partial success in flash message.
- What happens when bulk-moving connections that have different source groups? Each connection's original group is captured individually in the batch undo entry, so undo correctly restores each to its original group.

## Requirements *(mandatory)*

### Functional Requirements

**Bulk Move:**

- **FR-001**: `:mv`/`:move` commands MUST detect active visual selection and operate on all selected connections when visual mode is active
- **FR-002**: In visual mode, `:mv <group>` MUST treat the entire argument as the target group name (no connection name parsing)
- **FR-003**: After a successful bulk move, visual mode MUST exit automatically
- **FR-004**: Bulk move MUST show a flash message with the count: "Moved N connections to <group>"
- **FR-005**: Bulk move MUST skip connections already in the target group and report "Moved X of Y connections to <group>"
- **FR-006**: Bulk move MUST auto-create the target group if it doesn't exist
- **FR-007**: Single-connection `:mv <conn> <group>` behavior MUST remain unchanged when visual mode is not active

**Batch Undo:**

- **FR-008**: The undo system MUST support a batch operation type that groups multiple connection changes into a single undo/redo entry
- **FR-009**: Pressing `u` on a batch entry MUST revert ALL connections in the batch to their pre-operation state
- **FR-010**: Pressing `Ctrl+r` on a batch entry MUST re-apply ALL connection changes in the batch
- **FR-011**: A batch entry MUST count as a single slot in the undo stack (max 50 entries)

**Retrofit:**

- **FR-012**: Bulk tag operations (`:tag add/remove` in visual mode) MUST use batch undo entries instead of individual entries
- **FR-013**: Bulk delete (visual mode `d`) MUST use batch undo entries instead of individual entries

### Key Entities

- **BatchOperation**: A compound undo entry containing an ordered list of individual connection changes (before/after snapshots). Replaces multiple individual undo entries for bulk actions.
- **VisualSelection**: The set of selected connection indices/IDs from visual mode. Used as input to all bulk operations.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can bulk-move any number of selected connections to a group in under 3 seconds via a single command
- **SC-002**: Any bulk operation (move, tag, delete) can be fully reversed with a single undo keystroke
- **SC-003**: Zero increase in undo stack memory usage for bulk operations on up to 100 connections (batch entry replaces N individual entries)
- **SC-004**: Application compiles with zero new warnings and all existing tests pass
- **SC-005**: All bulk operations persist across application restarts
