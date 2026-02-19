# Data Model: Picker-Style Multi-Select

**Feature Branch**: `006-picker-multiselect`
**Date**: 2026-02-18

## Entities

### SelectionSet (refactored from VisualState)

Tracks which items in the connection list are currently selected.

| Field | Type | Description |
|-------|------|-------------|
| selected | map[int]bool | Set of selected row indices |
| visualActive | bool | Whether Visual mode is currently active |
| anchor | int | Visual mode anchor (starting row); only meaningful when visualActive=true |

**Operations**:
- `Toggle(idx)` — Add if absent, remove if present (Tab key)
- `Deselect(idx)` — Remove from set (Shift-Tab)
- `SelectAll(count)` — Add indices 0..count-1
- `DeselectAll()` — Clear entire set
- `AddRange(from, to)` — Add all indices in range (Visual mode extension)
- `EnterVisual(cursor)` — Set visualActive=true, anchor=cursor, add cursor to selected
- `ExitVisual()` — Set visualActive=false, preserve selected set
- `IsSelected(idx) bool` — Check membership
- `Count() int` — Number of selected items
- `SelectedIDs(rows) []string` — Map indices to row IDs (bounds-checked)
- `HasSelection() bool` — Count() > 0

**State transitions**:

```
EMPTY (no selection)
  │
  ├── Tab on item → SELECTED (1+ items in Normal mode)
  ├── Ctrl+A → ALL_SELECTED
  └── V → VISUAL_ACTIVE + SELECTED (anchor added)

SELECTED (1+ items, Normal mode)
  │
  ├── Tab on selected → toggle off (may return to EMPTY)
  ├── Tab on unselected → add to set
  ├── Shift-Tab → deselect item
  ├── Esc → EMPTY (clear all)
  ├── Ctrl+A → EMPTY (any selected → clear all)
  ├── V → VISUAL_ACTIVE (existing selection preserved, anchor added)
  ├── d/y/:tag/:move/:export → operate on selected → EMPTY (post-op clear)
  └── j/k/g/G → cursor moves, selection unchanged

VISUAL_ACTIVE (Visual mode, 1+ items)
  │
  ├── j/k/Ctrl+D/Ctrl+U/G → extend range (add to selected)
  ├── Esc → SELECTED (exit visual, keep selection)
  ├── d/y → operate on selected → EMPTY
  └── Tab → toggle item (within visual mode)

ALL_SELECTED
  │
  ├── Ctrl+A → EMPTY (toggle off)
  ├── Tab on item → deselect it (partial selection)
  └── d/y/:tag → operate on all → EMPTY
```

## Constraints

- Selection is index-based and session-scoped. Cleared on bulk operation completion, Esc (in Normal mode), or list rebuild (filter/search change).
- Maximum selection is bounded by the visible row count (typically <500 connections).
- Visual mode anchor is only meaningful while visualActive=true. It is not persisted across mode exits.
