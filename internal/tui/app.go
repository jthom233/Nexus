package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/audit"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/health"
	"github.com/dr4zz/nexus/internal/launcher"
	"github.com/dr4zz/nexus/internal/session"
	"github.com/dr4zz/nexus/internal/template"
	"github.com/dr4zz/nexus/internal/vault"
)

type viewKind int

const (
	viewList viewKind = iota
	viewDetail
	viewForm
	viewLog
	viewSessions
	viewPulse
	viewPaneLayout
	viewVault
	viewVaultForm
	viewGroupList
	viewFileBrowser
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
	log          *logsModel
	sessions     *SessionManager
	sessionsView sessionsViewModel
	tplStore     *template.TemplateStore
	quickConnect *QuickConnect
	pulse        *PulseModel
	auditLog     *audit.AuditLog
	finder       *FinderModel
	checker      *health.Checker
	paneLayout   *PaneLayoutModel

	// Vault
	vault        vault.Vault
	profileStore *vault.ProfileStore
	vaultView     vaultModel
	vaultFormView vaultFormModel

	// Group list
	groupListView groupListModel

	// Tutorial overlay
	tutorial tutorialModel

	// Ghost session manager (auto-connecting background sessions)
	ghostMgr *session.GhostManager

	// File browser (SFTP)
	fileBrowser fileBrowserModel

	// View stack
	viewStack []viewKind

	// Undo/Redo
	undoStack *UndoStack

	// Animation
	viewTransition    transition
	animationsEnabled bool

	// State
	mode         Mode
	selectionSet SelectionSet
	ready          bool
	bracketPending rune     // 0=none, ']'=next, '['=prev
	bracketNav     *BracketNav
	tagManager     *TagManager
}

// saveErrorMsg is returned when a background save fails.
type saveErrorMsg struct{ err error }

// NewApp creates the root application model.
func NewApp(cfg *config.Config, v vault.Vault) App {
	l := newLogsModel()
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

	var ps *vault.ProfileStore
	if v != nil {
		ps = vault.NewProfileStore(v)
		l.info("Credential vault opened")
	}

	tut := newTutorial()
	if !cfg.Settings.TutorialShown {
		tut.activate()
	}

	ghostMgr := session.NewGhostManager(cfg)
	ghostMgr.Start()

	return App{
		cfg:           cfg,
		vault:         v,
		profileStore:  ps,
		vaultView:     newVaultModel(),
		groupListView: newGroupListModel(),
		ghostMgr:      ghostMgr,
		fileBrowser:   newFileBrowserModel(),
		keys:         DefaultKeyMap(),
		header:       h,
		statusBar:    newStatusBar(),
		list:         newList(cfg),
		filter:       newFilter(),
		search:       newSearch(),
		command:      newCommand(),
		detail:       newDetail(),
		form:         newFormPtr(cfg.GroupNames(), tplStore, nil),
		tplStore:     tplStore,
		help:         newHelp(),
		confirm:      newConfirm(),
		leader:       newLeader(),
		log:          l,
		sessions:     NewSessionManager(),
		sessionsView: newSessionsView(),
		quickConnect: NewQuickConnect(),
		pulse:        NewPulseModel(),
		auditLog:     al,
		finder:       finder,
		paneLayout:   NewPaneLayoutModel(),
		tutorial:     tut,
		viewStack:    []viewKind{viewList},
		undoStack:    NewUndoStack(),
		bracketNav:   NewBracketNav(),
		tagManager:   NewTagManager(),
		checker: health.NewChecker(health.Options{
			Workers: cfg.Settings.HealthWorkers(),
			Timeout: cfg.Settings.HealthTimeout(),
			Enabled: cfg.Settings.HealthEnabled(),
		}),
		mode:              ModeNormal,
		selectionSet:      NewSelectionSet(),
		viewTransition:    newTransition(6), // 6 frames @ 16ms = ~96ms
		animationsEnabled: cfg.Settings.AnimationsEnabled(),
	}
}

