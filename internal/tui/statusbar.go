package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
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
	var parts []string
	for _, h := range hints {
		part := KeyHintKeyStyle.Render(h.key) + " " + KeyHintDescStyle.Render(h.desc)
		parts = append(parts, part)
	}
	line := " " + strings.Join(parts, "  ")
	return KeyHintBarStyle.Width(width).Render(line)
}

func modeIndicatorStyle(m Mode) lipgloss.Style {
	var bg lipgloss.Color
	switch m {
	case ModeNormal:
		bg = lipgloss.Color("#7aa2f7")
	case ModeInsert:
		bg = lipgloss.Color("#9ece6a")
	case ModeVisual:
		bg = lipgloss.Color("#bb9af7")
	case ModeCommand:
		bg = lipgloss.Color("#e0af68")
	default:
		bg = lipgloss.Color("#7aa2f7")
	}
	return lipgloss.NewStyle().
		Background(bg).
		Foreground(lipgloss.Color("#ffffff")).
		Bold(true).
		Padding(0, 1)
}

func (s statusBarModel) View() string {
	modeTag := modeIndicatorStyle(s.mode).Render(s.mode.String())

	left := fmt.Sprintf(" %d connections", s.total)
	if s.online > 0 || s.offline > 0 {
		left += fmt.Sprintf(" | %s %d online", StatusOnlineStyle.Render(StatusOnline), s.online)
		left += fmt.Sprintf(" | %s %d offline", StatusOfflineStyle.Render(StatusOffline), s.offline)
	}
	if s.sessionCount > 0 {
		left += fmt.Sprintf(" | ~ %d sessions", s.sessionCount)
	}

	right := ""
	if s.flash != nil {
		elapsed := time.Since(s.flash.at)
		if elapsed < 5*time.Second {
			style := FlashStyle
			if s.flash.kind == flashError {
				style = FlashErrorStyle
			}
			right = style.Render(s.flash.text)
		}
	}

	leftFull := modeTag + left
	gap := s.width - lipgloss.Width(leftFull) - lipgloss.Width(right) - 2
	if gap < 0 {
		gap = 0
	}
	padding := lipgloss.NewStyle().Width(gap).Render("")

	row := lipgloss.JoinHorizontal(lipgloss.Center, leftFull, padding, right)
	statusLine := StatusBarStyle.Width(s.width).Render(row)

	hintLine := renderKeyHints(hintsForView(s.view), s.width)

	return lipgloss.JoinVertical(lipgloss.Left, statusLine, hintLine)
}
