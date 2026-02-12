package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/health"
	"github.com/dr4zz/nexus/internal/launcher"
	"github.com/dr4zz/nexus/internal/session"
)

type viewKind int

const (
	viewList viewKind = iota
	viewDetail
	viewForm
	viewLog
	viewSessions
)

// App is the root bubbletea model.
type App struct {
	cfg    *config.Config
	keys   KeyMap
	width  int
	height int

	// Sub-models
	header       headerModel
	statusBar    statusBarModel
	list         listModel
	filter       filterModel
	command      commandModel
	detail       detailModel
	form         *formModel
	help         helpModel
	confirm      confirmModel
	log          *logModel
	sessions     *SessionManager
	sessionsView sessionsViewModel

	// View stack
	viewStack []viewKind

	// State
	ready bool
}

// NewApp creates the root application model.
func NewApp(cfg *config.Config) App {
	l := newLog()
	l.info("Nexus started")
	l.info("Loaded %d connections from config", len(cfg.Connections))
	return App{
		cfg:          cfg,
		keys:         DefaultKeyMap(),
		header:       newHeader(),
		statusBar:    newStatusBar(),
		list:         newList(cfg),
		filter:       newFilter(),
		command:      newCommand(),
		detail:       newDetail(),
		form:         newFormPtr(cfg.GroupNames()),
		help:         newHelp(),
		confirm:      newConfirm(),
		log:          l,
		sessions:     NewSessionManager(),
		sessionsView: newSessionsView(),
		viewStack:    []viewKind{viewList},
	}
}

func (a App) currentView() viewKind {
	if len(a.viewStack) == 0 {
		return viewList
	}
	return a.viewStack[len(a.viewStack)-1]
}

func (a *App) pushView(v viewKind) {
	a.viewStack = append(a.viewStack, v)
	a.updateBreadcrumbs()
}

func (a *App) popView() {
	if len(a.viewStack) > 1 {
		a.viewStack = a.viewStack[:len(a.viewStack)-1]
	}
	a.updateBreadcrumbs()
}

func (a *App) updateBreadcrumbs() {
	crumbs := []string{}
	for _, v := range a.viewStack {
		switch v {
		case viewList:
			if a.list.groupFilter != "" {
				crumbs = append(crumbs, a.list.groupFilter)
			} else {
				crumbs = append(crumbs, "All Connections")
			}
		case viewDetail:
			if c := a.list.selectedConnection(); c != nil {
				crumbs = append(crumbs, c.Name)
			} else {
				crumbs = append(crumbs, "Detail")
			}
		case viewForm:
			if a.form.isEdit {
				crumbs = append(crumbs, "Edit Connection")
			} else {
				crumbs = append(crumbs, "Add Connection")
			}
		case viewLog:
			crumbs = append(crumbs, "Event Log")
		case viewSessions:
			crumbs = append(crumbs, "Sessions")
		}
	}
	a.header.breadcrumbs = crumbs
}

func (a *App) updateSessionCount() {
	count := a.sessions.Count()
	a.statusBar.sessionCount = count
	a.header.sessionCount = count
}

