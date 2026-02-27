package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/dr4zz/nexus/internal/termcap"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/audit"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/health"
	"github.com/dr4zz/nexus/internal/importexport"
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

func (a App) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()

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
	if a.selectionSet.VisualActive() {
		return a.handleVisualKey(msg)
	}

	// --- Bulk operation intercept (Normal mode with a picker selection) ---
	// When items are selected via Tab/Ctrl+A and no motion sequence is pending,
	// the d and y keys operate on the full selection instead of the single cursor row.
	if a.selectionSet.HasSelection() && !a.list.table.motion.Pending() {
		switch {
		case key.Matches(msg, a.keys.Delete):
			return a.bulkDeleteSelected()
		case key.Matches(msg, a.keys.Yank):
			return a.bulkYankSelected()
		}
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
	switch {
	case k == "ctrl+a":
		// Toggle select-all / deselect-all for the visible (filtered) rows.
		rowCount := len(a.list.table.rows)
		if a.selectionSet.HasSelection() {
			a.selectionSet.DeselectAll()
			a.list.table.selectionSet = nil
		} else {
			a.selectionSet.SelectAll(rowCount)
			if a.selectionSet.HasSelection() {
				a.list.table.selectionSet = &a.selectionSet
			}
		}
		a.statusBar.visualCount = a.selectionSet.Count()
		return a, nil

	case key.Matches(msg, a.keys.Escape):
		// In Normal mode with a picker selection (but not visual mode), clear the selection.
		if a.selectionSet.HasSelection() && !a.selectionSet.VisualActive() {
			a.selectionSet.DeselectAll()
			a.list.table.selectionSet = nil
			a.statusBar.visualCount = 0
			return a, nil
		}
		// Otherwise fall through (no further esc handling at list level in normal mode).

	case k == "tab": // Toggle selection on current row
		if len(a.list.table.rows) == 0 {
			return a, nil
		}
		cursor := a.list.table.Cursor()
		a.selectionSet.Toggle(cursor)
		if a.selectionSet.HasSelection() {
			a.list.table.selectionSet = &a.selectionSet
		} else {
			a.list.table.selectionSet = nil
		}
		a.statusBar.visualCount = a.selectionSet.Count()
		return a, nil

	case k == "shift+tab": // Deselect current row (idempotent; no-op on empty list)
		if len(a.list.table.rows) == 0 {
			return a, nil
		}
		cursor := a.list.table.Cursor()
		a.selectionSet.Deselect(cursor)
		if a.selectionSet.HasSelection() {
			a.list.table.selectionSet = &a.selectionSet
		} else {
			a.list.table.selectionSet = nil
		}
		a.statusBar.visualCount = a.selectionSet.Count()
		return a, nil

	case k == " ": // Space = leader key
		if !a.leader.active {
			cmd := a.leader.activate()
			return a, cmd
		}
	case key.Matches(msg, a.keys.Quit):
		a.confirmQuit()
		return a, nil
	case k == "/":
		a.search.activate(SearchForward)
		a.search.width = a.width
		a.list.table.highlightText = ""
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.setMode(ModeInsert)
	case k == "?":
		a.search.activate(SearchReverse)
		a.search.width = a.width
		a.list.table.highlightText = ""
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.setMode(ModeInsert)
	case k == "n":
		return a.searchNextMatch(true)
	case k == "*":
		return a.searchUnderCursor(SearchForward)
	case k == "#":
		return a.searchUnderCursor(SearchReverse)
	case key.Matches(msg, a.keys.Command):
		a.command.activate()
		a.mode = ModeCommand
		a.statusBar.mode = ModeCommand
		return a, a.command.input.Focus()
	case key.Matches(msg, a.keys.Enter):
		return a.connectSelected()
	case key.Matches(msg, a.keys.Add):
		a.form.startAdd(a.cfg.GroupNames(), a.profileNames())
		a.form.width = a.width
		a.form.height = a.contentHeight()
		a.pushView(viewForm)
		a.help.view = "form"
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.form.form.Init()
	case key.Matches(msg, a.keys.Edit):
		if c := a.list.selectedConnection(); c != nil {
			conn := *c
			a.inheritGroupProfile(&conn)
			a.form.startEdit(conn, a.cfg.GroupNames(), a.profileNames())
			a.form.width = a.width
			a.form.height = a.contentHeight()
			a.pushView(viewForm)
			a.help.view = "form"
			a.mode = ModeInsert
			a.statusBar.mode = ModeInsert
			return a, a.form.form.Init()
		}
		return a, nil
	case key.Matches(msg, a.keys.Detail):
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
	case key.Matches(msg, a.keys.Refresh):
		a.log.info("Manual health check refresh")
		a.statusBar.setFlash("Refreshing...", flashInfo)
		return a, tea.Batch(
			a.checker.CheckAll(a.list.healthTargets()),
			scheduleFlashClear(),
		)
	case key.Matches(msg, a.keys.Sessions):
		a.sessionsView.setSessions(a.allSessions())
		a.sessionsView.setSize(a.width, a.contentHeight())
		a.pushView(viewSessions)
		a.help.view = "sessions"
		return a, nil

	case k == "ctrl+l": // View event log
		a.log.setSize(a.width, a.contentHeight())
		a.pushView(viewLog)
		return a, nil

	case k == "V": // Enter line-visual mode
		return a.enterVisualMode()

	case k == "N": // Jump to previous search match (opposite direction of n)
		return a.searchNextMatch(false)
	case k == "S": // Sort by Status
		if idx := a.list.sortKeyToColumnIndex("status"); idx >= 0 {
			a.list.table.CycleSort(idx)
		}
		return a, nil
	case k == "P": // Sort by Protocol
		if idx := a.list.sortKeyToColumnIndex("protocol"); idx >= 0 {
			a.list.table.CycleSort(idx)
		}
		return a, nil

	// Wide mode toggle

	case k == "u": // Undo
		return a.performUndo()
	case k == "ctrl+r": // Redo
		return a.performRedo()
	case k == "ctrl+w":
		a.list.toggleWideMode()
		return a, nil
	case k == "ctrl+p":
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
	a.selectionSet.EnterVisual(cursor)
	a.list.table.selectionSet = &a.selectionSet
	a.mode = ModeVisual
	a.statusBar.mode = ModeVisual
	a.statusBar.visualCount = a.selectionSet.Count()
	return a, a.setMode(ModeVisual)
}

// exitVisualMode returns to Normal mode but preserves any picker selection (Tab-selected items).
// The status bar is updated to reflect the remaining selection count.
func (a *App) exitVisualMode() tea.Cmd {
	a.selectionSet.ExitVisual()
	if !a.selectionSet.HasSelection() {
		a.list.table.selectionSet = nil
	}
	a.mode = ModeNormal
	a.statusBar.mode = ModeNormal
	// Show "N selected" in Normal mode if items are still Tab-selected after leaving Visual.
	a.statusBar.visualCount = a.selectionSet.Count()
	return a.setMode(ModeNormal)
}

// clearAllSelection fully resets all selection state: exits Visual mode, deselects all items,
// clears the table's selection pointer, resets mode to Normal, and zeroes the status-bar count.
// Use this after bulk operations where selections must no longer persist.
func (a *App) clearAllSelection() tea.Cmd {
	a.selectionSet.ExitVisual()
	a.selectionSet.DeselectAll()
	a.list.table.selectionSet = nil
	a.statusBar.mode = ModeNormal
	a.statusBar.visualCount = 0
	return a.setMode(ModeNormal)
}

// syncVisualStatus updates the statusbar with the current visual selection count.
func (a *App) syncVisualStatus() {
	a.statusBar.visualCount = a.selectionSet.Count()
}

// handleVisualKey processes key events while in Visual mode.
func (a App) handleVisualKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	cursor := a.list.table.Cursor()
	total := len(a.list.table.rows)

	switch {
	case key.Matches(msg, a.keys.Escape):
		cmd := a.exitVisualMode()
		return a, cmd

	case key.Matches(msg, a.keys.Down):
		// Move cursor down and extend selection range
		newCursor := min(cursor+1, total-1)
		a.list.table.MoveCursor(newCursor)
		a.selectionSet.UpdateRange(newCursor)
		a.list.table.selectionSet = &a.selectionSet
		a.syncVisualStatus()
		a.syncCursorPosition()
		return a, nil

	case key.Matches(msg, a.keys.Up):
		// Move cursor up and extend/contract selection range
		newCursor := max(cursor-1, 0)
		a.list.table.MoveCursor(newCursor)
		a.selectionSet.UpdateRange(newCursor)
		a.list.table.selectionSet = &a.selectionSet
		a.syncVisualStatus()
		a.syncCursorPosition()
		return a, nil

	case k == "G":
		// Jump to last row, extend selection
		a.list.table.MoveCursor(total - 1)
		a.selectionSet.UpdateRange(total - 1)
		a.list.table.selectionSet = &a.selectionSet
		a.syncVisualStatus()
		a.syncCursorPosition()
		return a, nil

	case k == "tab":
		// Cherry-pick / deselect individual items while staying in Visual mode.
		a.selectionSet.Toggle(cursor)
		a.list.table.selectionSet = &a.selectionSet
		a.syncVisualStatus()
		return a, nil

	case k == "ctrl+d":
		// Half-page down; extend selection range addditively.
		pageHalf := max(a.list.table.height/2, 1)
		newCursor := min(cursor+pageHalf, total-1)
		a.list.table.MoveCursor(newCursor)
		a.selectionSet.UpdateRange(newCursor)
		a.list.table.selectionSet = &a.selectionSet
		a.syncVisualStatus()
		a.syncCursorPosition()
		return a, nil

	case k == "ctrl+u":
		// Half-page up; extend selection range additively.
		pageHalf := max(a.list.table.height/2, 1)
		newCursor := max(cursor-pageHalf, 0)
		a.list.table.MoveCursor(newCursor)
		a.selectionSet.UpdateRange(newCursor)
		a.list.table.selectionSet = &a.selectionSet
		a.syncVisualStatus()
		a.syncCursorPosition()
		return a, nil

	case k == "v":
		// Toggle current item in/out of selection (non-contiguous)
		a.selectionSet.Toggle(cursor)
		a.list.table.selectionSet = &a.selectionSet
		a.syncVisualStatus()
		return a, nil

	case key.Matches(msg, a.keys.Delete):
		// Delete all selected connections (with confirmation)
		count := a.selectionSet.Count()
		if count == 0 {
			return a, nil
		}
		ids := a.selectionSet.SelectedIDs(a.list.table.rows)
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

	case key.Matches(msg, a.keys.Yank):
		// Yank all selected connection details to clipboard
		return a.yankVisualSelection()
	}

	return a, nil
}