// profileNames returns a slice of credential profile names from the profile
// store. Returns nil if the profile store is unavailable or an error occurs.
func (a *App) profileNames() []string {
	if a.profileStore == nil {
		return nil
	}
	profiles, err := a.profileStore.List()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, p.Name)
	}
	return names
}

// inheritGroupProfile fills in conn.CredentialProfile from the group's cred
// profile when the connection has no explicit profile assigned. This makes
// group inheritance visible in the edit form and persists the assignment when
// the connection is saved.
func (a *App) inheritGroupProfile(conn *config.Connection) {
	if conn.CredentialProfile != "" || conn.Group == "" || a.profileStore == nil {
		return
	}
	gp, err := a.profileStore.FindByGroup(conn.Group)
	if err == nil && gp != nil {
		conn.CredentialProfile = gp.Name
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
	if a.animationsEnabled {
		a.viewTransition.markPending()
	}
}

func (a *App) popView() {
	if len(a.viewStack) > 1 {
		a.viewStack = a.viewStack[:len(a.viewStack)-1]
	}
	a.syncHeaderView()
	a.syncStatusBarView()
	if a.animationsEnabled {
		a.viewTransition.markPending()
	}
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
	case viewPaneLayout:
		a.statusBar.view = "panes"
	case viewVault:
		a.statusBar.view = "vault"
	case viewVaultForm:
		a.statusBar.view = "vault-form"
	case viewGroupList:
		a.statusBar.view = "groups"
	case viewFileBrowser:
		a.statusBar.view = "sftp"
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
		a.header.setView("Logs", 3)
		a.header.setItemCount(0)
	case viewSessions:
		a.header.setView("Sessions", 2)
		a.header.setItemCount(a.sessions.Count())
	case viewPulse:
		a.header.setView("Pulse", 4)
		a.header.setItemCount(0)
	case viewPaneLayout:
		a.header.setView("Panes", 5)
		a.header.setItemCount(a.paneLayout.PaneCount())
	case viewVault:
		a.header.setView("Cred Profiles", 6)
		a.header.setItemCount(0)
	case viewVaultForm:
		name := "Add Cred Profile"
		if a.vaultFormView.isEdit {
			name = "Edit Cred Profile"
		}
		a.header.setView(name, 6)
		a.header.setItemCount(0)
	case viewGroupList:
		a.header.setView("Groups", 7)
		a.header.setItemCount(len(a.groupListView.groups))
	case viewFileBrowser:
		name := "SFTP Browser"
		if a.fileBrowser.connName != "" {
			name = "SFTP: " + a.fileBrowser.connName
		}
		a.header.setView(name, 8)
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

// allSessions returns the combined list of interactive sessions plus ghost sessions.
func (a *App) allSessions() []*session.ManagedSession {
	all := a.sessions.All()
	if a.ghostMgr != nil {
		all = append(all, a.ghostMgr.Sessions()...)
	}
	return all
}

// setDetailConnection sets the connection on the detail view and, if a
// profileStore is available, resolves credential provenance so the detail
// view can render source labels alongside each credential field.
func (a *App) setDetailConnection(c *config.Connection, status health.Status, latency string) {
	a.detail.setConnection(c, status, latency)
	if c != nil && a.profileStore != nil {
		_, sources, err := vault.ResolveCredentials(*c, a.profileStore)
		if err == nil {
			a.detail.credSources = sources
			a.detail.updateContent()
		}
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
		a.checker.CheckAll(a.list.healthTargets()),
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
		// Tutorial overlay intercepts all keys until dismissed.
		if a.tutorial.active {
			var done bool
			a.tutorial, done = a.tutorial.Update(msg)
			if done {
				a.cfg.Settings.TutorialShown = true
				if err := config.Save(a.cfg); err != nil {
					a.log.warn("Could not save tutorial state: %v", err)
				}
			}
			return a, nil
		}

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
				a.filter.debounceSeq++
				seq := a.filter.debounceSeq
				debounce := tea.Tick(filterDebounceInterval, func(time.Time) tea.Msg {
					return filterDebounceMsg{seq: seq}
				})
				return a, tea.Batch(cmd, debounce)
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
				switch a.finder.picker {
				case PickerGroups:
					if len(a.finder.results) > 0 && a.finder.cursor >= 0 && a.finder.cursor < len(a.finder.results) {
						group := a.finder.results[a.finder.cursor].Display
						a.finder.Deactivate()
						a.mode = ModeNormal
						a.statusBar.mode = ModeNormal
						a.list.groupFilter = group
						a.list.applyGroupFilter()
						a.syncHeaderView()
						a.syncCursorPosition()
						a.statusBar.setFlash("Filtered by group: "+group, flashInfo)
						return a, scheduleFlashClear()
					}
				case PickerTags:
					if len(a.finder.results) > 0 && a.finder.cursor >= 0 && a.finder.cursor < len(a.finder.results) {
						tag := a.finder.results[a.finder.cursor].Display
						a.finder.Deactivate()
						a.mode = ModeNormal
						a.statusBar.mode = ModeNormal
						a.applyTagFilter(tag)
						a.syncHeaderView()
						a.syncCursorPosition()
						a.statusBar.setFlash("Filtered by tag: "+tag, flashInfo)
						return a, scheduleFlashClear()
					}
				default:
					if c := a.finder.SelectedConnection(); c != nil {
						a.finder.Deactivate()
						a.mode = ModeNormal
						a.statusBar.mode = ModeNormal
						if c.Protocol == config.ProtoSSH {
							return a.connectManaged(*c)
						}
						return a.connectByID(c.ID)
					}
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

		if a.currentView() == viewVaultForm {
			var cmd tea.Cmd
			a.vaultFormView, cmd = a.vaultFormView.Update(msg)
			return a, cmd
		}

		if a.currentView() == viewLog {
			return a.handleLogKey(msg)
		}

		return a.handleKey(msg)

	case saveErrorMsg:
		a.log.error("Background save failed: %v", msg.err)
		return a, nil

	case filterDebounceMsg:
		// Only apply if this tick matches the latest keystroke sequence.
		if a.filter.active && msg.seq == a.filter.debounceSeq {
			a.list.applyFilter(a.filter.value())
			a.header.setFilter(a.filter.value())
			a.header.setItemCount(len(a.list.filtered))
			a.syncCursorPosition()
		}
		return a, nil

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
			a.checker.CheckAll(a.list.healthTargets()),
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
		cmds = append(cmds, scheduleFlashClear(), tea.ClearScreen)
		return a, tea.Batch(cmds...)

	case SessionDetachedMsg:
		a.log.info("Session %s detached (%s)", msg.SessionID, msg.ConnName)
		a.statusBar.setFlash(fmt.Sprintf("Session detached [s:sessions]"), flashInfo)
		a.updateSessionCount()
		cmds = append(cmds, scheduleFlashClear(), tea.ClearScreen)
		// Refresh sessions view so status shows "detached"
		if a.currentView() == viewSessions {
			a.sessionsView.setSessions(a.allSessions())
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
			a.sessionsView.setSessions(a.allSessions())
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

	case broadcastFlashMsg:
		a.statusBar.setFlash(msg.text, flashInfo)
		return a, scheduleFlashClear()

	case transitionTickMsg:
		if a.animationsEnabled {
			return a, a.viewTransition.advance()
		}
		return a, nil

	case launcher.GUILogLineMsg:
		level := logInfo
		switch msg.Level {
		case "warn":
			level = logWarn
		case "error":
			level = logError
		}
		a.log.addDebug(level, msg.Message)
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

	case VaultCloseMsg:
		a.popView()
		return a, nil

	case GroupListCloseMsg:
		a.popView()
		return a, nil

	case fileBrowserCloseMsg:
		a.popView()
		return a, nil

	case fileBrowserOpenMsg:
		var cmd tea.Cmd
		cmd = a.fileBrowser.open(msg)
		a.fileBrowser.width = a.width
		a.fileBrowser.height = a.contentHeight()
		a.pushView(viewFileBrowser)
		a.syncHeaderView()
		return a, cmd

	case fileBrowserReadyMsg:
		var cmd tea.Cmd
		a.fileBrowser, cmd = a.fileBrowser.Update(msg)
		a.syncHeaderView()
		return a, cmd

	case fileBrowserErrMsg:
		var cmd tea.Cmd
		a.fileBrowser, cmd = a.fileBrowser.Update(msg)
		return a, cmd

	case fileBrowserListDoneMsg:
		var cmd tea.Cmd
		a.fileBrowser, cmd = a.fileBrowser.Update(msg)
		return a, cmd

	case fileBrowserOpDoneMsg:
		var cmd tea.Cmd
		a.fileBrowser, cmd = a.fileBrowser.Update(msg)
		return a, cmd

	case GroupListDeleteMsg:
		connCount := 0
		for _, conn := range a.cfg.Connections {
			if conn.Group == msg.GroupName {
				connCount++
			}
		}
		if connCount > 0 {
			prompt := fmt.Sprintf("Group '%s' has %d connection(s). Move them to ungrouped and delete?", msg.GroupName, connCount)
			a.confirm.show(prompt, "delete-group:"+msg.GroupName, msg.GroupName)
			a.confirm.width = a.width
			a.confirm.height = a.height
			return a, nil
		}
		if err := a.cfg.DeleteGroup(msg.GroupName); err != nil {
			a.statusBar.setFlash("Error deleting group: "+err.Error(), flashError)
		} else {
			a.statusBar.setFlash("Group '"+msg.GroupName+"' deleted", flashInfo)
			a.refreshGroupListView()
		}
		return a, scheduleFlashClear()

	case VaultCreateMsg:
		a.vaultFormView = newVaultForm(a.cfg.Groups, nil)
		a.vaultFormView.setSize(a.width, a.contentHeight())
		a.pushView(viewVaultForm)
		return a, a.vaultFormView.form.Init()

	case VaultEditMsg:
		a.vaultFormView = newVaultForm(a.cfg.Groups, &msg.Profile)
		a.vaultFormView.setSize(a.width, a.contentHeight())
		a.pushView(viewVaultForm)
		return a, a.vaultFormView.form.Init()

	case VaultDeleteMsg:
		if a.profileStore != nil {
			// Count connections that reference this profile (direct or group-inherited).
			refCount := 0
			for _, conn := range a.cfg.Connections {
				if conn.CredentialProfile == msg.Profile.Name {
					refCount++
				} else if msg.Profile.Group != "" && conn.Group == msg.Profile.Group && conn.CredentialProfile == "" {
					refCount++
				}
			}
			if refCount > 0 {
				prompt := fmt.Sprintf("Cred profile '%s' is used by %d connection(s). Delete anyway?", msg.Profile.Name, refCount)
				a.confirm.show(prompt, "delete-profile:"+msg.Profile.Name, msg.Profile.Name)
				a.confirm.width = a.width
				a.confirm.height = a.height
				return a, nil
			}
			if err := a.profileStore.Delete(msg.Profile.Name); err != nil {
				a.statusBar.setFlash("Error deleting cred profile: "+err.Error(), flashError)
			} else {
				a.statusBar.setFlash("Cred profile '"+msg.Profile.Name+"' deleted", flashInfo)
				a.refreshVaultView()
			}
		}
		return a, scheduleFlashClear()

	case VaultFormSubmitMsg:
		a.handleVaultFormSubmit(msg)
		a.popView()
		return a, scheduleFlashClear()

	case VaultFormCancelMsg:
		a.popView()
		return a, nil

	case ConfirmResultMsg:
		return a.handleConfirmResult(msg)

	case ModeChangedMsg:
		a.statusBar.mode = msg.To
		return a, nil

	case AllPanesClosedMsg:
		if a.currentView() == viewPaneLayout {
			a.popView()
		}
		return a, nil

	case PaneOutputMsg:
		updated, cmd := a.paneLayout.Update(msg)
		a.paneLayout = &updated
		return a, cmd

	case PaneConnectedMsg:
		updated, cmd := a.paneLayout.Update(msg)
		a.paneLayout = &updated
		a.syncHeaderView()
		return a, cmd

	case PaneDisconnectedMsg:
		updated, cmd := a.paneLayout.Update(msg)
		a.paneLayout = &updated
		a.syncHeaderView()
		return a, cmd

	case panePickerMsg:
		a.resolveProfileCredentials(&msg.Connection)
		updated, cmd := a.paneLayout.Update(msg)
		a.paneLayout = &updated
		return a, cmd

	case paneSessionStartedMsg:
		updated, cmd := a.paneLayout.Update(msg)
		a.paneLayout = &updated
		a.syncHeaderView()
		return a, cmd

	default:
		// Forward unrecognized terminal escape sequences (e.g. Shift+Enter,
		// Ctrl+Alt+Enter) to the active SSH session when in insert mode.
		// Bubbletea dispatches unknown CSI sequences as an unexported []byte
		// type that we can't switch on directly.
		if a.mode == ModeInsert && a.currentView() == viewPaneLayout {
			if raw := extractRawBytes(msg); len(raw) > 0 {
				a.paneLayout.RouteInput(translateModifyOtherKeys(raw))
				return a, nil
			}
		}
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
	case viewVault:
		var cmd tea.Cmd
		a.vaultView, cmd = a.vaultView.Update(msg)
		cmds = append(cmds, cmd)
	case viewVaultForm:
		var cmd tea.Cmd
		a.vaultFormView, cmd = a.vaultFormView.Update(msg)
		cmds = append(cmds, cmd)
	case viewGroupList:
		var cmd tea.Cmd
		a.groupListView, cmd = a.groupListView.Update(msg)
		cmds = append(cmds, cmd)
	case viewFileBrowser:
		var cmd tea.Cmd
		a.fileBrowser, cmd = a.fileBrowser.Update(msg)
		cmds = append(cmds, cmd)
	case viewPaneLayout:
		updated, cmd := a.paneLayout.Update(msg)
		a.paneLayout = &updated
		cmds = append(cmds, cmd)
	}

	// Start any pending view transition (queued by pushView/popView).
	if cmd := a.viewTransition.consumePending(); cmd != nil {
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
	case viewPaneLayout:
		return a.handlePaneLayoutKey(msg)
	case viewVault:
		return a.handleVaultKey(msg)
	case viewVaultForm:
		// VaultForm keys are handled via the huh form Update in the pass-through switch.
		return a, nil
	case viewGroupList:
		return a.handleGroupListKey(msg)
	case viewFileBrowser:
		return a.handleFileBrowserKey(msg)
	}
	return a, nil
}

func (a *App) confirmQuit() {
	a.confirm.show("Quit Nexus?", "quit", "")
	a.confirm.width = a.width
	a.confirm.height = a.height
}

func (a App) View() string {
	if !a.ready {
		return "Loading..."
	}

	// Tutorial overlay — takes full screen on first launch.
	if a.tutorial.active {
		return a.tutorial.View()
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
	case viewPaneLayout:
		contentView = a.paneLayout.View()
	case viewVault:
		contentView = a.vaultView.View()
	case viewVaultForm:
		contentView = a.vaultFormView.View()
	case viewGroupList:
		contentView = a.groupListView.View()
	case viewFileBrowser:
		contentView = a.fileBrowser.View()
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
	a.tutorial.width = a.width
	a.tutorial.height = a.height
	a.leader.width = a.width
	a.quickConnect.width = a.width
	a.leader.height = a.height

	ch := a.contentHeight()
	a.list.setSize(a.width, ch)
	a.detail.setSize(a.width, ch)
	a.log.setSize(a.width, ch)
	a.sessionsView.setSize(a.width, ch)
	a.vaultFormView.setSize(a.width, ch)
	a.groupListView.width = a.width
	a.groupListView.height = ch
	a.fileBrowser.width = a.width
	a.fileBrowser.height = ch
	a.pulse.SetSize(a.width, ch)
	if a.currentView() == viewPaneLayout {
		a.paneLayout.SetSize(a.width, ch)
	}

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