func (a App) Init() tea.Cmd {
	return tea.Batch(
		health.CheckAll(a.list.healthTargets()),
		health.ScheduleTick(a.cfg.Settings.HealthInterval()),
	)
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.layout()
		return a, nil

	case tea.KeyMsg:
		// Global key handling that bypasses sub-models
		if a.confirm.active {
			var cmd tea.Cmd
			a.confirm, cmd = a.confirm.Update(msg)
			return a, cmd
		}

		if a.help.active {
			if msg.String() == "?" || msg.String() == "esc" || msg.String() == "q" {
				a.help.toggle()
			}
			return a, nil
		}

		if a.filter.active {
			switch msg.String() {
			case "esc":
				a.filter.deactivate()
				a.list.applyFilter("")
				return a, nil
			case "enter":
				a.filter.active = false
				a.filter.input.Blur()
				return a, nil
			default:
				var cmd tea.Cmd
				a.filter, cmd = a.filter.Update(msg)
				a.list.applyFilter(a.filter.value())
				return a, cmd
			}
		}

		if a.command.active {
			var cmd tea.Cmd
			a.command, cmd = a.command.Update(msg)
			return a, cmd
		}

		if a.currentView() == viewForm && a.form.active {
			cmd := a.form.Update(msg)
			return a, cmd
		}

		if a.currentView() == viewLog {
			return a.handleLogKey(msg)
		}

		return a.handleKey(msg)

	case health.ResultMsg:
		a.list.updateHealthResults(msg.Results)
		total, online, offline := a.list.countsByStatus()
		a.statusBar.total = total
		a.statusBar.online = online
		a.statusBar.offline = offline
		return a, nil

	case health.TickMsg:
		return a, tea.Batch(
			health.CheckAll(a.list.healthTargets()),
			health.ScheduleTick(a.cfg.Settings.HealthInterval()),
		)

	case launcher.LaunchFinishedMsg:
		if msg.Err != nil {
			fullErr := msg.Err.Error()
			// Log the full error
			a.log.error("Session failed for %s:\n%s", msg.ID, fullErr)
			// Truncate for status bar
			errStr := fullErr
			if idx := strings.IndexByte(errStr, '\n'); idx > 0 {
				errStr = errStr[:idx]
			}
			if len(errStr) > 100 {
				errStr = errStr[:100] + "..."
			}
			a.statusBar.setFlash("Error: "+errStr+" (L:logs)", flashError)
		} else {
			a.log.info("Session ended for %s", msg.ID)
			a.statusBar.setFlash("Session ended", flashInfo)
		}
		cmds = append(cmds, scheduleFlashClear())
		return a, tea.Batch(cmds...)

	case SessionDetachedMsg:
		a.log.info("Session %s detached (%s)", msg.SessionID, msg.ConnName)
		a.statusBar.setFlash(fmt.Sprintf("Session detached [s:sessions]"), flashInfo)
		a.updateSessionCount()
		cmds = append(cmds, scheduleFlashClear())
		// Start watching for session death
		if sess := a.sessions.Get(msg.SessionID); sess != nil {
			cmds = append(cmds, watchSessionDone(sess))
		}
		return a, tea.Batch(cmds...)

	case SessionDiedMsg:
		a.log.info("Session %s ended (%s)", msg.SessionID, msg.ConnName)
		a.sessions.Remove(msg.SessionID)
		a.updateSessionCount()
		a.statusBar.setFlash(fmt.Sprintf("Session %s ended", msg.ConnName), flashInfo)
		// Rebuild sessions view if currently visible
		if a.currentView() == viewSessions {
			a.sessionsView.setSessions(a.sessions.All())
		}
		cmds = append(cmds, scheduleFlashClear())
		return a, tea.Batch(cmds...)

	case FlashExpireMsg:
		a.statusBar.clearFlash()
		return a, nil

	case CommandMsg:
		return a.handleCommand(msg)

	case FormSubmitMsg:
		return a.handleFormSubmit(msg)

	case FormCancelMsg:
		a.popView()
		return a, nil

	case ConfirmResultMsg:
		return a.handleConfirmResult(msg)
	}

	// Pass through to active view
	switch a.currentView() {
	case viewList:
		var cmd tea.Cmd
		a.list, cmd = a.list.Update(msg)
		cmds = append(cmds, cmd)
	case viewDetail:
		var cmd tea.Cmd
		a.detail, cmd = a.detail.Update(msg)
		cmds = append(cmds, cmd)
	case viewForm:
		cmd := a.form.Update(msg)
		cmds = append(cmds, cmd)
	case viewLog:
		cmd := a.log.Update(msg)
		cmds = append(cmds, cmd)
	case viewSessions:
		var cmd tea.Cmd
		a.sessionsView, cmd = a.sessionsView.Update(msg)
		cmds = append(cmds, cmd)
	}

	return a, tea.Batch(cmds...)
}

func (a App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch a.currentView() {
	case viewList:
		return a.handleListKey(msg)
	case viewDetail:
		return a.handleDetailKey(msg)
	case viewSessions:
		return a.handleSessionsKey(msg)
	}
	return a, nil
}