// bulkDeleteSelected shows a bulk-delete confirmation for all Tab-selected connections
// (Normal mode, non-visual). After confirmation the confirm handler calls exitVisualMode
// (via "delete-visual" action) which also calls DeselectAll.
func (a App) bulkDeleteSelected() (tea.Model, tea.Cmd) {
	count := a.selectionSet.Count()
	if count == 0 {
		return a, nil
	}
	ids := a.selectionSet.SelectedIDs(a.list.table.rows)
	if len(ids) == 0 {
		return a, nil
	}
	prompt := fmt.Sprintf("Delete %d selected connections?", count)
	idStr := strings.Join(ids, ",")
	a.confirm.show(prompt, "delete-visual", idStr)
	a.confirm.width = a.width
	a.confirm.height = a.height
	return a, nil
}

// bulkYankSelected copies commands for all Tab-selected connections to the clipboard
// (Normal mode, non-visual). Clears the selection after yanking.
func (a App) bulkYankSelected() (tea.Model, tea.Cmd) {
	ids := a.selectionSet.SelectedIDs(a.list.table.rows)
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
	if err := termcap.DefaultClipboard().WriteAll(text); err != nil {
		a.log.error("Clipboard error: %v", err)
		a.statusBar.setFlash("Clipboard error: "+err.Error(), flashError)
	} else {
		a.log.info("Copied %d connection commands to clipboard", len(lines))
		a.statusBar.setFlash(fmt.Sprintf("Copied %d commands", len(lines)), flashInfo)
	}

	// Clear selection after yanking.
	clearCmd := a.clearAllSelection()
	return a, tea.Batch(clearCmd, scheduleFlashClear())
}

