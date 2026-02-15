package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// keyFromString converts a human-readable key string (like "j", "5", "ctrl+d",
// "up", "down") into the corresponding tea.KeyMsg value.
func keyFromString(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+a":
		return tea.KeyMsg{Type: tea.KeyCtrlA}
	case "ctrl+b":
		return tea.KeyMsg{Type: tea.KeyCtrlB}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+e":
		return tea.KeyMsg{Type: tea.KeyCtrlE}
	case "ctrl+f":
		return tea.KeyMsg{Type: tea.KeyCtrlF}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	default:
		// Single rune keys: "j", "k", "d", "y", "g", "G", "H", "M", "L", "0"-"9", etc.
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestMotionEngine(t *testing.T) {
	tests := []struct {
		name     string
		keys     []string
		cursor   int
		total    int
		pageSize int
		want     *MotionResult
	}{
		// Pure motions
		{"j moves down", []string{"j"}, 0, 10, 5, &MotionResult{Action: ActionMove, Target: 1, Count: 1}},
		{"k moves up", []string{"k"}, 5, 10, 5, &MotionResult{Action: ActionMove, Target: 4, Count: 1}},
		{"k at top stays", []string{"k"}, 0, 10, 5, &MotionResult{Action: ActionMove, Target: 0, Count: 1}},
		{"j at bottom stays", []string{"j"}, 9, 10, 5, &MotionResult{Action: ActionMove, Target: 9, Count: 1}},
		{"G goes to last", []string{"G"}, 0, 10, 5, &MotionResult{Action: ActionMove, Target: 9, Count: 1}},
		{"gg goes to first", []string{"g", "g"}, 5, 10, 5, &MotionResult{Action: ActionMove, Target: 0, Count: 1}},
		{"down arrow moves down", []string{"down"}, 0, 10, 5, &MotionResult{Action: ActionMove, Target: 1, Count: 1}},
		{"up arrow moves up", []string{"up"}, 5, 10, 5, &MotionResult{Action: ActionMove, Target: 4, Count: 1}},

		// Count + motion
		{"5j moves down 5", []string{"5", "j"}, 0, 10, 5, &MotionResult{Action: ActionMove, Target: 5, Count: 5}},
		{"3k moves up 3", []string{"3", "k"}, 5, 10, 5, &MotionResult{Action: ActionMove, Target: 2, Count: 3}},
		{"5G goes to item 5", []string{"5", "G"}, 0, 10, 5, &MotionResult{Action: ActionMove, Target: 4, Count: 5}},
		{"10j clamps to end", []string{"1", "0", "j"}, 5, 10, 5, &MotionResult{Action: ActionMove, Target: 9, Count: 10}},
		{"3gg goes to item 3", []string{"3", "g", "g"}, 5, 10, 5, &MotionResult{Action: ActionMove, Target: 2, Count: 3}},
		{"99k clamps to start", []string{"9", "9", "k"}, 5, 10, 5, &MotionResult{Action: ActionMove, Target: 0, Count: 99}},

		// Doubled operators
		{"dd deletes current", []string{"d", "d"}, 3, 10, 5, &MotionResult{Action: ActionOpLine, Target: 3, From: 3, To: 3, Count: 1}},
		{"yy yanks current", []string{"y", "y"}, 3, 10, 5, &MotionResult{Action: ActionOpLine, Target: 3, From: 3, To: 3, Count: 1}},
		{"3dd deletes 3", []string{"3", "d", "d"}, 3, 10, 5, &MotionResult{Action: ActionOpLine, Target: 3, From: 3, To: 5, Count: 3}},
		{"3yy yanks 3", []string{"3", "y", "y"}, 3, 10, 5, &MotionResult{Action: ActionOpLine, Target: 3, From: 3, To: 5, Count: 3}},
		{"5dd clamps to end", []string{"5", "d", "d"}, 7, 10, 5, &MotionResult{Action: ActionOpLine, Target: 7, From: 7, To: 9, Count: 5}},

		// Operator + motion
		{"dG deletes to end", []string{"d", "G"}, 3, 10, 5, &MotionResult{Action: ActionOpRange, Target: 9, From: 3, To: 9, Count: 1}},
		{"dgg deletes to start", []string{"d", "g", "g"}, 5, 10, 5, &MotionResult{Action: ActionOpRange, Target: 0, From: 0, To: 5, Count: 1}},
		{"d3j deletes range", []string{"d", "3", "j"}, 2, 10, 5, &MotionResult{Action: ActionOpRange, Target: 5, From: 2, To: 5, Count: 3}},
		{"d2k deletes range upward", []string{"d", "2", "k"}, 5, 10, 5, &MotionResult{Action: ActionOpRange, Target: 3, From: 3, To: 5, Count: 2}},
		{"yG yanks to end", []string{"y", "G"}, 2, 10, 5, &MotionResult{Action: ActionOpRange, Target: 9, From: 2, To: 9, Count: 1}},
		{"ygg yanks to start", []string{"y", "g", "g"}, 5, 10, 5, &MotionResult{Action: ActionOpRange, Target: 0, From: 0, To: 5, Count: 1}},

		// Page scrolling
		{"ctrl+d half page down", []string{"ctrl+d"}, 0, 20, 10, &MotionResult{Action: ActionPageDown, Count: 5}},
		{"ctrl+u half page up", []string{"ctrl+u"}, 10, 20, 10, &MotionResult{Action: ActionPageUp, Count: 5}},
		{"ctrl+f full page down", []string{"ctrl+f"}, 0, 20, 10, &MotionResult{Action: ActionPageDown, Count: 10}},
		{"ctrl+b full page up", []string{"ctrl+b"}, 10, 20, 10, &MotionResult{Action: ActionPageUp, Count: 10}},

		// H/M/L sentinel targets
		{"H returns sentinel", []string{"H"}, 5, 20, 10, &MotionResult{Action: ActionMove, Target: -2, Count: 1}},
		{"M returns sentinel", []string{"M"}, 5, 20, 10, &MotionResult{Action: ActionMove, Target: -3, Count: 1}},
		{"L returns sentinel", []string{"L"}, 5, 20, 10, &MotionResult{Action: ActionMove, Target: -4, Count: 1}},
		{"dH operator with sentinel", []string{"d", "H"}, 5, 20, 10, &MotionResult{Action: ActionOpRange, Target: -2, From: 5, Count: 1}},

		// Unknown key returns nil
		{"unknown key returns nil", []string{"x"}, 5, 10, 5, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewMotionEngine()
			var result *MotionResult
			for _, k := range tt.keys {
				result = engine.HandleKey(keyFromString(k), tt.cursor, tt.total, tt.pageSize)
			}
			if tt.want == nil && result != nil {
				t.Errorf("expected nil result, got %+v", result)
			}
			if tt.want != nil && result == nil {
				t.Errorf("expected %+v, got nil", tt.want)
			}
			if tt.want != nil && result != nil {
				if result.Action != tt.want.Action {
					t.Errorf("Action: got %v, want %v", result.Action, tt.want.Action)
				}
				if result.Target != tt.want.Target {
					t.Errorf("Target: got %v, want %v", result.Target, tt.want.Target)
				}
				if result.From != tt.want.From {
					t.Errorf("From: got %v, want %v", result.From, tt.want.From)
				}
				if result.To != tt.want.To {
					t.Errorf("To: got %v, want %v", result.To, tt.want.To)
				}
				if result.Count != tt.want.Count {
					t.Errorf("Count: got %v, want %v", result.Count, tt.want.Count)
				}
			}
		})
	}
}

