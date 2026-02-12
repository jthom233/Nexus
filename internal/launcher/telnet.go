package launcher

import (
	"strconv"

	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/session"

	tea "github.com/charmbracelet/bubbletea"
)

// TelnetLauncher launches native Telnet sessions (suspends TUI).
type TelnetLauncher struct{}

func (l *TelnetLauncher) Launch(conn config.Connection) tea.Cmd {
	sess := &session.TelnetSession{
		Host: conn.Host,
		Port: conn.EffectivePort(),
	}

	return tea.Exec(sess, func(err error) tea.Msg {
		return LaunchFinishedMsg{ID: conn.ID, Err: err}
	})
}

func (l *TelnetLauncher) Command(conn config.Connection) string {
	port := conn.EffectivePort()
	return "telnet (native) " + conn.Host + " " + strconv.Itoa(port)
}