// yankVisualSelection copies the details of all visually selected connections to the clipboard.
func (a App) yankVisualSelection() (tea.Model, tea.Cmd) {
	ids := a.selectionSet.SelectedIDs(a.list.table.rows)
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
	if err := termcap.DefaultClipboard().WriteAll(text); err != nil {
		a.log.error("Clipboard error: %v", err)
		a.statusBar.setFlash("Clipboard error: "+err.Error(), flashError)
	} else {
		a.log.info("Copied %d connection commands to clipboard", len(lines))
		a.statusBar.setFlash(fmt.Sprintf("Copied %d commands", len(lines)), flashInfo)
	}

	// Clear all selection after yanking (spec: selections must clear after bulk operations).
	clearCmd := a.clearAllSelection()
	return a, tea.Batch(clearCmd, scheduleFlashClear())
}

func (a App) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch {
	case k == " ": // Space = leader key
		if !a.leader.active {
			cmd := a.leader.activate()
			return a, cmd
		}
	case key.Matches(msg, a.keys.Quit):
		a.confirmQuit()
		return a, nil
	case key.Matches(msg, a.keys.Help):
		a.help.view = "detail"
		a.help.toggle()
		return a, nil
	case key.Matches(msg, a.keys.Escape):
		a.popView()
		a.help.view = "list"
		return a, nil
	case key.Matches(msg, a.keys.Enter):
		return a.connectSelected()
	case key.Matches(msg, a.keys.Edit):
		if c := a.list.selectedConnection(); c != nil {
			conn := *c
			a.inheritGroupProfile(&conn)
			a.form.startEdit(conn, a.cfg.GroupNames(), a.profileNames())
			a.form.width = a.width
			a.form.height = a.contentHeight()
			a.pushView(viewForm)
			a.help.view = "form"
			a.mode = ModeInsert
			a.statusBar.mode = ModeInsert
			return a, a.form.form.Init()
		}
		return a, nil
	case key.Matches(msg, a.keys.ShowPassword):
		a.detail.showPassword = !a.detail.showPassword
		a.detail.updateContent()
		return a, nil
	case key.Matches(msg, a.keys.Sessions):
		a.sessionsView.setSessions(a.allSessions())
		a.sessionsView.setSize(a.width, a.contentHeight())
		a.pushView(viewSessions)
		a.help.view = "sessions"
		return a, nil
	case k == "ctrl+l":
		a.log.setSize(a.width, a.contentHeight())
		a.pushView(viewLog)
		return a, nil
	}

	var cmd tea.Cmd
	a.detail, cmd = a.detail.Update(msg)
	return a, cmd
}

func (a App) handleSessionsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch {
	case k == " ": // Space = leader key
		if !a.leader.active {
			cmd := a.leader.activate()
			return a, cmd
		}
	case key.Matches(msg, a.keys.Quit):
		a.confirmQuit()
		return a, nil
	case key.Matches(msg, a.keys.Help):
		a.help.view = "sessions"
		a.help.toggle()
		return a, nil
	case key.Matches(msg, a.keys.Escape):
		a.popView()
		a.help.view = "list"
		return a, nil
	case key.Matches(msg, a.keys.Enter):
		return a.reattachSelected()
	case key.Matches(msg, a.keys.KillSession):
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
	case k == "ctrl+l":
		a.log.setSize(a.width, a.contentHeight())
		a.pushView(viewLog)
		return a, nil
	case key.Matches(msg, a.keys.Down):
		a.sessionsView.table.MoveDown(1)
		return a, nil
	case key.Matches(msg, a.keys.Up):
		a.sessionsView.table.MoveUp(1)
		return a, nil
	case k == "g":
		a.sessionsView.table.GotoTop()
		return a, nil
	case k == "G":
		a.sessionsView.table.GotoBottom()
		return a, nil
	}

	// Pass remaining keys to table
	var cmd tea.Cmd
	a.sessionsView, cmd = a.sessionsView.Update(msg)
	return a, cmd
}

