package tui

import (
	"errors"
	"fmt"
	"strings"
	"os"
	"path/filepath"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/health"
	"github.com/dr4zz/nexus/internal/launcher"
	"github.com/dr4zz/nexus/internal/session"
	"github.com/dr4zz/nexus/internal/audit"
	"github.com/dr4zz/nexus/internal/template"
	"github.com/dr4zz/nexus/internal/importexport"
)

type viewKind int

const (
	viewList viewKind = iota
	viewDetail
	viewForm
	viewLog
	viewSessions
	viewPulse
	viewAuditLog
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
	search       searchModel
	command      commandModel
	detail       detailModel
	form         *formModel
	help         helpModel
	confirm      confirmModel
	leader       leaderModel
	log          *logModel
	sessions     *SessionManager
	sessionsView sessionsViewModel
	tree         *TreeModel
	tplStore     *template.TemplateStore
	quickConnect *QuickConnect
	pulse        *PulseModel
	auditLog     *audit.AuditLog
	auditLogView *LogViewModel
	finder       *FinderModel

	// View stack
	viewStack []viewKind

	// Marks + Jump list
	marks       *MarkManager
	markPending rune // 0=none, 'm'=set-mark, 0x27=jump-to-mark, 0x60=backtick

	// Undo/Redo
	undoStack *UndoStack

	// State
	mode        Mode
	visualState VisualState
	ready          bool
	viewMode       string // "table" or "tree"
	treeFoldPending bool  // true when "z" was pressed in tree mode
	bracketPending rune     // 0=none, ']'=next, '['=prev
	bracketNav     *BracketNav
	tagManager     *TagManager
}