func (a App) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return a, tea.Quit
	case "?":
		a.help.view = "list"
		a.help.toggle()
		return a, nil
	case "/":
		a.filter.activate()
		return a, a.filter.input.Focus()
	case ":":
		a.command.activate()
		return a, a.command.input.Focus()
	case "enter":
		return a.connectSelected()
	case "a":
		a.form.startAdd(a.cfg.GroupNames())
		a.form.width = a.width
		a.form.height = a.contentHeight()
		a.pushView(viewForm)
		a.help.view = "form"
		return a, a.form.form.Init()
	case "e":
		if c := a.list.selectedConnection(); c != nil {
			a.form.startEdit(*c, a.cfg.GroupNames())
			a.form.width = a.width
			a.form.height = a.contentHeight()
			a.pushView(viewForm)
			a.help.view = "form"
			return a, a.form.form.Init()
		}
		return a, nil
	case "d":
		if c := a.list.selectedConnection(); c != nil {
			a.confirm.show(
				"Delete connection '"+c.Name+"'?",
				"delete",
				c.ID,
			)
			a.confirm.width = a.width
			a.confirm.height = a.height
		}
		return a, nil
	case "D":
		if c := a.list.selectedConnection(); c != nil {
			st := a.list.statuses[c.ID]
			latStr := ""
			if st.status == health.Online || st.status == health.Degraded {
				latStr = st.latency.String()
			}
			a.detail.setConnection(c, st.status, latStr)
			a.detail.setSize(a.width, a.contentHeight())
			a.pushView(viewDetail)
			a.help.view = "detail"
		}
		return a, nil
	case "g":
		a.list.cycleGroup()
		a.updateBreadcrumbs()
		return a, nil
	case "r":
		a.log.info("Manual health check refresh")
		a.statusBar.setFlash("Refreshing...", flashInfo)
		return a, tea.Batch(
			health.CheckAll(a.list.healthTargets()),
			scheduleFlashClear(),
		)
	case "y":
		return a.yankCommand()
	case "s":
		a.sessionsView.setSessions(a.sessions.All())
		a.sessionsView.setSize(a.width, a.contentHeight())
		a.pushView(viewSessions)
		a.help.view = "sessions"
		return a, nil
	case "L":
		a.log.setSize(a.width, a.contentHeight())
		a.pushView(viewLog)
		return a, nil
	}

	// Pass navigation keys to table
	var cmd tea.Cmd
	a.list, cmd = a.list.Update(msg)
	return a, cmd
}

func (a App) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return a, tea.Quit
	case "?":
		a.help.view = "detail"
		a.help.toggle()
		return a, nil
	case "esc":
		a.popView()
		a.help.view = "list"
		return a, nil
	case "enter":
		return a.connectSelected()
	case "e":
		if c := a.list.selectedConnection(); c != nil {
			a.form.startEdit(*c, a.cfg.GroupNames())
			a.form.width = a.width
			a.form.height = a.contentHeight()
			a.pushView(viewForm)
			a.help.view = "form"
			return a, a.form.form.Init()
		}
		return a, nil
	case "p":
		a.detail.showPassword = !a.detail.showPassword
		a.detail.updateContent()
		return a, nil
	case "s":
		a.sessionsView.setSessions(a.sessions.All())
		a.sessionsView.setSize(a.width, a.contentHeight())
		a.pushView(viewSessions)
		a.help.view = "sessions"
		return a, nil
	case "L":
		a.log.setSize(a.width, a.contentHeight())
		a.pushView(viewLog)
		return a, nil
	}

	var cmd tea.Cmd
	a.detail, cmd = a.detail.Update(msg)
	return a, cmd
}

func (a App) handleSessionsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return a, tea.Quit
	case "?":
		a.help.view = "sessions"
		a.help.toggle()
		return a, nil
	case "esc":
		a.popView()
		a.help.view = "list"
		return a, nil
	case "enter":
		return a.reattachSelected()
	case "d":
		if sess := a.sessionsView.selectedSession(); sess != nil {
			a.confirm.show(
				"Kill session '"+sess.Name+"' ("+sess.ID+")?",
				"kill-session",
				sess.ID,
			)
			a.confirm.width = a.width
			a.confirm.height = a.height
		}
		return a, nil
	case "L":
		a.log.setSize(a.width, a.contentHeight())
		a.pushView(viewLog)
		return a, nil
	}

	// Pass navigation keys to table
	var cmd tea.Cmd
	a.sessionsView, cmd = a.sessionsView.Update(msg)
	return a, cmd
}