// handlePaneLayoutKey routes key events when the pane layout view is active.
// In Normal mode, Space triggers the leader key menu.
// In Insert mode, all key input is forwarded as raw bytes to the active pane.
func (a App) handlePaneLayoutKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()

	if a.mode == ModeInsert {
		// ESC (or alt+esc due to terminal escape-sequence timing ambiguity)
		// exits insert mode without forwarding to the session.
		// Check msg.Type directly to catch both "esc" and "alt+esc" variants.
		// Do not pre-set a.mode here — let setMode() perform the transition so
		// that a ModeChangedMsg is properly emitted for any listeners.
		if msg.Type == tea.KeyEsc {
			a.statusBar.mode = ModeNormal
			return a, a.setMode(ModeNormal)
		}
		// Forward input as raw bytes to the active pane's session.
		data := keyMsgToBytes(msg)
		if len(data) > 0 {
			a.paneLayout.RouteInput(data)
		}
		return a, nil
	}

	// When the active pane has an open connection picker, route picker keys
	// before the normal mode handler so j/k/Enter/Esc go to the picker.
	if active := a.paneLayout.ActivePane(); active != nil && active.Picker != nil && active.Picker.Active {
		switch {
		case key.Matches(msg, a.keys.Down):
			active.Picker.MoveDown()
			return a, nil
		case key.Matches(msg, a.keys.Up):
			active.Picker.MoveUp()
			return a, nil
		case key.Matches(msg, a.keys.Enter):
			conn := active.Picker.Confirm()
			if conn != nil {
				// Deliver the selection as a message so PaneLayoutModel.Update
				// can start the background session and transition the pane state.
				pickerPaneID := active.ID
				capturedConn := *conn
				return a, func() tea.Msg {
					return panePickerMsg{PaneID: pickerPaneID, Connection: capturedConn}
				}
			}
			// No connections available — just close the picker.
			active.Picker = nil
			return a, nil
		case key.Matches(msg, a.keys.Escape):
			active.Picker.Cancel()
			active.Picker = nil
			return a, nil
		}
		return a, nil
	}

	// Normal mode key handling
	switch {
	case k == " ": // Space = leader key
		if !a.leader.active {
			cmd := a.leader.activate()
			return a, cmd
		}
	case key.Matches(msg, a.keys.Escape):
		// Pop the pane layout view (return to connection list)
		a.popView()
		return a, nil
	case key.Matches(msg, a.keys.Quit):
		a.confirmQuit()
		return a, nil
	case k == "i":
		// Enter insert mode to send input to the active pane.
		// Do not pre-set a.mode — let setMode() perform the transition so that
		// a ModeChangedMsg is properly emitted for any listeners.
		a.statusBar.mode = ModeInsert
		return a, a.setMode(ModeInsert)
	case key.Matches(msg, a.keys.Enter):
		active := a.paneLayout.ActivePane()
		if active == nil {
			return a, nil
		}
		switch active.State {
		case PaneEmpty:
			// Open the connection picker so the user can select a connection.
			a.paneLayout.OpenPickerForPane(active.ID, a.cfg.Connections)
		case PaneDisconnected:
			// Attempt to reconnect using the previously selected connection.
			if active.Connection != nil {
				capturedConn := *active.Connection
				capturedPaneID := active.ID
				return a, func() tea.Msg {
					return panePickerMsg{PaneID: capturedPaneID, Connection: capturedConn}
				}
			}
			// No stored connection — re-open the picker to let the user choose.
			a.paneLayout.OpenPickerForPane(active.ID, a.cfg.Connections)
		}
		return a, nil
	}

	return a, nil
}

// keyMsgToBytes converts a bubbletea KeyMsg to the raw bytes that a terminal
// program expects. Printable runes are encoded as UTF-8; special keys are
// mapped to their ANSI/VT100 escape sequences.
// translateModifyOtherKeys replaces xterm modifyOtherKeys escape sequences
// (\x1b[27;modifier;keycode~) with the plain character they represent.
func translateModifyOtherKeys(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	i := 0
	for i < len(raw) {
		if i+4 < len(raw) && raw[i] == 0x1b && raw[i+1] == '[' && raw[i+2] == '2' && raw[i+3] == '7' && raw[i+4] == ';' {
			j := i + 5
			for j < len(raw) && raw[j] >= '0' && raw[j] <= '9' {
				j++
			}
			if j < len(raw) && raw[j] == ';' {
				j++
				codeStart := j
				for j < len(raw) && raw[j] >= '0' && raw[j] <= '9' {
					j++
				}
				if j < len(raw) && raw[j] == '~' && j > codeStart {
					keycode := 0
					for _, c := range raw[codeStart:j] {
						keycode = keycode*10 + int(c-'0')
					}
					if keycode > 0 && keycode < 128 {
						out = append(out, byte(keycode))
					}
					i = j + 1
					continue
				}
			}
		}
		out = append(out, raw[i])
		i++
	}
	return out
}

