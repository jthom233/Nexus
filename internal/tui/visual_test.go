package tui

import (
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// NewSelectionSet
// ---------------------------------------------------------------------------

func TestSelectionSet_NewIsEmpty(t *testing.T) {
	ss := NewSelectionSet()
	if ss.VisualActive() {
		t.Error("new SelectionSet should not have visual mode active")
	}
	if ss.HasSelection() {
		t.Error("new SelectionSet should have no selection")
	}
	if ss.Count() != 0 {
		t.Errorf("new SelectionSet Count() = %d, want 0", ss.Count())
	}
}

// ---------------------------------------------------------------------------
// EnterVisual
// ---------------------------------------------------------------------------

func TestSelectionSet_EnterVisual(t *testing.T) {
	ss := NewSelectionSet()
	ss.EnterVisual(3)

	if !ss.VisualActive() {
		t.Error("SelectionSet should have visual mode active after EnterVisual")
	}
	if ss.Anchor() != 3 {
		t.Errorf("Anchor() = %d, want 3", ss.Anchor())
	}
	if ss.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (anchor should be selected)", ss.Count())
	}
	if !ss.IsSelected(3) {
		t.Error("anchor row should be selected")
	}
	if ss.IsSelected(0) {
		t.Error("non-anchor row should not be selected")
	}
}

func TestSelectionSet_EnterVisual_PreservesExistingSelection(t *testing.T) {
	ss := NewSelectionSet()
	// Pre-select items 1 and 5 via Toggle (simulating Tab presses)
	ss.Toggle(1)
	ss.Toggle(5)

	// Enter visual mode at cursor 3 — existing selections must be kept
	ss.EnterVisual(3)

	if !ss.IsSelected(1) {
		t.Error("row 1 should still be selected after EnterVisual")
	}
	if !ss.IsSelected(3) {
		t.Error("anchor row 3 should be selected")
	}
	if !ss.IsSelected(5) {
		t.Error("row 5 should still be selected after EnterVisual")
	}
	if ss.Count() != 3 {
		t.Errorf("Count() = %d, want 3", ss.Count())
	}
}

// ---------------------------------------------------------------------------
// ExitVisual
// ---------------------------------------------------------------------------

