package launcher

import (
	"fmt"
	"strconv"

	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/session"

	tea "github.com/charmbracelet/bubbletea"
)

// SSHLauncher launches native SSH sessions (suspends TUI).
type SSHLauncher struct{}

func (l *SSHLauncher) Launch(conn config.Connection) tea.Cmd {
	sess := &session.SSHSession{
		Host:         conn.Host,
		Port:         conn.EffectivePort(),
		Username:     conn.Username,
		Password:     conn.Password,
		IdentityFile: conn.IdentityFile,
		ProxyJump:    conn.ProxyJump,
		ProxyCommand: conn.ProxyCommand,
	}

	// tea.Exec suspends the TUI and gives us raw terminal access.
	return tea.Exec(sess, func(err error) tea.Msg {
		return LaunchFinishedMsg{ID: conn.ID, Err: err}
	})
}

func (l *SSHLauncher) Command(conn config.Connection) string {
	args := l.buildDisplayArgs(conn)
	prefix := "ssh (native)"
	if conn.Password != "" {
		prefix = "ssh (native, password auth)"
	}
	return prefix + " " + joinArgs(args)
}

func (l *SSHLauncher) buildDisplayArgs(conn config.Connection) []string {
	var args []string
	if conn.IdentityFile != "" {
		args = append(args, "-i", conn.IdentityFile)
	}
	if conn.ProxyJump != "" {
		args = append(args, "-J", conn.ProxyJump)
	}
	port := conn.EffectivePort()
	if port != 22 {
		args = append(args, "-p", strconv.Itoa(port))
	}
	target := conn.Host
	if conn.Username != "" {
		target = fmt.Sprintf("%s@%s", conn.Username, conn.Host)
	}
	args = append(args, target)
	return args
}
