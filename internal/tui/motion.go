package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Operator represents a pending vim operator.
type Operator int

const (
	OpNone   Operator = iota
	OpDelete          // d
	OpYank            // y
	OpChange          // c
)

func (o Operator) String() string {
	switch o {
	case OpDelete:
		return "d"
	case OpYank:
		return "y"
	case OpChange:
		return "c"
	default:
		return ""
	}
}

// MotionResult describes what a motion resolved to.
type MotionResult struct {
	// Type of action to take
	Action MotionAction
	// For movement: absolute target row (0-based index in list)
	Target int
	// For operator+motion: range [From, To] inclusive
	From int
	To   int
	// Count that was applied
	Count int
}

// MotionAction describes the type of action a motion resolved to.
type MotionAction int

const (
	ActionNone     MotionAction = iota
	ActionMove                  // Move cursor to Target
	ActionOpRange               // Apply operator to range [From, To]
	ActionOpLine                // Apply operator to current line (dd, yy, cc)
	ActionPageDown              // Scroll down (half or full page)
	ActionPageUp                // Scroll up (half or full page)
)

// MotionEngine processes vim-style count + operator + motion sequences.
type MotionEngine struct {
	count    int      // accumulated count prefix (0 means no count = 1)
	operator Operator // pending operator (OpNone = just moving)
	pending  string   // pending key sequence (e.g., "g" waiting for second "g")
}

// NewMotionEngine creates a new MotionEngine in its idle state.
func NewMotionEngine() *MotionEngine {
	return &MotionEngine{}
}

// HandleKey processes a key event and returns a MotionResult if the sequence is complete.
// cursor is the current cursor position (0-based), total is the total number of items,
// pageSize is the number of visible rows.
// Returns nil if the key was consumed but the sequence is not complete yet (e.g., count digit, pending "g").
// Returns a MotionResult if the sequence resolved to an action.
// Returns nil and resets the engine to idle if the key is not part of a motion sequence.
func (m *MotionEngine) HandleKey(key tea.KeyMsg, cursor, total, pageSize int) *MotionResult {
	k := key.String()

	// Handle count prefix digits (1-9 to start, then 0-9 for subsequent digits)
	if m.count > 0 && k >= "0" && k <= "9" {
		m.count = m.count*10 + int(k[0]-'0')
		return nil // consumed, sequence continues
	}
	if k >= "1" && k <= "9" && m.pending == "" {
		m.count = int(k[0] - '0')
		return nil // consumed, sequence continues
	}

	count := m.count
	if count == 0 {
		count = 1
	}

	// Handle operator keys
	switch k {
	case "d":
		if m.operator == OpDelete {
			// dd -- delete current line(s)
			result := &MotionResult{Action: ActionOpLine, Target: cursor, From: cursor, To: cursor, Count: count}
			result.To = min(cursor+count-1, total-1)
			m.Reset()
			return result
		}
		if m.operator == OpNone {
			m.operator = OpDelete
			return nil // waiting for motion
		}
	case "y":
		if m.operator == OpYank {
			// yy -- yank current line(s)
			result := &MotionResult{Action: ActionOpLine, Target: cursor, From: cursor, To: cursor, Count: count}
			result.To = min(cursor+count-1, total-1)
			m.Reset()
			return result
		}
		if m.operator == OpNone {
			m.operator = OpYank
			return nil // waiting for motion
		}
	case "c":
		if m.operator == OpChange {
			// cc -- change current line(s)
			result := &MotionResult{Action: ActionOpLine, Target: cursor, From: cursor, To: cursor, Count: count}
			result.To = min(cursor+count-1, total-1)
			m.Reset()
			return result
		}
		if m.operator == OpNone {
			m.operator = OpChange
			return nil // waiting for motion
		}
	}

	// Handle motions (work both standalone and with operators)
	var target int
	hasMotion := true

	switch {
	case k == "j" || k == "down":
		target = min(cursor+count, total-1)
	case k == "k" || k == "up":
		target = max(cursor-count, 0)
	case k == "g":
		if m.pending == "g" {
			// gg -- go to first (or count-th) item
			if m.count > 0 {
				target = min(m.count-1, total-1) // 1-based in vim
			} else {
				target = 0
			}
			m.pending = ""
		} else {
			m.pending = "g"
			return nil // waiting for second g
		}
	case k == "G":
		if m.count > 0 {
			target = min(m.count-1, total-1) // {count}G goes to line count (1-based)
		} else {
			target = total - 1 // G goes to last
		}
	case k == "H":
		// Top of visible screen -- sentinel value for caller to interpret
		result := &MotionResult{Action: ActionMove, Target: -2, Count: count}
		if m.operator != OpNone {
			result.Action = ActionOpRange
			result.From = cursor
		}
		m.Reset()
		return result
	case k == "M":
		// Middle of visible screen -- sentinel value for caller to interpret
		result := &MotionResult{Action: ActionMove, Target: -3, Count: count}
		if m.operator != OpNone {
			result.Action = ActionOpRange
			result.From = cursor
		}
		m.Reset()
		return result
	case k == "L":
		// Bottom of visible screen -- sentinel value for caller to interpret
		result := &MotionResult{Action: ActionMove, Target: -4, Count: count}
		if m.operator != OpNone {
			result.Action = ActionOpRange
			result.From = cursor
		}
		m.Reset()
		return result
	case k == "ctrl+d":
		m.Reset()
		return &MotionResult{Action: ActionPageDown, Count: pageSize / 2}
	case k == "ctrl+u":
		m.Reset()
		return &MotionResult{Action: ActionPageUp, Count: pageSize / 2}
	case k == "ctrl+f":
		m.Reset()
		return &MotionResult{Action: ActionPageDown, Count: pageSize}
	case k == "ctrl+b":
		m.Reset()
		return &MotionResult{Action: ActionPageUp, Count: pageSize}
	default:
		hasMotion = false
	}

	if !hasMotion {
		// Key is not part of a motion sequence
		m.Reset()
		return nil
	}

	// Motion resolved
	if m.operator != OpNone {
		// Operator + motion: create range
		from, to := cursor, target
		if from > to {
			from, to = to, from
		}
		result := &MotionResult{
			Action: ActionOpRange,
			Target: target,
			From:   from,
			To:     to,
			Count:  count,
		}
		m.Reset()
		return result
	}

	// Pure motion (no operator)
	result := &MotionResult{Action: ActionMove, Target: target, Count: count}
	m.Reset()
	return result
}

// Pending returns true if the engine is in the middle of a sequence.
func (m *MotionEngine) Pending() bool {
	return m.count > 0 || m.operator != OpNone || m.pending != ""
}

// PendingOperator returns the currently pending operator.
func (m *MotionEngine) PendingOperator() Operator {
	return m.operator
}

// PendingDisplay returns a string representation of the current pending state for the statusbar.
func (m *MotionEngine) PendingDisplay() string {
	s := ""
	if m.count > 0 {
		s += fmt.Sprintf("%d", m.count)
	}
	if m.operator != OpNone {
		s += m.operator.String()
	}
	if m.pending != "" {
		s += m.pending
	}
	return s
}

// Reset clears all pending state.
func (m *MotionEngine) Reset() {
	m.count = 0
	m.operator = OpNone
	m.pending = ""
}