func TestSelectionSet_ExitVisual_KeepsSelection(t *testing.T) {
	ss := NewSelectionSet()
	ss.EnterVisual(3)
	ss.UpdateRange(5)
	ss.ExitVisual()

	if ss.VisualActive() {
		t.Error("SelectionSet should not have visual mode active after ExitVisual")
	}
	// Selection must be preserved after ExitVisual
	if ss.Count() == 0 {
		t.Error("selection should be preserved after ExitVisual")
	}
	for i := 3; i <= 5; i++ {
		if !ss.IsSelected(i) {
			t.Errorf("row %d should still be selected after ExitVisual", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Toggle
// ---------------------------------------------------------------------------

func TestSelectionSet_Toggle_AddAndRemove(t *testing.T) {
	ss := NewSelectionSet()

	// Toggle on
	ss.Toggle(7)
	if !ss.IsSelected(7) {
		t.Error("row 7 should be selected after first Toggle")
	}
	if ss.Count() != 1 {
		t.Errorf("Count() = %d, want 1", ss.Count())
	}

	// Toggle off
	ss.Toggle(7)
	if ss.IsSelected(7) {
		t.Error("row 7 should be deselected after second Toggle")
	}
	if ss.Count() != 0 {
		t.Errorf("Count() = %d, want 0", ss.Count())
	}
}

func TestSelectionSet_Toggle_WorksWithoutVisualMode(t *testing.T) {
	ss := NewSelectionSet()
	// Toggle without entering visual mode — must work
	ss.Toggle(3)
	if ss.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (Toggle should work without visual mode)", ss.Count())
	}
}

// ---------------------------------------------------------------------------
// Deselect
// ---------------------------------------------------------------------------

func TestSelectionSet_Deselect_Idempotent(t *testing.T) {
	ss := NewSelectionSet()
	ss.Toggle(4)
	ss.Toggle(5)

	// Deselect row 4
	ss.Deselect(4)
	if ss.IsSelected(4) {
		t.Error("row 4 should be deselected")
	}
	if ss.Count() != 1 {
		t.Errorf("Count() = %d, want 1", ss.Count())
	}

	// Deselect again — must be idempotent (no panic, no change)
	ss.Deselect(4)
	if ss.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (Deselect is idempotent)", ss.Count())
	}

	// Deselect a row that was never selected — must be idempotent
	ss.Deselect(99)
	if ss.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (Deselect of unselected row is idempotent)", ss.Count())
	}
}

// ---------------------------------------------------------------------------
// SelectAll / DeselectAll
// ---------------------------------------------------------------------------

func TestSelectionSet_SelectAll(t *testing.T) {
	ss := NewSelectionSet()
	ss.SelectAll(5)

	if ss.Count() != 5 {
		t.Errorf("Count() = %d, want 5 after SelectAll(5)", ss.Count())
	}
	for i := 0; i < 5; i++ {
		if !ss.IsSelected(i) {
			t.Errorf("row %d should be selected after SelectAll(5)", i)
		}
	}
}

func TestSelectionSet_DeselectAll_ClearsSelection(t *testing.T) {
	ss := NewSelectionSet()
	ss.SelectAll(5)
	ss.EnterVisual(2)

	ss.DeselectAll()

	if ss.Count() != 0 {
		t.Errorf("Count() = %d, want 0 after DeselectAll", ss.Count())
	}
	// DeselectAll must NOT change visualActive
	if !ss.VisualActive() {
		t.Error("DeselectAll should not change visualActive state")
	}
}

func TestSelectionSet_DeselectAll_ZeroCount(t *testing.T) {
	ss := NewSelectionSet()
	ss.DeselectAll() // should not panic on empty set
	if ss.Count() != 0 {
		t.Errorf("Count() = %d, want 0", ss.Count())
	}
}

// ---------------------------------------------------------------------------
// UpdateRange — basic range selection
// ---------------------------------------------------------------------------

func TestSelectionSet_UpdateRange_Down(t *testing.T) {
	ss := NewSelectionSet()
	ss.EnterVisual(2) // anchor at row 2

	ss.UpdateRange(5) // cursor moved to row 5
	// Range 2-5 = 4 items
	if ss.Count() != 4 {
		t.Errorf("Count() = %d, want 4 (rows 2,3,4,5)", ss.Count())
	}
	for i := 2; i <= 5; i++ {
		if !ss.IsSelected(i) {
			t.Errorf("row %d should be selected", i)
		}
	}
	if ss.IsSelected(1) {
		t.Error("row 1 should not be selected")
	}
	if ss.IsSelected(6) {
		t.Error("row 6 should not be selected")
	}
}

func TestSelectionSet_UpdateRange_Up(t *testing.T) {
	ss := NewSelectionSet()
	ss.EnterVisual(5) // anchor at row 5

	ss.UpdateRange(2) // cursor moved up to row 2
	if ss.Count() != 4 {
		t.Errorf("Count() = %d, want 4 (rows 2,3,4,5)", ss.Count())
	}
	for i := 2; i <= 5; i++ {
		if !ss.IsSelected(i) {
			t.Errorf("row %d should be selected", i)
		}
	}
}

func TestSelectionSet_UpdateRange_Additive(t *testing.T) {
	// Key new behavior: UpdateRange is additive.
	// Tab-select items 1, 5, 10 → enter visual at 3 → extend to 7
	// Result must be {1, 3, 4, 5, 6, 7, 10}
	ss := NewSelectionSet()
	ss.Toggle(1)
	ss.Toggle(5)
	ss.Toggle(10)

	ss.EnterVisual(3)
	ss.UpdateRange(7)

	expected := []int{1, 3, 4, 5, 6, 7, 10}
	got := ss.SelectedIndices()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("SelectedIndices() = %v, want %v", got, expected)
	}
}