// extractRawBytes extracts raw byte data from a tea.Msg whose underlying type
// is []byte (e.g. Bubbletea's unexported unknownCSISequenceMsg) or byte
// (unknownInputByteMsg). Returns nil for all other message types.
func extractRawBytes(msg tea.Msg) []byte {
	v := reflect.ValueOf(msg)
	switch v.Kind() {
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return v.Bytes()
		}
	case reflect.Uint8:
		return []byte{byte(v.Uint())}
	}
	return nil
}

func keyMsgToBytes(msg tea.KeyMsg) []byte {
	k := msg.String()

	// Printable runes — encode as UTF-8
	if msg.Type == tea.KeyRunes {
		s := string(msg.Runes)
		return []byte(s)
	}

	switch k {
	case "enter":
		return []byte{'\r'}
	case "shift+enter", "ctrl+enter", "alt+enter", "ctrl+alt+enter":
		// Modified Enter — most remote shells treat these as plain Enter.
		return []byte{'\r'}
	case "shift+tab":
		return []byte{0x1b, '[', 'Z'}
	case "tab":
		return []byte{'\t'}
	case "backspace":
		return []byte{0x7f}
	case "space":
		return []byte{' '}

	// Common control keys
	case "ctrl+a":
		return []byte{0x01}
	case "ctrl+b":
		return []byte{0x02}
	case "ctrl+c":
		return []byte{0x03}
	case "ctrl+d":
		return []byte{0x04}
	case "ctrl+e":
		return []byte{0x05}
	case "ctrl+f":
		return []byte{0x06}
	case "ctrl+g":
		return []byte{0x07}
	case "ctrl+h":
		return []byte{0x08}
	case "ctrl+i":
		return []byte{0x09}
	case "ctrl+j":
		return []byte{0x0a}
	case "ctrl+k":
		return []byte{0x0b}
	case "ctrl+l":
		return []byte{0x0c}
	case "ctrl+m":
		return []byte{0x0d}
	case "ctrl+n":
		return []byte{0x0e}
	case "ctrl+o":
		return []byte{0x0f}
	case "ctrl+p":
		return []byte{0x10}
	case "ctrl+q":
		return []byte{0x11}
	case "ctrl+r":
		return []byte{0x12}
	case "ctrl+s":
		return []byte{0x13}
	case "ctrl+t":
		return []byte{0x14}
	case "ctrl+u":
		return []byte{0x15}
	case "ctrl+v":
		return []byte{0x16}
	case "ctrl+w":
		return []byte{0x17}
	case "ctrl+x":
		return []byte{0x18}
	case "ctrl+y":
		return []byte{0x19}
	case "ctrl+z":
		return []byte{0x1a}

	// Escape
	case "esc":
		return []byte{0x1b}

	// Arrow keys (ANSI)
	case "up":
		return []byte{0x1b, '[', 'A'}
	case "down":
		return []byte{0x1b, '[', 'B'}
	case "right":
		return []byte{0x1b, '[', 'C'}
	case "left":
		return []byte{0x1b, '[', 'D'}

	// Modified arrow keys (xterm)
	case "shift+up":
		return []byte{0x1b, '[', '1', ';', '2', 'A'}
	case "shift+down":
		return []byte{0x1b, '[', '1', ';', '2', 'B'}
	case "shift+right":
		return []byte{0x1b, '[', '1', ';', '2', 'C'}
	case "shift+left":
		return []byte{0x1b, '[', '1', ';', '2', 'D'}
	case "alt+up":
		return []byte{0x1b, '[', '1', ';', '3', 'A'}
	case "alt+down":
		return []byte{0x1b, '[', '1', ';', '3', 'B'}
	case "alt+right":
		return []byte{0x1b, '[', '1', ';', '3', 'C'}
	case "alt+left":
		return []byte{0x1b, '[', '1', ';', '3', 'D'}
	case "ctrl+up":
		return []byte{0x1b, '[', '1', ';', '5', 'A'}
	case "ctrl+down":
		return []byte{0x1b, '[', '1', ';', '5', 'B'}
	case "ctrl+right":
		return []byte{0x1b, '[', '1', ';', '5', 'C'}
	case "ctrl+left":
		return []byte{0x1b, '[', '1', ';', '5', 'D'}

	// Navigation keys
	case "home":
		return []byte{0x1b, '[', 'H'}
	case "end":
		return []byte{0x1b, '[', 'F'}
	case "pgup":
		return []byte{0x1b, '[', '5', '~'}
	case "pgdown":
		return []byte{0x1b, '[', '6', '~'}
	case "delete":
		return []byte{0x1b, '[', '3', '~'}
	case "insert":
		return []byte{0x1b, '[', '2', '~'}

	// Function keys
	case "f1":
		return []byte{0x1b, 'O', 'P'}
	case "f2":
		return []byte{0x1b, 'O', 'Q'}
	case "f3":
		return []byte{0x1b, 'O', 'R'}
	case "f4":
		return []byte{0x1b, 'O', 'S'}
	case "f5":
		return []byte{0x1b, '[', '1', '5', '~'}
	case "f6":
		return []byte{0x1b, '[', '1', '7', '~'}
	case "f7":
		return []byte{0x1b, '[', '1', '8', '~'}
	case "f8":
		return []byte{0x1b, '[', '1', '9', '~'}
	case "f9":
		return []byte{0x1b, '[', '2', '0', '~'}
	case "f10":
		return []byte{0x1b, '[', '2', '1', '~'}
	case "f11":
		return []byte{0x1b, '[', '2', '3', '~'}
	case "f12":
		return []byte{0x1b, '[', '2', '4', '~'}
	}

	// Single printable ASCII character
	if len(k) == 1 {
		return []byte(k)
	}

	return nil
}