// NewApp creates the root application model.
func NewApp(cfg *config.Config) App {
	l := newLog()
	l.info("Nexus started")
	l.info("Loaded %d connections from config", len(cfg.Connections))

	h := newHeader()
	h.setConfigPath(config.ConfigPath())
	h.setItemCount(len(cfg.Connections))

	tplStore := template.NewTemplateStore()
	tplStore.LoadUserTemplates(cfg.Templates)

	finder := NewFinder()

	var al *audit.AuditLog
	if a, err := audit.Open(""); err == nil {
		al = a
		l.info("Audit log opened: %s", audit.DefaultPath())
	} else {
		l.warn("Could not open audit log: %v", err)
	}

	return App{
		cfg:          cfg,
		keys:         DefaultKeyMap(),
		header:       h,
		statusBar:    newStatusBar(),
		list:         newList(cfg),
		filter:       newFilter(),
		search:       newSearch(),
		command:      newCommand(),
		detail:       newDetail(),
		form:         newFormPtr(cfg.GroupNames(), tplStore),
		tplStore:     tplStore,
		help:         newHelp(),
		confirm:      newConfirm(),
		leader:       newLeader(),
		log:          l,
		sessions:     NewSessionManager(),
		sessionsView: newSessionsView(),
		tree:         NewTreeModel(cfg.Connections),
		quickConnect: NewQuickConnect(),
		pulse:        NewPulseModel(),
		auditLogView: NewLogViewModel(),
		auditLog:     al,
		finder:       finder,
		viewStack:    []viewKind{viewList},
		viewMode:     "table",
		marks:        NewMarkManager(),
		undoStack:    NewUndoStack(),
		bracketNav:   NewBracketNav(),
		tagManager:   NewTagManager(),
		mode:         ModeNormal,
		visualState:  NewVisualState(),
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
	case viewPulse:
		a.statusBar.view = "pulse"
	case viewAuditLog:
		a.statusBar.view = "audit"
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
	case viewPulse:
		a.header.setView("Pulse", 4)
		a.header.setItemCount(0)
	case viewAuditLog:
		a.header.setView("Audit Log", 5)
		a.header.setItemCount(0)
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

		if a.search.active {
			return a.handleSearchInput(msg)
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


		if a.quickConnect.active {
			// Handle Enter specially to check for parse errors
			if msg.String() == "enter" {
				target := strings.TrimSpace(a.quickConnect.input.Value())
				if target != "" {
					_, err := ParseTarget(target)
					if err != nil {
						a.statusBar.setFlash("Invalid target: "+err.Error(), flashError)
						return a, scheduleFlashClear()
					}
				}
			}
			cmd := a.quickConnect.Update(msg)
			if !a.quickConnect.active {
				a.statusBar.mode = ModeNormal
				modeCmd := a.setMode(ModeNormal)
				if cmd != nil {
					return a, tea.Batch(cmd, modeCmd)
				}
				return a, modeCmd
			}
			return a, cmd
		}
		if a.finder.active {
			switch msg.String() {
			case "esc":
				a.finder.Deactivate()
				a.mode = ModeNormal
				a.statusBar.mode = ModeNormal
				return a, a.setMode(ModeNormal)
			case "enter":
				if c := a.finder.SelectedConnection(); c != nil {
					a.finder.Deactivate()
					a.mode = ModeNormal
					a.statusBar.mode = ModeNormal
					if c.Protocol == config.ProtoSSH {
						return a.connectManaged(*c)
					}
					return a.connectByID(c.ID)
				}
				return a, nil
			default:
				cmd := a.finder.Update(msg)
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
		if a.currentView() == viewPulse {
			a.pulse.Refresh(a.list.filtered, a.list.statuses, a.sessions.Count())
		}
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
			a.logAuditEvent(audit.AuditEvent{
				ConnectionID:   msg.ID,
				ConnectionName: msg.ID,
				EventType:      audit.EventError,
				Details:        fullErr,
			})
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
			a.logAuditEvent(audit.AuditEvent{
				ConnectionID:   msg.ID,
				ConnectionName: msg.ID,
				EventType:      audit.EventDisconnect,
			})
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

	case QuickConnectMsg:
		return a.handleQuickConnect(msg)

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
	case viewPulse:
		// Pulse has no model update needed
	case viewAuditLog:
		cmd := a.auditLogView.Update(msg)
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

// handleTreeKey handles key input when the tree view is active.
func (a App) handleTreeKey(msg tea.KeyMsg, k string) (tea.Model, tea.Cmd) {
	// Handle fold pending state (z prefix).
	if a.treeFoldPending {
		a.treeFoldPending = false
		switch k {
		case "a":
			a.tree.Toggle(a.tree.Cursor())
		case "o":
			a.tree.Expand(a.tree.Cursor())
		case "c":
			a.tree.Collapse(a.tree.Cursor())
		case "R":
			a.tree.ExpandAll()
		case "M":
			a.tree.CollapseAll()
		}
		return a, nil
	}

	switch k {
	case "j", "down":
		a.tree.CursorDown()
		a.syncCursorPosition()
		return a, nil
	case "k", "up":
		a.tree.CursorUp()
		a.syncCursorPosition()
		return a, nil
	case "G":
		a.tree.MoveCursor(a.tree.RowCount() - 1)
		a.syncCursorPosition()
		return a, nil
	case "g":
		a.tree.MoveCursor(0)
		a.syncCursorPosition()
		return a, nil
	case "z":
		a.treeFoldPending = true
		return a, nil
	case "enter":
		if connID, ok := a.tree.SelectedConnection(); ok {
			return a.connectByID(connID)
		} else if _, ok := a.tree.SelectedFolder(); ok {
			a.tree.Toggle(a.tree.Cursor())
		}
		return a, nil
	case " ": // Space = leader key
		if !a.leader.active {
			cmd := a.leader.activate()
			return a, cmd
		}
	case "q":
		a.confirmQuit()
		return a, nil
	case "D":
		if connID, ok := a.tree.SelectedConnection(); ok {
			c := a.cfg.FindConnection(connID)
			if c != nil {
				st := a.list.statuses[c.ID]
				latStr := ""
				if st.status == health.Online || st.status == health.Degraded {
					latStr = st.latency.String()
				}
				a.detail.setConnection(c, st.status, latStr)
				a.detail.setSize(a.width, a.contentHeight())
				a.pushView(viewDetail)
				a.help.view = "detail"
				return a, nil
			}
		}
		return a, nil
	}

	return a, nil
}

func (a App) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()

	// Tree mode key handling — delegate to tree-specific handler.
	if a.viewMode == "tree" {
		return a.handleTreeKey(msg, k)
	}

	// --- Mark pending state handling (two-key sequences: m+letter, \x27+letter, `+`) ---
	if a.markPending != 0 {
		defer func() { a.markPending = 0 }()
		switch a.markPending {
		case 'm':
			// m + a-z: set mark at current position
			if len(k) == 1 && k[0] >= 'a' && k[0] <= 'z' {
				a.marks.SetMark(rune(k[0]), a.viewName(), a.currentCursor())
				a.statusBar.setFlash(fmt.Sprintf("Mark '%s' set", k), flashInfo)
				return a, scheduleFlashClear()
			}
			// Invalid follow-up key — cancel silently
			return a, nil
		case 0x27: // apostrophe (')
			// ' + a-z: jump to mark
			if len(k) == 1 && k[0] >= 'a' && k[0] <= 'z' {
				mk, ok := a.marks.GetMark(rune(k[0]))
				if !ok {
					a.statusBar.setFlash(fmt.Sprintf("Mark '%s' not set", k), flashError)
					return a, scheduleFlashClear()
				}
				// Push current position before jumping
				a.marks.PushJump(a.viewName(), a.currentCursor())
				a.navigateToMark(mk)
				a.statusBar.setFlash(fmt.Sprintf("Jumped to mark '%s'", k), flashInfo)
				return a, scheduleFlashClear()
			}
			return a, nil
		case 0x60: // backtick (`)
			// ` + `: jump to last position
			if k == "`" {
				mk, ok := a.marks.LastJump()
				if !ok {
					a.statusBar.setFlash("No previous jump", flashError)
					return a, scheduleFlashClear()
				}
				a.marks.PushJump(a.viewName(), a.currentCursor())
				a.navigateToMark(mk)
				return a, nil
			}
			return a, nil
		}
		return a, nil
	}

	// --- Marks: initiate two-key sequences ---
	switch k {
	case "m":
		a.markPending = 'm'
		return a, nil
	case "'":
		a.markPending = 0x27 // apostrophe
		return a, nil
	case "`":
		a.markPending = 0x60 // backtick
		return a, nil
	case "ctrl+o":
		mk, ok := a.marks.JumpBack()
		if !ok {
			return a, nil
		}
		a.navigateToMark(mk)
		return a, nil
	case "ctrl+i":
		mk, ok := a.marks.JumpForward()
		if !ok {
			return a, nil
		}
		a.navigateToMark(mk)
		return a, nil
	}

	// --- Bracket-pending state handling (two-key sequences: ]x / [x) ---
	if a.bracketPending != 0 {
		pending := a.bracketPending
		a.bracketPending = 0
		return a.handleBracketMotion(pending, k)
	}

	// --- Bracket: initiate two-key sequences ---
	switch k {
	case "]":
		a.bracketPending = ']'
		return a, nil
	case "[":
		a.bracketPending = '['
		return a, nil
	}


	// Handle Visual mode keys first if visual mode is active.
	if a.visualState.Active() {
		return a.handleVisualKey(msg)
	}

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
	case "/":
		a.search.activate(SearchForward)
		a.search.width = a.width
		a.list.table.highlightText = ""
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.setMode(ModeInsert)
	case "?":
		a.search.activate(SearchReverse)
		a.search.width = a.width
		a.list.table.highlightText = ""
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.setMode(ModeInsert)
	case "n":
		return a.searchNextMatch(true)
	case "*":
		return a.searchUnderCursor(SearchForward)
	case "#":
		return a.searchUnderCursor(SearchReverse)
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
			// Record position before view change
			a.marks.PushJump(a.viewName(), a.currentCursor())
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
		// Record position before view change
		a.marks.PushJump(a.viewName(), a.currentCursor())
		a.sessionsView.setSessions(a.sessions.All())
		a.sessionsView.setSize(a.width, a.contentHeight())
		a.pushView(viewSessions)
		a.help.view = "sessions"
		return a, nil

	case "V": // Enter line-visual mode
		return a.enterVisualMode()

	case "N": // Jump to previous search match (opposite direction of n)
		return a.searchNextMatch(false)
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

	case "u": // Undo
		return a.performUndo()
	case "ctrl+r": // Redo
		return a.performRedo()
	case "ctrl+w":
		a.list.toggleWideMode()
		return a, nil
	case "ctrl+p":
		a.finder.SetSize(a.width, a.height)
		a.finder.Activate(PickerConnections, a.cfg.Connections)
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.finder.prompt.Focus()
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

// enterVisualMode switches to Visual mode with the current cursor as anchor.
func (a App) enterVisualMode() (tea.Model, tea.Cmd) {
	cursor := a.list.table.Cursor()
	a.visualState.Enter(cursor)
	a.list.table.visualState = &a.visualState
	a.mode = ModeVisual
	a.statusBar.mode = ModeVisual
	a.statusBar.visualCount = a.visualState.Count()
	return a, a.setMode(ModeVisual)
}

// exitVisualMode returns to Normal mode and clears all visual selection.
func (a *App) exitVisualMode() tea.Cmd {
	a.visualState.Exit()
	a.list.table.visualState = nil
	a.mode = ModeNormal
	a.statusBar.mode = ModeNormal
	a.statusBar.visualCount = 0
	return a.setMode(ModeNormal)
}

// syncVisualStatus updates the statusbar with the current visual selection count.
func (a *App) syncVisualStatus() {
	a.statusBar.visualCount = a.visualState.Count()
}

// handleVisualKey processes key events while in Visual mode.
func (a App) handleVisualKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	cursor := a.list.table.Cursor()
	total := len(a.list.table.rows)

	switch k {
	case "esc":
		cmd := a.exitVisualMode()
		return a, cmd

	case "j", "down":
		// Move cursor down and extend selection range
		newCursor := min(cursor+1, total-1)
		a.list.table.MoveCursor(newCursor)
		a.visualState.UpdateRange(newCursor)
		a.list.table.visualState = &a.visualState
		a.syncVisualStatus()
		a.syncCursorPosition()
		return a, nil

	case "k", "up":
		// Move cursor up and extend/contract selection range
		newCursor := max(cursor-1, 0)
		a.list.table.MoveCursor(newCursor)
		a.visualState.UpdateRange(newCursor)
		a.list.table.visualState = &a.visualState
		a.syncVisualStatus()
		a.syncCursorPosition()
		return a, nil

	case "G":
		// Jump to last row, extend selection
		a.list.table.MoveCursor(total - 1)
		a.visualState.UpdateRange(total - 1)
		a.list.table.visualState = &a.visualState
		a.syncVisualStatus()
		a.syncCursorPosition()
		return a, nil

	case "v":
		// Toggle current item in/out of selection (non-contiguous)
		a.visualState.ToggleItem(cursor)
		a.list.table.visualState = &a.visualState
		a.syncVisualStatus()
		return a, nil

	case "d":
		// Delete all selected connections (with confirmation)
		count := a.visualState.Count()
		if count == 0 {
			return a, nil
		}
		ids := a.visualState.SelectedIDs(a.list.table.rows)
		if len(ids) == 0 {
			return a, nil
		}
		// Store selected IDs for the confirm handler
		prompt := fmt.Sprintf("Delete %d selected connections?", count)
		// Join IDs with comma for the confirm action
		idStr := strings.Join(ids, ",")
		a.confirm.show(prompt, "delete-visual", idStr)
		a.confirm.width = a.width
		a.confirm.height = a.height
		return a, nil

	case "y":
		// Yank all selected connection details to clipboard
		return a.yankVisualSelection()
	}

	return a, nil
}

// yankVisualSelection copies the details of all visually selected connections to the clipboard.
func (a App) yankVisualSelection() (tea.Model, tea.Cmd) {
	ids := a.visualState.SelectedIDs(a.list.table.rows)
	if len(ids) == 0 {
		return a, nil
	}

	var lines []string
	for _, id := range ids {
		c := a.cfg.FindConnection(id)
		if c == nil {
			continue
		}
		l, err := launcher.ForProtocol(c.Protocol)
		if err != nil {
			continue
		}
		lines = append(lines, l.Command(*c))
	}

	if len(lines) == 0 {
		return a, nil
	}

	text := strings.Join(lines, "\n")
	if err := clipboard.WriteAll(text); err != nil {
		a.log.error("Clipboard error: %v", err)
		a.statusBar.setFlash("Clipboard error: "+err.Error(), flashError)
	} else {
		a.log.info("Copied %d connection commands to clipboard", len(lines))
		a.statusBar.setFlash(fmt.Sprintf("Copied %d commands", len(lines)), flashInfo)
	}

	// Exit visual mode after yanking
	a.exitVisualMode()
	return a, scheduleFlashClear()
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

func (a App) handlePulseKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		a.confirmQuit()
		return a, nil
	case "esc":
		a.popView()
		return a, nil
	case "r":
		a.log.info("Manual health check refresh from pulse view")
		a.statusBar.setFlash("Refreshing...", flashInfo)
		return a, tea.Batch(
			health.CheckAll(a.list.healthTargets()),
			scheduleFlashClear(),
		)
	}
	return a, nil
}

func (a App) handleAuditLogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		a.confirmQuit()
		return a, nil
	case "esc":
		a.popView()
		return a, nil
	}

	// Pass scroll keys to viewport
	cmd := a.auditLogView.Update(msg)
	return a, cmd
}

func (a App) openAuditLogView() (tea.Model, tea.Cmd) {
	if a.auditLog != nil {
		events, err := a.auditLog.Query(audit.AuditFilter{Limit: 200})
		if err != nil {
			a.log.error("Failed to query audit log: %v", err)
		} else {
			a.auditLogView.SetEvents(events)
		}
	}
	a.auditLogView.SetSize(a.width, a.contentHeight())
	a.pushView(viewAuditLog)
	return a, nil
}

func (a *App) logAuditEvent(ev audit.AuditEvent) {
	if a.auditLog == nil {
		return
	}
	if err := a.auditLog.Log(ev); err != nil {
		a.log.warn("Audit log write error: %v", err)
	}
}

// trackConnectionUsage updates LastConnectedAt and increments ConnectCount
// for the connection with the given ID, then saves the config.
func (a *App) trackConnectionUsage(connID string) {
	for i := range a.cfg.Connections {
		if a.cfg.Connections[i].ID == connID {
			now := time.Now()
			a.cfg.Connections[i].LastConnectedAt = &now
			a.cfg.Connections[i].ConnectCount++
			if err := config.Save(a.cfg); err != nil {
				a.log.error("Failed to save connection usage: %v", err)
			}
			return
		}
	}
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

// connectByID launches a connection by its ID.
func (a App) connectByID(id string) (tea.Model, tea.Cmd) {
	c := a.cfg.FindConnection(id)
	if c == nil {
		return a, nil
	}

	a.trackConnectionUsage(c.ID)

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

func (a App) connectSelected() (tea.Model, tea.Cmd) {
	c := a.list.selectedConnection()
	if c == nil {
		return a, nil
	}

	// Track connection usage (LastConnectedAt, ConnectCount)
	a.trackConnectionUsage(c.ID)

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
		a.sessionsView.setSessions(a.sessions.All())
		a.sessionsView.setSize(a.width, a.contentHeight())
		a.pushView(viewSessions)
		a.help.view = "sessions"
		return a, nil
	case "pulse":
		a.pulse.SetSize(a.width, a.contentHeight())
		a.pulse.Refresh(a.list.filtered, a.list.statuses, a.sessions.Count())
		a.pushView(viewPulse)
		return a, nil
	case "logs":
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
		deleted := 0
		for _, id := range ids {
			conn := a.cfg.FindConnection(id)
			name := id
			if conn != nil {
				name = conn.Name
				// Capture snapshot and index for undo before deleting.
				snapshot := *conn
				idx := -1
				for i, c := range a.cfg.Connections {
					if c.ID == id {
						idx = i
						break
					}
				}
				if err := a.cfg.DeleteConnection(id); err != nil {
					a.log.error("Delete failed for %s: %v", name, err)
				} else {
					if idx >= 0 {
						a.undoStack.Push(Operation{
							Type:   UndoOpDelete,
							ConnID: id,
							Name:   name,
							Index:  idx,
							Before: snapshot,
						})
					}
					a.log.info("Deleted connection: %s", name)
					deleted++
				}
			} else {
				if err := a.cfg.DeleteConnection(id); err != nil {
					a.log.error("Delete failed for %s: %v", name, err)
				} else {
					a.log.info("Deleted connection: %s", name)
					deleted++
				}
			}
		}
		a.statusBar.setFlash(fmt.Sprintf("Deleted %d connections", deleted), flashInfo)

		// Exit visual mode
		a.exitVisualMode()

		// Rebuild the list
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

// handleSearchInput processes key events while the search bar is active.
func (a App) handleSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch k {
	case "esc":
		// Cancel search, clear filter and highlight
		a.search.deactivate()
		a.list.table.setFilter("")
		a.list.table.highlightText = ""
		a.list.applyFilter("")
		a.header.setFilter("")
		a.header.setItemCount(len(a.list.filtered))
		a.statusBar.mode = ModeNormal
		a.syncCursorPosition()
		return a, a.setMode(ModeNormal)

	case "enter":
		// Confirm search
		query := a.search.query
		a.search.computeMatches(a.list.table.rows)
		a.search.confirm()

		if query != "" {
			ps := parseSearchQuery(query)
			a.list.table.highlightText = ps.pattern
			a.header.setFilter(query)
		} else {
			a.list.table.highlightText = ""
			a.header.setFilter("")
		}
		a.statusBar.mode = ModeNormal
		a.syncCursorPosition()
		return a, a.setMode(ModeNormal)

	case "up":
		a.search.historyUp()
		a.applyIncrementalSearch()
		return a, nil

	case "down":
		a.search.historyDown()
		a.applyIncrementalSearch()
		return a, nil

	case "backspace", "ctrl+h":
		a.search.handleBackspace()
		a.applyIncrementalSearch()
		return a, nil

	default:
		// Handle regular character input
		if len(k) == 1 {
			a.search.handleRune(rune(k[0]))
			a.applyIncrementalSearch()
			return a, nil
		}
		// Handle rune messages for multi-byte characters
		if msg.Type == tea.KeyRunes {
			for _, r := range msg.Runes {
				a.search.handleRune(r)
			}
			a.applyIncrementalSearch()
			return a, nil
		}
	}
	return a, nil
}

// applyIncrementalSearch updates the table filter and match indices as the user types.
func (a *App) applyIncrementalSearch() {
	query := a.search.query
	if query == "" {
		a.list.applyFilter("")
		a.list.table.highlightText = ""
		a.header.setFilter("")
		a.header.setItemCount(len(a.list.filtered))
		a.search.matches = nil
		a.syncCursorPosition()
		return
	}

	ps := parseSearchQuery(query)
	a.list.table.highlightText = ps.pattern

	// Apply filter based on parsed search mode
	a.applySearchFilter(ps)
	a.header.setFilter(query)
	a.header.setItemCount(len(a.list.filtered))

	// Compute matches on the filtered rows
	a.search.computeMatches(a.list.table.rows)

	// Jump to first match if there are any
	if len(a.search.matches) > 0 {
		var target int
		if a.search.mode == SearchForward {
			target = a.search.matches[0]
		} else {
			target = a.search.matches[len(a.search.matches)-1]
		}
		a.list.table.MoveCursor(target)
		a.search.matchIdx = target
	}
	a.syncCursorPosition()
}

// applySearchFilter filters the list based on the parsed search query.
func (a *App) applySearchFilter(ps parsedSearch) {
	if ps.pattern == "" && ps.mode != "inverse" {
		a.list.applyFilter("")
		return
	}

	switch ps.mode {
	case "tag":
		a.applyTagFilter(ps.pattern)
	case "group":
		a.applyGroupSearchFilter(ps.pattern)
	default:
		// For fuzzy, inverse, strict-fuzzy we apply at the row level
		a.applyRowFilter(ps)
	}
}

// applyRowFilter filters list rows using the search matching logic.
func (a *App) applyRowFilter(ps parsedSearch) {
	base := a.list.groupFilteredConns()
	if ps.pattern == "" && ps.mode != "inverse" {
		a.list.filtered = base
		a.list.rebuildTable()
		return
	}

	// Build temporary rows for matching, then filter connections
	var result []config.Connection
	for _, c := range base {
		searchable := strings.Join([]string{c.Name, c.Host, string(c.Protocol), c.Group, strings.Join(c.Tags, " ")}, " ")
		tempRow := Row{Cells: []string{"", c.Name, c.Host, c.Protocol.Label(), c.Group, ""}}

		switch ps.mode {
		case "inverse":
			if !fuzzyMatch(ps.pattern, searchable) {
				result = append(result, c)
			}
		case "strict-fuzzy":
			if strictFuzzyMatch(ps.pattern, searchable) {
				result = append(result, c)
			}
		default: // "fuzzy"
			_ = tempRow // satisfy compiler
			if fuzzyMatch(ps.pattern, searchable) {
				result = append(result, c)
			}
		}
	}
	a.list.filtered = result
	a.list.rebuildTable()
}

// applyTagFilter filters connections by a specific tag.
func (a *App) applyTagFilter(tag string) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	base := a.list.groupFilteredConns()
	if tag == "" {
		a.list.filtered = base
		a.list.rebuildTable()
		return
	}

	var result []config.Connection
	for _, c := range base {
		for _, t := range c.Tags {
			if strings.ToLower(strings.TrimSpace(t)) == tag {
				result = append(result, c)
				break
			}
		}
	}
	a.list.filtered = result
	a.list.rebuildTable()
}

// applyGroupSearchFilter filters connections by group name.
func (a *App) applyGroupSearchFilter(group string) {
	group = strings.TrimSpace(group)
	base := a.list.groupFilteredConns()
	if group == "" {
		a.list.filtered = base
		a.list.rebuildTable()
		return
	}

	var result []config.Connection
	for _, c := range base {
		if strings.EqualFold(strings.TrimSpace(c.Group), group) {
			result = append(result, c)
		}
	}
	a.list.filtered = result
	a.list.rebuildTable()
}

// searchNextMatch jumps to the next (or previous) search match.
// sameDirection=true means follow the original search direction, false means opposite.
func (a App) searchNextMatch(sameDirection bool) (tea.Model, tea.Cmd) {
	if !a.search.confirmed || a.search.query == "" {
		return a, nil
	}

	// Recompute matches on current rows
	a.search.computeMatches(a.list.table.rows)
	if len(a.search.matches) == 0 {
		a.statusBar.setFlash("Pattern not found: "+a.search.query, flashError)
		return a, scheduleFlashClear()
	}

	cursor := a.list.table.Cursor()
	forward := a.search.mode == SearchForward
	if !sameDirection {
		forward = !forward
	}

	target := a.search.nextMatch(cursor, forward)
	if target >= 0 {
		// Record position before search jump
		a.marks.PushJump(a.viewName(), cursor)
		a.list.table.MoveCursor(target)
		a.search.matchIdx = target
		a.syncCursorPosition()
	}
	return a, nil
}

// searchUnderCursor searches for the name of the connection under the cursor.
func (a App) searchUnderCursor(mode SearchMode) (tea.Model, tea.Cmd) {
	c := a.list.selectedConnection()
	if c == nil {
		return a, nil
	}

	name := c.Name
	a.search.searchForName(name, mode)
	a.search.computeMatches(a.list.table.rows)
	a.list.table.highlightText = name
	a.header.setFilter(name)

	// Jump to next/previous match from current position
	cursor := a.list.table.Cursor()
	forward := mode == SearchForward
	target := a.search.nextMatch(cursor, forward)
	if target >= 0 && target != cursor {
		// Record position before search-under-cursor jump
		a.marks.PushJump(a.viewName(), cursor)
		a.list.table.MoveCursor(target)
		a.search.matchIdx = target
	}
	a.syncCursorPosition()
	return a, nil
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
		if a.quickConnect.active {
			parts = append(parts, a.quickConnect.View())
		}
		parts = append(parts, a.list.View())
		if a.search.active {
			parts = append(parts, a.search.View())
		}
		contentView = lipgloss.JoinVertical(lipgloss.Left, parts...)
	case viewDetail:
		contentView = a.detail.View()
	case viewForm:
		contentView = a.form.View()
	case viewLog:
		contentView = a.log.View()
	case viewSessions:
		contentView = a.sessionsView.View()
	case viewPulse:
		contentView = a.pulse.View()
	case viewAuditLog:
		contentView = a.auditLogView.View()
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

	// Overlay finder on top of main content
	if a.finder.active {
		return a.finder.View()
	}

	return base
}

func (a *App) layout() {
	a.ready = true
	a.header.width = a.width
	a.statusBar.width = a.width
	a.filter.width = a.width
	a.search.width = a.width
	a.command.width = a.width
	a.help.width = a.width
	a.help.height = a.height
	a.confirm.width = a.width
	a.confirm.height = a.height
	a.leader.width = a.width
	a.quickConnect.width = a.width
	a.leader.height = a.height

	ch := a.contentHeight()
	a.list.setSize(a.width, ch)
	a.detail.setSize(a.width, ch)
	a.log.setSize(a.width, ch)
	a.sessionsView.setSize(a.width, ch)
	a.pulse.SetSize(a.width, ch)
	a.auditLogView.SetSize(a.width, ch)

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

// viewName returns a string name for the current view (used by the mark system).
func (a App) viewName() string {
	switch a.currentView() {
	case viewList:
		return "list"
	case viewDetail:
		return "detail"
	case viewForm:
		return "form"
	case viewLog:
		return "log"
	case viewSessions:
		return "sessions"
	case viewPulse:
		return "pulse"
	case viewAuditLog:
		return "audit"
	default:
		return "list"
	}
}

// currentCursor returns the cursor position for the current view.
func (a App) currentCursor() int {
	switch a.currentView() {
	case viewList:
		return a.list.table.Cursor()
	case viewSessions:
		return a.sessionsView.table.Cursor()
	default:
		return 0
	}
}

// navigateToMark jumps to the view and cursor recorded in a Mark.
func (a *App) navigateToMark(mk Mark) {
	// Switch view if needed
	target := viewList
	switch mk.View {
	case "list":
		target = viewList
	case "detail":
		target = viewDetail
	case "sessions":
		target = viewSessions
	case "pulse":
		target = viewPulse
	case "audit":
		target = viewAuditLog
	case "log":
		target = viewLog
	default:
		target = viewList
	}

	if a.currentView() != target {
		// Pop back to list first, then push target if not list
		for len(a.viewStack) > 1 {
			a.viewStack = a.viewStack[:len(a.viewStack)-1]
		}
		if target != viewList {
			a.pushView(target)
		}
		a.syncHeaderView()
		a.syncStatusBarView()
	}

	// Set cursor
	switch target {
	case viewList:
		a.list.table.SetCursor(mk.Cursor)
	case viewSessions:
		a.sessionsView.table.SetCursor(mk.Cursor)
	}
	a.syncCursorPosition()
}


// performUndo undoes the most recent destructive operation.
func (a App) performUndo() (tea.Model, tea.Cmd) {
	op, ok := a.undoStack.Undo()
	if !ok {
		a.statusBar.setFlash("Nothing to undo", flashError)
		return a, scheduleFlashClear()
	}

	var flashMsg string
	switch op.Type {
	case UndoOpAdd:
		// Undo add = delete the connection that was added.
		if err := a.cfg.DeleteConnection(op.After.ID); err != nil {
			a.log.error("Undo add failed: %v", err)
			a.statusBar.setFlash("Undo failed: "+err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		flashMsg = fmt.Sprintf("Undo: removed '%s'", op.Name)

	case UndoOpDelete:
		// Undo delete = re-insert the connection at its original index.
		if err := a.cfg.InsertConnectionAt(op.Before, op.Index); err != nil {
			a.log.error("Undo delete failed: %v", err)
			a.statusBar.setFlash("Undo failed: "+err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		flashMsg = fmt.Sprintf("Undo: restored '%s'", op.Name)

	case UndoOpEdit:
		// Undo edit = restore the before snapshot.
		if err := a.cfg.UpdateConnection(op.Before); err != nil {
			a.log.error("Undo edit failed: %v", err)
			a.statusBar.setFlash("Undo failed: "+err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		flashMsg = fmt.Sprintf("Undo: reverted '%s'", op.Name)

	case UndoOpTagChange:
		// Undo tag change = restore the before snapshot.
		if err := a.cfg.UpdateConnection(op.Before); err != nil {
			a.log.error("Undo tag change failed: %v", err)
			a.statusBar.setFlash("Undo failed: "+err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		flashMsg = fmt.Sprintf("Undo: restored tags on '%s'", op.Name)
	}

	a.log.info("%s", flashMsg)
	a.statusBar.setFlash(flashMsg, flashInfo)
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

// performRedo re-applies the most recently undone operation.
func (a App) performRedo() (tea.Model, tea.Cmd) {
	op, ok := a.undoStack.Redo()
	if !ok {
		a.statusBar.setFlash("Nothing to redo", flashError)
		return a, scheduleFlashClear()
	}

	var flashMsg string
	switch op.Type {
	case UndoOpAdd:
		// Redo add = re-add the connection.
		if err := a.cfg.AddConnection(op.After); err != nil {
			a.log.error("Redo add failed: %v", err)
			a.statusBar.setFlash("Redo failed: "+err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		flashMsg = fmt.Sprintf("Redo: added '%s'", op.Name)

	case UndoOpDelete:
		// Redo delete = delete the connection again.
		if err := a.cfg.DeleteConnection(op.Before.ID); err != nil {
			a.log.error("Redo delete failed: %v", err)
			a.statusBar.setFlash("Redo failed: "+err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		flashMsg = fmt.Sprintf("Redo: deleted '%s'", op.Name)

	case UndoOpEdit:
		// Redo edit = re-apply the after snapshot.
		if err := a.cfg.UpdateConnection(op.After); err != nil {
			a.log.error("Redo edit failed: %v", err)
			a.statusBar.setFlash("Redo failed: "+err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		flashMsg = fmt.Sprintf("Redo: re-applied edit on '%s'", op.Name)

	case UndoOpTagChange:
		// Redo tag change = re-apply the after snapshot.
		if err := a.cfg.UpdateConnection(op.After); err != nil {
			a.log.error("Redo tag change failed: %v", err)
			a.statusBar.setFlash("Redo failed: "+err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		flashMsg = fmt.Sprintf("Redo: re-applied tags on '%s'", op.Name)
	}

	a.log.info("%s", flashMsg)
	a.statusBar.setFlash(flashMsg, flashInfo)
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


// handleBracketMotion processes the second key in a bracket motion sequence.
// pending is ']' or '[', key is the follow-up character (c/g/e/s/f).
func (a App) handleBracketMotion(pending rune, key string) (tea.Model, tea.Cmd) {
	if len(key) != 1 {
		return a, nil // invalid follow-up, cancel silently
	}

	conns := a.list.filtered
	cursor := a.list.table.Cursor()
	var target int

	// Build a health status map for error navigation.
	healthStatuses := make(map[string]health.Status, len(a.list.statuses))
	for id, cs := range a.list.statuses {
		healthStatuses[id] = cs.status
	}

	switch key {
	case "c":
		if pending == ']' {
			target = a.bracketNav.NextConnection(cursor, conns)
		} else {
			target = a.bracketNav.PrevConnection(cursor, conns)
		}
	case "g":
		if pending == ']' {
			target = a.bracketNav.NextGroup(cursor, conns)
		} else {
			target = a.bracketNav.PrevGroup(cursor, conns)
		}
	case "e":
		if pending == ']' {
			target = a.bracketNav.NextError(cursor, conns, healthStatuses)
		} else {
			target = a.bracketNav.PrevError(cursor, conns, healthStatuses)
		}
	case "s":
		sessions := a.sessions.All()
		if pending == ']' {
			target = a.bracketNav.NextSession(cursor, conns, sessions)
		} else {
			target = a.bracketNav.PrevSession(cursor, conns, sessions)
		}
	case "f":
		if pending == ']' {
			target = a.bracketNav.NextFavorite(cursor, conns)
		} else {
			target = a.bracketNav.PrevFavorite(cursor, conns)
		}
	default:
		return a, nil // unknown follow-up, cancel silently
	}

	if target != cursor {
		a.list.table.MoveCursor(target)
		a.syncCursorPosition()
	}
	return a, nil
}

// handleTagCommand processes :tag add/remove commands.
func (a App) handleTagCommand(args string) (tea.Model, tea.Cmd) {
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		a.statusBar.setFlash("Usage: :tag <add|remove|list> <tag>", flashError)
		return a, scheduleFlashClear()
	}

	subcmd := strings.ToLower(parts[0])
	tagArg := strings.TrimSpace(parts[1])

	switch subcmd {
	case "add":
		if tagArg == "" {
			a.statusBar.setFlash("Usage: :tag add <tag>", flashError)
			return a, scheduleFlashClear()
		}
		// Apply to visual selection if active, otherwise to current connection.
		if a.visualState.Active() {
			return a.tagAddVisual(tagArg)
		}
		c := a.list.selectedConnection()
		if c == nil {
			a.statusBar.setFlash("No connection selected", flashError)
			return a, scheduleFlashClear()
		}
		before := *c
		if a.tagManager.AddTag(c, tagArg) {
			// Persist the change.
			if err := a.cfg.UpdateConnection(*c); err != nil {
				a.log.error("Failed to save tag: %v", err)
				a.statusBar.setFlash("Error saving tag", flashError)
			} else {
				a.undoStack.Push(Operation{
					Type:   UndoOpTagChange,
					ConnID: c.ID,
					Name:   c.Name,
					Before: before,
					After:  *c,
				})
				a.log.info("Added tag '%s' to %s", tagArg, c.Name)
				a.statusBar.setFlash(fmt.Sprintf("Added tag '%s' to %s", tagArg, c.Name), flashInfo)
			}
			a.list.rebuildTable()
		} else {
			a.statusBar.setFlash(fmt.Sprintf("Tag '%s' already exists on %s", tagArg, c.Name), flashInfo)
		}
		return a, scheduleFlashClear()

	case "remove":
		if tagArg == "" {
			a.statusBar.setFlash("Usage: :tag remove <tag>", flashError)
			return a, scheduleFlashClear()
		}
		if a.visualState.Active() {
			return a.tagRemoveVisual(tagArg)
		}
		c := a.list.selectedConnection()
		if c == nil {
			a.statusBar.setFlash("No connection selected", flashError)
			return a, scheduleFlashClear()
		}
		before := *c
		if a.tagManager.RemoveTag(c, tagArg) {
			if err := a.cfg.UpdateConnection(*c); err != nil {
				a.log.error("Failed to save tag removal: %v", err)
				a.statusBar.setFlash("Error saving tag removal", flashError)
			} else {
				a.undoStack.Push(Operation{
					Type:   UndoOpTagChange,
					ConnID: c.ID,
					Name:   c.Name,
					Before: before,
					After:  *c,
				})
				a.log.info("Removed tag '%s' from %s", tagArg, c.Name)
				a.statusBar.setFlash(fmt.Sprintf("Removed tag '%s' from %s", tagArg, c.Name), flashInfo)
			}
			a.list.rebuildTable()
		} else {
			a.statusBar.setFlash(fmt.Sprintf("Tag '%s' not found on %s", tagArg, c.Name), flashInfo)
		}
		return a, scheduleFlashClear()

	case "list":
		return a.handleTagsListCommand()

	default:
		a.statusBar.setFlash("Usage: :tag <add|remove|list> <tag>", flashError)
		return a, scheduleFlashClear()
	}
}

// tagAddVisual adds a tag to all visually selected connections.
func (a App) tagAddVisual(tag string) (tea.Model, tea.Cmd) {
	indices := a.visualState.SelectedIndices()
	added := 0
	for _, idx := range indices {
		if idx >= 0 && idx < len(a.list.filtered) {
			c := &a.list.filtered[idx]
			before := *c
			if a.tagManager.AddTag(c, tag) {
				_ = a.cfg.UpdateConnection(*c)
				a.undoStack.Push(Operation{
					Type:   UndoOpTagChange,
					ConnID: c.ID,
					Name:   c.Name,
					Before: before,
					After:  *c,
				})
				added++
			}
		}
	}
	a.list.rebuildTable()
	a.statusBar.setFlash(fmt.Sprintf("Added tag '%s' to %d connection(s)", tag, added), flashInfo)
	return a, scheduleFlashClear()
}

// tagRemoveVisual removes a tag from all visually selected connections.
func (a App) tagRemoveVisual(tag string) (tea.Model, tea.Cmd) {
	indices := a.visualState.SelectedIndices()
	removed := 0
	for _, idx := range indices {
		if idx >= 0 && idx < len(a.list.filtered) {
			c := &a.list.filtered[idx]
			before := *c
			if a.tagManager.RemoveTag(c, tag) {
				_ = a.cfg.UpdateConnection(*c)
				a.undoStack.Push(Operation{
					Type:   UndoOpTagChange,
					ConnID: c.ID,
					Name:   c.Name,
					Before: before,
					After:  *c,
				})
				removed++
			}
		}
	}
	a.list.rebuildTable()
	a.statusBar.setFlash(fmt.Sprintf("Removed tag '%s' from %d connection(s)", tag, removed), flashInfo)
	return a, scheduleFlashClear()
}

// handleTagsListCommand shows all unique tags with counts.
func (a App) handleTagsListCommand() (tea.Model, tea.Cmd) {
	tags := a.tagManager.AllTags(a.cfg.Connections)
	if len(tags) == 0 {
		a.statusBar.setFlash("No tags found", flashInfo)
		return a, scheduleFlashClear()
	}
	// Format a compact summary for the status bar.
	var parts []string
	for _, t := range tags {
		parts = append(parts, fmt.Sprintf("%s(%d)", t.Name, t.Count))
	}
	summary := "Tags: " + strings.Join(parts, ", ")
	if len(summary) > 120 {
		summary = summary[:117] + "..."
	}
	a.log.info("%s", a.tagManager.FormatTagList(tags))
	a.statusBar.setFlash(summary, flashInfo)
	return a, scheduleFlashClear()
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
	a.statusBar.setFlash("Sort command not yet implemented", flashInfo)
	return a, scheduleFlashClear()
}

func scheduleFlashClear() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return FlashExpireMsg{}
	})
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
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
		a.detail.setConnection(c, a.detail.status, a.detail.latency)
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
		a.detail.setConnection(c, a.detail.status, a.detail.latency)
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
		health.CheckAll(a.list.healthTargets()),
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
	if a.visualState.Active() {
		// Export only visually selected connections.
		indices := a.visualState.SelectedIndices()
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
	return a, scheduleFlashClear()
}
