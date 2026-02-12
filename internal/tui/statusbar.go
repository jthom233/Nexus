package tui

import (
	"fmt"
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
	width        int
}

func newStatusBar() statusBarModel {
	return statusBarModel{}
}

func (s *statusBarModel) setFlash(text string, kind flashKind) {
	s.flash = &flash{text: text, kind: kind, at: time.Now()}
}

func (s *statusBarModel) clearFlash() {
	s.flash = nil
}

func (s statusBarModel) View() string {
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

	gap := s.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 0 {
		gap = 0
	}
	padding := lipgloss.NewStyle().Width(gap).Render("")

	row := lipgloss.JoinHorizontal(lipgloss.Center, left, padding, right)
	return StatusBarStyle.Width(s.width).Render(row)
}
