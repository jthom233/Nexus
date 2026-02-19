# Research: Picker-Style Multi-Select

**Feature Branch**: `006-picker-multiselect`
**Date**: 2026-02-18

## Decision 1: Selection State Architecture

**Decision**: Refactor `VisualState` into a unified `SelectionSet` that works in both Normal and Visual modes. The set stores selected row indices in a `map[int]bool`. Tab-toggle and Visual-range both add/remove from the same set. Visual mode becomes an optional acceleration layer — not a prerequisite for selection.

**Rationale**: The current `VisualState` already has a `selection map[int]bool` and `toggled map[int]bool`. The refactored model collapses these into a single `selected` map. Tab-toggle directly mutates the map. Visual mode's `UpdateRange` adds range items to the same map. When Visual mode exits, the map persists. This is simpler than the current anchor+range+toggled model and eliminates the need for the `toggled` survival logic.

**Alternatives considered**:
- Keep VisualState as-is, add Tab handling as a separate selection mechanism: Would create two independent selection systems that need to be merged. Rejected — unnecessary complexity.
- Use row IDs instead of indices: Safer across filter changes, but requires rebuilding the ID→index mapping on every render. Rejected — too much churn for the current use case. Indices are fine since selection is cleared when the list changes.

## Decision 2: Tab Key Rebinding

**Decision**: Remove Tab from `cycleGroup()` in Normal mode. Bind Tab to toggle-select current item, Shift-Tab to deselect current item. Group cycling is already accessible via leader `g f` (fuzzy picker).

**Rationale**: The leader `g f` already opens a `PickerGroups` fuzzy picker — a strictly better UX than the Tab linear cycler. Freeing Tab for selection aligns with the Snacks.nvim/MiniPick conventions the user requested.

**Alternatives considered**:
- Keep Tab for group cycling and use a different key for toggle-select: Would conflict with the user's explicit request. Rejected.
- Add Shift-Tab for group cycling (reverse cycle): Shift-Tab is claimed for deselect. Rejected.

## Decision 3: Visual Mode Behavior Change

**Decision**: Visual mode (`V`) is retained but its Esc behavior changes. Currently, `Exit()` clears all selection state. In the new model, exiting Visual mode with Esc only exits the mode — it does NOT clear the selection set. A second Esc in Normal mode clears all selections. Bulk operations (`d`, `y`) remain available in Visual mode and also become available in Normal mode when items are selected.

**Rationale**: The two-stage Esc (clarification answer) preserves the user's selection work when they exit Visual mode. Visual mode is just a faster way to add items — not the only way to trigger operations. This matches picker semantics where selection persists until explicitly cleared or consumed by an operation.

**Alternatives considered**:
- Single Esc clears everything: Would lose selection state on Visual mode exit. Rejected per user clarification.
- Remove Visual mode entirely: User explicitly said to keep it for Ctrl+D/Ctrl+U range selection. Rejected.

## Decision 4: Bulk Operation Dispatch in Normal Mode

**Decision**: Move bulk operations (`d`, `y`) from being Visual-mode-only keybindings to working in Normal mode when items are selected. In Normal mode: if `selectionSet.Count() > 0`, `d`/`y` operate on all selected items. If `Count() == 0`, they operate on the current cursor item (existing behavior). The `:tag`, `:move`, `:export` commands already check `visualState.Active()` — these will be updated to check `selectionSet.Count() > 0` instead.

**Rationale**: This is the core behavioral change that makes selection useful without requiring Visual mode. Currently `d` in Normal mode deletes the cursor item with confirmation. With the new model, `d` checks for selected items first.

**Alternatives considered**:
- Require entering Visual mode to trigger bulk operations: Would defeat the purpose of picker-style selection. Rejected.

## Decision 5: Item Resolution Consistency

**Decision**: Standardize all bulk operations to use `SelectedIDs(rows)` for item resolution, not `SelectedIndices()` + `list.filtered[idx]`. The index-based path used by `:tag` and `:export` is fragile when filters are active.

**Rationale**: The researcher found that `:tag` and `:export` use `SelectedIndices()` + `filtered[idx]` while `d`, `y`, `:mv` use `SelectedIDs(rows)`. The ID-based path is safer because it's independent of index shifts. This also fixes a latent bug.

**Alternatives considered**:
- Keep both resolution paths: Inconsistent and fragile. Rejected.

## Decision 6: Visual Indicator for Selected Items

**Decision**: Reuse the existing `th.Selection` background color for selected items. Add a `✓` checkmark prefix to the first column of selected rows. The current render path already handles `isSelected` — it just needs to remain active when `visualState` is nil (i.e., in Normal mode with a non-empty selection set).

**Rationale**: The existing `renderRow()` already has an `isSelected` parameter and applies `th.Selection` background. The checkmark adds a clear affordance beyond just color, matching picker conventions. Minimal rendering changes needed.

**Alternatives considered**:
- Color-only indicator: Less accessible, harder to distinguish from cursor. Rejected.
- Separate selection column: Would require table layout changes. Over-engineering. Rejected.
