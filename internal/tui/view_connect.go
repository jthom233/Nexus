package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dr4zz/nexus/internal/termcap"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dr4zz/nexus/internal/audit"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/launcher"
	"github.com/dr4zz/nexus/internal/session"
	"github.com/dr4zz/nexus/internal/vault"
)

func (a *App) logAuditEvent(ev audit.AuditEvent) {
	if a.auditLog == nil {
		return
	}
	if err := a.auditLog.Log(ev); err != nil {
		a.log.warn("Audit log write failed: %v", err)
	}
}

func (a *App) trackConnectionUsage(connID string) tea.Cmd {
	for i := range a.cfg.Connections {
		if a.cfg.Connections[i].ID == connID {
			now := time.Now()
			a.cfg.Connections[i].LastConnectedAt = &now
			a.cfg.Connections[i].ConnectCount++
			cfg := a.cfg
			return func() tea.Msg {
				if err := config.Save(cfg); err != nil {
					return saveErrorMsg{err}
				}
				return nil
			}
		}
	}
	return nil
}

// toggleFavorite toggles the Favorite flag on the currently selected connection.
func (a App) toggleFavorite() (tea.Model, tea.Cmd) {
	c := a.list.selectedConnection()
	if c == nil {
		return a, nil
	}

	// Toggle on the canonical config.Connections slice
	for i := range a.cfg.Connections {
		if a.cfg.Connections[i].ID == c.ID {
			a.cfg.Connections[i].Favorite = !a.cfg.Connections[i].Favorite
			newState := a.cfg.Connections[i].Favorite
			if err := config.Save(a.cfg); err != nil {
				a.log.error("Failed to save favorite toggle: %v", err)
				a.statusBar.setFlash("Error saving config", flashError)
				return a, scheduleFlashClear()
			}
			label := "unfavorited"
			if newState {
				label = "favorited"
			}
			a.log.info("Connection %s %s", c.Name, label)
			a.statusBar.setFlash(c.Name+" "+label, flashInfo)
			break
		}
	}

	// Refresh the list to reflect the change
	a.list.filtered = a.list.groupFilteredConns()
	a.list.applyViewMode()
	a.list.rebuildTable()
	a.syncHeaderView()
	a.syncCursorPosition()

	return a, scheduleFlashClear()
}

// toggleGhost toggles auto_connect (ghost session) on the selected connection.
// When auto_connect is enabled the GhostManager starts a persistent background
// session immediately; when disabled the existing ghost session is killed.
func (a App) toggleGhost() (tea.Model, tea.Cmd) {
	c := a.list.selectedConnection()
	if c == nil {
		a.statusBar.setFlash("No connection selected", flashError)
		return a, scheduleFlashClear()
	}
	if c.Protocol != config.ProtoSSH {
		a.statusBar.setFlash("Ghost sessions require SSH protocol", flashError)
		return a, scheduleFlashClear()
	}

	for i := range a.cfg.Connections {
		if a.cfg.Connections[i].ID != c.ID {
			continue
		}
		a.cfg.Connections[i].AutoConnect = !a.cfg.Connections[i].AutoConnect
		newState := a.cfg.Connections[i].AutoConnect
		conn := a.cfg.Connections[i]

		if err := config.Save(a.cfg); err != nil {
			a.log.error("Failed to save ghost toggle: %v", err)
			a.statusBar.setFlash("Error saving config", flashError)
			return a, scheduleFlashClear()
		}

		if newState {
			// Start the ghost session immediately.
			if a.ghostMgr != nil {
				a.ghostMgr.StartConn(conn)
			}
			a.log.info("Ghost enabled for %s", conn.Name)
			a.statusBar.setFlash(conn.Name+": ghost enabled", flashInfo)
		} else {
			// Kill the existing ghost session.
			if a.ghostMgr != nil {
				a.ghostMgr.StopConn(conn.ID)
			}
			a.log.info("Ghost disabled for %s", conn.Name)
			a.statusBar.setFlash(conn.Name+": ghost disabled", flashInfo)
		}
		break
	}

	return a, scheduleFlashClear()
}

