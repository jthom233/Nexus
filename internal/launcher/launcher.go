package launcher

import (
	"fmt"

	"github.com/dr4zz/nexus/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

// LaunchFinishedMsg is sent when a session (any protocol) exits.
type LaunchFinishedMsg struct {
	ID  string
	Err error
}

// Launcher creates a tea.Cmd that launches the given connection.
type Launcher interface {
	Launch(conn config.Connection) tea.Cmd
	Command(conn config.Connection) string
}

// ForProtocol returns the appropriate launcher for the protocol.
func ForProtocol(proto config.Protocol) (Launcher, error) {
	switch proto {
	case config.ProtoSSH:
		return &SSHLauncher{}, nil
	case config.ProtoRDP:
		return &RDPLauncher{}, nil
	case config.ProtoVNC:
		return &VNCLauncher{}, nil
	case config.ProtoTelnet:
		return &TelnetLauncher{}, nil
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", proto)
	}
}