func TestSelectionSet_UpdateRange_NoOp_WhenVisualInactive(t *testing.T) {
	ss := NewSelectionSet()
	ss.UpdateRange(5) // should be a no-op
	if ss.Count() != 0 {
		t.Errorf("Count() = %d, want 0 (UpdateRange when visual inactive)", ss.Count())
	}
}

func TestSelectionSet_UpdateRange_SingleItem(t *testing.T) {
	ss := NewSelectionSet()
	ss.EnterVisual(3)
	ss.UpdateRange(3) // cursor same as anchor

	if ss.Count() != 1 {
		t.Errorf("Count() = %d, want 1 for single-item range", ss.Count())
	}
	if !ss.IsSelected(3) {
		t.Error("anchor/cursor row should be selected")
	}
}

// ---------------------------------------------------------------------------
// Read accessors
// ---------------------------------------------------------------------------

func TestSelectionSet_HasSelection(t *testing.T) {
	ss := NewSelectionSet()
	if ss.HasSelection() {
		t.Error("new SelectionSet should not HasSelection")
	}
	ss.Toggle(2)
	if !ss.HasSelection() {
		t.Error("SelectionSet should HasSelection after Toggle")
	}
}

func TestSelectionSet_SelectedIndices_Sorted(t *testing.T) {
	ss := NewSelectionSet()
	ss.Toggle(9)
	ss.Toggle(1)
	ss.EnterVisual(5)
	ss.UpdateRange(7)

	got := ss.SelectedIndices()
	want := []int{1, 5, 6, 7, 9}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SelectedIndices() = %v, want %v", got, want)
	}
}

func TestSelectionSet_SelectedIDs(t *testing.T) {
	rows := []Row{
		{ID: "a"},
		{ID: "b"},
		{ID: "c"},
		{ID: "d"},
		{ID: "e"},
	}

	ss := NewSelectionSet()
	ss.EnterVisual(1) // anchor at "b"
	ss.UpdateRange(3) // range to "d"

	ids := ss.SelectedIDs(rows)
	want := []string{"b", "c", "d"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("SelectedIDs() = %v, want %v", ids, want)
	}
}

func TestSelectionSet_SelectedIDs_OutOfBounds(t *testing.T) {
	rows := []Row{
		{ID: "a"},
		{ID: "b"},
	}

	ss := NewSelectionSet()
	ss.EnterVisual(0)
	ss.Toggle(5) // out of bounds

	ids := ss.SelectedIDs(rows)
	// Should only include row 0, not the out-of-bounds index
	want := []string{"a"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("SelectedIDs() = %v, want %v", ids, want)
	}
}

// ---------------------------------------------------------------------------
// SelectAll / DeselectAll — toggle select-all (T028)
// ---------------------------------------------------------------------------

func TestSelectionSet_SelectAll_ThenDeselectAll(t *testing.T) {
	ss := NewSelectionSet()
	ss.SelectAll(4)
	if ss.Count() != 4 {
		t.Fatalf("Count() = %d, want 4 after SelectAll(4)", ss.Count())
	}
	ss.DeselectAll()
	if ss.Count() != 0 {
		t.Errorf("Count() = %d, want 0 after DeselectAll", ss.Count())
	}
	if ss.HasSelection() {
		t.Error("HasSelection() should be false after DeselectAll")
	}
}

func TestSelectionSet_SelectAll_ZeroCount(t *testing.T) {
	ss := NewSelectionSet()
	ss.SelectAll(0)
	if ss.HasSelection() {
		t.Error("HasSelection() should be false after SelectAll(0)")
	}
}

// ---------------------------------------------------------------------------
// Visual mode: UpdateRange is additive across multiple movements (T031)
// ---------------------------------------------------------------------------