// connectByID launches a connection by its ID.
func (a App) connectByID(id string) (tea.Model, tea.Cmd) {
	c := a.cfg.FindConnection(id)
	if c == nil {
		return a, nil
	}

	saveCmd := a.trackConnectionUsage(c.ID)

	if c.Protocol == config.ProtoSSH {
		m, connectCmd := a.connectManaged(*c)
		return m, tea.Batch(connectCmd, saveCmd)
	}

	// Resolve credentials from profile before launching non-SSH connections.
	conn := *c
	a.resolveProfileCredentials(&conn)

	l, err := launcher.ForProtocol(conn.Protocol)
	if err != nil {
		a.log.error("Unsupported protocol %s for %s: %v", conn.Protocol, conn.Name, err)
		a.statusBar.setFlash(err.Error(), flashError)
		return a, scheduleFlashClear()
	}

	cmdStr := l.Command(conn)
	a.log.info("Connecting to %s (%s) via %s", conn.Name, conn.HostPort(), conn.Protocol.Label())
	a.log.info("Command: %s", cmdStr)
	a.statusBar.setFlash("Connecting to "+conn.Name+"...", flashInfo)
	return a, tea.Batch(l.Launch(conn), saveCmd)
}

func (a App) connectSelected() (tea.Model, tea.Cmd) {
	c := a.list.selectedConnection()
	if c == nil {
		return a, nil
	}

	saveCmd := a.trackConnectionUsage(c.ID)

	if c.Protocol == config.ProtoSSH {
		m, connectCmd := a.connectManaged(*c)
		return m, tea.Batch(connectCmd, saveCmd)
	}

	// Resolve credentials from profile before launching non-SSH connections.
	conn := *c
	a.resolveProfileCredentials(&conn)

	l, err := launcher.ForProtocol(conn.Protocol)
	if err != nil {
		a.log.error("Unsupported protocol %s for %s: %v", conn.Protocol, conn.Name, err)
		a.statusBar.setFlash(err.Error(), flashError)
		return a, scheduleFlashClear()
	}

	cmdStr := l.Command(conn)
	a.log.info("Connecting to %s (%s) via %s", conn.Name, conn.HostPort(), conn.Protocol.Label())
	a.log.info("Command: %s", cmdStr)
	a.statusBar.setFlash("Connecting to "+conn.Name+"...", flashInfo)
	return a, tea.Batch(l.Launch(conn), saveCmd)
}

func (a App) resolveProfileCredentials(c *config.Connection) {
	if a.profileStore == nil {
		a.log.warn("Credential vault not available — skipping profile resolution")
		return
	}
	resolved, sources, err := vault.ResolveCredentials(*c, a.profileStore)
	if err != nil {
		a.log.warn("Credential resolve error for %s: %v", c.Name, err)
		return
	}
	if resolved == nil {
		return
	}
	if resolved.Username != "" {
		c.Username = resolved.Username
	}
	if resolved.Password != "" {
		c.Password = resolved.Password
	}
	if resolved.IdentityFile != "" {
		c.IdentityFile = resolved.IdentityFile
	}
	if resolved.Domain != "" {
		c.Domain = resolved.Domain
	}
	if resolved.VNCPassword != "" {
		c.VNCPassword = resolved.VNCPassword
	}
	// Audit log when a profile credential was used.
	hasProfileSource := false
	for _, src := range sources {
		if src != "direct" {
			hasProfileSource = true
			a.logAuditEvent(audit.AuditEvent{
				EventType:      audit.EventCredentialUse,
				ConnectionID:   c.ID,
				ConnectionName: c.Name,
			})
			break
		}
	}
	// Log what was resolved from profiles (skip if everything is direct or missing).
	if hasProfileSource {
		var sourceParts []string
		for _, field := range []string{"username", "password", "identity_file", "domain", "vnc_password"} {
			if src, ok := sources[field]; ok && src != "direct" {
				sourceParts = append(sourceParts, fmt.Sprintf("%s from %s", field, src))
			}
		}
		if len(sourceParts) > 0 {
			a.log.info("Resolved credentials for %s: %s", c.Name, strings.Join(sourceParts, ", "))
		}
	}
}

