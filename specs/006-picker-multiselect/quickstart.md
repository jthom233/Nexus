# Developer Quickstart: Picker-Style Multi-Select

**Feature Branch**: `006-picker-multiselect`

## Prerequisites

- Go 1.21+
- Understanding of Bubbletea Update()/View() cycle
- Familiarity with current VisualState (internal/tui/visual.go)

## Architecture Overview

```
┌──────────────────────────────────────────────────────┐
│  App (internal/tui/app.go)                           │
│                                                      │
│  ┌──────────────────────────────────────────────┐    │
│  │  SelectionSet (refactored from VisualState)  │    │
│  │  ┌──────────────────────────────────┐        │    │
│  │  │ selected map[int]bool            │        │    │
│  │  │ visualActive bool               │        │    │
│  │  │ anchor int                      │        │    │
│  │  └──────────────────────────────────┘        │    │
│  │                                              │    │
│  │  Tab → Toggle(idx)                           │    │
│  │  Shift-Tab → Deselect(idx)                   │    │
│  │  Ctrl+A → SelectAll/DeselectAll              │    │
│  │  V → EnterVisual(cursor)                     │    │
│  │  Esc (visual) → ExitVisual() [keeps set]     │    │
│  │  Esc (normal) → DeselectAll()                │    │
│  └──────────────────────────────────────────────┘    │
│       │                                              │
│       ▼                                              │
│  ┌──────────────────────────────────────────────┐    │
│  │  tableModel.selectionSet *SelectionSet       │    │
│  │  renderRow(): isSelected → ✓ + th.Selection  │    │
│  └──────────────────────────────────────────────┘    │
│       │                                              │
│       ▼                                              │
│  ┌──────────────────────────────────────────────┐    │
│  │  Bulk operations (d, y, :tag, :move, :export)│    │
│  │  if selectionSet.HasSelection():             │    │
│  │    operate on SelectedIDs(rows)              │    │
│  │  else:                                       │    │
│  │    operate on cursor item (existing behavior)│    │
│  └──────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────┘
```

## Key Files to Modify

| File | Change |
|------|--------|
| `internal/tui/visual.go` | Refactor `VisualState` → `SelectionSet`. Replace anchor+range+toggled with unified selected map. Add Toggle, Deselect, SelectAll, DeselectAll, EnterVisual, ExitVisual methods. |
| `internal/tui/visual_test.go` | Update all 16 tests for new API. Add tests for Toggle, Deselect, SelectAll, two-stage Esc. |
| `internal/tui/table.go` | Rename `visualState *VisualState` → `selectionSet *SelectionSet`. Add ✓ checkmark prefix for selected rows. Keep rendering active when set is non-nil (not just in visual mode). |
| `internal/tui/app.go` | Bind Tab/Shift-Tab/Ctrl+A in Normal mode. Remove Tab→cycleGroup. Update V/Esc for two-stage behavior. Move d/y bulk handling to Normal mode (selection-aware). Update all `visualState.Active()` guards to `selectionSet.HasSelection()`. |
| `internal/tui/list.go` | Remove `cycleGroup()` or leave as internal (no key binding). |
| `internal/tui/statusbar.go` | Show selection count in Normal mode too (not just ModeVisual). |
| `internal/tui/bulk.go` | Update `ResolveVisualIDs` → use SelectionSet. Standardize all operations to use `SelectedIDs(rows)`. |
| `internal/tui/mode.go` | ModeVisual remains but is now a sub-state of selection, not the only selection mechanism. |

## Build & Test

```bash
# Build
go build ./...

# Unit tests
go test ./internal/tui/...

# Vet
go vet ./...

# Manual test: Tab on 3 items, verify checkmarks, press d, confirm delete
# Manual test: V, Ctrl+D, Esc — verify items stay selected, d works
# Manual test: Ctrl+A selects all, Ctrl+A again deselects all
# Manual test: Tab replaces group cycling — leader g f still works
```
