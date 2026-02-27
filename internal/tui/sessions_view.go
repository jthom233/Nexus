package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/session"
)

type sessionsViewModel struct {
	table    table.Model
	sessions []*session.ManagedSession
	width    int
	height   int
	ready    bool
}

func newSessionsView() sessionsViewModel {
	return sessionsViewModel{}
}

func (v *sessionsViewModel) setSize(width, height int) {
	v.width = width
	v.height = height
	v.rebuildTable()
}

func (v *sessionsViewModel) setSessions(sessions []*session.ManagedSession) {
	v.sessions = sessions
	v.rebuildTable()
}

func (v *sessionsViewModel) selectedSession() *session.ManagedSession {
	if len(v.sessions) == 0 {
		return nil
	}
	row := v.table.Cursor()
	if row < 0 || row >= len(v.sessions) {
		return nil
	}
	return v.sessions[row]
}

func (v *sessionsViewModel) rebuildTable() {
	columns := []table.Column{
		{Title: "#", Width: 4},
		{Title: "NAME", Width: 0},
		{Title: "HOST", Width: 0},
		{Title: "PROTO", Width: 6},
		{Title: "STATUS", Width: 12},
		{Title: "UPTIME", Width: 10},
	}

	// Calculate dynamic widths
	fixed := 4 + 6 + 12 + 10 + 12 // # + proto + status + uptime + padding
	remaining := v.width - fixed
	if remaining < 20 {
		remaining = 20
	}
	nameW := remaining * 50 / 100
	hostW := remaining * 50 / 100
	columns[1].Width = nameW
	columns[2].Width = hostW

	rows := make([]table.Row, len(v.sessions))
	for i, s := range v.sessions {
		name := s.Name
		if s.IsGhost {
			name = "[ghost] " + name
		}
		rows[i] = table.Row{
			s.ID,
			truncate(name, nameW),
			truncate(fmt.Sprintf("%s:%d", s.Host, s.Port), hostW),
			strings.ToUpper(s.Protocol),
			statusLabel(s.Status()),
			formatUptime(s.Uptime()),
		}
	}

	h := v.height
	if h < 3 {
		h = 3
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(h),
	)

	s := table.DefaultStyles()
	s.Header = TableHeaderStyle
	s.Selected = TableSelectedStyle
	s.Cell = TableRowStyle
	t.SetStyles(s)

	// Preserve cursor
	cursor := 0
	if v.ready {
		cursor = v.table.Cursor()
		if cursor >= len(rows) {
			cursor = len(rows) - 1
		}
		if cursor < 0 {
			cursor = 0
		}
	}
	t.SetCursor(cursor)
	v.table = t
	v.ready = true
}

func (v sessionsViewModel) Update(msg tea.Msg) (sessionsViewModel, tea.Cmd) {
	var cmd tea.Cmd
	v.table, cmd = v.table.Update(msg)
	return v, cmd
}

func (v sessionsViewModel) View() string {
	if len(v.sessions) == 0 {
		empty := lipgloss.NewStyle().
			Foreground(ColorSubtle).
			Padding(2, 0).
			Width(v.width).
			Align(lipgloss.Center).
			Render("No active sessions")
		return empty
	}
	if !v.ready {
		return ""
	}
	return v.table.View()
}

func statusLabel(s session.SessionStatus) string {
	switch s {
	case session.StatusConnecting:
		return StatusDegradedStyle.Render("connecting")
	case session.StatusConnected:
		return StatusOnlineStyle.Render("attached")
	case session.StatusDetached:
		return lipgloss.NewStyle().Foreground(ColorYellow).Render("detached")
	case session.StatusClosed:
		return StatusOfflineStyle.Render("closed")
	}
	return "unknown"
}

func formatUptime(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
