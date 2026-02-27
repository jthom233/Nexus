package launcher

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/ipc"

	tea "github.com/charmbracelet/bubbletea"
)

// RDPLauncher launches RDP sessions via the GUI process (IPC).
type RDPLauncher struct {
	// prog is the Bubbletea program reference used to forward GUI log lines.
	// When nil, log lines received over IPC are silently discarded.
	prog *tea.Program
}

func (l *RDPLauncher) Launch(conn config.Connection) tea.Cmd {
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

		// Build options payload
		opts, _ := json.Marshal(map[string]interface{}{
			"resolution":         conn.RDPOptions.Resolution,
			"fullscreen":         conn.RDPOptions.Fullscreen,
			"dynamic_resolution": conn.RDPOptions.DynamicResolution,
			"security":           conn.RDPOptions.Security,
		})

		// Send open-tab command
		cmd := ipc.OpenTabCmd{
			ConnID:   conn.ID,
			Protocol: "rdp",
			Host:     conn.Host,
			Port:     conn.EffectivePort(),
			Username: conn.Username,
			Password: conn.Password,
			Domain:   conn.Domain,
			Options:  opts,
		}
		if err := client.Send(ipc.MsgOpenTab, &cmd); err != nil {
			return LaunchFinishedMsg{ID: conn.ID, Err: fmt.Errorf("ipc send: %w", err)}
		}

		// Wait for response (tab-opened, tab-error, tab-closed, or log-line).
		for {
			env, err := client.Recv()
			if err != nil {
				return LaunchFinishedMsg{ID: conn.ID, Err: fmt.Errorf("ipc recv: %w", err)}
			}
			switch env.Type {
			case ipc.MsgTabOpened:
				// Tab opened successfully - now wait for close
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
			case ipc.MsgLogLine:
				var evt ipc.LogLineEvent
				if err := ipc.DecodePayload(env, &evt); err == nil && l.prog != nil {
					l.prog.Send(GUILogLineMsg{
						Level:     evt.Level,
						Message:   evt.Message,
						Timestamp: evt.Timestamp,
					})
				}
				continue
			}
		}
	}
}

func (l *RDPLauncher) Command(conn config.Connection) string {
	auth := ""
	if conn.Username != "" {
		auth = conn.Username + "@"
	}
	return fmt.Sprintf("rdp (native) %s%s:%d", auth, conn.Host, conn.EffectivePort())
}

var (
	guiCmd   *exec.Cmd
	guiMu    sync.Mutex
	guiError error
)

// ensureGUI starts the nexus-gui process if it's not already running.
// The mutex only guards shared state (guiCmd, guiError) and is never held
// across sleeps, preventing deadlock on concurrent or repeated calls.
func ensureGUI() error {
	// Fast path: GUI is already responsive, no need to acquire the lock.
	if ipc.Probe() {
		return nil
	}

	guiMu.Lock()

	// If a previous launch failed, report it and reset so the next call
	// can try again.
	if guiCmd != nil && guiError != nil {
		err := guiError
		guiCmd = nil
		guiError = nil
		guiMu.Unlock()
		return fmt.Errorf("nexus-gui exited: %w", err)
	}

	// If guiCmd is already set, another goroutine launched the process.
	// Release the lock and fall through to the wait loop below.
	if guiCmd == nil {
		// Find nexus-gui binary
		guiBinary, err := findGUIBinary()
		if err != nil {
			guiMu.Unlock()
			return err
		}

		logPath := filepath.Join(os.TempDir(), "nexus-gui.log")
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			guiMu.Unlock()
			return fmt.Errorf("create gui log: %w", err)
		}

		cmd := exec.Command(guiBinary)
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		cmd.Stdin = nil
		cmd.Env = os.Environ()
		// Start in its own process group so it survives TUI exit
		setProcGroup(cmd)

		if err := cmd.Start(); err != nil {
			logFile.Close()
			guiMu.Unlock()
			return fmt.Errorf("start nexus-gui: %w", err)
		}
		guiCmd = cmd

		// Monitor the process in background; only touches shared state
		// under the mutex.
		go func() {
			waitErr := cmd.Wait()
			logFile.Close()
			guiMu.Lock()
			guiError = waitErr
			guiMu.Unlock()
		}()
	}

	guiMu.Unlock()

	// Wait for GUI to be ready (socket available). The mutex is NOT held
	// during the sleep so concurrent callers don't block each other and
	// the background monitor goroutine can update guiError freely.
	logPath := filepath.Join(os.TempDir(), "nexus-gui.log")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)

		guiMu.Lock()
		startErr := guiError
		guiMu.Unlock()

		if startErr != nil {
			logData, _ := os.ReadFile(logPath)
			return fmt.Errorf("nexus-gui crashed: %v\n%s", startErr, string(logData))
		}
		if ipc.Probe() {
			return nil
		}
	}

	return fmt.Errorf("nexus-gui did not become ready (check %s)", logPath)
}

// findGUIBinary locates the nexus-gui binary.
func findGUIBinary() (string, error) {
	// Check adjacent to the current executable
	selfPath, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(selfPath)
		for _, name := range []string{"nexus-gui", "nexus-gui.exe"} {
			candidate := filepath.Join(dir, name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}

	// Check PATH
	path, err := exec.LookPath("nexus-gui")
	if err == nil {
		return path, nil
	}

	return "", fmt.Errorf("nexus-gui binary not found; build it with: go build ./cmd/nexus-gui")
}
