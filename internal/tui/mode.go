package tui

import tea "github.com/charmbracelet/bubbletea"

// Mode represents the current input mode.
type Mode int

const (
	ModeNormal  Mode = iota
	ModeInsert
	ModeVisual
	ModeCommand
)

func (m Mode) String() string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeInsert:
		return "INSERT"
	case ModeVisual:
		return "VISUAL"
	case ModeCommand:
		return "COMMAND"
	default:
		return "UNKNOWN"
	}
}

// ModeChangedMsg is sent when the mode changes.
type ModeChangedMsg struct {
	From Mode
	To   Mode
}

func (a *App) setMode(m Mode) tea.Cmd {
	if a.mode == m {
		return nil
	}
	old := a.mode
	a.mode = m
	return func() tea.Msg {
		return ModeChangedMsg{From: old, To: m}
	}
}