func (a App) handleLogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch {
	case key.Matches(msg, a.keys.Quit):
		a.confirmQuit()
		return a, nil
	case key.Matches(msg, a.keys.Escape):
		a.popView()
		return a, nil
	case k == "tab":
		a.log.nextType()
		return a, nil
	case key.Matches(msg, a.keys.Yank):
		// Yank (copy) all log entries to system clipboard
		text := a.log.plainText()
		if err := termcap.DefaultClipboard().WriteAll(text); err != nil {
			a.statusBar.setFlash("Clipboard error: "+err.Error(), flashError)
		} else {
			a.statusBar.setFlash("Copied log to clipboard", flashInfo)
		}
		return a, scheduleFlashClear()
	}

	// Pass scroll keys to viewport
	cmd := a.log.Update(msg)
	return a, cmd
}

func (a App) handlePulseKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Quit):
		a.confirmQuit()
		return a, nil
	case key.Matches(msg, a.keys.Escape):
		a.popView()
		return a, nil
	case key.Matches(msg, a.keys.Refresh):
		a.log.info("Manual health check refresh from pulse view")
		a.statusBar.setFlash("Refreshing...", flashInfo)
		return a, tea.Batch(
			a.checker.CheckAll(a.list.healthTargets()),
			scheduleFlashClear(),
		)
	}
	return a, nil
}


func (a *App) logAuditEvent(ev audit.AuditEvent) {
	if a.auditLog == nil {
		return
	}
	if err := a.auditLog.Log(ev); err != nil {
		a.log.warn("Audit log write error: %v", err)
	}
	// Mirror audit event into the unified log view.
	level := logInfo
	if ev.EventType == audit.EventError {
		level = logError
	}
	msg := fmt.Sprintf("[%s] %s", ev.EventType, ev.ConnectionName)
	if ev.Details != "" {
		msg += ": " + ev.Details
	}
	a.log.addAudit(level, msg)
}

// trackConnectionUsage updates LastConnectedAt and increments ConnectCount
// for the connection with the given ID. Returns a tea.Cmd that saves the
// config in the background so it doesn't block the connection flow.
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

// saveErrorMsg is returned when a background save fails.
type saveErrorMsg struct{ err error }

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

	// Track connection usage (LastConnectedAt, ConnectCount) — save runs in background
	saveCmd := a.trackConnectionUsage(c.ID)

	// For SSH and Telnet, use managed sessions with detach support
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

// resolveProfileCredentials merges credentials from the vault profile into the
// connection. Call this before launching any protocol — SSH, RDP, VNC, or Telnet.
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

// handleVaultKey processes key events while the vault profile list view is active.
func (a App) handleVaultKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch {
	case k == " ": // Space = leader key
		if !a.leader.active {
			cmd := a.leader.activate()
			return a, cmd
		}
	case key.Matches(msg, a.keys.Quit):
		a.confirmQuit()
		return a, nil
	case key.Matches(msg, a.keys.Help):
		a.help.view = "vault"
		a.help.toggle()
		return a, nil
	}
	// Forward to vault model for navigation and actions (j/k/g/G, enter, p, c, e, d, esc).
	var cmd tea.Cmd
	a.vaultView, cmd = a.vaultView.Update(msg)
	return a, cmd
}

// refreshVaultView reloads profiles from the store and updates vaultView.
func (a *App) refreshVaultView() {
	if a.profileStore == nil {
		return
	}
	profiles, err := a.profileStore.List()
	if err != nil {
		a.statusBar.setFlash("Error loading cred profiles: "+err.Error(), flashError)
		return
	}
	// Count connections per profile (direct assignment or group inheritance).
	counts := make(map[string]int)
	groupProfiles := make(map[string]string)
	for _, p := range profiles {
		if p.Group != "" {
			groupProfiles[p.Group] = p.Name
		}
		for _, conn := range a.cfg.Connections {
			if conn.CredentialProfile == p.Name {
				counts[p.Name]++
			} else if p.Group != "" && conn.Group == p.Group && conn.CredentialProfile == "" {
				counts[p.Name]++
			}
		}
	}
	a.vaultView.setProfiles(profiles)
	a.vaultView.setConnCounts(counts)
	a.list.groupProfiles = groupProfiles
}

