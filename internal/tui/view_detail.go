package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/key"
	"github.com/dr4zz/nexus/internal/health"
)

func (a App) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch {
	case k == " ": // Space = leader key
		if !a.leader.active {
			cmd := a.leader.activate()
			return a, cmd
		}
	case key.Matches(msg, a.keys.Quit):
		a.confirmQuit()
		return a, nil
	case key.Matches(msg, a.keys.Help):
		a.help.view = "detail"
		a.help.toggle()
		return a, nil
	case key.Matches(msg, a.keys.Escape):
		a.popView()
		a.help.view = "list"
		return a, nil
	case key.Matches(msg, a.keys.Enter):
		return a.connectSelected()
	case key.Matches(msg, a.keys.Edit):
		if c := a.list.selectedConnection(); c != nil {
			conn := *c
			a.inheritGroupProfile(&conn)
			a.form.startEdit(conn, a.cfg.GroupNames(), a.profileNames())
			a.form.width = a.width
			a.form.height = a.contentHeight()
			a.pushView(viewForm)
			a.help.view = "form"
			a.mode = ModeInsert
			a.statusBar.mode = ModeInsert
			return a, a.form.form.Init()
		}
		return a, nil
	case key.Matches(msg, a.keys.ShowPassword):
		a.detail.showPassword = !a.detail.showPassword
		a.detail.updateContent()
		return a, nil
	case key.Matches(msg, a.keys.Sessions):
		a.sessionsView.setSessions(a.allSessions())
		a.sessionsView.setSize(a.width, a.contentHeight())
		a.pushView(viewSessions)
		a.help.view = "sessions"
		return a, nil
	case k == "ctrl+l":
		a.log.setSize(a.width, a.contentHeight())
		a.pushView(viewLog)
		return a, nil
	}

	var cmd tea.Cmd
	a.detail, cmd = a.detail.Update(msg)
	return a, cmd
}

// handleDetailConnStatus returns the health status and latency for the currently selected connection.
func (a App) handleDetailConnStatus() (health.Status, string) {
	c := a.list.selectedConnection()
	if c == nil {
		return health.Unknown, ""
	}
	st := a.list.statuses[c.ID]
	latStr := ""
	if st.status == health.Online || st.status == health.Degraded {
		latStr = st.latency.String()
	}
	return st.status, latStr
}
