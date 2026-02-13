package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/audit"
	"github.com/dr4zz/nexus/internal/theme"
)

// LogViewModel displays audit events in a scrollable list.
type LogViewModel struct {
	events   []audit.AuditEvent
	viewport viewport.Model
	ready    bool
	width    int
	height   int
}

// NewLogViewModel creates a new audit log view model.
func NewLogViewModel() *LogViewModel {
	return &LogViewModel{}
}

// SetSize configures the available render area.
func (m *LogViewModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	if !m.ready {
		m.viewport = viewport.New(w, h)
		m.ready = true
	} else {
		m.viewport.Width = w
		m.viewport.Height = h
	}
	m.updateContent()
}

// SetEvents loads audit events into the view.
func (m *LogViewModel) SetEvents(events []audit.AuditEvent) {
	m.events = events
	m.updateContent()
}

// Update processes bubbletea messages for viewport scrolling.
func (m *LogViewModel) Update(msg tea.Msg) tea.Cmd {
	if !m.ready {
		return nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return cmd
}

// View renders the audit log view.
func (m *LogViewModel) View() string {
	if !m.ready {
		return ""
	}

	t := theme.Current()

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Info).
		Padding(0, 1).
		Render("Audit Log")

	scrollHint := lipgloss.NewStyle().
		Foreground(t.Muted).
		Render(fmt.Sprintf(" %d events  j/k scroll  esc close", len(m.events)))

	header := lipgloss.JoinHorizontal(lipgloss.Center, title, scrollHint)

	return lipgloss.JoinVertical(lipgloss.Left, header, "", m.viewport.View())
}

func (m *LogViewModel) updateContent() {
	if !m.ready {
		return
	}

	t := theme.Current()

	if len(m.events) == 0 {
		m.viewport.SetContent(
			lipgloss.NewStyle().Foreground(t.Subtle).Padding(1, 2).Render("No audit events recorded yet."),
		)
		return
	}

	// Column widths
	tsW := 20
	connW := 20
	eventW := 16
	durW := 10
	detailW := m.width - tsW - connW - eventW - durW - 12 // padding
	if detailW < 10 {
		detailW = 10
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(t.Header)
	rowStyle := lipgloss.NewStyle().Foreground(t.Fg)
	subtleStyle := lipgloss.NewStyle().Foreground(t.Subtle)

	var b strings.Builder

	// Header row
	b.WriteString(fmt.Sprintf("  %s %s %s %s %s\n",
		headerStyle.Width(tsW).Render("TIMESTAMP"),
		headerStyle.Width(connW).Render("CONNECTION"),
		headerStyle.Width(eventW).Render("EVENT"),
		headerStyle.Width(durW).Align(lipgloss.Right).Render("DURATION"),
		headerStyle.Width(detailW).Render("DETAILS"),
	))

	b.WriteString(subtleStyle.Render(strings.Repeat("─", m.width-4)) + "\n")

	// Event rows (most recent at bottom for natural scroll)
	for _, ev := range m.events {
		ts := ev.Timestamp.Format("2006-01-02 15:04:05")
		conn := ev.ConnectionName
		if conn == "" {
			conn = ev.ConnectionID
		}
		if len(conn) > connW {
			conn = conn[:connW-1] + "~"
		}

		eventStr := string(ev.EventType)
		eventColor := t.Fg
		switch ev.EventType {
		case audit.EventConnect:
			eventColor = t.StatusOnline
		case audit.EventDisconnect:
			eventColor = t.Info
		case audit.EventError:
			eventColor = t.Error
		case audit.EventCredentialUse:
			eventColor = t.Warning
		case audit.EventConfigChange:
			eventColor = t.Accent
		}

		dur := ev.Duration
		if dur == "" {
			dur = "-"
		}

		details := ev.Details
		if len(details) > detailW {
			details = details[:detailW-1] + "~"
		}

		b.WriteString(fmt.Sprintf("  %s %s %s %s %s\n",
			subtleStyle.Width(tsW).Render(ts),
			rowStyle.Width(connW).Render(conn),
			lipgloss.NewStyle().Foreground(eventColor).Width(eventW).Render(eventStr),
			rowStyle.Width(durW).Align(lipgloss.Right).Render(dur),
			rowStyle.Width(detailW).Render(details),
		))
	}

	m.viewport.SetContent(b.String())
	m.viewport.GotoBottom()
}
