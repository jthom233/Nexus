package tui

import (
	"reflect"
	"testing"

	"github.com/dr4zz/nexus/internal/config"
)

func TestBulkOpType_String(t *testing.T) {
	tests := []struct {
		op   BulkOpType
		want string
	}{
		{BulkConnect, "connect"},
		{BulkDelete, "delete"},
		{BulkTag, "tag"},
		{BulkMove, "move"},
		{BulkExport, "export"},
		{BulkHealthCheck, "health-check"},
		{BulkOpType(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.op.String(); got != tt.want {
			t.Errorf("BulkOpType(%d).String() = %q, want %q", tt.op, got, tt.want)
		}
	}
}

func TestBulkProgress_ProgressText(t *testing.T) {
	tests := []struct {
		name string
		bp   BulkProgress
		want string
	}{
		{
			name: "empty",
			bp:   BulkProgress{},
			want: "",
		},
		{
			name: "in progress",
			bp:   BulkProgress{Total: 10, Completed: 5, Current: "server-5"},
			want: "[5/10] 50% server-5",
		},
		{
			name: "with failures",
			bp:   BulkProgress{Total: 10, Completed: 7, Failed: 2, Current: "server-9"},
			want: "[7/10] 70% (2 failed) server-9",
		},
		{
			name: "complete",
			bp:   BulkProgress{Total: 5, Completed: 5, Current: ""},
			want: "[5/5] 100% ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.bp.ProgressText()
			if got != tt.want {
				t.Errorf("ProgressText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBulkProgress_IsComplete(t *testing.T) {
	tests := []struct {
		name string
		bp   BulkProgress
		want bool
	}{
		{"not started", BulkProgress{Total: 5}, false},
		{"in progress", BulkProgress{Total: 5, Completed: 3}, false},
		{"all completed", BulkProgress{Total: 5, Completed: 5}, true},
		{"completed with failures", BulkProgress{Total: 5, Completed: 3, Failed: 2}, true},
		{"zero total", BulkProgress{Total: 0}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.bp.IsComplete(); got != tt.want {
				t.Errorf("IsComplete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveRangeIDs(t *testing.T) {
	conns := []config.Connection{
		{ID: "a", Name: "alpha"},
		{ID: "b", Name: "bravo"},
		{ID: "c", Name: "charlie"},
		{ID: "d", Name: "delta"},
		{ID: "e", Name: "echo"},
	}

	tests := []struct {
		name  string
		start int
		end   int
		want  []string
	}{
		{"full range", 0, 4, []string{"a", "b", "c", "d", "e"}},
		{"partial range", 1, 3, []string{"b", "c", "d"}},
		{"single item", 2, 2, []string{"c"}},
		{"clamped start", -1, 2, []string{"a", "b", "c"}},
		{"clamped end", 3, 10, []string{"d", "e"}},
		{"inverted range", 3, 1, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveRangeIDs(conns, tt.start, tt.end)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ResolveRangeIDs(%d, %d) = %v, want %v", tt.start, tt.end, got, tt.want)
			}
		})
	}
}

func TestResolveRangeIDs_EmptySlice(t *testing.T) {
	got := ResolveRangeIDs(nil, 0, 5)
	if got != nil {
		t.Errorf("expected nil for empty slice, got %v", got)
	}
}

func TestResolveVisualIDs(t *testing.T) {
	rows := []Row{
		{ID: "a"},
		{ID: "b"},
		{ID: "c"},
		{ID: "d"},
	}

	// nil visual state
	if ids := ResolveVisualIDs(nil, rows); ids != nil {
		t.Errorf("expected nil for nil visual state, got %v", ids)
	}

	// inactive visual state
	vs := NewVisualState()
	if ids := ResolveVisualIDs(&vs, rows); ids != nil {
		t.Errorf("expected nil for inactive visual state, got %v", ids)
	}

	// active visual state with selection
	vs.Enter(1)
	vs.UpdateRange(3)
	ids := ResolveVisualIDs(&vs, rows)
	want := []string{"b", "c", "d"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("ResolveVisualIDs() = %v, want %v", ids, want)
	}
}

func TestConnectionsByIDs(t *testing.T) {
	cfg := &config.Config{
		Connections: []config.Connection{
			{ID: "a", Name: "alpha"},
			{ID: "b", Name: "bravo"},
			{ID: "c", Name: "charlie"},
		},
	}

	tests := []struct {
		name string
		ids  []string
		want []string // expected names
	}{
		{"all", []string{"a", "b", "c"}, []string{"alpha", "bravo", "charlie"}},
		{"subset", []string{"b", "c"}, []string{"bravo", "charlie"}},
		{"with missing", []string{"a", "x", "c"}, []string{"alpha", "charlie"}},
		{"empty", nil, nil},
		{"none found", []string{"x", "y"}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conns := ConnectionsByIDs(cfg, tt.ids)
			var names []string
			for _, c := range conns {
				names = append(names, c.Name)
			}
			if len(tt.want) == 0 && len(names) == 0 {
				return // both empty/nil — OK
			}
			if !reflect.DeepEqual(names, tt.want) {
				t.Errorf("ConnectionsByIDs() names = %v, want %v", names, tt.want)
			}
		})
	}
}

func TestBulkOperation_Struct(t *testing.T) {
	op := BulkOperation{
		Type:      BulkTag,
		TargetIDs: []string{"a", "b", "c"},
	}
	if op.Type != BulkTag {
		t.Errorf("expected BulkTag, got %v", op.Type)
	}
	if len(op.TargetIDs) != 3 {
		t.Errorf("expected 3 target IDs, got %d", len(op.TargetIDs))
	}
}

func TestBulkResultMsg_Struct(t *testing.T) {
	msg := BulkResultMsg{
		Type:      BulkHealthCheck,
		Completed: 8,
		Failed:    2,
		Total:     10,
		Message:   "Health check complete",
	}
	if msg.Type != BulkHealthCheck {
		t.Errorf("expected BulkHealthCheck, got %v", msg.Type)
	}
	if msg.Completed != 8 {
		t.Errorf("expected Completed=8, got %d", msg.Completed)
	}
}
