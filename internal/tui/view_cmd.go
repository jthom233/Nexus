package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/importexport"
	"github.com/dr4zz/nexus/internal/launcher"
)

func (a App) handleCommand(msg CommandMsg) (tea.Model, tea.Cmd) {
	a.log.info("Command: :%s %s", msg.Name, msg.Args)
	switch msg.Name {
	case "q", "quit":
		a.confirmQuit()
		return a, nil
	case "add":
		a.form.startAdd(a.cfg.GroupNames(), a.profileNames())
		a.form.width = a.width
		a.form.height = a.contentHeight()
		a.pushView(viewForm)
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.form.form.Init()
	case "connect":
		if c := a.cfg.FindConnection(msg.Args); c != nil {
			if c.Protocol == config.ProtoSSH {
				return a.connectManaged(*c)
			}
			conn := *c
			a.resolveProfileCredentials(&conn)
			l, err := launcher.ForProtocol(conn.Protocol)
			if err != nil {
				a.log.error("Unsupported protocol: %v", err)
				a.statusBar.setFlash(err.Error(), flashError)
				return a, scheduleFlashClear()
			}
			a.log.info("Connecting to %s via command", conn.Name)
			return a, l.Launch(conn)
		}
		// Not found by name — try quick connect parsing
		if msg.Args != "" {
			conn, err := ParseTarget(msg.Args)
			if err == nil {
				a.log.info("Quick connecting to %s via command", msg.Args)
				return a.launchQuickConnect(conn)
			}
		}
		a.log.warn("Connection not found: %s", msg.Args)
		a.statusBar.setFlash("Connection not found: "+msg.Args, flashError)
		return a, scheduleFlashClear()
	case "edit":
		if c := a.cfg.FindConnection(msg.Args); c != nil {
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
		a.statusBar.setFlash("Connection not found: "+msg.Args, flashError)
		return a, scheduleFlashClear()
	case "delete":
		if c := a.cfg.FindConnection(msg.Args); c != nil {
			a.confirm.show("Delete connection '"+c.Name+"'?", "delete", c.ID)
			a.confirm.width = a.width
			a.confirm.height = a.height
			return a, nil
		}
		a.statusBar.setFlash("Connection not found: "+msg.Args, flashError)
		return a, scheduleFlashClear()
	case "group":
		if msg.Args == "" {
			a.list.groupFilter = ""
		} else {
			a.list.groupFilter = msg.Args
		}
		a.list.applyGroupFilter()
		a.syncHeaderView()
		a.syncCursorPosition()
		return a, nil
	case "favorites":
		a.list.setViewMode(listViewFavorites)
		a.syncHeaderView()
		a.syncCursorPosition()
		a.log.info("View: favorites")
		a.statusBar.setFlash("Showing favorites", flashInfo)
		return a, scheduleFlashClear()
	case "recent":
		a.list.setViewMode(listViewRecent)
		a.syncHeaderView()
		a.syncCursorPosition()
		a.log.info("View: recent")
		a.statusBar.setFlash("Sorted by recent", flashInfo)
		return a, scheduleFlashClear()
	case "frequent":
		a.list.setViewMode(listViewFrequent)
		a.syncHeaderView()
		a.syncCursorPosition()
		a.log.info("View: frequent")
		a.statusBar.setFlash("Sorted by frequency", flashInfo)
		return a, scheduleFlashClear()
	case "all":
		a.list.groupFilter = ""
		a.list.setViewMode(listViewDefault)
		a.syncHeaderView()
		a.syncCursorPosition()
		return a, nil
	case "import-ssh":
		return a.importSSH()
	case "sessions":
		a.sessionsView.setSessions(a.allSessions())
		a.sessionsView.setSize(a.width, a.contentHeight())
		a.pushView(viewSessions)
		a.help.view = "sessions"
		return a, nil
	case "pulse":
		a.pulse.SetSize(a.width, a.contentHeight())
		a.pulse.Refresh(a.list.filtered, a.list.statuses, a.sessions.Count())
		a.pushView(viewPulse)
		return a, nil
	case "log":
		a.log.setSize(a.width, a.contentHeight())
		a.pushView(viewLog)
		return a, nil
	case "help":
		a.help.toggle()
		return a, nil
	case "tag":
		return a.handleTagCommand(msg.Args)
	case "tags":
		return a.handleTagsListCommand()
	case "filter":
		return a.handleFilterCommand(msg.Args)
	case "sort":
		return a.handleSortCommand(msg.Args)
	case "note":
		return a.handleNoteCommand(msg.Args)
	case "field":
		return a.handleFieldCommand(msg.Args)
	case "import":
		return a.handleImportCommand(msg.Args)
	case "export":
		return a.handleExportCommand(msg.Args)
	case "mkdir":
		if msg.Args == "" {
			a.statusBar.setFlash("Usage: :mkdir <name>", flashError)
			return a, scheduleFlashClear()
		}
		if err := a.cfg.AddGroup(msg.Args); err != nil {
			a.statusBar.setFlash(err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		a.log.info("Group created: %s", msg.Args)
		a.statusBar.setFlash("Group created: "+msg.Args, flashInfo)
		return a, scheduleFlashClear()
	case "rmdir":
		if msg.Args == "" {
			a.statusBar.setFlash("Usage: :rmdir <name>", flashError)
			return a, scheduleFlashClear()
		}
		if err := a.cfg.DeleteGroup(msg.Args); err != nil {
			a.statusBar.setFlash(err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		a.log.info("Group deleted: %s", msg.Args)
		a.statusBar.setFlash("Group deleted: "+msg.Args, flashInfo)
		return a, scheduleFlashClear()
	case "layout":
		name := strings.TrimSpace(msg.Args)
		if name == "" {
			a.statusBar.setFlash("Usage: :layout <2h|2v|3v|2x2|main-side>", flashError)
			return a, scheduleFlashClear()
		}
		if !IsValidPreset(name) {
			a.statusBar.setFlash(fmt.Sprintf("Unknown layout: %q — supported: 2h, 2v, 3v, 2x2, main-side", name), flashError)
			return a, scheduleFlashClear()
		}
		return a.applyPresetAction(name)
	case "mv", "move":
		args := strings.TrimSpace(msg.Args)
		if args == "" {
			a.statusBar.setFlash("Usage: :mv [connection] <group>", flashError)
			return a, scheduleFlashClear()
		}

		// Bulk move: if any connections are selected, move all selected connections.
		if a.selectionSet.HasSelection() {
			targetGroup := args
			ids := ResolveVisualIDs(&a.selectionSet, a.list.table.rows)
			if len(ids) == 0 {
				a.statusBar.setFlash("No connections selected", flashError)
				return a, scheduleFlashClear()
			}
			// Auto-create target group if it doesn't exist (no save — we save once after all moves).
			if targetGroup != "" && a.cfg.FindGroup(targetGroup) == nil {
				a.cfg.Groups = append(a.cfg.Groups, config.Group{Name: targetGroup})
			}
			// Snapshot before, execute bulk move, snapshot after.
			var children []Operation
			total := len(ids)
			for _, id := range ids {
				c := a.cfg.FindConnection(id)
				if c == nil || c.Group == targetGroup {
					continue
				}
				before := *c
				c.Group = targetGroup
				after := *c
				children = append(children, Operation{
					Type:   UndoOpEdit,
					ConnID: c.ID,
					Name:   c.Name,
					Before: before,
					After:  after,
				})
			}
			moved := len(children)
			if moved > 0 {
				a.undoStack.PushBatch("bulk move to "+targetGroup, children)
				_ = config.Save(a.cfg)
			}
			clearCmd := a.clearAllSelection()
			a.list.filtered = a.list.groupFilteredConns()
			a.list.rebuildTable()
			a.syncHeaderView()
			a.syncCursorPosition()
			if moved == total {
				a.log.info("Moved %d connections to group %s", moved, targetGroup)
				a.statusBar.setFlash(fmt.Sprintf("Moved %d connections to %s", moved, targetGroup), flashInfo)
			} else {
				a.log.info("Moved %d of %d connections to group %s", moved, total, targetGroup)
				a.statusBar.setFlash(fmt.Sprintf("Moved %d of %d connections to %s", moved, total, targetGroup), flashInfo)
			}
			return a, tea.Batch(clearCmd, scheduleFlashClear())
		}

		// Single-connection move (existing behavior).
		var conn *config.Connection
		var targetGroup string
		// Try to interpret the first word as a connection identifier.
		// If it matches, the remainder is the group name.
		// Otherwise treat the entire string as a group name for the selected connection.
		parts := strings.SplitN(args, " ", 2)
		if len(parts) == 2 {
			if c := a.cfg.FindConnection(parts[0]); c != nil {
				conn = c
				targetGroup = strings.TrimSpace(parts[1])
			}
		}
		if conn == nil {
			// No connection identifier matched — use selected connection, entire arg is the group.
			conn = a.list.selectedConnection()
			if conn == nil {
				a.statusBar.setFlash("No connection selected", flashError)
				return a, scheduleFlashClear()
			}
			targetGroup = args
		}
		before := *conn
		if err := a.cfg.MoveConnection(conn.ID, targetGroup); err != nil {
			a.statusBar.setFlash(err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		after := *conn
		a.undoStack.Push(Operation{
			Type:   UndoOpEdit,
			ConnID: conn.ID,
			Name:   conn.Name,
			Before: before,
			After:  after,
		})
		a.list.filtered = a.list.groupFilteredConns()
		a.list.rebuildTable()
		a.syncHeaderView()
		a.syncCursorPosition()
		a.log.info("Moved %s to group %s", conn.Name, targetGroup)
		a.statusBar.setFlash("Moved "+conn.Name+" to group "+targetGroup, flashInfo)
		return a, scheduleFlashClear()
	case "vault":
		sub := strings.TrimSpace(msg.Args)
		switch {
		case sub == "" || sub == "list":
			return a, a.openVaultView()
		case sub == "add":
			a.vaultFormView = newVaultForm(a.cfg.Groups, nil)
			a.vaultFormView.setSize(a.width, a.contentHeight())
			a.pushView(viewVaultForm)
			return a, a.vaultFormView.form.Init()
		case strings.HasPrefix(sub, "rename "):
			return a.handleVaultRename(strings.TrimPrefix(sub, "rename "))
		default:
			a.statusBar.setFlash("Usage: :vault [list|add|rename <old> <new>]", flashError)
			return a, scheduleFlashClear()
		}
	case "cred":
		sub := strings.TrimSpace(msg.Args)
		switch sub {
		case "", "who":
			return a, a.openVaultView()
		case "save":
			a.handleProfileSaveFrom()
			return a, a.vaultFormView.form.Init()
		case "clear":
			a.handleProfileRemove()
			return a, scheduleFlashClear()
		case "orphans":
			return a.handleCredOrphans()
		default:
			a.statusBar.setFlash("Usage: :cred [save|clear|who|orphans]", flashError)
			return a, scheduleFlashClear()
		}
	case "tutorial":
		a.tutorial.activate()
		a.tutorial.width = a.width
		a.tutorial.height = a.height
		return a, nil
	case "ghost":
		return a.toggleGhost()
	case "sftp":
		// :sftp [connection-name] — open SFTP file browser
		var c *config.Connection
		if msg.Args != "" {
			c = a.cfg.FindConnection(msg.Args)
			if c == nil {
				a.statusBar.setFlash("Connection not found: "+msg.Args, flashError)
				return a, scheduleFlashClear()
			}
		} else {
			c = a.list.selectedConnection()
		}
		return a.openFileBrowser(c)
	default:
		a.log.warn("Unknown command: %s", msg.Name)
		a.statusBar.setFlash("Unknown command: "+msg.Name, flashError)
		return a, scheduleFlashClear()
	}
}

func (a App) importSSH() (tea.Model, tea.Cmd) {
	a.log.info("Importing SSH config...")
	conns, err := config.ImportSSHConfig()
	if err != nil {
		a.log.error("SSH import failed: %v", err)
		a.statusBar.setFlash("SSH import error: "+err.Error(), flashError)
		return a, scheduleFlashClear()
	}

	added := 0
	for _, c := range conns {
		if a.cfg.FindConnection(c.ID) == nil {
			if err := a.cfg.AddConnection(c); err == nil {
				a.log.info("Imported: %s (%s)", c.Name, c.Host)
				added++
			}
		}
	}

	a.list.filtered = a.cfg.Connections
	a.list.rebuildTable()
	a.log.info("SSH import complete: %d new connections", added)
	a.statusBar.setFlash(
		lipgloss.NewStyle().Render("Imported "+itoa(added)+" SSH connections"),
		flashInfo,
	)
	a.syncHeaderView()
	a.syncCursorPosition()

	return a, tea.Batch(
		a.checker.CheckAll(a.list.healthTargets()),
		scheduleFlashClear(),
	)
}

func (a App) handleFormSubmit(msg FormSubmitMsg) (tea.Model, tea.Cmd) {
	var err error
	var beforeSnapshot config.Connection
	if msg.IsEdit {
		// Capture the before-snapshot for undo.
		if existing := a.cfg.FindConnection(msg.Conn.ID); existing != nil {
			beforeSnapshot = *existing
		}
		err = a.cfg.UpdateConnection(msg.Conn)
	} else {
		err = a.cfg.AddConnection(msg.Conn)
	}

	a.popView()
	a.mode = ModeNormal
	a.statusBar.mode = ModeNormal

	if err != nil {
		a.log.error("Save failed for %s: %v", msg.Conn.Name, err)
		a.statusBar.setFlash("Save error: "+err.Error(), flashError)
	} else {
		// Record undo operation on success.
		if msg.IsEdit {
			a.undoStack.Push(Operation{
				Type:   UndoOpEdit,
				ConnID: msg.Conn.ID,
				Name:   beforeSnapshot.Name,
				Before: beforeSnapshot,
				After:  msg.Conn,
			})
		} else {
			a.undoStack.Push(Operation{
				Type:  UndoOpAdd,
				Name:  msg.Conn.Name,
				After: msg.Conn,
			})
		}
		action := "Added"
		if msg.IsEdit {
			action = "Updated"
		}
		a.log.info("%s connection: %s (%s://%s)", action, msg.Conn.Name, msg.Conn.Protocol, msg.Conn.HostPort())
		a.statusBar.setFlash(action+" "+msg.Conn.Name, flashInfo)
	}

	a.list.filtered = a.list.groupFilteredConns()
	a.list.rebuildTable()
	total, online, offline := a.list.countsByStatus()
	a.statusBar.total = total
	a.statusBar.online = online
	a.statusBar.offline = offline
	a.syncHeaderView()
	a.syncCursorPosition()

	return a, tea.Batch(
		a.checker.CheckAll(a.list.healthTargets()),
		scheduleFlashClear(),
	)
}

func (a App) handleConfirmResult(msg ConfirmResultMsg) (tea.Model, tea.Cmd) {
	if !msg.Confirmed {
		a.log.info("Cancelled %s for %s", msg.Action, msg.ID)
		return a, nil
	}

	switch msg.Action {
	case "quit":
		return a, tea.Quit

	case "delete":
		conn := a.cfg.FindConnection(msg.ID)
		name := msg.ID
		var deletedConn config.Connection
		deleteIndex := -1
		if conn != nil {
			name = conn.Name
			deletedConn = *conn
			// Find the index in cfg.Connections for undo reinsertion.
			for i, c := range a.cfg.Connections {
				if c.ID == msg.ID {
					deleteIndex = i
					break
				}
			}
		}
		if err := a.cfg.DeleteConnection(msg.ID); err != nil {
			a.log.error("Delete failed for %s: %v", name, err)
			a.statusBar.setFlash("Delete error: "+err.Error(), flashError)
		} else {
			// Record undo operation on success.
			if deleteIndex >= 0 {
				a.undoStack.Push(Operation{
					Type:   UndoOpDelete,
					ConnID: msg.ID,
					Name:   name,
					Index:  deleteIndex,
					Before: deletedConn,
				})
			}
			a.log.info("Deleted connection: %s", name)
			a.statusBar.setFlash("Deleted "+name, flashInfo)
		}
		a.list.filtered = a.list.groupFilteredConns()
		a.list.rebuildTable()
		total, online, offline := a.list.countsByStatus()
		a.statusBar.total = total
		a.statusBar.online = online
		a.statusBar.offline = offline
		a.syncHeaderView()
		a.syncCursorPosition()
		return a, scheduleFlashClear()

	case "delete-visual":
		// Delete multiple connections from visual selection
		idStr := msg.ID
		ids := strings.Split(idStr, ",")
		var children []Operation
		for _, id := range ids {
			conn := a.cfg.FindConnection(id)
			if conn == nil {
				a.cfg.DeleteConnectionNoSave(id)
				continue
			}
			name := conn.Name
			snapshot := *conn
			idx := -1
			for i, c := range a.cfg.Connections {
				if c.ID == id {
					idx = i
					break
				}
			}
			a.cfg.DeleteConnectionNoSave(id)
			children = append(children, Operation{
				Type:   UndoOpDelete,
				ConnID: id,
				Name:   name,
				Index:  idx,
				Before: snapshot,
			})
			a.log.info("Deleted connection: %s", name)
		}
		if len(children) > 0 {
			a.undoStack.PushBatch("bulk delete", children)
			_ = config.Save(a.cfg)
		}
		a.statusBar.setFlash(fmt.Sprintf("Deleted %d connections", len(children)), flashInfo)

		// Exit visual mode and clear selection (connections no longer exist).
		clearCmd := a.clearAllSelection()

		// Rebuild the list
		a.list.filtered = a.list.groupFilteredConns()
		a.list.rebuildTable()
		total, online, offline := a.list.countsByStatus()
		a.statusBar.total = total
		a.statusBar.online = online
		a.statusBar.offline = offline
		a.syncHeaderView()
		a.syncCursorPosition()
		return a, tea.Batch(clearCmd, scheduleFlashClear())

	case "kill-session":
		sess := a.sessions.Get(msg.ID)
		if sess != nil {
			a.log.info("Killing session %s (%s)", sess.ID, sess.Name)
			sess.Kill()
			a.sessions.Remove(msg.ID)
			a.updateSessionCount()
			a.statusBar.setFlash("Killed session "+sess.Name, flashInfo)
			// Rebuild sessions view
			if a.currentView() == viewSessions {
				a.sessionsView.setSessions(a.allSessions())
			}
		}
		return a, scheduleFlashClear()
	}

	// Handle vault profile deletion confirmation (action = "delete-profile:<name>").
	if strings.HasPrefix(msg.Action, "delete-profile:") && a.profileStore != nil {
		profileName := strings.TrimPrefix(msg.Action, "delete-profile:")
		if err := a.profileStore.Delete(profileName); err != nil {
			a.statusBar.setFlash("Error deleting cred profile: "+err.Error(), flashError)
		} else {
			a.statusBar.setFlash("Cred profile '"+profileName+"' deleted", flashInfo)
			a.refreshVaultView()
		}
		return a, scheduleFlashClear()
	}

	// Handle group deletion confirmation (action = "delete-group:<name>").
	if strings.HasPrefix(msg.Action, "delete-group:") {
		groupName := strings.TrimPrefix(msg.Action, "delete-group:")
		// Unassign all connections from the group
		for i := range a.cfg.Connections {
			if a.cfg.Connections[i].Group == groupName {
				a.cfg.Connections[i].Group = ""
			}
		}
		if err := a.cfg.DeleteGroup(groupName); err != nil {
			a.statusBar.setFlash("Error deleting group: "+err.Error(), flashError)
		} else {
			a.statusBar.setFlash("Group '"+groupName+"' deleted", flashInfo)
			a.refreshGroupListView()
			// Rebuild list in case connections were ungrouped
			a.list.filtered = a.list.groupFilteredConns()
			a.list.rebuildTable()
			a.syncHeaderView()
			a.syncCursorPosition()
		}
		return a, scheduleFlashClear()
	}

	return a, nil
}

// handleFilterCommand processes :filter commands with tag boolean logic.
func (a App) handleFilterCommand(args string) (tea.Model, tea.Cmd) {
	args = strings.TrimSpace(args)
	if args == "" {
		// Clear all filters.
		a.list.filtered = a.list.groupFilteredConns()
		a.list.applyViewMode()
		a.list.rebuildTable()
		a.header.setFilter("")
		a.header.setItemCount(len(a.list.filtered))
		a.syncCursorPosition()
		a.statusBar.setFlash("Filters cleared", flashInfo)
		return a, scheduleFlashClear()
	}

	// Check for -t flag (tag filter with boolean logic).
	if strings.HasPrefix(args, "-t ") {
		expr := strings.TrimPrefix(args, "-t ")
		expr = strings.TrimSpace(expr)
		if expr == "" {
			a.statusBar.setFlash("Usage: :filter -t <tag> [AND|OR <tag>...]", flashError)
			return a, scheduleFlashClear()
		}
		filter := ParseTagFilter(expr)
		base := a.list.groupFilteredConns()
		a.list.filtered = FilterByTags(base, filter)
		a.list.rebuildTable()
		a.header.setFilter("-t " + expr)
		a.header.setItemCount(len(a.list.filtered))
		a.syncCursorPosition()
		opStr := "AND"
		if filter.Op == TagFilterOr {
			opStr = "OR"
		}
		if len(filter.Tags) == 1 {
			a.statusBar.setFlash(fmt.Sprintf("Filtered by tag: %s (%d results)", filter.Tags[0], len(a.list.filtered)), flashInfo)
		} else {
			a.statusBar.setFlash(fmt.Sprintf("Filtered by tags (%s): %s (%d results)", opStr, strings.Join(filter.Tags, ", "), len(a.list.filtered)), flashInfo)
		}
		a.log.info("Tag filter: %s %s -> %d results", opStr, strings.Join(filter.Tags, ", "), len(a.list.filtered))
		return a, scheduleFlashClear()
	}

	// Check for -g flag (group filter).
	if strings.HasPrefix(args, "-g ") {
		group := strings.TrimPrefix(args, "-g ")
		group = strings.TrimSpace(group)
		a.list.groupFilter = group
		a.list.applyGroupFilter()
		a.header.setFilter("-g " + group)
		a.header.setItemCount(len(a.list.filtered))
		a.syncHeaderView()
		a.syncCursorPosition()
		a.statusBar.setFlash(fmt.Sprintf("Filtered by group: %s (%d results)", group, len(a.list.filtered)), flashInfo)
		return a, scheduleFlashClear()
	}

	// Default: fuzzy filter on the pattern.
	a.list.applyFilter(args)
	a.header.setFilter(args)
	a.header.setItemCount(len(a.list.filtered))
	a.syncCursorPosition()
	a.statusBar.setFlash(fmt.Sprintf("Filtered: %s (%d results)", args, len(a.list.filtered)), flashInfo)
	return a, scheduleFlashClear()
}

// handleSortCommand processes :sort commands.
func (a App) handleSortCommand(args string) (tea.Model, tea.Cmd) {
	field := strings.TrimSpace(strings.ToLower(args))
	if field == "" {
		a.statusBar.setFlash("Usage: :sort <field>", flashError)
		return a, scheduleFlashClear()
	}
	if idx := a.list.sortKeyToColumnIndex(field); idx >= 0 {
		a.list.table.CycleSort(idx)
		a.statusBar.setFlash(fmt.Sprintf("Sorted by %s", field), flashInfo)
		return a, scheduleFlashClear()
	}
	a.statusBar.setFlash("Unknown sort field: "+field, flashError)
	return a, scheduleFlashClear()
}

// handleNoteCommand handles the :note command to set notes on the current connection.
func (a App) handleNoteCommand(args string) (tea.Model, tea.Cmd) {
	c := a.list.selectedConnection()
	if c == nil {
		a.statusBar.setFlash("No connection selected", flashError)
		return a, scheduleFlashClear()
	}

	text := strings.TrimSpace(args)
	if text == "" {
		// Clear the note
		c.Notes = ""
		a.log.info("Cleared note for %s", c.Name)
		a.statusBar.setFlash("Note cleared for "+c.Name, flashInfo)
	} else {
		c.Notes = text
		a.log.info("Set note for %s: %s", c.Name, text)
		a.statusBar.setFlash("Note saved for "+c.Name, flashInfo)
	}

	if err := a.cfg.UpdateConnection(*c); err != nil {
		a.log.error("Failed to save note: %v", err)
		a.statusBar.setFlash("Failed to save: "+err.Error(), flashError)
	}

	// Refresh detail view if visible
	if a.currentView() == viewDetail {
		a.setDetailConnection(c, a.detail.status, a.detail.latency)
	}

	return a, scheduleFlashClear()
}

// handleFieldCommand handles :field set <key> <value> and :field remove <key>.
func (a App) handleFieldCommand(args string) (tea.Model, tea.Cmd) {
	c := a.list.selectedConnection()
	if c == nil {
		a.statusBar.setFlash("No connection selected", flashError)
		return a, scheduleFlashClear()
	}

	parts := strings.Fields(args)
	if len(parts) < 2 {
		a.statusBar.setFlash("Usage: :field set <key> <value> | :field remove <key>", flashError)
		return a, scheduleFlashClear()
	}

	switch parts[0] {
	case "set":
		if len(parts) < 3 {
			a.statusBar.setFlash("Usage: :field set <key> <value>", flashError)
			return a, scheduleFlashClear()
		}
		key := parts[1]
		// Value is everything after the key, preserving spaces
		valStart := strings.Index(args, parts[1]) + len(parts[1])
		value := strings.TrimSpace(args[valStart:])
		if c.CustomFields == nil {
			c.CustomFields = make(map[string]string)
		}
		c.CustomFields[key] = value
		a.log.info("Set field %s=%s for %s", key, value, c.Name)
		a.statusBar.setFlash(fmt.Sprintf("Field '%s' set on %s", key, c.Name), flashInfo)

	case "remove", "rm", "del":
		key := parts[1]
		if c.CustomFields == nil || c.CustomFields[key] == "" {
			a.statusBar.setFlash(fmt.Sprintf("Field '%s' not found", key), flashError)
			return a, scheduleFlashClear()
		}
		delete(c.CustomFields, key)
		// Clean up empty map
		if len(c.CustomFields) == 0 {
			c.CustomFields = nil
		}
		a.log.info("Removed field %s from %s", key, c.Name)
		a.statusBar.setFlash(fmt.Sprintf("Field '%s' removed from %s", key, c.Name), flashInfo)

	default:
		a.statusBar.setFlash("Usage: :field set <key> <value> | :field remove <key>", flashError)
		return a, scheduleFlashClear()
	}

	if err := a.cfg.UpdateConnection(*c); err != nil {
		a.log.error("Failed to save field: %v", err)
		a.statusBar.setFlash("Failed to save: "+err.Error(), flashError)
	}

	// Refresh detail view if visible
	if a.currentView() == viewDetail {
		a.setDetailConnection(c, a.detail.status, a.detail.latency)
	}

	return a, scheduleFlashClear()
}

// ---------- Import / Export command handlers ----------

func (a App) handleImportCommand(args string) (tea.Model, tea.Cmd) {
	parts := strings.SplitN(strings.TrimSpace(args), " ", 2)
	if len(parts) < 2 || parts[1] == "" {
		a.statusBar.setFlash("Usage: :import <csv|json> <path>", flashError)
		return a, scheduleFlashClear()
	}

	format := strings.ToLower(parts[0])
	path := strings.TrimSpace(parts[1])

	// Expand ~ to home directory.
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}

	f, err := os.Open(path)
	if err != nil {
		a.log.error("Import open: %v", err)
		a.statusBar.setFlash("Cannot open file: "+err.Error(), flashError)
		return a, scheduleFlashClear()
	}
	defer f.Close()

	var imp importexport.Importer
	switch format {
	case "csv":
		imp = &importexport.CSVImporter{}
	case "json":
		imp = &importexport.JSONImporter{}
	default:
		a.statusBar.setFlash("Unsupported import format: "+format+" (use csv or json)", flashError)
		return a, scheduleFlashClear()
	}

	conns, err := imp.Import(f)
	if err != nil {
		a.log.error("Import parse: %v", err)
		a.statusBar.setFlash("Import error: "+err.Error(), flashError)
		return a, scheduleFlashClear()
	}

	merged, result := importexport.ApplyMerge(a.cfg.Connections, conns, importexport.MergeSkip)
	a.cfg.Connections = merged
	if err := config.Save(a.cfg); err != nil {
		a.log.error("Import save: %v", err)
		a.statusBar.setFlash("Save error: "+err.Error(), flashError)
		return a, scheduleFlashClear()
	}

	a.list.filtered = a.cfg.Connections
	a.list.rebuildTable()
	a.syncHeaderView()
	a.syncCursorPosition()

	msg := fmt.Sprintf("Imported %d created, %d skipped", result.Created, result.Skipped)
	a.log.info("Import %s: %s", format, msg)
	a.statusBar.setFlash(msg, flashInfo)

	return a, tea.Batch(
		a.checker.CheckAll(a.list.healthTargets()),
		scheduleFlashClear(),
	)
}

func (a App) handleExportCommand(args string) (tea.Model, tea.Cmd) {
	parts := strings.SplitN(strings.TrimSpace(args), " ", 2)
	if len(parts) < 2 || parts[1] == "" {
		a.statusBar.setFlash("Usage: :export <csv|json|yaml> <path>", flashError)
		return a, scheduleFlashClear()
	}

	format := strings.ToLower(parts[0])
	path := strings.TrimSpace(parts[1])

	// Expand ~ to home directory.
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}

	// Determine which connections to export.
	var conns []config.Connection
	if a.selectionSet.HasSelection() {
		// Export only selected connections.
		indices := a.selectionSet.SelectedIndices()
		for _, idx := range indices {
			if idx >= 0 && idx < len(a.list.filtered) {
				conns = append(conns, a.list.filtered[idx])
			}
		}
	} else {
		conns = a.list.filtered
	}

	if len(conns) == 0 {
		a.statusBar.setFlash("No connections to export", flashError)
		return a, scheduleFlashClear()
	}

	opts := importexport.ExportOptions{IncludeCredentials: false}

	var exp importexport.Exporter
	switch format {
	case "csv":
		exp = &importexport.CSVExporter{Options: opts}
	case "json":
		exp = &importexport.JSONExporter{Options: opts}
	case "yaml":
		exp = &importexport.YAMLExporter{Options: opts}
	default:
		a.statusBar.setFlash("Unsupported export format: "+format+" (use csv, json, or yaml)", flashError)
		return a, scheduleFlashClear()
	}

	outFile, err := os.Create(path)
	if err != nil {
		a.log.error("Export create: %v", err)
		a.statusBar.setFlash("Cannot create file: "+err.Error(), flashError)
		return a, scheduleFlashClear()
	}
	defer outFile.Close()

	if err := exp.Export(outFile, conns); err != nil {
		a.log.error("Export write: %v", err)
		a.statusBar.setFlash("Export error: "+err.Error(), flashError)
		return a, scheduleFlashClear()
	}

	flashMsg := fmt.Sprintf("Exported %d connections to %s", len(conns), filepath.Base(path))
	a.log.info("Export %s: %s", format, flashMsg)
	a.statusBar.setFlash(flashMsg, flashInfo)

	// Clear selection after export.
	clearCmd := a.clearAllSelection()
	return a, tea.Batch(clearCmd, scheduleFlashClear())
}