func TestMotionEngine_Pending(t *testing.T) {
	engine := NewMotionEngine()

	if engine.Pending() {
		t.Error("new engine should not be pending")
	}

	// After typing "5", should be pending
	engine.HandleKey(keyFromString("5"), 0, 10, 5)
	if !engine.Pending() {
		t.Error("engine should be pending after count digit")
	}

	// After completing motion, should not be pending
	engine.HandleKey(keyFromString("j"), 0, 10, 5)
	if engine.Pending() {
		t.Error("engine should not be pending after completed motion")
	}

	// After typing "d", should be pending
	engine.HandleKey(keyFromString("d"), 0, 10, 5)
	if !engine.Pending() {
		t.Error("engine should be pending after operator")
	}
	if engine.PendingOperator() != OpDelete {
		t.Errorf("PendingOperator() = %v, want OpDelete", engine.PendingOperator())
	}

	// After completing with "d" (dd), should not be pending
	engine.HandleKey(keyFromString("d"), 0, 10, 5)
	if engine.Pending() {
		t.Error("engine should not be pending after dd")
	}

	// After typing "g", should be pending
	engine.HandleKey(keyFromString("g"), 0, 10, 5)
	if !engine.Pending() {
		t.Error("engine should be pending after 'g'")
	}
}

func TestMotionEngine_PendingDisplay(t *testing.T) {
	engine := NewMotionEngine()

	if got := engine.PendingDisplay(); got != "" {
		t.Errorf("empty engine PendingDisplay() = %q, want %q", got, "")
	}

	// "5" -> "5"
	engine.HandleKey(keyFromString("5"), 0, 10, 5)
	if got := engine.PendingDisplay(); got != "5" {
		t.Errorf("after '5': PendingDisplay() = %q, want %q", got, "5")
	}

	// "5d" -> "5d"
	engine.HandleKey(keyFromString("d"), 0, 10, 5)
	if got := engine.PendingDisplay(); got != "5d" {
		t.Errorf("after '5d': PendingDisplay() = %q, want %q", got, "5d")
	}

	// Reset and test "g" pending
	engine.Reset()
	engine.HandleKey(keyFromString("g"), 0, 10, 5)
	if got := engine.PendingDisplay(); got != "g" {
		t.Errorf("after 'g': PendingDisplay() = %q, want %q", got, "g")
	}

	// Reset and test "d" alone -> "d"
	engine.Reset()
	engine.HandleKey(keyFromString("d"), 0, 10, 5)
	if got := engine.PendingDisplay(); got != "d" {
		t.Errorf("after 'd': PendingDisplay() = %q, want %q", got, "d")
	}
}

