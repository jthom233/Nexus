package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/health"
	"github.com/dr4zz/nexus/internal/termcap"
	"github.com/dr4zz/nexus/internal/vault"
)

func (a App) executeLeaderAction(action *LeaderAction) (tea.Model, tea.Cmd) {
	switch action.Command {
	// Favorites
	case "toggle-favorite":
		return a.toggleFavorite()

	// Connect
	case "connect-selected":
		return a.connectSelected()
	case "quick-connect":
		a.quickConnect.width = a.width
	a.finder.SetSize(a.width, a.height)
		a.quickConnect.Activate()
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.quickConnect.input.Focus()
	case "add-connection":
		a.form.startAdd(a.cfg.GroupNames(), a.profileNames())
		a.form.width = a.width
		a.form.height = a.contentHeight()
		a.pushView(viewForm)
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.form.form.Init()
	case "edit-connection":
		if c := a.list.selectedConnection(); c != nil {
			conn := *c
			a.inheritGroupProfile(&conn)
			a.form.startEdit(conn, a.cfg.GroupNames(), a.profileNames())
			a.form.width = a.width
			a.form.height = a.contentHeight()
			a.pushView(viewForm)
			a.mode = ModeInsert
			a.statusBar.mode = ModeInsert
			return a, a.form.form.Init()
		}
		a.statusBar.setFlash("No connection selected", flashError)
		return a, scheduleFlashClear()
	case "delete-connection":
		if c := a.list.selectedConnection(); c != nil {
			a.confirm.show("Delete connection '"+c.Name+"'?", "delete", c.ID)
			a.confirm.width = a.width
			a.confirm.height = a.height
			return a, nil
		}
		a.statusBar.setFlash("No connection selected", flashError)
		return a, scheduleFlashClear()
	case "sftp-browser":
		c := a.list.selectedConnection()
		return a.openFileBrowser(c)

	// Find
	case "fuzzy-find":
		a.finder.SetSize(a.width, a.height)
		a.finder.Activate(PickerConnections, a.cfg.Connections)
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.finder.prompt.Focus()
	case "find-by-tag":
		// Filter connections by tags
		a.finder.SetSize(a.width, a.height)
		a.finder.Activate(PickerTags, a.cfg.Connections)
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.finder.prompt.Focus()

	case "find-by-group":
		a.finder.SetSize(a.width, a.height)
		a.finder.Activate(PickerGroups, a.cfg.Connections)
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.finder.prompt.Focus()
	case "find-sessions":
		a.finder.SetSize(a.width, a.height)
		a.finder.Activate(PickerSessions, a.cfg.Connections)
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.finder.prompt.Focus()
	case "find-recent":
		recent := SortByRecent(a.cfg.Connections)
		a.finder.SetSize(a.width, a.height)
		a.finder.Activate(PickerRecent, recent)
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.finder.prompt.Focus()
	case "show-frequent":
		a.list.setViewMode(listViewFrequent)
		a.syncHeaderView()
		a.syncCursorPosition()
		a.log.info("View: frequent")
		a.statusBar.setFlash("Sorted by frequency", flashInfo)
		return a, scheduleFlashClear()
	case "show-favorites":
		a.list.setViewMode(listViewFavorites)
		a.syncHeaderView()
		a.syncCursorPosition()
		a.log.info("View: favorites")
		a.statusBar.setFlash("Showing favorites", flashInfo)
		return a, scheduleFlashClear()
	case "show-all":
		a.list.groupFilter = ""
		a.list.setViewMode(listViewDefault)
		a.syncHeaderView()
		a.syncCursorPosition()
		a.statusBar.setFlash("Showing all connections", flashInfo)
		return a, scheduleFlashClear()

	// Sessions
	case "sessions":
		a.sessionsView.setSessions(a.allSessions())
		a.sessionsView.setSize(a.width, a.contentHeight())
		a.pushView(viewSessions)
		a.help.view = "sessions"
		return a, nil
	case "kill-session-prompt":
		if c := a.list.selectedConnection(); c != nil {
			// Check if there's an active session for this connection
			for _, sess := range a.sessions.All() {
				if sess.ConnID == c.ID {
					sess.Kill()
					a.sessions.Remove(sess.ID)
					a.updateSessionCount()
					a.statusBar.setFlash("Killed session for "+c.Name, flashInfo)
					if a.currentView() == viewSessions {
						a.sessionsView.setSessions(a.allSessions())
					}
					return a, scheduleFlashClear()
				}
			}
			a.statusBar.setFlash("No active session for "+c.Name, flashInfo)
			return a, scheduleFlashClear()
		}
		a.statusBar.setFlash("No connection selected", flashError)
		return a, scheduleFlashClear()
	case "kill-all-sessions":
		all := a.sessions.All()
		count := len(all)
		if count == 0 {
			a.statusBar.setFlash("No active sessions", flashInfo)
			return a, scheduleFlashClear()
		}
		for _, sess := range all {
			sess.Kill()
			a.sessions.Remove(sess.ID)
		}
		a.updateSessionCount()
		a.statusBar.setFlash(fmt.Sprintf("Killed %d session(s)", count), flashInfo)
		if a.currentView() == viewSessions {
			a.sessionsView.setSessions(a.allSessions())
		}
		return a, scheduleFlashClear()
	case "toggle-ghost":
		return a.toggleGhost()

	// Groups
	case "create-group":
		a.command.activate()
		a.command.input.SetValue("mkdir ")
		a.command.input.SetCursor(6)
		a.mode = ModeCommand
		a.statusBar.mode = ModeCommand
		return a, a.command.input.Focus()
	case "group-list":
		a.openGroupListView()
		return a, nil
	case "move-to-group":
		a.command.activate()
		a.command.input.SetValue("mv ")
		a.command.input.SetCursor(3)
		a.mode = ModeCommand
		a.statusBar.mode = ModeCommand
		return a, a.command.input.Focus()
	case "filter-by-group":
		a.finder.SetSize(a.width, a.height)
		a.finder.Activate(PickerGroups, a.cfg.Connections)
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.finder.prompt.Focus()

	// Import
	case "import-ssh":
		return a.importSSH()
	case "import-csv":
		a.command.activate()
		a.command.input.SetValue("import csv ")
		a.command.input.SetCursor(11)
		a.mode = ModeCommand
		a.statusBar.mode = ModeCommand
		return a, a.command.input.Focus()
	case "import-json":
		a.command.activate()
		a.command.input.SetValue("import json ")
		a.command.input.SetCursor(12)
		a.mode = ModeCommand
		a.statusBar.mode = ModeCommand
		return a, a.command.input.Focus()

	// Export
	case "export-all":
		a.command.activate()
		a.command.input.SetValue("export json ")
		a.command.input.SetCursor(12)
		a.mode = ModeCommand
		a.statusBar.mode = ModeCommand
		return a, a.command.input.Focus()
	case "export-selection":
		a.command.activate()
		a.command.input.SetValue("export json ")
		a.command.input.SetCursor(12)
		a.mode = ModeCommand
		a.statusBar.mode = ModeCommand
		return a, a.command.input.Focus()
	case "export-group":
		a.command.activate()
		a.command.input.SetValue("export yaml ")
		a.command.input.SetCursor(12)
		a.mode = ModeCommand
		a.statusBar.mode = ModeCommand
		return a, a.command.input.Focus()

	// Tags
	case "filter-by-tag":
		a.command.activate()
		a.command.input.SetValue("filter -t ")
		a.command.input.SetCursor(10)
		a.mode = ModeCommand
		a.statusBar.mode = ModeCommand
		return a, a.command.input.Focus()
	case "add-tag":
		a.command.activate()
		a.command.input.SetValue("tag add ")
		a.command.input.SetCursor(8)
		a.mode = ModeCommand
		a.statusBar.mode = ModeCommand
		return a, a.command.input.Focus()
	case "remove-tag":
		a.command.activate()
		a.command.input.SetValue("tag remove ")
		a.command.input.SetCursor(11)
		a.mode = ModeCommand
		a.statusBar.mode = ModeCommand
		return a, a.command.input.Focus()

	// View
	case "view-table":
		// Already in table view if on list — just ensure we're there
		if a.currentView() != viewList {
			a.popView()
		}
		return a, nil
	case "view-detail":
		if c := a.list.selectedConnection(); c != nil {
			st := a.list.statuses[c.ID]
			latStr := ""
			if st.status == health.Online || st.status == health.Degraded {
				latStr = st.latency.String()
			}
			a.setDetailConnection(c, st.status, latStr)
			a.detail.setSize(a.width, a.contentHeight())
			a.pushView(viewDetail)
			a.help.view = "detail"
		}
		return a, nil
	case "view-wide":
		a.list.toggleWideMode()
		return a, nil
	case "view-log":
		a.log.setSize(a.width, a.contentHeight())
		a.pushView(viewLog)
		return a, nil

	// Sort
	case "sort-name":
		if idx := a.list.sortKeyToColumnIndex("name"); idx >= 0 {
			a.list.table.CycleSort(idx)
			a.statusBar.setFlash("Sorted by name", flashInfo)
			return a, scheduleFlashClear()
		}
		return a, nil
	case "sort-host":
		if idx := a.list.sortKeyToColumnIndex("host"); idx >= 0 {
			a.list.table.CycleSort(idx)
			a.statusBar.setFlash("Sorted by host", flashInfo)
			return a, scheduleFlashClear()
		}
		return a, nil
	case "sort-group":
		if idx := a.list.sortKeyToColumnIndex("group"); idx >= 0 {
			a.list.table.CycleSort(idx)
			a.statusBar.setFlash("Sorted by group", flashInfo)
			return a, scheduleFlashClear()
		}
		return a, nil
	case "sort-protocol":
		if idx := a.list.sortKeyToColumnIndex("protocol"); idx >= 0 {
			a.list.table.CycleSort(idx)
			a.statusBar.setFlash("Sorted by protocol", flashInfo)
			return a, scheduleFlashClear()
		}
		return a, nil
	case "sort-status":
		if idx := a.list.sortKeyToColumnIndex("status"); idx >= 0 {
			a.list.table.CycleSort(idx)
			a.statusBar.setFlash("Sorted by status", flashInfo)
			return a, scheduleFlashClear()
		}
		return a, nil
	case "sort-latency":
		if idx := a.list.sortKeyToColumnIndex("latency"); idx >= 0 {
			a.list.table.CycleSort(idx)
			a.statusBar.setFlash("Sorted by latency", flashInfo)
			return a, scheduleFlashClear()
		}
		return a, nil
	case "sort-fav":
		if idx := a.list.sortKeyToColumnIndex("fav"); idx >= 0 {
			a.list.table.CycleSort(idx)
			a.statusBar.setFlash("Sorted by favorite", flashInfo)
			return a, scheduleFlashClear()
		}
		return a, nil

	// Health
	case "check-all":
		a.log.info("Manual health check refresh via leader")
		a.statusBar.setFlash("Refreshing health checks...", flashInfo)
		return a, tea.Batch(
			a.checker.CheckAll(a.list.healthTargets()),
			scheduleFlashClear(),
		)
	case "pulse-view":
		a.pulse.SetSize(a.width, a.contentHeight())
		a.pulse.Refresh(a.list.filtered, a.list.statuses, a.sessions.Count())
		a.pushView(viewPulse)
		return a, nil

	// Password
	case "show-password":
		if a.currentView() == viewDetail {
			a.detail.showPassword = !a.detail.showPassword
			a.detail.updateContent()
		} else {
			a.statusBar.setFlash("Open detail view first (D)", flashInfo)
			return a, scheduleFlashClear()
		}
		return a, nil
	case "copy-password":
		if c := a.list.selectedConnection(); c != nil {
			if c.Password != "" {
				if err := termcap.DefaultClipboard().WriteAll(c.Password); err != nil {
					a.statusBar.setFlash("Failed to copy: "+err.Error(), flashError)
					return a, scheduleFlashClear()
				}
				a.statusBar.setFlash("Password copied to clipboard", flashInfo)
				return a, scheduleFlashClear()
			}
			a.statusBar.setFlash("No password set for "+c.Name, flashInfo)
			return a, scheduleFlashClear()
		}
		a.statusBar.setFlash("No connection selected", flashError)
		return a, scheduleFlashClear()

	// Data
	case "set-note":
		a.command.activate()
		a.command.input.SetValue("note ")
		a.command.input.SetCursor(5)
		a.mode = ModeCommand
		a.statusBar.mode = ModeCommand
		return a, a.command.input.Focus()
	case "manage-fields":
		a.command.activate()
		a.command.input.SetValue("field ")
		a.command.input.SetCursor(6)
		a.mode = ModeCommand
		a.statusBar.mode = ModeCommand
		return a, a.command.input.Focus()

	// Options
	case "toggle-health":
		enabled := a.cfg.Settings.HealthEnabled()
		enabled = !enabled
		a.cfg.Settings.HealthCheckEnabled = &enabled
		a.checker.SetEnabled(enabled)
		_ = config.Save(a.cfg)
		if enabled {
			a.statusBar.setFlash("Health monitoring enabled", flashInfo)
			return a, tea.Batch(
				a.checker.CheckAll(a.list.healthTargets()),
				health.ScheduleTick(a.cfg.Settings.HealthInterval()),
				scheduleFlashClear(),
			)
		}
		a.list.clearHealthResults()
		a.statusBar.setFlash("Health monitoring disabled", flashInfo)
		return a, scheduleFlashClear()
	case "keybindings":
		a.statusBar.setFlash("Keybinding editor not yet implemented", flashInfo)
		return a, scheduleFlashClear()

	// Help (direct action from "?")
	case "?":
		a.help.view = "list"
		a.help.toggle()
		return a, nil

	// Window / pane layout actions
	case "split-vertical":
		if a.currentView() != viewPaneLayout {
			a.paneLayout.SetSize(a.width, a.contentHeight())
			a.pushView(viewPaneLayout)
		}
		cmd := a.paneLayout.SplitVertical()
		// Open the connection picker on the newly created pane so the user
		// can immediately pick a connection without an extra Enter.
		if newID := a.paneLayout.LastCreatedPaneID; newID != "" {
			a.paneLayout.OpenPickerForPane(newID, a.cfg.Connections)
		}
		return a, cmd
	case "split-horizontal":
		if a.currentView() != viewPaneLayout {
			a.paneLayout.SetSize(a.width, a.contentHeight())
			a.pushView(viewPaneLayout)
		}
		cmd := a.paneLayout.SplitHorizontal()
		// Open the connection picker on the newly created pane.
		if newID := a.paneLayout.LastCreatedPaneID; newID != "" {
			a.paneLayout.OpenPickerForPane(newID, a.cfg.Connections)
		}
		return a, cmd
	case "focus-left":
		a.paneLayout.FocusDirection(DirLeft)
		return a, nil
	case "focus-right":
		a.paneLayout.FocusDirection(DirRight)
		return a, nil
	case "focus-up":
		a.paneLayout.FocusDirection(DirUp)
		return a, nil
	case "focus-down":
		a.paneLayout.FocusDirection(DirDown)
		return a, nil
	case "close-pane":
		cmd := a.paneLayout.ClosePane()
		return a, cmd
	case "broadcast-toggle":
		cmd := a.paneLayout.ToggleBroadcast()
		a.statusBar.broadcasting = a.paneLayout.IsBroadcasting()
		return a, cmd
	case "equalize-panes":
		a.paneLayout.EqualizeAll()
		return a, nil
	case "zoom-pane":
		a.paneLayout.ZoomToggle()
		return a, nil
	case "resize-right":
		a.paneLayout.ResizeActive(DirRight, 0.05)
		return a, nil
	case "resize-left":
		a.paneLayout.ResizeActive(DirLeft, 0.05)
		return a, nil
	case "resize-down":
		a.paneLayout.ResizeActive(DirDown, 0.05)
		return a, nil
	case "resize-up":
		a.paneLayout.ResizeActive(DirUp, 0.05)
		return a, nil

	// Preset layout actions
	case "preset-2h":
		return a.applyPresetAction("2h")
	case "preset-2v":
		return a.applyPresetAction("2v")
	case "preset-3v":
		return a.applyPresetAction("3v")
	case "preset-2x2":
		return a.applyPresetAction("2x2")
	case "preset-main-side":
		return a.applyPresetAction("main-side")

	// Profile / vault actions
	case "profile-list":
		return a, a.openVaultView()
	case "profile-create":
		a.vaultFormView = newVaultForm(a.cfg.Groups, nil)
		a.vaultFormView.setSize(a.width, a.contentHeight())
		a.pushView(viewVaultForm)
		return a, a.vaultFormView.form.Init()
	case "profile-edit":
		return a, a.openVaultView()
	case "profile-delete":
		return a, a.openVaultView()
	case "profile-assign":
		a.handleProfileAssign()
		return a, nil
	case "profile-save-from":
		a.handleProfileSaveFrom()
		return a, a.vaultFormView.form.Init()
	case "profile-remove":
		a.handleProfileRemove()
		return a, scheduleFlashClear()
	case "profile-who":
		return a, a.openVaultView()

	default:
		a.statusBar.setFlash(fmt.Sprintf("Action: %s", action.Command), flashInfo)
		return a, scheduleFlashClear()
	}
}

