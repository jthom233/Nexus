package tui

import (
	"reflect"
	"testing"
)

func TestVisualState_NewIsInactive(t *testing.T) {
	vs := NewVisualState()
	if vs.Active() {
		t.Error("new VisualState should not be active")
	}
	if vs.Count() != 0 {
		t.Errorf("new VisualState Count() = %d, want 0", vs.Count())
	}
}

func TestVisualState_Enter(t *testing.T) {
	vs := NewVisualState()
	vs.Enter(3)

	if !vs.Active() {
		t.Error("VisualState should be active after Enter")
	}
	if vs.Anchor() != 3 {
		t.Errorf("Anchor() = %d, want 3", vs.Anchor())
	}
	if vs.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (anchor should be selected)", vs.Count())
	}
	if !vs.IsSelected(3) {
		t.Error("anchor row should be selected")
	}
	if vs.IsSelected(0) {
		t.Error("non-anchor row should not be selected")
	}
}

func TestVisualState_Exit(t *testing.T) {
	vs := NewVisualState()
	vs.Enter(3)
	vs.UpdateRange(5)
	vs.Exit()

	if vs.Active() {
		t.Error("VisualState should be inactive after Exit")
	}
	if vs.Count() != 0 {
		t.Errorf("Count() = %d, want 0 after Exit", vs.Count())
	}
	if vs.IsSelected(3) {
		t.Error("no rows should be selected after Exit")
	}
}

func TestVisualState_UpdateRange_Down(t *testing.T) {
	vs := NewVisualState()
	vs.Enter(2) // anchor at row 2

	vs.UpdateRange(5) // cursor moved to row 5
	if vs.Count() != 4 {
		t.Errorf("Count() = %d, want 4 (rows 2,3,4,5)", vs.Count())
	}
	for i := 2; i <= 5; i++ {
		if !vs.IsSelected(i) {
			t.Errorf("row %d should be selected", i)
		}
	}
	if vs.IsSelected(1) {
		t.Error("row 1 should not be selected")
	}
	if vs.IsSelected(6) {
		t.Error("row 6 should not be selected")
	}
}

func TestVisualState_UpdateRange_Up(t *testing.T) {
	vs := NewVisualState()
	vs.Enter(5) // anchor at row 5

	vs.UpdateRange(2) // cursor moved up to row 2
	if vs.Count() != 4 {
		t.Errorf("Count() = %d, want 4 (rows 2,3,4,5)", vs.Count())
	}
	for i := 2; i <= 5; i++ {
		if !vs.IsSelected(i) {
			t.Errorf("row %d should be selected", i)
		}
	}
}

func TestVisualState_UpdateRange_Contracts(t *testing.T) {
	vs := NewVisualState()
	vs.Enter(2)

	// Extend to row 6
	vs.UpdateRange(6)
	if vs.Count() != 5 {
		t.Errorf("Count() = %d, want 5 (rows 2-6)", vs.Count())
	}

	// Contract back to row 4
	vs.UpdateRange(4)
	if vs.Count() != 3 {
		t.Errorf("Count() = %d, want 3 (rows 2-4)", vs.Count())
	}
	if vs.IsSelected(5) {
		t.Error("row 5 should no longer be selected after contraction")
	}
	if vs.IsSelected(6) {
		t.Error("row 6 should no longer be selected after contraction")
	}
}

func TestVisualState_ToggleItem(t *testing.T) {
	vs := NewVisualState()
	vs.Enter(2)

	// Toggle an item outside the range
	vs.ToggleItem(7)
	if !vs.IsSelected(7) {
		t.Error("row 7 should be selected after toggle")
	}
	if vs.Count() != 2 {
		t.Errorf("Count() = %d, want 2 (anchor + toggled)", vs.Count())
	}

	// Toggle it again to deselect
	vs.ToggleItem(7)
	if vs.IsSelected(7) {
		t.Error("row 7 should be deselected after second toggle")
	}
	if vs.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (only anchor)", vs.Count())
	}
}