// handleVaultRename renames a credential profile and updates all referencing connections.
// args is expected to be "<old-name> <new-name>".
func (a App) handleVaultRename(args string) (tea.Model, tea.Cmd) {
	if a.profileStore == nil {
		a.statusBar.setFlash("Vault not available", flashError)
		return a, scheduleFlashClear()
	}
	parts := strings.SplitN(strings.TrimSpace(args), " ", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		a.statusBar.setFlash("Usage: :vault rename <old-name> <new-name>", flashError)
		return a, scheduleFlashClear()
	}
	oldName := strings.TrimSpace(parts[0])
	newName := strings.TrimSpace(parts[1])

	profile, err := a.profileStore.Get(oldName)
	if err != nil {
		a.statusBar.setFlash("Error looking up cred profile: "+err.Error(), flashError)
		return a, scheduleFlashClear()
	}
	if profile == nil {
		a.statusBar.setFlash("Cred profile '"+oldName+"' not found", flashError)
		return a, scheduleFlashClear()
	}

	profile.Name = newName
	if err := a.profileStore.Update(*profile); err != nil {
		a.statusBar.setFlash("Error renaming cred profile: "+err.Error(), flashError)
		return a, scheduleFlashClear()
	}

	// Update all connections that directly reference the old profile name.
	updated := 0
	for i := range a.cfg.Connections {
		if a.cfg.Connections[i].CredentialProfile == oldName {
			a.cfg.Connections[i].CredentialProfile = newName
			updated++
		}
	}
	if updated > 0 {
		if err := config.Save(a.cfg); err != nil {
			a.log.error("Failed to save connection references after rename: %v", err)
		}
	}

	a.refreshVaultView()
	a.list.rebuildTable()
	a.statusBar.setFlash(fmt.Sprintf("Renamed profile '%s' to '%s' (%d connection(s) updated)", oldName, newName, updated), flashInfo)
	return a, scheduleFlashClear()
}

// handleCredOrphans finds connections that reference a credential profile that no longer exists.
func (a App) handleCredOrphans() (tea.Model, tea.Cmd) {
	if a.profileStore == nil {
		a.statusBar.setFlash("Vault not available", flashError)
		return a, scheduleFlashClear()
	}
	profiles, err := a.profileStore.List()
	if err != nil {
		a.statusBar.setFlash("Error loading cred profiles: "+err.Error(), flashError)
		return a, scheduleFlashClear()
	}

	// Build a set of known profile names.
	known := make(map[string]struct{}, len(profiles))
	for _, p := range profiles {
		known[p.Name] = struct{}{}
	}

	// Find connections with a CredentialProfile that is not in the known set.
	var orphans []string
	for _, conn := range a.cfg.Connections {
		if conn.CredentialProfile == "" {
			continue
		}
		if _, ok := known[conn.CredentialProfile]; !ok {
			orphans = append(orphans, conn.Name+" ("+conn.CredentialProfile+")")
		}
	}

	if len(orphans) == 0 {
		a.statusBar.setFlash("No orphaned references found", flashInfo)
	} else {
		a.statusBar.setFlash(fmt.Sprintf("Orphaned connections (%d): %s", len(orphans), strings.Join(orphans, ", ")), flashWarn)
	}
	return a, scheduleFlashClear()
}

// handleVaultFormSubmit persists a submitted vault profile form.
func (a *App) handleVaultFormSubmit(msg VaultFormSubmitMsg) {
	if a.profileStore == nil {
		a.statusBar.setFlash("Vault not available", flashError)
		return
	}
	if msg.IsEdit {
		if err := a.profileStore.Update(msg.Profile); err != nil {
			a.statusBar.setFlash("Error updating cred profile: "+err.Error(), flashError)
			return
		}
		a.statusBar.setFlash("Cred profile '"+msg.Profile.Name+"' updated", flashInfo)
	} else {
		if _, err := a.profileStore.Create(msg.Profile); err != nil {
			a.statusBar.setFlash("Error creating cred profile: "+err.Error(), flashError)
			return
		}
		a.statusBar.setFlash("Cred profile '"+msg.Profile.Name+"' created", flashInfo)
	}
	a.refreshVaultView()
}

// openVaultView refreshes the vault profile list and pushes the vault view.
func (a *App) openVaultView() tea.Cmd {
	a.refreshVaultView()
	a.pushView(viewVault)
	return nil
}

// openGroupListView computes connection counts per group, updates the group list model, and pushes the view.
func (a *App) openGroupListView() {
	a.refreshGroupListView()
	a.pushView(viewGroupList)
}

// refreshGroupListView recomputes connection counts per group and updates the group list model.
func (a *App) refreshGroupListView() {
	counts := make(map[string]int)
	for _, conn := range a.cfg.Connections {
		if conn.Group != "" {
			counts[conn.Group]++
		}
	}
	a.groupListView.setGroups(a.cfg.Groups)
	a.groupListView.setConnCounts(counts)
}

// handleGroupListKey processes key events while the group list view is active.
func (a App) handleGroupListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch {
	case k == " ": // Space = leader key
		if !a.leader.active {
			cmd := a.leader.activate()
			return a, cmd
		}
	case key.Matches(msg, a.keys.Quit):
		a.confirmQuit()
		return a, nil
	case key.Matches(msg, a.keys.Help):
		a.help.view = "groups"
		a.help.toggle()
		return a, nil
	}
	// Forward to group list model for navigation and actions (j/k/g/G, d, esc).
	var cmd tea.Cmd
	a.groupListView, cmd = a.groupListView.Update(msg)
	return a, cmd
}

