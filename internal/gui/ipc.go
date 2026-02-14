package gui

import (
	"encoding/json"

	"github.com/dr4zz/nexus/internal/ipc"
)

// IPCManager handles IPC communication on the GUI side.
type IPCManager struct {
	server *ipc.Server
	app    *App
}

// NewIPCManager creates a new IPC manager for the GUI process.
func NewIPCManager(app *App) (*IPCManager, error) {
	mgr := &IPCManager{app: app}

	server, err := ipc.NewServer(mgr.handleMessage)
	if err != nil {
		return nil, err
	}
	mgr.server = server

	return mgr, nil
}

// Serve starts the IPC server. Blocks until Close is called.
func (m *IPCManager) Serve() error {
	return m.server.Serve()
}

// Close shuts down the IPC manager.
func (m *IPCManager) Close() error {
	return m.server.Close()
}

// SendTabOpened broadcasts a tab-opened event.
func (m *IPCManager) SendTabOpened(connID string) {
	m.server.Broadcast(ipc.MsgTabOpened, &ipc.TabOpenedEvent{ConnID: connID})
}

// SendTabClosed broadcasts a tab-closed event.
func (m *IPCManager) SendTabClosed(connID string) {
	m.server.Broadcast(ipc.MsgTabClosed, &ipc.TabClosedEvent{ConnID: connID})
}

// SendTabError broadcasts a tab-error event.
func (m *IPCManager) SendTabError(connID, errMsg string) {
	m.server.Broadcast(ipc.MsgTabError, &ipc.TabErrorEvent{
		ConnID: connID,
		Error:  errMsg,
	})
}

func (m *IPCManager) handleMessage(env *ipc.Envelope, reply func(string, interface{}) error) {
	switch env.Type {
	case ipc.MsgOpenTab:
		var cmd ipc.OpenTabCmd
		if err := ipc.DecodePayload(env, &cmd); err != nil {
			return
		}
		// Convert options from JSON
		var opts map[string]interface{}
		if cmd.Options != nil {
			json.Unmarshal(cmd.Options, &opts)
		}
		err := m.app.OpenTab(cmd.ConnID, cmd.Protocol, cmd.Host, cmd.Port,
			cmd.Username, cmd.Password, cmd.Domain, opts, reply)
		// Auto-restore window when a new tab is opened
		m.app.RestoreWindow()
		if err != nil {
			reply(ipc.MsgTabError, &ipc.TabErrorEvent{
				ConnID: cmd.ConnID,
				Error:  err.Error(),
			})
		}

	case ipc.MsgCloseTab:
		var cmd ipc.CloseTabCmd
		if err := ipc.DecodePayload(env, &cmd); err != nil {
			return
		}
		m.app.CloseTab(cmd.ConnID)
		reply(ipc.MsgTabClosed, &ipc.TabClosedEvent{ConnID: cmd.ConnID})

	case ipc.MsgFocusTab:
		var cmd ipc.FocusTabCmd
		if err := ipc.DecodePayload(env, &cmd); err != nil {
			return
		}
		m.app.tabs.Focus(cmd.ConnID)

	case ipc.MsgRestoreWindow:
		m.app.RestoreWindow()
	}
}