func (a App) handleLogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return a, tea.Quit
	case "esc":
		a.popView()
		return a, nil
	}

	// Pass scroll keys to viewport
	cmd := a.log.Update(msg)
	return a, cmd
}

func (a App) connectSelected() (tea.Model, tea.Cmd) {
	c := a.list.selectedConnection()
	if c == nil {
		return a, nil
	}

	// For SSH and Telnet, use managed sessions with detach support
	if c.Protocol == config.ProtoSSH {
		return a.connectManaged(*c)
	}

	l, err := launcher.ForProtocol(c.Protocol)
	if err != nil {
		a.log.error("Unsupported protocol %s for %s: %v", c.Protocol, c.Name, err)
		a.statusBar.setFlash(err.Error(), flashError)
		return a, scheduleFlashClear()
	}

	cmdStr := l.Command(*c)
	a.log.info("Connecting to %s (%s) via %s", c.Name, c.HostPort(), c.Protocol.Label())
	a.log.info("Command: %s", cmdStr)
	a.statusBar.setFlash("Connecting to "+c.Name+"...", flashInfo)
	return a, l.Launch(*c)
}

func (a App) connectManaged(c config.Connection) (tea.Model, tea.Cmd) {
	managed := session.NewManagedSession(
		"", // ID assigned by SessionManager
		c.Name,
		c.ID,
		string(c.Protocol),
		c.Host,
		c.EffectivePort(),
		c.Username,
		c.Password,
		c.IdentityFile,
	)
	a.sessions.Add(managed)
	a.updateSessionCount()

	a.log.info("Connecting to %s (%s) via managed %s [%s]", c.Name, c.HostPort(), c.Protocol.Label(), managed.ID)
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
	if err := clipboard.WriteAll(cmd); err != nil {
		a.log.error("Clipboard error: %v", err)
		a.statusBar.setFlash("Clipboard error: "+err.Error(), flashError)
	} else {
		a.log.info("Copied to clipboard: %s", cmd)
		a.statusBar.setFlash("Copied: "+cmd, flashInfo)
	}
	return a, scheduleFlashClear()
}