// handleFileBrowserKey forwards key events to the file browser model.
func (a App) handleFileBrowserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.fileBrowser, cmd = a.fileBrowser.Update(msg)
	return a, cmd
}

// openFileBrowser opens the SFTP file browser for the given connection.
func (a App) openFileBrowser(c *config.Connection) (tea.Model, tea.Cmd) {
	if c == nil {
		a.statusBar.setFlash("No SSH connection selected", flashError)
		return a, scheduleFlashClear()
	}
	if c.Protocol != config.ProtoSSH {
		a.statusBar.setFlash("SFTP requires an SSH connection", flashError)
		return a, scheduleFlashClear()
	}
	a.resolveProfileCredentials(c)
	return a, func() tea.Msg {
		return fileBrowserOpenMsg{
			connName:     c.Name,
			host:         c.Host,
			port:         c.EffectivePort(),
			username:     c.Username,
			password:     c.Password,
			identityFile: c.IdentityFile,
			proxyJump:    c.ProxyJump,
		}
	}
}

// handleProfileAssign opens the vault view so the user can select a profile to assign.
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

	case UndoOpBatch:
		// Undo batch = undo all children in reverse order.
		for i := len(op.Children) - 1; i >= 0; i-- {
			if err := a.undoSingleOp(op.Children[i]); err != nil {
				a.log.error("Undo batch child failed: %v", err)
			}
		}
		flashMsg = fmt.Sprintf("Undo: %s (%d items)", op.Name, len(op.Children))
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
		a.checker.CheckAll(a.list.healthTargets()),
		scheduleFlashClear(),
	)
}

// undoSingleOp applies undo logic for a single (non-batch) operation.
func (a *App) undoSingleOp(op Operation) error {
	switch op.Type {
	case UndoOpAdd:
		return a.cfg.DeleteConnection(op.After.ID)
	case UndoOpDelete:
		return a.cfg.InsertConnectionAt(op.Before, op.Index)
	case UndoOpEdit, UndoOpTagChange:
		return a.cfg.UpdateConnection(op.Before)
	}
	return nil
}

// redoSingleOp applies redo logic for a single (non-batch) operation.
func (a *App) redoSingleOp(op Operation) error {
	switch op.Type {
	case UndoOpAdd:
		return a.cfg.AddConnection(op.After)
	case UndoOpDelete:
		return a.cfg.DeleteConnection(op.Before.ID)
	case UndoOpEdit, UndoOpTagChange:
		return a.cfg.UpdateConnection(op.After)
	}
	return nil
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

	case UndoOpBatch:
		// Redo batch = redo all children in forward order.
		for _, child := range op.Children {
			if err := a.redoSingleOp(child); err != nil {
				a.log.error("Redo batch child failed: %v", err)
			}
		}
		flashMsg = fmt.Sprintf("Redo: %s (%d items)", op.Name, len(op.Children))
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
		a.checker.CheckAll(a.list.healthTargets()),
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
		// Apply to selection if any, otherwise to current connection.
		if a.selectionSet.HasSelection() {
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
		if a.selectionSet.HasSelection() {
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
	ids := a.selectionSet.SelectedIDs(a.list.table.rows)
	var children []Operation
	for _, id := range ids {
		c := a.cfg.FindConnection(id)
		if c == nil {
			continue
		}
		before := *c
		if a.tagManager.AddTag(c, tag) {
			_ = a.cfg.UpdateConnection(*c)
			children = append(children, Operation{
				Type:   UndoOpTagChange,
				ConnID: c.ID,
				Name:   c.Name,
				Before: before,
				After:  *c,
			})
		}
	}
	if len(children) > 0 {
		a.undoStack.PushBatch("bulk tag add "+tag, children)
	}
	clearCmd := a.clearAllSelection()
	a.list.rebuildTable()
	a.statusBar.setFlash(fmt.Sprintf("Added tag '%s' to %d connection(s)", tag, len(children)), flashInfo)
	return a, tea.Batch(clearCmd, scheduleFlashClear())
}

// tagRemoveVisual removes a tag from all visually selected connections.
func (a App) tagRemoveVisual(tag string) (tea.Model, tea.Cmd) {
	ids := a.selectionSet.SelectedIDs(a.list.table.rows)
	var children []Operation
	for _, id := range ids {
		c := a.cfg.FindConnection(id)
		if c == nil {
			continue
		}
		before := *c
		if a.tagManager.RemoveTag(c, tag) {
			_ = a.cfg.UpdateConnection(*c)
			children = append(children, Operation{
				Type:   UndoOpTagChange,
				ConnID: c.ID,
				Name:   c.Name,
				Before: before,
				After:  *c,
			})
		}
	}
	if len(children) > 0 {
		a.undoStack.PushBatch("bulk tag remove "+tag, children)
	}
	clearCmd := a.clearAllSelection()
	a.list.rebuildTable()
	a.statusBar.setFlash(fmt.Sprintf("Removed tag '%s' from %d connection(s)", tag, len(children)), flashInfo)
	return a, tea.Batch(clearCmd, scheduleFlashClear())
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
	a.statusBar.setFlash("Unknown sort field: "+field, flashError)
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