// applyPresetAction switches to viewPaneLayout (if not already there) and
// applies a named pane layout preset, then opens a connection picker on every
// new empty pane.
func (a App) applyPresetAction(presetName string) (tea.Model, tea.Cmd) {
	if a.currentView() != viewPaneLayout {
		a.paneLayout.SetSize(a.width, a.contentHeight())
		a.pushView(viewPaneLayout)
	}
	cmd := a.paneLayout.ApplyPreset(presetName)
	// Open the connection picker on every new empty pane.
	for _, p := range a.paneLayout.AllPanes() {
		if p.State == PaneEmpty {
			a.paneLayout.OpenPickerForPane(p.ID, a.cfg.Connections)
		}
	}
	return a, cmd
}

func (a *App) handleProfileAssign() {
	if a.profileStore == nil {
		a.statusBar.setFlash("Vault not available", flashError)
		return
	}
	conn := a.list.selectedConnection()
	if conn == nil {
		a.statusBar.setFlash("No connection selected", flashWarn)
		return
	}
	profiles, err := a.profileStore.List()
	if err != nil || len(profiles) == 0 {
		a.statusBar.setFlash("No profiles available", flashWarn)
		return
	}
	a.refreshVaultView()
	a.pushView(viewVault)
}

// handleProfileSaveFrom pre-populates a new vault form with the selected connection's credentials.
func (a *App) handleProfileSaveFrom() {
	if a.profileStore == nil {
		a.statusBar.setFlash("Vault not available", flashError)
		return
	}
	conn := a.list.selectedConnection()
	if conn == nil {
		a.statusBar.setFlash("No connection selected", flashWarn)
		return
	}
	profile := &vault.CredentialProfile{
		Username:     conn.Username,
		Password:     conn.Password,
		IdentityFile: conn.IdentityFile,
		Domain:       conn.Domain,
		VNCPassword:  conn.VNCPassword,
		Group:        conn.Group,
	}
	a.vaultFormView = newVaultForm(a.cfg.Groups, profile)
	a.vaultFormView.setSize(a.width, a.contentHeight())
	a.vaultFormView.isEdit = false
	a.vaultFormView.editID = ""
	a.pushView(viewVaultForm)
}

// handleProfileRemove clears the credential profile assignment from the selected connection.
func (a *App) handleProfileRemove() {
	conn := a.list.selectedConnection()
	if conn == nil {
		a.statusBar.setFlash("No connection selected", flashWarn)
		return
	}
	if conn.CredentialProfile == "" {
		a.statusBar.setFlash("No profile assigned", flashWarn)
		return
	}
	for i := range a.cfg.Connections {
		if a.cfg.Connections[i].ID == conn.ID {
			a.cfg.Connections[i].CredentialProfile = ""
			break
		}
	}
	_ = config.Save(a.cfg)
	a.statusBar.setFlash("Cred profile removed from '"+conn.Name+"'", flashInfo)
}