func (a App) handleCommand(msg CommandMsg) (tea.Model, tea.Cmd) {
	a.log.info("Command: :%s %s", msg.Name, msg.Args)
	switch msg.Name {
	case "q", "quit":
		return a, tea.Quit
	case "add":
		a.form.startAdd(a.cfg.GroupNames())
		a.form.width = a.width
		a.form.height = a.contentHeight()
		a.pushView(viewForm)
		return a, a.form.form.Init()
	case "connect":
		if c := a.cfg.FindConnection(msg.Args); c != nil {
			if c.Protocol == config.ProtoSSH {
				return a.connectManaged(*c)
			}
			l, err := launcher.ForProtocol(c.Protocol)
			if err != nil {
				a.log.error("Unsupported protocol: %v", err)
				a.statusBar.setFlash(err.Error(), flashError)
				return a, scheduleFlashClear()
			}
			a.log.info("Connecting to %s via command", c.Name)
			return a, l.Launch(*c)
		}
		a.log.warn("Connection not found: %s", msg.Args)
		a.statusBar.setFlash("Connection not found: "+msg.Args, flashError)
		return a, scheduleFlashClear()
	case "edit":
		if c := a.cfg.FindConnection(msg.Args); c != nil {
			a.form.startEdit(*c, a.cfg.GroupNames())
			a.form.width = a.width
			a.form.height = a.contentHeight()
			a.pushView(viewForm)
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
		a.updateBreadcrumbs()
		return a, nil
	case "all":
		a.list.groupFilter = ""
		a.list.applyGroupFilter()
		a.updateBreadcrumbs()
		return a, nil
	case "import-ssh":
		return a.importSSH()
	case "sessions":
		a.sessionsView.setSessions(a.sessions.All())
		a.sessionsView.setSize(a.width, a.contentHeight())
		a.pushView(viewSessions)
		a.help.view = "sessions"
		return a, nil
	case "logs":
		a.log.setSize(a.width, a.contentHeight())
		a.pushView(viewLog)
		return a, nil
	case "help":
		a.help.toggle()
		return a, nil
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

	return a, tea.Batch(
		health.CheckAll(a.list.healthTargets()),
		scheduleFlashClear(),
	)
}

func (a App) handleFormSubmit(msg FormSubmitMsg) (tea.Model, tea.Cmd) {
	var err error
	if msg.IsEdit {
		err = a.cfg.UpdateConnection(msg.Conn)
	} else {
		err = a.cfg.AddConnection(msg.Conn)
	}

	a.popView()

	if err != nil {
		a.log.error("Save failed for %s: %v", msg.Conn.Name, err)
		a.statusBar.setFlash("Save error: "+err.Error(), flashError)
	} else {
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

	return a, tea.Batch(
		health.CheckAll(a.list.healthTargets()),
		scheduleFlashClear(),
	)
}

func (a App) handleConfirmResult(msg ConfirmResultMsg) (tea.Model, tea.Cmd) {
	if !msg.Confirmed {
		a.log.info("Cancelled %s for %s", msg.Action, msg.ID)
		return a, nil
	}

	switch msg.Action {
	case "delete":
		conn := a.cfg.FindConnection(msg.ID)
		name := msg.ID
		if conn != nil {
			name = conn.Name
		}
		if err := a.cfg.DeleteConnection(msg.ID); err != nil {
			a.log.error("Delete failed for %s: %v", name, err)
			a.statusBar.setFlash("Delete error: "+err.Error(), flashError)
		} else {
			a.log.info("Deleted connection: %s", name)
			a.statusBar.setFlash("Deleted "+name, flashInfo)
		}
		a.list.filtered = a.list.groupFilteredConns()
		a.list.rebuildTable()
		total, online, offline := a.list.countsByStatus()
		a.statusBar.total = total
		a.statusBar.online = online
		a.statusBar.offline = offline
		return a, scheduleFlashClear()

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
				a.sessionsView.setSessions(a.sessions.All())
			}
		}
		return a, scheduleFlashClear()
	}

	return a, nil
}

func (a App) View() string {
	if !a.ready {
		return "Loading..."
	}

	headerView := a.header.View()
	statusView := a.statusBar.View()

	var contentView string
	switch a.currentView() {
	case viewList:
		parts := []string{}
		if a.filter.active || a.filter.value() != "" {
			parts = append(parts, a.filter.View())
		}
		if a.command.active {
			parts = append(parts, a.command.View())
		}
		parts = append(parts, a.list.View())
		contentView = lipgloss.JoinVertical(lipgloss.Left, parts...)
	case viewDetail:
		contentView = a.detail.View()
	case viewForm:
		contentView = a.form.View()
	case viewLog:
		contentView = a.log.View()
	case viewSessions:
		contentView = a.sessionsView.View()
	}

	main := lipgloss.JoinVertical(lipgloss.Left, headerView, contentView, statusView)

	// Overlay help or confirm on top
	if a.confirm.active {
		return a.confirm.View()
	}
	if a.help.active {
		return a.help.View()
	}

	return main
}

func (a *App) layout() {
	a.ready = true
	a.header.width = a.width
	a.statusBar.width = a.width
	a.filter.width = a.width
	a.command.width = a.width
	a.help.width = a.width
	a.help.height = a.height
	a.confirm.width = a.width
	a.confirm.height = a.height

	ch := a.contentHeight()
	a.list.setSize(a.width, ch)
	a.detail.setSize(a.width, ch)
	a.log.setSize(a.width, ch)
	a.sessionsView.setSize(a.width, ch)

	total, online, offline := a.list.countsByStatus()
	a.statusBar.total = total
	a.statusBar.online = online
	a.statusBar.offline = offline
	a.updateSessionCount()
}

func (a App) contentHeight() int {
	// header ~ 2 lines, statusbar ~ 2 lines, some padding
	h := a.height - 4
	if h < 5 {
		h = 5
	}
	return h
}

func scheduleFlashClear() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return FlashExpireMsg{}
	})
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
