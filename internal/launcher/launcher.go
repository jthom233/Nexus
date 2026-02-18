package launcher

import (
	"fmt"
	"time"

	"github.com/dr4zz/nexus/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

// LaunchFinishedMsg is sent when a session (any protocol) exits.
type LaunchFinishedMsg struct {
	ID  string
	Err error
}

// GUILogLineMsg is sent by the RDPLauncher when the GUI subprocess broadcasts
// a structured log line over IPC. The TUI model handles this message to
// aggregate GUI log output alongside TUI events.
type GUILogLineMsg struct {
	Level     string
	Message   string
	Timestamp time.Time
}

// Launcher creates a tea.Cmd that launches the given connection.
type Launcher interface {
	Launch(conn config.Connection) tea.Cmd
	Command(conn config.Connection) string
}

// globalProg holds the Bubbletea program reference used by the RDPLauncher to
// forward GUI log lines into the TUI event loop. It is set once by
// SetProgram() before any launcher is used.
var globalProg *tea.Program

// SetProgram stores the Bubbletea program reference so that launchers can
// deliver asynchronous messages (e.g. GUI log lines) to the TUI model.
// Call this once, immediately after tea.NewProgram, before p.Run().
func SetProgram(p *tea.Program) {
	globalProg = p
}

// ForProtocol returns the appropriate launcher for the protocol.
func ForProtocol(proto config.Protocol) (Launcher, error) {
	switch proto {
	case config.ProtoSSH:
		return &SSHLauncher{}, nil
	case config.ProtoRDP:
		return &RDPLauncher{prog: globalProg}, nil
	case config.ProtoVNC:
		return &VNCLauncher{}, nil
	case config.ProtoTelnet:
		return &TelnetLauncher{}, nil
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", proto)
	}
}
