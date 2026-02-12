package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/theme"
)

type flashKind int

const (
	flashInfo flashKind = iota
	flashError
)

type flash struct {
	text string
	kind flashKind
	at   time.Time
}

// FlashExpireMsg is sent to clear expired flash messages.
type FlashExpireMsg struct{}

type statusBarModel struct {
	total        int
	online       int
	offline      int
	sessionCount int
	cursor       int // current cursor position (1-based for display)
	itemCount    int // total items for position display
	flash        *flash
	view         string
	mode         Mode
	width        int
}

func newStatusBar() statusBarModel {
	return statusBarModel{view: "list", mode: ModeNormal}
}

func (s *statusBarModel) setFlash(text string, kind flashKind) {
	s.flash = &flash{text: text, kind: kind, at: time.Now()}
}

func (s *statusBarModel) clearFlash() {
	s.flash = nil
}

type keyHint struct {
	key  string
	desc string
}

func hintsForView(view string) []keyHint {
	switch view {
	case "list":
		return []keyHint{
			{"a", "Add"}, {"e", "Edit"}, {"d", "Delete"}, {"D", "Detail"},
			{"/", "Filter"}, {":", "Cmd"}, {"?", "Help"}, {"q", "Quit"},
		}
	case "detail":
		return []keyHint{
			{"enter", "Connect"}, {"e", "Edit"}, {"p", "Password"},
			{"esc", "Back"}, {"?", "Help"}, {"q", "Quit"},
		}
	case "sessions":
		return []keyHint{
			{"enter", "Reattach"}, {"d", "Kill"},
			{"esc", "Back"}, {"?", "Help"}, {"q", "Quit"},
		}
	case "form":
		return []keyHint{
			{"tab", "Next"}, {"shift+tab", "Prev"},
			{"enter", "Submit"}, {"esc", "Cancel"},
		}
	case "log":
		return []keyHint{
			{"j/k", "Scroll"}, {"esc", "Back"}, {"q", "Quit"},
		}
	default:
		return []keyHint{{"?", "Help"}, {"q", "Quit"}}
	}
}

func renderKeyHints(hints []keyHint, width int) string {
	t := theme.Current()
	keyStyle := lipgloss.NewStyle().
		Foreground(t.Accent).
		Bold(true)
	descStyle := lipgloss.NewStyle().
		Foreground(t.Subtle)
	barStyle := lipgloss.NewStyle().
		Background(t.Bg).
		Width(width).
		Padding(0, 1)

	var parts []string
	for _, h := range hints {
		part := keyStyle.Render(h.key) + " " + descStyle.Render(h.desc)
		parts = append(parts, part)
	}
	line := strings.Join(parts, "  ")
	return barStyle.Render(line)
}

func modeIndicatorStyle(m Mode) lipgloss.Style {
	t := theme.Current()
	var bg lipgloss.Color
	switch m {
	case ModeNormal:
		bg = t.ModeNormal
	case ModeInsert:
		bg = t.ModeInsert
	case ModeVisual:
		bg = t.ModeVisual
	case ModeCommand:
		bg = t.ModeCommand
	default:
		bg = t.ModeNormal
	}
	return lipgloss.NewStyle().
		Background(bg).
		Foreground(lipgloss.Color("#ffffff")).
		Bold(true).
		Padding(0, 1)
}

func (s statusBarModel) View() string {
	t := theme.Current()

	// --- Line 1: Status line ---
	modeTag := modeIndicatorStyle(s.mode).Render(s.mode.String())

	statsStyle := lipgloss.NewStyle().
		Foreground(t.Subtle).
		Background(t.Highlight)
	onlineStyle := lipgloss.NewStyle().
		Foreground(t.StatusOnline).
		Background(t.Highlight)
	offlineStyle := lipgloss.NewStyle().
		Foreground(t.StatusOffline).
		Background(t.Highlight)

	left := statsStyle.Render(fmt.Sprintf(" %d connections", s.total))
	if s.online > 0 || s.offline > 0 {
		left += statsStyle.Render(" | ")
		left += onlineStyle.Render(fmt.Sprintf("%s %d online", StatusOnline, s.online))
		left += statsStyle.Render(" | ")
		left += offlineStyle.Render(fmt.Sprintf("%s %d offline", StatusOffline, s.offline))
	}
	if s.sessionCount > 0 {
		left += statsStyle.Render(fmt.Sprintf(" | ~ %d sessions", s.sessionCount))
	}

	// Position indicator (right side)
	posStyle := lipgloss.NewStyle().
		Foreground(t.Subtle).
		Background(t.Highlight)
	posText := ""
	if s.itemCount > 0 {
		posText = posStyle.Render(fmt.Sprintf("%d/%d", s.cursor, s.itemCount))
	}

	// Flash message (far right)
	flashText := ""
	if s.flash != nil {
		elapsed := time.Since(s.flash.at)
		if elapsed < 5*time.Second {
			flashStyle := lipgloss.NewStyle().
				Foreground(t.Success).
				Background(t.Highlight)
			if s.flash.kind == flashError {
				flashStyle = lipgloss.NewStyle().
					Foreground(t.Error).
					Background(t.Highlight)
			}
			flashText = flashStyle.Render(s.flash.text)
		}
	}

	// Assemble right side: position + flash
	right := ""
	if posText != "" && flashText != "" {
		right = posText + statsStyle.Render("  ") + flashText
	} else if posText != "" {
		right = posText
	} else if flashText != "" {
		right = flashText
	}

	leftFull := modeTag + left
	gap := s.width - lipgloss.Width(leftFull) - lipgloss.Width(right) - 2
	if gap < 0 {
		gap = 0
	}

	statusLineBg := lipgloss.NewStyle().
		Background(t.Highlight).
		Width(s.width)

	padding := strings.Repeat(" ", gap)
	row := leftFull + padding + right
	statusLine := statusLineBg.Render(row)

	// --- Line 2: Keyhint bar ---
	hintLine := renderKeyHints(hintsForView(s.view), s.width)

	return lipgloss.JoinVertical(lipgloss.Left, statusLine, hintLine)
}