func (a App) connectManaged(c config.Connection) (tea.Model, tea.Cmd) {
	// Resolve credentials from profile (group-profile then named-profile then inline).
	a.resolveProfileCredentials(&c)

	managed := session.NewManagedSession(session.ManagedSessionOptions{
		ID:           "",
		Name:         c.Name,
		ConnID:       c.ID,
		Protocol:     string(c.Protocol),
		Host:         c.Host,
		Port:         c.EffectivePort(),
		Username:     c.Username,
		Password:     c.Password,
		IdentityFile: c.IdentityFile,
		ProxyJump:    c.ProxyJump,
		ProxyCommand: c.ProxyCommand,
	})
	managed.PortForwards = c.PortForwards
	if !c.Hooks.IsEmpty() {
		managed.SetHooks(c.Hooks)
	}
	a.sessions.Add(managed)
	a.updateSessionCount()

	a.log.info("Connecting to %s (%s) via managed %s [%s]", c.Name, c.HostPort(), c.Protocol.Label(), managed.ID)
	a.logAuditEvent(audit.AuditEvent{
		ConnectionID:   c.ID,
		ConnectionName: c.Name,
		EventType:      audit.EventConnect,
		Details:        fmt.Sprintf("%s %s via %s", c.Protocol.Label(), c.HostPort(), managed.ID),
	})
	a.statusBar.setFlash("Connecting to "+c.Name+"...", flashInfo)

	sessID := managed.ID
	connID := c.ID
	connName := c.Name

	return a, tea.Exec(managed, func(err error) tea.Msg {
		if errors.Is(err, session.ErrDetached) {
			return SessionDetachedMsg{
				SessionID: sessID,
				ConnID:    connID,
				ConnName:  connName,
			}
		}
		// Session ended (exit or error) — clean up via LaunchFinishedMsg
		return launcher.LaunchFinishedMsg{ID: connID, Err: err}
	})
}

func (a App) reattachSelected() (tea.Model, tea.Cmd) {
	sess := a.sessionsView.selectedSession()
	if sess == nil {
		return a, nil
	}

	if !sess.IsAlive() {
		a.statusBar.setFlash("Session is no longer alive", flashError)
		return a, scheduleFlashClear()
	}

	return a.reattachSession(sess)
}

func (a App) reattachSession(sess *session.ManagedSession) (tea.Model, tea.Cmd) {
	a.log.info("Reattaching to session %s (%s)", sess.ID, sess.Name)
	a.statusBar.setFlash("Reattaching to "+sess.Name+"...", flashInfo)

	sessID := sess.ID
	connID := sess.ConnID
	connName := sess.Name

	return a, tea.Exec(sess, func(err error) tea.Msg {
		if errors.Is(err, session.ErrDetached) {
			return SessionDetachedMsg{
				SessionID: sessID,
				ConnID:    connID,
				ConnName:  connName,
			}
		}
		// Session ended
		return launcher.LaunchFinishedMsg{ID: connID, Err: err}
	})
}

// handleQuickConnect processes a QuickConnectMsg from the quick connect prompt.
func (a App) handleQuickConnect(msg QuickConnectMsg) (tea.Model, tea.Cmd) {
	return a.launchQuickConnect(msg.Conn)
}

// launchQuickConnect launches a connection from a quick connect target.
func (a App) launchQuickConnect(conn config.Connection) (tea.Model, tea.Cmd) {
	a.log.info("Quick connect: %s (%s://%s:%d)", conn.Name, conn.Protocol, conn.Host, conn.Port)

	if conn.Protocol == config.ProtoSSH {
		return a.connectManaged(conn)
	}

	l, err := launcher.ForProtocol(conn.Protocol)
	if err != nil {
		a.log.error("Unsupported protocol %s: %v", conn.Protocol, err)
		a.statusBar.setFlash(err.Error(), flashError)
		return a, scheduleFlashClear()
	}

	a.statusBar.setFlash("Connecting to "+conn.Name+"...", flashInfo)
	return a, l.Launch(conn)
}

func (a App) yankCommand() (tea.Model, tea.Cmd) {
	c := a.list.selectedConnection()
	if c == nil {
		return a, nil
	}

	l, err := launcher.ForProtocol(c.Protocol)
	if err != nil {
		return a, nil
	}

	cmd := l.Command(*c)
	if err := termcap.DefaultClipboard().WriteAll(cmd); err != nil {
		a.log.error("Clipboard error: %v", err)
		a.statusBar.setFlash("Clipboard error: "+err.Error(), flashError)
	} else {
		a.log.info("Copied to clipboard: %s", cmd)
		a.statusBar.setFlash("Copied: "+cmd, flashInfo)
	}
	return a, scheduleFlashClear()
}
