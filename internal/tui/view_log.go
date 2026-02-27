package tui

import (
	"github.com/dr4zz/nexus/internal/termcap"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/key"
)

func (a App) handleLogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch {
	case key.Matches(msg, a.keys.Quit):
		a.confirmQuit()
		return a, nil
	case key.Matches(msg, a.keys.Escape):
		a.popView()
		return a, nil
	case k == "tab":
		a.log.nextType()
		return a, nil
	case key.Matches(msg, a.keys.Yank):
		// Yank (copy) all log entries to system clipboard
		text := a.log.plainText()
		if err := termcap.DefaultClipboard().WriteAll(text); err != nil {
			a.statusBar.setFlash("Clipboard error: "+err.Error(), flashError)
		} else {
			a.statusBar.setFlash("Copied log to clipboard", flashInfo)
		}
		return a, scheduleFlashClear()
	}

	// Pass scroll keys to viewport
	cmd := a.log.Update(msg)
	return a, cmd
}
