# Feature Specification: Picker-Style Multi-Select

**Feature Branch**: `006-picker-multiselect`
**Created**: 2026-02-18
**Status**: Draft
**Input**: User description: "Replace vim-style Visual mode selection with picker-style multi-select. Tab toggles current item selected/deselected, Shift-Tab deselects, Ctrl+A toggles select all. Remove group cycling from Tab key (users use leader g f). Keep V visual mode for range selection with Ctrl+D/Ctrl+U. Selection is a toggled state on items in Normal mode. Operations (d, y, :tag, :move, :export) work on selected items or fall back to current cursor item if none selected."

## Clarifications

### Session 2026-02-18

- Q: How does Esc behave when in Visual mode with selected items? → A: Two-stage Esc: first Esc exits Visual mode (keeps selection intact), second Esc in Normal mode clears all selections.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Toggle-Select Individual Items (Priority: P1)

A user is looking at their connection list and wants to select specific connections for a bulk operation. They navigate with `j`/`k` to each connection they want and press Tab to mark it as selected. A visual indicator (e.g., a checkmark or highlight) appears next to each selected item. They can continue navigating freely — moving the cursor does not change the selection. They press Tab again on a selected item to deselect it. They can also press Shift-Tab to deselect without risk of accidentally toggling on.

**Why this priority**: This is the core interaction — selecting items is the foundation for all bulk operations. Without it, nothing else works. The picker-style Tab toggle is the standard pattern in tools like Snacks.nvim and MiniPick.

**Independent Test**: Can be fully tested by navigating a connection list with 5+ items, pressing Tab on 3 non-contiguous items, verifying each shows a selected indicator, then pressing Tab on one of them again to deselect it.

**Acceptance Scenarios**:

1. **Given** a connection list with multiple items, **When** the user presses Tab on an unselected item, **Then** that item becomes visually marked as selected and the cursor stays on that item.
2. **Given** a selected item under the cursor, **When** the user presses Tab, **Then** that item becomes deselected and the visual indicator is removed.
3. **Given** one or more selected items, **When** the user navigates with `j`/`k`, **Then** the selection does not change — only the cursor moves.
4. **Given** selected items in the list, **When** the user presses Shift-Tab on any item, **Then** that item is deselected (Shift-Tab always deselects, never selects).

---

### User Story 2 - Bulk Operations on Selected Items (Priority: P2)

A user has selected 4 connections using Tab. They press `d` to delete them and see a confirmation prompt listing all 4. After confirming, all 4 are removed. Alternatively, they could use `:tag add production` to tag all 4, or `y` to copy their connection commands, or `:move servers` to move them to a group.

When no items are selected, these operations fall back to the current cursor item — behaving exactly as they do today.

**Why this priority**: Selection is only useful if operations act on it. This story ensures that all existing bulk operations work with the new toggle-based selection, and that single-item operations remain unchanged when nothing is selected.

**Independent Test**: Can be tested by selecting 3 items with Tab, running `:tag add test`, verifying all 3 get tagged. Then deselect all, run `:tag add test2` on a single cursor item, verify only that one item gets tagged.

**Acceptance Scenarios**:

1. **Given** 3 items selected with Tab, **When** the user presses `d`, **Then** a confirmation prompt shows all 3 items and deletes them upon confirmation.
2. **Given** 3 items selected, **When** the user runs `:tag add <tag>`, **Then** the tag is added to all 3 selected items.
3. **Given** 3 items selected, **When** the user presses `y`, **Then** the connection commands for all 3 are copied to the clipboard.
4. **Given** no items selected, **When** the user presses `d`, **Then** only the current cursor item is affected (existing single-item behavior preserved).
5. **Given** items were selected and a bulk operation completes, **When** the operation finishes, **Then** all selections are cleared (fresh state).

---

### User Story 3 - Select All and Visual Mode Range Select (Priority: P3)

A user wants to quickly select every connection in the list. They press Ctrl+A and all items become selected. They press Ctrl+A again and all items become deselected.

Separately, for contiguous range selection, a user presses `V` to enter Visual mode. In Visual mode, `j`/`k`/`Ctrl+D`/`Ctrl+U` extend the selection range, making it easy to select large blocks of items. Items selected via Visual mode are added to the same selection set as Tab-toggled items. When the user exits Visual mode (Esc), the range-selected items remain in the selection set — Visual mode is just a fast way to add items to the selection.

**Why this priority**: Select-all and range-select are convenience features that build on the core toggle-select. They're less critical than individual selection and bulk operations but complete the selection UX for power users.

**Independent Test**: Can be tested by pressing Ctrl+A on a list of 10 items, verifying all 10 are selected. Also: press V, then Ctrl+D to select a half-page range, press Esc to exit Visual mode, verify the range-selected items stay selected and can be operated on with `d`/`y`/etc.

**Acceptance Scenarios**:

