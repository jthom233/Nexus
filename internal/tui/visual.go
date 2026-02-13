package tui

// VisualState tracks the state of Visual mode, supporting both line-visual
// (V, extending a contiguous range with j/k) and toggle-select (v, for
// non-contiguous multi-select).
type VisualState struct {
	active    bool         // whether visual mode is active
	anchor    int          // the cursor position when visual mode was entered (line-visual)
	selection map[int]bool // set of selected row indices
	toggled   map[int]bool // indices explicitly toggled with v (persisted across range updates)
}

// NewVisualState creates a new, inactive VisualState.
func NewVisualState() VisualState {
	return VisualState{
		selection: make(map[int]bool),
		toggled:   make(map[int]bool),
	}
}

// Enter activates line-visual mode with the given cursor position as anchor.
// The anchor row is automatically included in the selection.
func (v *VisualState) Enter(cursor int) {
	v.active = true
	v.anchor = cursor
	v.selection = map[int]bool{cursor: true}
	v.toggled = make(map[int]bool)
}

// Exit deactivates visual mode and clears all selection state.
func (v *VisualState) Exit() {
	v.active = false
	v.anchor = 0
	v.selection = make(map[int]bool)
	v.toggled = make(map[int]bool)
}

// Active returns true if visual mode is currently active.
func (v *VisualState) Active() bool {
	return v.active
}

// Anchor returns the anchor position (the cursor when V was pressed).
func (v *VisualState) Anchor() int {
	return v.anchor
}

// IsSelected returns true if the given row index is currently selected.
func (v *VisualState) IsSelected(idx int) bool {
	return v.selection[idx]
}

// Count returns the number of selected rows.
func (v *VisualState) Count() int {
	return len(v.selection)
}

// SelectedIndices returns a sorted slice of all selected row indices.
func (v *VisualState) SelectedIndices() []int {
	indices := make([]int, 0, len(v.selection))
	for idx := range v.selection {
		indices = append(indices, idx)
	}
	sortInts(indices)
	return indices
}

// UpdateRange recalculates the contiguous selection range from anchor to cursor.
// This is used in line-visual mode (V) when the cursor moves with j/k.
// Items explicitly toggled with v are preserved in addition to the range.
func (v *VisualState) UpdateRange(cursor int) {
	if !v.active {
		return
	}

	lo, hi := v.anchor, cursor
	if lo > hi {
		lo, hi = hi, lo
	}

	newSel := make(map[int]bool, hi-lo+1+len(v.toggled))
	for i := lo; i <= hi; i++ {
		newSel[i] = true
	}
	for idx := range v.toggled {
		newSel[idx] = true
	}

	v.selection = newSel
}

// ToggleItem toggles the given row index in/out of the selection set.
// Used for non-contiguous multi-select (v key). The item is tracked
// in the toggled set so it persists across range updates.
func (v *VisualState) ToggleItem(idx int) {
	if !v.active {
		return
	}
	if v.toggled[idx] {
		delete(v.toggled, idx)
		delete(v.selection, idx)
	} else {
		v.toggled[idx] = true
		v.selection[idx] = true
	}
}

// SelectedIDs returns the IDs of selected rows from the given rows slice.
func (v *VisualState) SelectedIDs(rows []Row) []string {
	indices := v.SelectedIndices()
	ids := make([]string, 0, len(indices))
	for _, idx := range indices {
		if idx >= 0 && idx < len(rows) {
			ids = append(ids, rows[idx].ID)
		}
	}
	return ids
}

// sortInts sorts a slice of ints in ascending order (simple insertion sort
// for typically small selection sets).
func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		key := a[i]
		j := i - 1
		for j >= 0 && a[j] > key {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = key
	}
}