func TestSelectionSet_UpdateRange_MultipleMovements_Additive(t *testing.T) {
	// Simulate: EnterVisual at 5, move down to 7, then move back up to 3.
	// All rows 3..7 must be selected (additive — no removals).
	ss := NewSelectionSet()
	ss.EnterVisual(5)
	ss.UpdateRange(7) // extends 5-7
	ss.UpdateRange(3) // adds 3-5; existing 5-7 stay

	for i := 3; i <= 7; i++ {
		if !ss.IsSelected(i) {
			t.Errorf("row %d should be selected after additive UpdateRange", i)
		}
	}
	if ss.Count() != 5 {
		t.Errorf("Count() = %d, want 5 (rows 3-7)", ss.Count())
	}
}

// ---------------------------------------------------------------------------
// Visual mode: Tab cherry-picks inside visual mode (T032)
// ---------------------------------------------------------------------------

func TestSelectionSet_Toggle_InVisualMode(t *testing.T) {
	ss := NewSelectionSet()
	ss.EnterVisual(3)
	ss.UpdateRange(6) // rows 3-6 selected

	// Tab-deselect row 4 (already selected — toggle removes it)
	ss.Toggle(4)
	if ss.IsSelected(4) {
		t.Error("row 4 should be deselected after Toggle inside visual range")
	}

	// Toggle row 4 back
	ss.Toggle(4)
	if !ss.IsSelected(4) {
		t.Error("row 4 should be re-selected after second Toggle")
	}
}

func TestSelectionSet_Toggle_NewItemInVisualMode(t *testing.T) {
	ss := NewSelectionSet()
	ss.EnterVisual(3)
	ss.UpdateRange(5) // rows 3-5

	// Tab-select row 10 (outside the visual range — cherry-pick)
	ss.Toggle(10)
	if !ss.IsSelected(10) {
		t.Error("row 10 should be selected after Toggle outside visual range")
	}
	// Visual range items must remain selected
	for i := 3; i <= 5; i++ {
		if !ss.IsSelected(i) {
			t.Errorf("row %d should still be selected", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Two-stage Esc: ExitVisual keeps selection, DeselectAll clears it (T030)
// ---------------------------------------------------------------------------

func TestSelectionSet_TwoStageEsc(t *testing.T) {
	ss := NewSelectionSet()
	ss.EnterVisual(2)
	ss.UpdateRange(5) // rows 2-5

	// Stage 1: ExitVisual — mode exits but selection is preserved.
	ss.ExitVisual()
	if ss.VisualActive() {
		t.Error("VisualActive() should be false after ExitVisual")
	}
	if !ss.HasSelection() {
		t.Error("selection should be preserved after ExitVisual (two-stage Esc stage 1)")
	}

	// Stage 2: DeselectAll — selection is now cleared.
	ss.DeselectAll()
	if ss.HasSelection() {
		t.Error("selection should be cleared after DeselectAll (two-stage Esc stage 2)")
	}
}

// ---------------------------------------------------------------------------
// sortInts (unchanged helper)
// ---------------------------------------------------------------------------

func TestSortInts(t *testing.T) {
	tests := []struct {
		name  string
		input []int
		want  []int
	}{
		{"empty", []int{}, []int{}},
		{"single", []int{5}, []int{5}},
		{"sorted", []int{1, 2, 3}, []int{1, 2, 3}},
		{"reversed", []int{3, 2, 1}, []int{1, 2, 3}},
		{"random", []int{5, 1, 9, 3, 7}, []int{1, 3, 5, 7, 9}},
		{"duplicates", []int{3, 1, 3, 2}, []int{1, 2, 3, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := make([]int, len(tt.input))
			copy(input, tt.input)
			sortInts(input)
			if !reflect.DeepEqual(input, tt.want) {
				t.Errorf("sortInts(%v) = %v, want %v", tt.input, input, tt.want)
			}
		})
	}
}
