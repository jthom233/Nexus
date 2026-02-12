package launcher

import (
	"fmt"
	"time"

	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/ipc"

	tea "github.com/charmbracelet/bubbletea"
)

// VNCLauncher launches VNC sessions via the GUI process (IPC).
type VNCLauncher struct{}

func (l *VNCLauncher) Launch(conn config.Connection) tea.Cmd {
	return func() tea.Msg {
		// Ensure GUI process is running
		if err := ensureGUI(); err != nil {
			return LaunchFinishedMsg{ID: conn.ID, Err: fmt.Errorf("gui launch: %w", err)}
		}

		// Connect to GUI via IPC
		client, err := ipc.Connect(10 * time.Second)
		if err != nil {
			return LaunchFinishedMsg{ID: conn.ID, Err: fmt.Errorf("ipc connect: %w", err)}
		}
		defer client.Close()

		// Send open-tab command
		password := conn.VNCPassword
		if password == "" {
			password = conn.Password
		}
		cmd := ipc.OpenTabCmd{
			ConnID:   conn.ID,
			Protocol: "vnc",
			Host:     conn.Host,
			Port:     conn.EffectivePort(),
			Password: password,
		}
		if err := client.Send(ipc.MsgOpenTab, &cmd); err != nil {
			return LaunchFinishedMsg{ID: conn.ID, Err: fmt.Errorf("ipc send: %w", err)}
		}

		// Wait for response
		for {
			env, err := client.Recv()
			if err != nil {
				return LaunchFinishedMsg{ID: conn.ID, Err: fmt.Errorf("ipc recv: %w", err)}
			}
			switch env.Type {
			case ipc.MsgTabOpened:
				continue
			case ipc.MsgTabClosed:
				var evt ipc.TabClosedEvent
				ipc.DecodePayload(env, &evt)
				if evt.ConnID == conn.ID {
					return LaunchFinishedMsg{ID: conn.ID}
				}
			case ipc.MsgTabError:
				var evt ipc.TabErrorEvent
				ipc.DecodePayload(env, &evt)
				if evt.ConnID == conn.ID {
					return LaunchFinishedMsg{ID: conn.ID, Err: fmt.Errorf("%s", evt.Error)}
				}
			}
		}
	}
}

func (l *VNCLauncher) Command(conn config.Connection) string {
	return fmt.Sprintf("vnc (native) %s:%d", conn.Host, conn.EffectivePort())
}
