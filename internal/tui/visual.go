package tui

// SelectionSet tracks multi-select state, supporting both picker-style Tab
// toggles and visual-range selection (V mode with j/k navigation).
//
// Key design properties:
//   - A single `selected` map holds all selections regardless of how they
//     were added (Tab toggle, visual range, SelectAll).
//   - Visual mode (visualActive) is an overlay: entering/exiting it does NOT
//     clear the selection map. Selections accumulate.
//   - UpdateRange is additive: it adds the anchor..cursor range to `selected`
//     but never removes items that were selected outside the range.
type SelectionSet struct {
	visualActive bool         // whether visual (range) mode is active
	anchor       int          // the cursor position when visual mode was entered
	selected     map[int]bool // set of all selected row indices
}

// NewSelectionSet creates a new, empty SelectionSet.
func NewSelectionSet() SelectionSet {
	return SelectionSet{
		selected: make(map[int]bool),
	}
}

// ensureMap lazily initialises the selected map to handle zero-value structs.
func (s *SelectionSet) ensureMap() {
	if s.selected == nil {
		s.selected = make(map[int]bool)
	}
}

// Toggle adds idx to the selection if it is not already selected, or removes
// it if it is. Works regardless of whether visual mode is active.
func (s *SelectionSet) Toggle(idx int) {
	s.ensureMap()
	if s.selected[idx] {
		delete(s.selected, idx)
	} else {
		s.selected[idx] = true
	}
}

// Deselect unconditionally removes idx from the selection. Idempotent.
func (s *SelectionSet) Deselect(idx int) {
	s.ensureMap()
	delete(s.selected, idx)
}

// SelectAll adds indices 0..count-1 to the selection.
func (s *SelectionSet) SelectAll(count int) {
	s.ensureMap()
	for i := 0; i < count; i++ {
		s.selected[i] = true
	}
}

// DeselectAll clears all selections. Does NOT change visualActive.
func (s *SelectionSet) DeselectAll() {
	s.selected = make(map[int]bool)
}

// EnterVisual activates visual (range) mode with cursor as the anchor.
// The cursor row is added to the selection. Existing selections are preserved.
func (s *SelectionSet) EnterVisual(cursor int) {
	s.ensureMap()
	s.visualActive = true
	s.anchor = cursor
	s.selected[cursor] = true
}

// ExitVisual deactivates visual mode. The selected map is NOT cleared.
func (s *SelectionSet) ExitVisual() {
	s.visualActive = false
}

// VisualActive returns true if visual (range) mode is currently active.
func (s *SelectionSet) VisualActive() bool {
	return s.visualActive
}

// Anchor returns the anchor position set when EnterVisual was called.
func (s *SelectionSet) Anchor() int {
	return s.anchor
}

// UpdateRange adds all indices in [min(anchor,cursor)..max(anchor,cursor)] to
// the selection. This is additive: items selected outside the range are kept.
// No-op when visual mode is not active.
func (s *SelectionSet) UpdateRange(cursor int) {
	if !s.visualActive {
		return
	}
	s.ensureMap()

	lo, hi := s.anchor, cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	for i := lo; i <= hi; i++ {
		s.selected[i] = true
	}
}

// IsSelected returns true if idx is currently selected.
func (s *SelectionSet) IsSelected(idx int) bool {
	if s.selected == nil {
		return false
	}
	return s.selected[idx]
}

// Count returns the number of selected rows.
func (s *SelectionSet) Count() int {
	return len(s.selected)
}

// HasSelection returns true if at least one row is selected.
func (s *SelectionSet) HasSelection() bool {
	return s.Count() > 0
}

// SelectedIndices returns a sorted slice of all selected row indices.
func (s *SelectionSet) SelectedIndices() []int {
	indices := make([]int, 0, len(s.selected))
	for idx := range s.selected {
		indices = append(indices, idx)
	}
	sortInts(indices)
	return indices
}

// SelectedIDs returns the IDs of selected rows from the given rows slice.
// Indices that are out of bounds are silently skipped.
func (s *SelectionSet) SelectedIDs(rows []Row) []string {
	indices := s.SelectedIndices()
	ids := make([]string, 0, len(indices))
	for _, idx := range indices {
		if idx >= 0 && idx < len(rows) {
			ids = append(ids, rows[idx].ID)
		}
	}
	return ids
}

// ---------------------------------------------------------------------------
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
