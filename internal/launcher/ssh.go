package launcher

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/hooks"
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
		PortForwards: conn.PortForwards,
	}

	// Wrap in a hook-aware session if hooks are configured
	if !conn.Hooks.IsEmpty() {
		hookedSess := &hookedSSHSession{
			inner:     sess,
			connHooks: conn.Hooks,
			runner:    hooks.NewHookRunner(),
			env:       hooks.ConnectionEnv(conn.ID, conn.Host, conn.EffectivePort(), conn.Username, string(conn.Protocol)),
		}
		return tea.Exec(hookedSess, func(err error) tea.Msg {
			return LaunchFinishedMsg{ID: conn.ID, Err: err}
		})
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
	for _, pf := range conn.PortForwards {
		switch pf.Type {
		case config.PortForwardLocal:
			args = append(args, "-L", pf.LocalAddr+":"+pf.RemoteAddr)
		case config.PortForwardRemote:
			args = append(args, "-R", pf.LocalAddr+":"+pf.RemoteAddr)
		case config.PortForwardDynamic:
			args = append(args, "-D", pf.LocalAddr)
		}
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

// hookedSSHSession wraps an SSHSession and runs lifecycle hooks around it.
// It implements tea.ExecCommand (Run, SetStdin, SetStdout, SetStderr).
type hookedSSHSession struct {
	inner     *session.SSHSession
	connHooks hooks.Hooks
	runner    *hooks.HookRunner
	env       map[string]string
}

func (h *hookedSSHSession) SetStdin(r io.Reader)  { h.inner.SetStdin(r) }
func (h *hookedSSHSession) SetStdout(w io.Writer)  { h.inner.SetStdout(w) }
func (h *hookedSSHSession) SetStderr(w io.Writer)  { h.inner.SetStderr(w) }

func (h *hookedSSHSession) Run() error {
	ctx := context.Background()

	// Run pre-connect hooks
	if hks := h.connHooks.ForEvent(hooks.PreConnect); len(hks) > 0 {
		_, err := h.runner.RunHooks(ctx, hooks.PreConnect, hks, h.env)
		if err != nil {
			return fmt.Errorf("pre-connect hook: %w", err)
		}
	}

	// Run the actual SSH session
	sessErr := h.inner.Run()

	// PostConnect hooks were already run inside Run() if the session connected
	// successfully (see below for managed sessions), but for direct SSH sessions
	// we treat the end of Run() as post-disconnect.

	// Run post-disconnect hooks (best-effort, ignore errors from runner)
	if hks := h.connHooks.ForEvent(hooks.PostDisconnect); len(hks) > 0 {
		h.runner.RunHooks(ctx, hooks.PostDisconnect, hks, h.env)
	}

	return sessErr
}
