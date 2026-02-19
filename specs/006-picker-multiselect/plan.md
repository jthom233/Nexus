# Implementation Plan: Picker-Style Multi-Select

**Branch**: `006-picker-multiselect` | **Date**: 2026-02-18 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/006-picker-multiselect/spec.md`

## Summary

Replace vim-style Visual mode selection with picker-style multi-select. Refactor `VisualState` into a unified `SelectionSet` that works in both Normal and Visual modes. Tab toggles items, Shift-Tab deselects, Ctrl+A toggles select-all. Visual mode (`V`) is retained for range selection but items persist in the selection set after exiting. Bulk operations (`d`, `y`, `:tag`, `:move`, `:export`) operate on selected items when any exist, falling back to cursor item when none are selected. Remove Tab-based group cycling (leader `g f` covers this).

## Technical Context

**Language/Version**: Go 1.25 (per go.mod)
**Primary Dependencies**: charmbracelet/bubbletea v1.3.10, charmbracelet/bubbles v1.0.0, charmbracelet/lipgloss v1.1.0
**Storage**: N/A — in-memory selection state only
**Testing**: Go standard `testing` package; 16 existing tests in visual_test.go
**Target Platform**: Linux (primary), Windows (secondary)
**Project Type**: Single Go project — CLI/TUI
**Performance Goals**: Instant selection toggle (<50ms), no stutter during rapid Tab presses
**Constraints**: Selection is index-based, session-scoped, cleared on bulk operations
**Scale/Scope**: Single user, connection list typically <500 items

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Constitution is an unfilled template — no project-specific gates defined. Proceeding without violations.

**Post-Phase 1 re-check**: No violations. The design modifies 7 existing files within established `internal/tui/` package. No new files, no new external dependencies. Follows existing Bubbletea patterns.

## Project Structure

### Documentation (this feature)

```text
specs/006-picker-multiselect/
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
internal/tui/
├── visual.go            # MODIFY — refactor VisualState → SelectionSet
│                        #   Replace anchor+range+toggled with unified selected map
│                        #   Add Toggle, Deselect, SelectAll, DeselectAll
│                        #   Rename EnterVisual/ExitVisual (preserve set on exit)
├── visual_test.go       # MODIFY — update all 16 tests for new API
│                        #   Add tests for Toggle, SelectAll, two-stage Esc
├── table.go             # MODIFY — rename visualState → selectionSet
│                        #   Add ✓ prefix for selected rows
│                        #   Keep rendering when set is non-nil (not just visual mode)
├── app.go               # MODIFY — bind Tab/Shift-Tab/Ctrl+A in Normal mode
│                        #   Remove Tab→cycleGroup
│                        #   Update V/Esc for two-stage behavior
│                        #   Move d/y to Normal mode (selection-aware)
│                        #   Update visualState.Active() → selectionSet.HasSelection()
├── list.go              # MODIFY — remove cycleGroup() binding (keep function internal)
├── statusbar.go         # MODIFY — show selection count in Normal mode too
├── bulk.go              # MODIFY — standardize all ops to use SelectedIDs(rows)
│                        #   Fix :tag/:export to use SelectedIDs instead of SelectedIndices
└── mode.go              # NO CHANGES — ModeVisual retained
```

**Structure Decision**: All modifications within `internal/tui/`. No new files — this is a refactor of existing selection logic, not new infrastructure.

## Implementation Design

### Component 1: SelectionSet (`visual.go`)

Refactor `VisualState` → `SelectionSet`:

**Struct**:
```
SelectionSet {
    selected     map[int]bool  // unified selection set
    visualActive bool          // whether Visual mode is active
    anchor       int           // Visual mode anchor row
}
```

**Methods**:
- `Toggle(idx)` — If selected, remove; if not, add. Works regardless of visualActive.
- `Deselect(idx)` — Remove from selected. Idempotent.
- `SelectAll(count)` — Add indices 0..count-1 to selected.
- `DeselectAll()` — Clear selected map. Does NOT change visualActive.
- `EnterVisual(cursor)` — Set visualActive=true, anchor=cursor, add cursor to selected.
- `ExitVisual()` — Set visualActive=false ONLY. Do NOT clear selected.
- `UpdateRange(cursor)` — Recompute range [min(anchor,cursor)..max(anchor,cursor)], add all to selected. Does not remove items outside range (additive only).
- `IsSelected(idx)` / `Count()` / `HasSelection()` / `SelectedIDs(rows)` / `SelectedIndices()` — Read accessors, same semantics.

**Key behavior change**: `UpdateRange` is now **additive** — it adds the range to the existing set but does not remove items that were outside the range. This means if a user Tab-selects items 1,5,10, then enters Visual mode and extends from 3-7, the result is {1, 3, 4, 5, 6, 7, 10} — nothing is lost.

### Component 2: Table Rendering (`table.go`)

- Rename `visualState *VisualState` → `selectionSet *SelectionSet`
- Set `selectionSet = &a.selectionSet` whenever `HasSelection()` is true (not just in Visual mode)
- Set `selectionSet = nil` when selection is empty
- In `renderRow()`: when `isSelected`, prepend `✓ ` to the first column content
- Keep existing `th.Selection` background for selected rows

### Component 3: Normal Mode Key Bindings (`app.go`)

**New bindings in Normal mode list view**:
- `tab` → `a.selectionSet.Toggle(cursor)`, sync status bar, sync table pointer
- `shift+tab` → `a.selectionSet.Deselect(cursor)`, sync status bar
- `ctrl+a` → if `HasSelection()`: `DeselectAll()`; else: `SelectAll(rowCount)`. Sync status bar.
- `esc` → if `HasSelection()`: `DeselectAll()`, sync, return (consume Esc). Else: existing Esc behavior.

**Modified bindings**:
- `V` → `EnterVisual(cursor)` (same as before, but preserves existing selection)
- `d` in Normal mode → if `HasSelection()`: show bulk delete confirmation; else: existing single-item delete
- `y` in Normal mode → if `HasSelection()`: yank all selected; else: existing single-item behavior

**Remove**: `case "tab": a.list.cycleGroup()` binding

### Component 4: Visual Mode Changes (`app.go`)

- `handleVisualKey` remains but with updated behavior:
  - `esc` → `ExitVisual()` only (preserves selection set), switch to Normal mode
  - `j`/`k`/`G`/`Ctrl+D`/`Ctrl+U` → move cursor + `UpdateRange(cursor)` (additive)
  - `tab` → `Toggle(cursor)` (works within Visual mode too)
  - `d`/`y` → operate on selection, clear selection, exit Visual mode

### Component 5: Status Bar (`statusbar.go`)

- Show selection count whenever `HasSelection()`, not just in ModeVisual
- Format: `"N selected"` appended to the mode indicator (or replacing it when in Normal mode with selections)
- When `visualActive`: show `"VISUAL (N selected)"`
- When Normal mode with selections: show `"N selected"` in the status bar

### Component 6: Bulk Operation Standardization (`bulk.go`, `app.go`)

- Replace all `visualState.Active()` guards with `selectionSet.HasSelection()`
- Standardize `:tag add/remove` to use `SelectedIDs(rows)` instead of `SelectedIndices()` + `filtered[idx]`
- Standardize `:export` to use `SelectedIDs(rows)` instead of `SelectedIndices()` + `filtered[idx]`
- Fix `:export` to clear selection after completion (pre-existing bug)
- All bulk operations call `DeselectAll()` after completion

### Data Flow

```
NORMAL MODE:
  Tab → selectionSet.Toggle(cursor)
    → table.selectionSet = &a.selectionSet (if HasSelection) or nil
    → statusBar.visualCount = selectionSet.Count()
    → renderRow sees isSelected → ✓ + th.Selection background

  Esc (with selections) → selectionSet.DeselectAll()
    → table.selectionSet = nil
    → statusBar.visualCount = 0

  d (with selections) → SelectedIDs(rows) → confirm prompt → delete → DeselectAll()
  d (no selections) → existing single-item delete

VISUAL MODE:
  V → selectionSet.EnterVisual(cursor) [adds cursor, preserves existing]
    → mode = ModeVisual

  j/k/Ctrl+D/Ctrl+U → UpdateRange(cursor) [additive — adds range to set]
    → statusBar updated

  Esc → selectionSet.ExitVisual() [keeps selected set]
    → mode = ModeNormal
    → table.selectionSet still points to set (items remain visible)

  Second Esc → DeselectAll() [clears everything]
    → table.selectionSet = nil
```

### Error Handling

- Toggle/Deselect on empty list → no-op (bounds check)
- SelectAll with count=0 → no-op
- SelectedIDs with out-of-bounds indices → silently skipped (existing behavior)
- Visual mode UpdateRange with anchor == cursor → single item selected (existing behavior)

## Complexity Tracking

No constitution violations to justify — design stays within existing patterns.
