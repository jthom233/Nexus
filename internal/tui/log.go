package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type logLevel int

const (
	logInfo logLevel = iota
	logWarn
	logError
)

type logEntry struct {
	time    time.Time
	level   logLevel
	message string
}

type logModel struct {
	entries  []logEntry
	viewport viewport.Model
	ready    bool
	width    int
	height   int
}

func newLog() *logModel {
	return &logModel{}
}

func (l *logModel) add(level logLevel, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	l.entries = append(l.entries, logEntry{
		time:    time.Now(),
		level:   level,
		message: msg,
	})
	l.updateContent()
}

func (l *logModel) info(format string, args ...any)  { l.add(logInfo, format, args...) }
func (l *logModel) warn(format string, args ...any)  { l.add(logWarn, format, args...) }
func (l *logModel) error(format string, args ...any) { l.add(logError, format, args...) }

func (l *logModel) setSize(width, height int) {
	l.width = width
	l.height = height
	if !l.ready {
		l.viewport = viewport.New(width, height)
		l.ready = true
	} else {
		l.viewport.Width = width
		l.viewport.Height = height
	}
	l.updateContent()
}

func (l *logModel) updateContent() {
	if !l.ready {
		return
	}

	var b strings.Builder
	for _, e := range l.entries {
		ts := e.time.Format("15:04:05")

		var levelStyle lipgloss.Style
		var levelTag string
		switch e.level {
		case logInfo:
			levelStyle = lipgloss.NewStyle().Foreground(ColorCyan)
			levelTag = "INFO"
		case logWarn:
			levelStyle = lipgloss.NewStyle().Foreground(ColorYellow)
			levelTag = "WARN"
		case logError:
			levelStyle = lipgloss.NewStyle().Foreground(ColorRed).Bold(true)
			levelTag = "ERR "
		}

		timeStr := lipgloss.NewStyle().Foreground(ColorSubtle).Render(ts)
		tag := levelStyle.Render(levelTag)

		// For multi-line messages, indent continuation lines
		lines := strings.Split(e.message, "\n")
		first := fmt.Sprintf("%s %s %s", timeStr, tag, lines[0])
		b.WriteString(first + "\n")
		for _, line := range lines[1:] {
			if strings.TrimSpace(line) != "" {
				indent := lipgloss.NewStyle().Foreground(ColorSubtle).Render("              ")
				b.WriteString(indent + line + "\n")
			}
		}
	}

	if len(l.entries) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(ColorSubtle).Render("  No log entries yet."))
	}

	l.viewport.SetContent(b.String())
	// Auto-scroll to bottom
	l.viewport.GotoBottom()
}

func (l *logModel) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	l.viewport, cmd = l.viewport.Update(msg)
	return cmd
}

func (l *logModel) View() string {
	if !l.ready {
		return ""
	}
	title := HelpTitleStyle.Render("Event Log")
	scrollHint := lipgloss.NewStyle().Foreground(ColorSubtle).Render(
		fmt.Sprintf(" %d entries  j/k to scroll  esc to close", len(l.entries)),
	)
	header := lipgloss.JoinHorizontal(lipgloss.Center, title, scrollHint)
	return lipgloss.JoinVertical(lipgloss.Left, header, "", l.viewport.View())
}
