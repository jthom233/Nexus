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

type logType int

const (
	logTypeEvent logType = iota // TUI events
	logTypeAudit                // Audit trail
	logTypeDebug                // GUI subprocess
)

type logEntry struct {
	time    time.Time
	level   logLevel
	lt      logType
	message string
}

type logsModel struct {
	entries    []logEntry
	viewport   viewport.Model
	ready      bool
	width      int
	height     int
	activeType int // 0=All, 1=Events, 2=Audit, 3=Debug
	dirty      bool
	maxEntries int
	evictBatch int
}

func newLogsModel() *logsModel {
	return &logsModel{
		maxEntries: 10000,
		evictBatch: 1000,
	}
}

func (l *logsModel) add(lt logType, level logLevel, msg string) {
	l.entries = append(l.entries, logEntry{
		time:    time.Now(),
		level:   level,
		lt:      lt,
		message: msg,
	})
	if len(l.entries) > l.maxEntries {
		l.entries = l.entries[l.evictBatch:]
	}
	l.dirty = true
}

func (l *logsModel) addEvent(level logLevel, format string, args ...interface{}) {
	l.add(logTypeEvent, level, fmt.Sprintf(format, args...))
}

func (l *logsModel) addAudit(level logLevel, msg string) {
	l.add(logTypeAudit, level, msg)
}

func (l *logsModel) addDebug(level logLevel, msg string) {
	l.add(logTypeDebug, level, msg)
}

// Backward-compatible aliases that map to addEvent.
func (l *logsModel) info(format string, args ...any)  { l.addEvent(logInfo, format, args...) }
func (l *logsModel) warn(format string, args ...any)  { l.addEvent(logWarn, format, args...) }
func (l *logsModel) error(format string, args ...any) { l.addEvent(logError, format, args...) }

func (l *logsModel) nextType() {
	l.activeType = (l.activeType + 1) % 4
	l.dirty = true
}

func (l *logsModel) setSize(width, height int) {
	l.width = width
	l.height = height
	if !l.ready {
		l.viewport = viewport.New(width, height)
		l.ready = true
	} else {
		l.viewport.Width = width
		l.viewport.Height = height
	}
	l.dirty = true
}

func (l *logsModel) rebuildContent() {
	// Filter entries according to activeType.
	var filtered []logEntry
	for _, e := range l.entries {
		switch l.activeType {
		case 0: // All
			filtered = append(filtered, e)
		case 1: // Events
			if e.lt == logTypeEvent {
				filtered = append(filtered, e)
			}
		case 2: // Audit
			if e.lt == logTypeAudit {
				filtered = append(filtered, e)
			}
		case 3: // Debug
			if e.lt == logTypeDebug {
				filtered = append(filtered, e)
			}
		}
	}

	var b strings.Builder
	for _, e := range filtered {
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

		// Type tag prefix
		var typeTag string
		switch e.lt {
		case logTypeAudit:
			typeTag = lipgloss.NewStyle().Foreground(ColorYellow).Render("[AUD]") + " "
		case logTypeDebug:
			typeTag = lipgloss.NewStyle().Foreground(ColorSubtle).Render("[DBG]") + " "
		default:
			typeTag = ""
		}

		timeStr := lipgloss.NewStyle().Foreground(ColorSubtle).Render(ts)
		tag := levelStyle.Render(levelTag)

		// For multi-line messages, indent continuation lines.
		lines := strings.Split(e.message, "\n")
		first := fmt.Sprintf("%s %s %s%s", timeStr, tag, typeTag, lines[0])
		b.WriteString(first + "\n")
		for _, line := range lines[1:] {
			if strings.TrimSpace(line) != "" {
				indent := lipgloss.NewStyle().Foreground(ColorSubtle).Render("              ")
				b.WriteString(indent + line + "\n")
			}
		}
	}

	if len(filtered) == 0 {
		emptyMessages := [4]string{
			"No log entries yet.",
			"No event messages yet.",
			"No audit entries yet.",
			"No debug messages yet.",
		}
		b.WriteString(lipgloss.NewStyle().Foreground(ColorSubtle).Render("  " + emptyMessages[l.activeType]))
	}

	l.viewport.SetContent(b.String())
	l.viewport.GotoBottom()
	l.dirty = false
}

// plainText returns all log entries as unformatted text for clipboard copy.
func (l *logsModel) plainText() string {
	var b strings.Builder
	for _, e := range l.entries {
		ts := e.time.Format("15:04:05")
		var level string
		switch e.level {
		case logInfo:
			level = "INFO"
		case logWarn:
			level = "WARN"
		case logError:
			level = "ERR "
		}
		b.WriteString(fmt.Sprintf("%s %s %s\n", ts, level, e.message))
	}
	return b.String()
}

func (l *logsModel) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	l.viewport, cmd = l.viewport.Update(msg)
	return cmd
}

func (l *logsModel) View() string {
	if !l.ready {
		return ""
	}
	if l.dirty {
		l.rebuildContent()
	}

	typeNames := [4]string{"All", "Events", "Audit", "Debug"}
	title := HelpTitleStyle.Render(fmt.Sprintf("Logs [%s]", typeNames[l.activeType]))
	scrollHint := lipgloss.NewStyle().Foreground(ColorSubtle).Render(
		fmt.Sprintf(" %d entries  tab next-filter  j/k scroll  y copy  esc close", l.viewport.TotalLineCount()),
	)
	header := lipgloss.JoinHorizontal(lipgloss.Center, title, scrollHint)
	return lipgloss.JoinVertical(lipgloss.Left, header, "", l.viewport.View())
}