1. **Given** no items are selected, **When** the user presses Ctrl+A, **Then** all items in the list become selected.
2. **Given** all items are selected, **When** the user presses Ctrl+A, **Then** all items become deselected.
3. **Given** some (but not all) items are selected, **When** the user presses Ctrl+A, **Then** all items become deselected (clear partial selection first).
4. **Given** normal mode, **When** the user presses V, **Then** Visual mode activates and the cursor row is added to the selection.
5. **Given** Visual mode is active, **When** the user moves with `j`/`k`/`Ctrl+D`/`Ctrl+U`, **Then** all rows in the range from anchor to cursor are added to the selection set.
6. **Given** Visual mode is active with a range selected, **When** the user presses Esc, **Then** Visual mode exits but the selected items remain selected (they don't disappear).

---

### Edge Cases

- What happens when Tab is pressed on an empty list? Nothing — no selection change, no error.
- What happens when selected items are filtered out (e.g., user searches/filters the list)? Selected items that are no longer visible retain their selection state. If they reappear when the filter is cleared, they are still selected.
- What happens when a selected item is deleted by another operation? The selection state for that item is removed along with the item.
- What happens when Shift-Tab is pressed on an already-deselected item? Nothing — Shift-Tab is idempotent on deselected items.
- What happens when the user presses Esc with items selected but not in Visual mode? Esc clears all selections (deselects everything), consistent with "cancel" semantics.
- What happens to the group cycling that Tab previously did? Group cycling is removed from Tab. Users access group filtering via the existing leader key sequence (leader `g f`).
- What happens when a user Tab-selects some items, then enters Visual mode to add more? Both selection methods add to the same selection set. The Tab-selected items are preserved when entering Visual mode, and the range-selected items are added alongside them.
- What happens when Visual mode range overlaps with already Tab-selected items? No conflict — items are in a set. They can't be "double selected."

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Tab key MUST toggle the current cursor item between selected and deselected states in the connection list view (Normal mode).
- **FR-002**: Shift-Tab key MUST deselect the current cursor item (deselect only, never select).
- **FR-003**: Ctrl+A MUST toggle select-all: if any items are selected, deselect all; if none are selected, select all visible items.
- **FR-004**: Navigating with `j`/`k`/`g`/`G` and other movement keys in Normal mode MUST NOT change the selection state of any items.
- **FR-005**: Selected items MUST be visually distinguished from unselected items (e.g., checkmark, highlight, or color change).
- **FR-006**: The status bar MUST display the count of currently selected items when at least one item is selected.
- **FR-007**: Bulk operations (`d`, `y`, `:tag add/remove`, `:move`, `:export`) MUST operate on all selected items when one or more items are selected.
- **FR-008**: When no items are selected, bulk-capable operations MUST fall back to operating on the current cursor item only, preserving existing single-item behavior.
- **FR-009**: After a bulk operation completes (or is cancelled), all selections MUST be cleared.
- **FR-010**: Esc key in Normal mode MUST clear all selections when items are selected (first Esc clears selection; subsequent Esc triggers other behavior like closing views). In Visual mode, Esc exits Visual mode but preserves the selection — a second Esc in Normal mode then clears it.
- **FR-011**: Visual mode (`V`) MUST be retained for contiguous range selection. Items selected via Visual mode MUST be added to the same selection set used by Tab-toggle.
- **FR-012**: When exiting Visual mode (Esc), range-selected items MUST remain in the selection set — they are not deselected on exit.
- **FR-013**: Tab key MUST no longer cycle through connection groups. Group filtering is accessed via existing leader key sequences only.
- **FR-014**: Selection state MUST be preserved when the list is scrolled or the cursor moves off-screen.
- **FR-015**: Visual mode movement keys (`j`/`k`/`Ctrl+D`/`Ctrl+U`/`G`) MUST extend the selection range and add those items to the selection set.

### Key Entities

- **SelectionSet**: A set of selected item identifiers. Items can be added individually (Tab), removed individually (Tab or Shift-Tab), added in ranges (Visual mode), or bulk-toggled (Ctrl+A). The set persists across Normal mode navigation and is cleared after bulk operations or Esc.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can select 5 non-contiguous items in a list of 20 in under 10 seconds (navigate + Tab per item).
- **SC-002**: All existing bulk operations (delete, yank, tag, move, export) work identically on multi-selected items as they did in Visual mode.
- **SC-003**: Single-item operations work unchanged when no items are selected — zero regression.
- **SC-004**: Users can select all items with a single keypress (Ctrl+A) and deselect all with another press.
- **SC-005**: The selected item count is always visible in the status area when items are selected.
- **SC-006**: Users can combine Tab-select and Visual mode range-select to build complex selections efficiently.

## Assumptions

- The leader key sequence for group filtering (leader `g f`) already exists and works. This feature does not need to create a new group filtering mechanism — it only removes the Tab-based group cycling shortcut.
- The connection list is the primary (and currently only) view where multi-select applies. Other list views (e.g., session list) are out of scope for this feature.
- The visual indicator for selected items should be consistent with the existing application theme and styling conventions.
- Undo support for bulk operations (already implemented for tag and move) continues to work unchanged with the new selection mechanism.
- The existing `VisualState` struct can be refactored to support the unified selection set model — both Tab-toggle and Visual-range add to the same set.
- The `V` keybinding and Visual mode remain but are simplified: Visual mode is just a way to add a contiguous range to the selection set, not a separate operating mode for bulk operations.