func TestVisualState_TogglePreservedAcrossRangeUpdate(t *testing.T) {
	vs := NewVisualState()
	vs.Enter(2)

	// Toggle an item outside the expected range
	vs.ToggleItem(8)

	// Update the contiguous range
	vs.UpdateRange(4)

	// Range should be 2-4 plus the toggled item 8
	if vs.Count() != 4 {
		t.Errorf("Count() = %d, want 4 (rows 2,3,4 + toggled 8)", vs.Count())
	}
	if !vs.IsSelected(8) {
		t.Error("toggled row 8 should persist across range update")
	}
	for i := 2; i <= 4; i++ {
		if !vs.IsSelected(i) {
			t.Errorf("row %d should be selected in range", i)
		}
	}
}

func TestVisualState_SelectedIndices_Sorted(t *testing.T) {
	vs := NewVisualState()
	vs.Enter(5)
	vs.ToggleItem(1)
	vs.ToggleItem(9)
	vs.UpdateRange(7)

	got := vs.SelectedIndices()
	want := []int{1, 5, 6, 7, 9}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SelectedIndices() = %v, want %v", got, want)
	}
}

func TestVisualState_SelectedIDs(t *testing.T) {
	rows := []Row{
		{ID: "a"},
		{ID: "b"},
		{ID: "c"},
		{ID: "d"},
		{ID: "e"},
	}

	vs := NewVisualState()
	vs.Enter(1) // anchor at "b"
	vs.UpdateRange(3) // range to "d"

	ids := vs.SelectedIDs(rows)
	want := []string{"b", "c", "d"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("SelectedIDs() = %v, want %v", ids, want)
	}
}

func TestVisualState_SelectedIDs_OutOfBounds(t *testing.T) {
	rows := []Row{
		{ID: "a"},
		{ID: "b"},
	}

	vs := NewVisualState()
	vs.Enter(0)
	vs.ToggleItem(5) // out of bounds

	ids := vs.SelectedIDs(rows)
	// Should only include row 0, not the out-of-bounds index
	want := []string{"a"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("SelectedIDs() = %v, want %v", ids, want)
	}
}

func TestVisualState_ToggleItemWhenInactive(t *testing.T) {
	vs := NewVisualState()
	// Toggle without entering visual mode should be a no-op
	vs.ToggleItem(3)
	if vs.Count() != 0 {
		t.Errorf("Count() = %d, want 0 (toggle when inactive)", vs.Count())
	}
}

func TestVisualState_UpdateRangeWhenInactive(t *testing.T) {
	vs := NewVisualState()
	// UpdateRange without entering visual mode should be a no-op
	vs.UpdateRange(5)
	if vs.Count() != 0 {
		t.Errorf("Count() = %d, want 0 (UpdateRange when inactive)", vs.Count())
	}
}

func TestVisualState_ReEnter(t *testing.T) {
	vs := NewVisualState()
	vs.Enter(2)
	vs.UpdateRange(5)
	vs.ToggleItem(8)

	// Re-enter at a different position
	vs.Enter(0)
	if vs.Count() != 1 {
		t.Errorf("Count() = %d, want 1 after re-Enter", vs.Count())
	}
	if !vs.IsSelected(0) {
		t.Error("new anchor should be selected")
	}
	if vs.IsSelected(2) {
		t.Error("old anchor should not be selected after re-Enter")
	}
	if vs.IsSelected(8) {
		t.Error("old toggled item should not be selected after re-Enter")
	}
}

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

func TestVisualState_SingleItemRange(t *testing.T) {
	vs := NewVisualState()
	vs.Enter(3)
	vs.UpdateRange(3) // cursor same as anchor

	if vs.Count() != 1 {
		t.Errorf("Count() = %d, want 1 for single-item range", vs.Count())
	}
	if !vs.IsSelected(3) {
		t.Error("anchor/cursor row should be selected")
	}
}
