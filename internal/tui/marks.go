package tui

// Mark records a position in the TUI: which view and which cursor index.
type Mark struct {
	View   string
	Cursor int
}

const jumpListMaxSize = 100

// MarkManager manages vim-style marks (a-z) and a jump list for navigation history.
type MarkManager struct {
	marks     map[rune]Mark
	jumpList  []Mark
	jumpIndex int // points to the current position in jumpList (-1 = at head)
}

// NewMarkManager creates an empty MarkManager.
func NewMarkManager() *MarkManager {
	return &MarkManager{
		marks:     make(map[rune]Mark),
		jumpIndex: -1,
	}
}

// SetMark stores a named mark (a-z) at the given view and cursor position.
func (m *MarkManager) SetMark(name rune, view string, cursor int) {
	if name < 'a' || name > 'z' {
		return
	}
	m.marks[name] = Mark{View: view, Cursor: cursor}
}

// GetMark retrieves a named mark. Returns false if the mark is not set.
func (m *MarkManager) GetMark(name rune) (Mark, bool) {
	mk, ok := m.marks[name]
	return mk, ok
}

// AllMarks returns a copy of all set marks.
func (m *MarkManager) AllMarks() map[rune]Mark {
	out := make(map[rune]Mark, len(m.marks))
	for k, v := range m.marks {
		out[k] = v
	}
	return out
}

// PushJump records the current position before a jump.
// If the user has jumped back (jumpIndex >= 0), entries after the current
// position are discarded (fork behavior, like vim).
func (m *MarkManager) PushJump(view string, cursor int) {
	mk := Mark{View: view, Cursor: cursor}

	// If we're in the middle of the jump list (user jumped back), truncate forward history.
	if m.jumpIndex >= 0 && m.jumpIndex < len(m.jumpList)-1 {
		m.jumpList = m.jumpList[:m.jumpIndex+1]
	}

	m.jumpList = append(m.jumpList, mk)

	// Enforce max size by trimming from the front.
	if len(m.jumpList) > jumpListMaxSize {
		excess := len(m.jumpList) - jumpListMaxSize
		m.jumpList = m.jumpList[excess:]
	}

	// Reset index to head (latest entry).
	m.jumpIndex = len(m.jumpList) - 1
}

// JumpBack moves backward in the jump list (Ctrl+o).
// Returns the destination mark and true, or false if already at the oldest entry.
func (m *MarkManager) JumpBack() (Mark, bool) {
	if len(m.jumpList) == 0 {
		return Mark{}, false
	}
	if m.jumpIndex <= 0 {
		// Already at the oldest entry or before it.
		return Mark{}, false
	}
	m.jumpIndex--
	return m.jumpList[m.jumpIndex], true
}

// JumpForward moves forward in the jump list (Ctrl+i).
// Returns the destination mark and true, or false if already at the newest entry.
func (m *MarkManager) JumpForward() (Mark, bool) {
	if len(m.jumpList) == 0 {
		return Mark{}, false
	}
	if m.jumpIndex >= len(m.jumpList)-1 {
		// Already at the newest entry.
		return Mark{}, false
	}
	m.jumpIndex++
	return m.jumpList[m.jumpIndex], true
}

// LastJump returns the previous jump position (`` backtick-backtick).
// This is the entry just before the current jumpIndex, without modifying the index.
// If the jump list has at least 2 entries, it swaps the user between the two most
// recent positions.
func (m *MarkManager) LastJump() (Mark, bool) {
	if len(m.jumpList) < 2 {
		return Mark{}, false
	}
	// The "last" position is one step before current index.
	idx := m.jumpIndex
	if idx <= 0 {
		idx = 1
	}
	target := idx - 1
	return m.jumpList[target], true
}
