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
	leader       leaderModel
	log          *logModel
	sessions     *SessionManager
	sessionsView sessionsViewModel

	// View stack
	viewStack []viewKind

	// State
	mode  Mode
	ready bool
}

// NewApp creates the root application model.
func NewApp(cfg *config.Config) App {
	l := newLog()
	l.info("Nexus started")
	l.info("Loaded %d connections from config", len(cfg.Connections))

	h := newHeader()
	h.setConfigPath(config.ConfigPath())
	h.setItemCount(len(cfg.Connections))

	return App{
		cfg:          cfg,
		keys:         DefaultKeyMap(),
		header:       h,
		statusBar:    newStatusBar(),
		list:         newList(cfg),
		filter:       newFilter(),
		command:      newCommand(),
		detail:       newDetail(),
		form:         newFormPtr(cfg.GroupNames()),
		help:         newHelp(),
		confirm:      newConfirm(),
		leader:       newLeader(),
		log:          l,
		sessions:     NewSessionManager(),
		sessionsView: newSessionsView(),
		viewStack:    []viewKind{viewList},
		mode:         ModeNormal,
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
	a.syncHeaderView()
	a.syncStatusBarView()
}

func (a *App) popView() {
	if len(a.viewStack) > 1 {
		a.viewStack = a.viewStack[:len(a.viewStack)-1]
	}
	a.syncHeaderView()
	a.syncStatusBarView()
}

func (a *App) syncStatusBarView() {
	switch a.currentView() {
	case viewList:
		a.statusBar.view = "list"
	case viewDetail:
		a.statusBar.view = "detail"
	case viewForm:
		a.statusBar.view = "form"
	case viewLog:
		a.statusBar.view = "log"
	case viewSessions:
		a.statusBar.view = "sessions"
	}
}

func (a *App) syncHeaderView() {
	v := a.currentView()
	switch v {
	case viewList:
		name := "Connections"
		if a.list.groupFilter != "" {
			name = a.list.groupFilter
		}
		a.header.setView(name, 1)
		a.header.setItemCount(len(a.list.filtered))
	case viewDetail:
		name := "Detail"
		if c := a.list.selectedConnection(); c != nil {
			name = "Detail: " + c.Name
		}
		a.header.setView(name, 1)
		a.header.setItemCount(0)
	case viewForm:
		name := "Add Connection"
		if a.form.isEdit {
			name = "Edit Connection"
		}
		a.header.setView(name, 1)
		a.header.setItemCount(0)
	case viewLog:
		a.header.setView("Event Log", 3)
		a.header.setItemCount(0)
	case viewSessions:
		a.header.setView("Sessions", 2)
		a.header.setItemCount(a.sessions.Count())
	}
}

func (a *App) updateSessionCount() {
	count := a.sessions.Count()
	a.statusBar.sessionCount = count
	// Update header item count if currently on sessions view
	if a.currentView() == viewSessions {
		a.header.setItemCount(count)
	}
}

// syncCursorPosition updates the statusbar with the current table cursor position.
func (a *App) syncCursorPosition() {
	switch a.currentView() {
	case viewList:
		a.statusBar.cursor = a.list.table.Cursor() + 1 // 1-based
		a.statusBar.itemCount = len(a.list.table.rows)
	case viewSessions:
		a.statusBar.cursor = a.sessionsView.table.Cursor() + 1
		a.statusBar.itemCount = len(a.sessionsView.sessions)
	default:
		a.statusBar.cursor = 0
		a.statusBar.itemCount = 0
	}
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

		// Handle leader key input
		if a.leader.active {
			k := msg.String()
			if k == "esc" {
				a.leader.dismiss()
				return a, nil
			}
			action := a.leader.handleKey(k)
			if action != nil {
				a.leader.dismiss()
				return a.executeLeaderAction(action)
			}
			// If handleKey returned nil but didn't dismiss, it navigated to sub-group
			// If it did dismiss (unknown key), just return
			return a, nil
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
				a.header.setFilter("")
				a.header.setItemCount(len(a.list.filtered))
				a.statusBar.mode = ModeNormal
				a.syncCursorPosition()
				return a, a.setMode(ModeNormal)
			case "enter":
				a.filter.active = false
				a.filter.input.Blur()
				a.header.setFilter(a.filter.value())
				a.header.setItemCount(len(a.list.filtered))
				a.statusBar.mode = ModeNormal
				a.syncCursorPosition()
				return a, a.setMode(ModeNormal)
			default:
				var cmd tea.Cmd
				a.filter, cmd = a.filter.Update(msg)
				a.list.applyFilter(a.filter.value())
				a.header.setFilter(a.filter.value())
				a.header.setItemCount(len(a.list.filtered))
				a.syncCursorPosition()
				return a, cmd
			}
		}

		if a.command.active {
			var cmd tea.Cmd
			a.command, cmd = a.command.Update(msg)
			if !a.command.active {
				// Command was deactivated (esc or enter)
				a.statusBar.mode = ModeNormal
				modeCmd := a.setMode(ModeNormal)
				if cmd != nil {
					return a, tea.Batch(cmd, modeCmd)
				}
				return a, modeCmd
			}
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
		a.syncCursorPosition()
		return a, nil

	case health.TickMsg:
		return a, tea.Batch(
			health.CheckAll(a.list.healthTargets()),
			health.ScheduleTick(a.cfg.Settings.HealthInterval()),
		)

	case launcher.LaunchFinishedMsg:
		// Clean up dead managed sessions
		a.sessions.CleanDead()
		a.updateSessionCount()

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
		// Refresh sessions view so status shows "detached"
		if a.currentView() == viewSessions {
			a.sessionsView.setSessions(a.sessions.All())
		}
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

	case LeaderTimeoutMsg:
		// Timeout expired — popup is already showing, nothing to do.
		// The popup appears on activate; the timeout just keeps it visible.
		return a, nil

	case FlashExpireMsg:
		a.statusBar.clearFlash()
		return a, nil

	case CommandMsg:
		return a.handleCommand(msg)

	case FormSubmitMsg:
		return a.handleFormSubmit(msg)

	case FormCancelMsg:
		a.popView()
		a.statusBar.mode = ModeNormal
		return a, a.setMode(ModeNormal)

	case ConfirmResultMsg:
		return a.handleConfirmResult(msg)

	case ModeChangedMsg:
		a.statusBar.mode = msg.To
		return a, nil
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

func (a *App) confirmQuit() {
	a.confirm.show("Quit Nexus?", "quit", "")
	a.confirm.width = a.width
	a.confirm.height = a.height
}

func (a App) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	motion := a.list.table.motion

	// Capture the pending operator before the engine processes this key,
	// because HandleKey resets the engine upon producing a result.
	pendingOp := motion.PendingOperator()

	// Pass key events to the motion engine first.
	result := motion.HandleKey(msg, a.list.table.cursor, len(a.list.table.rows), a.list.table.height)

	if result != nil {
		// Store the operator that was active when this result was produced.
		a.list.table.lastOperator = pendingOp
		// Motion sequence completed — apply the result.
		return a.applyMotionResult(result)
	}

	// If the engine is now pending (accumulating a sequence), consume the key.
	if motion.Pending() {
		return a, nil
	}

	// --- App-level key handlers (non-motion keys) ---
	switch k {
	case " ": // Space = leader key
		if !a.leader.active {
			cmd := a.leader.activate()
			return a, cmd
		}
	case "q":
		a.confirmQuit()
		return a, nil
	case "?":
		a.help.view = "list"
		a.help.toggle()
		return a, nil
	case "/":
		a.filter.activate()
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.filter.input.Focus()
	case ":":
		a.command.activate()
		a.mode = ModeCommand
		a.statusBar.mode = ModeCommand
		return a, a.command.input.Focus()
	case "enter":
		return a.connectSelected()
	case "a":
		a.form.startAdd(a.cfg.GroupNames())
		a.form.width = a.width
		a.form.height = a.contentHeight()
		a.pushView(viewForm)
		a.help.view = "form"
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.form.form.Init()
	case "e":
		if c := a.list.selectedConnection(); c != nil {
			a.form.startEdit(*c, a.cfg.GroupNames())
			a.form.width = a.width
			a.form.height = a.contentHeight()
			a.pushView(viewForm)
			a.help.view = "form"
			a.mode = ModeInsert
			a.statusBar.mode = ModeInsert
			return a, a.form.form.Init()
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
	case "r":
		a.log.info("Manual health check refresh")
		a.statusBar.setFlash("Refreshing...", flashInfo)
		return a, tea.Batch(
			health.CheckAll(a.list.healthTargets()),
			scheduleFlashClear(),
		)
	case "s":
		a.sessionsView.setSessions(a.sessions.All())
		a.sessionsView.setSize(a.width, a.contentHeight())
		a.pushView(viewSessions)
		a.help.view = "sessions"
		return a, nil

	// Sort keys (Shift+letter)
	case "N": // Sort by Name
		if idx := a.list.sortKeyToColumnIndex("name"); idx >= 0 {
			a.list.table.CycleSort(idx)
		}
		return a, nil
	case "S": // Sort by Status
		if idx := a.list.sortKeyToColumnIndex("status"); idx >= 0 {
			a.list.table.CycleSort(idx)
		}
		return a, nil
	case "P": // Sort by Protocol
		if idx := a.list.sortKeyToColumnIndex("protocol"); idx >= 0 {
			a.list.table.CycleSort(idx)
		}
		return a, nil

	// Wide mode toggle
	case "ctrl+w":
		a.list.toggleWideMode()
		return a, nil
	}

	return a, nil
}

// applyMotionResult handles a completed motion/operator result from the motion engine.
func (a App) applyMotionResult(result *MotionResult) (tea.Model, tea.Cmd) {
	switch result.Action {
	case ActionMove:
		target := result.Target
		// Resolve H/M/L sentinel values
		if target < 0 {
			target = a.list.table.ResolveScreenTarget(target)
		}
		a.list.table.MoveCursor(target)
		a.syncCursorPosition()
		return a, nil

	case ActionPageDown:
		a.list.table.PageDown(result.Count)
		a.syncCursorPosition()
		return a, nil

	case ActionPageUp:
		a.list.table.PageUp(result.Count)
		a.syncCursorPosition()
		return a, nil

	case ActionOpLine:
		// dd = delete, yy = yank — operator was captured before engine reset
		return a.handleOperatorLine(result)

	case ActionOpRange:
		return a.handleOperatorRange(result)
	}
	return a, nil
}

// handleOperatorLine handles dd/yy/cc operator-on-line results.
// The operator type is tracked via table.lastOperator, which is captured
// from the motion engine's pending state before HandleKey resets it.
func (a App) handleOperatorLine(result *MotionResult) (tea.Model, tea.Cmd) {
	op := a.list.table.lastOperator
	switch op {
	case OpDelete:
		if c := a.list.selectedConnection(); c != nil {
			a.confirm.show(
				"Delete connection '"+c.Name+"'?",
				"delete",
				c.ID,
			)
			a.confirm.width = a.width
			a.confirm.height = a.height
		}
	case OpYank:
		return a.yankCommand()
	}
	return a, nil
}

// handleOperatorRange handles operator+motion range results (e.g., dG, ygg).
func (a App) handleOperatorRange(result *MotionResult) (tea.Model, tea.Cmd) {
	op := a.list.table.lastOperator
	switch op {
	case OpDelete:
		// For range delete, still confirm for the selected connection
		if c := a.list.selectedConnection(); c != nil {
			a.confirm.show(
				"Delete connection '"+c.Name+"'?",
				"delete",
				c.ID,
			)
			a.confirm.width = a.width
			a.confirm.height = a.height
		}
	case OpYank:
		return a.yankCommand()
	}
	return a, nil
}

func (a App) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case " ": // Space = leader key
		if !a.leader.active {
			cmd := a.leader.activate()
			return a, cmd
		}
	case "q":
		a.confirmQuit()
		return a, nil
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
			a.mode = ModeInsert
			a.statusBar.mode = ModeInsert
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
	case " ": // Space = leader key
		if !a.leader.active {
			cmd := a.leader.activate()
			return a, cmd
		}
	case "q":
		a.confirmQuit()
		return a, nil
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
		a.confirmQuit()
		return a, nil
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
		c.ProxyJump,
		c.ProxyCommand,
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
		a.confirmQuit()
		return a, nil
	case "add":
		a.form.startAdd(a.cfg.GroupNames())
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
	case "all":
		a.list.groupFilter = ""
		a.list.applyGroupFilter()
		a.syncHeaderView()
		a.syncCursorPosition()
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
	a.syncHeaderView()
	a.syncCursorPosition()

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
	a.mode = ModeNormal
	a.statusBar.mode = ModeNormal

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
	a.syncHeaderView()
	a.syncCursorPosition()

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
	case "quit":
		return a, tea.Quit

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
		a.syncHeaderView()
		a.syncCursorPosition()
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

// executeLeaderAction maps leader action command strings to existing app functionality.
func (a App) executeLeaderAction(action *LeaderAction) (tea.Model, tea.Cmd) {
	switch action.Command {
	// Connect
	case "connect-selected":
		return a.connectSelected()
	case "quick-connect":
		a.statusBar.setFlash("Quick connect not yet implemented", flashInfo)
		return a, scheduleFlashClear()

	// Find
	case "fuzzy-find":
		a.filter.activate()
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.filter.input.Focus()
	case "find-by-tag":
		a.statusBar.setFlash("Find by tag not yet implemented", flashInfo)
		return a, scheduleFlashClear()
	case "find-by-group":
		a.statusBar.setFlash("Find by group not yet implemented", flashInfo)
		return a, scheduleFlashClear()

	// Sessions
	case "sessions":
		a.sessionsView.setSessions(a.sessions.All())
		a.sessionsView.setSize(a.width, a.contentHeight())
		a.pushView(viewSessions)
		a.help.view = "sessions"
		return a, nil
	case "kill-session-prompt":
		a.statusBar.setFlash("Kill session: open sessions view first", flashInfo)
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
			a.sessionsView.setSessions(a.sessions.All())
		}
		return a, scheduleFlashClear()

	// Groups
	case "list-groups":
		a.statusBar.setFlash("List groups not yet implemented", flashInfo)
		return a, scheduleFlashClear()
	case "filter-by-group":
		a.statusBar.setFlash("Filter by group not yet implemented", flashInfo)
		return a, scheduleFlashClear()

	// Import
	case "import-ssh":
		return a.importSSH()
	case "import-csv":
		a.statusBar.setFlash("CSV import not yet implemented", flashInfo)
		return a, scheduleFlashClear()
	case "import-json":
		a.statusBar.setFlash("JSON import not yet implemented", flashInfo)
		return a, scheduleFlashClear()

	// Export
	case "export-all":
		a.statusBar.setFlash("Export all not yet implemented", flashInfo)
		return a, scheduleFlashClear()
	case "export-selection":
		a.statusBar.setFlash("Export selection not yet implemented", flashInfo)
		return a, scheduleFlashClear()
	case "export-group":
		a.statusBar.setFlash("Export group not yet implemented", flashInfo)
		return a, scheduleFlashClear()

	// Tags
	case "filter-by-tag":
		a.statusBar.setFlash("Filter by tag not yet implemented", flashInfo)
		return a, scheduleFlashClear()
	case "add-tag":
		a.statusBar.setFlash("Add tag not yet implemented", flashInfo)
		return a, scheduleFlashClear()
	case "remove-tag":
		a.statusBar.setFlash("Remove tag not yet implemented", flashInfo)
		return a, scheduleFlashClear()

	// View
	case "view-tree":
		a.statusBar.setFlash("Tree view not yet implemented", flashInfo)
		return a, scheduleFlashClear()
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
			a.detail.setConnection(c, st.status, latStr)
			a.detail.setSize(a.width, a.contentHeight())
			a.pushView(viewDetail)
			a.help.view = "detail"
		}
		return a, nil
	case "view-wide":
		a.list.toggleWideMode()
		return a, nil

	// Health
	case "check-all":
		a.log.info("Manual health check refresh via leader")
		a.statusBar.setFlash("Refreshing health checks...", flashInfo)
		return a, tea.Batch(
			health.CheckAll(a.list.healthTargets()),
			scheduleFlashClear(),
		)
	case "check-selected":
		a.statusBar.setFlash("Check selected not yet implemented", flashInfo)
		return a, scheduleFlashClear()

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
		a.statusBar.setFlash("Copy password not yet implemented", flashInfo)
		return a, scheduleFlashClear()

	// Options
	case "theme":
		a.statusBar.setFlash("Theme picker not yet implemented", flashInfo)
		return a, scheduleFlashClear()
	case "keybindings":
		a.statusBar.setFlash("Keybinding editor not yet implemented", flashInfo)
		return a, scheduleFlashClear()

	// Help (direct action from "?")
	case "?":
		a.help.view = "list"
		a.help.toggle()
		return a, nil

	default:
		a.statusBar.setFlash(fmt.Sprintf("Action: %s", action.Command), flashInfo)
		return a, scheduleFlashClear()
	}
}

func (a App) View() string {
	if !a.ready {
		return "Loading..."
	}

	// Overlay help or confirm on top — takes full screen
	if a.confirm.active {
		return a.confirm.View()
	}
	if a.help.active {
		return a.help.View()
	}

	// Main layout: header + content + statusbar
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

	// Calculate available height for content and pad/truncate to fill
	contentH := a.height - lipgloss.Height(headerView) - lipgloss.Height(statusView)
	if contentH < 1 {
		contentH = 1
	}
	contentView = lipgloss.NewStyle().Height(contentH).Width(a.width).Render(contentView)

	base := lipgloss.JoinVertical(lipgloss.Left, headerView, contentView, statusView)

	// Overlay leader key popup on top of main content
	if a.leader.active {
		return a.leader.View()
	}

	return base
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
	a.leader.width = a.width
	a.leader.height = a.height

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
	a.syncHeaderView()
	a.syncCursorPosition()
}

func (a App) contentHeight() int {
	// header = 3 lines (logo + config path + title bar)
	// statusbar = 2 lines (status line + keyhint bar)
	h := a.height - 5
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