func TestMotionEngine_Reset(t *testing.T) {
	engine := NewMotionEngine()

	// Build up state
	engine.HandleKey(keyFromString("5"), 0, 10, 5)
	engine.HandleKey(keyFromString("d"), 0, 10, 5)

	engine.Reset()

	if engine.Pending() {
		t.Error("engine should not be pending after Reset()")
	}
	if engine.PendingOperator() != OpNone {
		t.Errorf("PendingOperator() after Reset() = %v, want OpNone", engine.PendingOperator())
	}
	if got := engine.PendingDisplay(); got != "" {
		t.Errorf("PendingDisplay() after Reset() = %q, want %q", got, "")
	}
}

func TestMotionEngine_UnknownKeyResetsState(t *testing.T) {
	engine := NewMotionEngine()

	// Type "d" then an unknown key like "x"
	engine.HandleKey(keyFromString("d"), 0, 10, 5)
	if !engine.Pending() {
		t.Error("engine should be pending after 'd'")
	}

	result := engine.HandleKey(keyFromString("x"), 0, 10, 5)
	if result != nil {
		t.Errorf("unknown key after operator should return nil, got %+v", result)
	}
	if engine.Pending() {
		t.Error("engine should reset after unknown key")
	}
}

func TestMotionEngine_SequentialUse(t *testing.T) {
	// Ensure the engine can be reused across multiple sequences
	engine := NewMotionEngine()

	// First: "j"
	r1 := engine.HandleKey(keyFromString("j"), 0, 10, 5)
	if r1 == nil || r1.Target != 1 {
		t.Errorf("first j: got %+v, want Target=1", r1)
	}

	// Second: "5j"
	engine.HandleKey(keyFromString("5"), 1, 10, 5)
	r2 := engine.HandleKey(keyFromString("j"), 1, 10, 5)
	if r2 == nil || r2.Target != 6 {
		t.Errorf("5j from 1: got %+v, want Target=6", r2)
	}

	// Third: "dd"
	engine.HandleKey(keyFromString("d"), 6, 10, 5)
	r3 := engine.HandleKey(keyFromString("d"), 6, 10, 5)
	if r3 == nil || r3.Action != ActionOpLine {
		t.Errorf("dd: got %+v, want ActionOpLine", r3)
	}
}

func TestOperator_String(t *testing.T) {
	tests := []struct {
		op   Operator
		want string
	}{
		{OpNone, ""},
		{OpDelete, "d"},
		{OpYank, "y"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.op.String(); got != tt.want {
				t.Errorf("Operator(%d).String() = %q, want %q", tt.op, got, tt.want)
			}
		})
	}
}
