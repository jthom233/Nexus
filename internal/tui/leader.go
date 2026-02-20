package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// LeaderTimeoutMsg fires when the leader key timeout expires.
type LeaderTimeoutMsg struct{}

// LeaderAction represents a concrete action to execute.
type LeaderAction struct {
	Label   string // display label
	Command string // action identifier mapped in executeLeaderAction
}

// LeaderItem represents a single entry in a leader group (either an action or a sub-group).
type LeaderItem struct {
	Key     string       // trigger key
	Label   string       // display label
	Action  string       // action identifier (empty if IsGroup)
	IsGroup bool         // if true, navigates to sub-group
	Items   []LeaderItem // sub-items (only if IsGroup)
}

// LeaderGroup represents a top-level group in the leader key menu.
type LeaderGroup struct {
	Key   string
	Label string
	Items []LeaderItem
}

// leaderModel manages the leader key state machine.
type leaderModel struct {
	active bool   // leader key was pressed, waiting for follow-up
	level  int    // nesting depth (0 = root, 1 = sub-group)
	group  string // current group key (e.g., "c", "f", "s")
	width  int
	height int
}

func newLeader() leaderModel {
	return leaderModel{}
}

// activate is called when Space is pressed. Starts the timeout.
func (l *leaderModel) activate() tea.Cmd {
	l.active = true
	l.level = 0
	l.group = ""
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return LeaderTimeoutMsg{}
	})
}

// handleKey processes a key while leader is active. Returns the action to take
// or nil if the key navigates to a sub-group or is invalid.
func (l *leaderModel) handleKey(key string) *LeaderAction {
	groups := leaderGroups()

	if l.level == 0 {
		// Root level: look for a matching group key
		for _, g := range groups {
			if g.Key == key {
				// Check if this group has a direct action (like "?")
				if len(g.Items) == 0 {
					// Direct action group (e.g., help)
					return &LeaderAction{Label: g.Label, Command: g.Key}
				}
				// Navigate into the sub-group
				l.level = 1
				l.group = g.Key
				return nil
			}
		}
		// Unknown key at root level: dismiss
		l.dismiss()
		return nil
	}

	// Sub-group level: look for a matching item key
	for _, g := range groups {
		if g.Key == l.group {
			for _, item := range g.Items {
				if item.Key == key {
					if item.IsGroup {
						// Nested sub-group (future expansion)
						l.level++
						return nil
					}
					return &LeaderAction{
						Label:   item.Label,
						Command: item.Action,
					}
				}
			}
			// Unknown key in sub-group: dismiss
			l.dismiss()
			return nil
		}
	}

	l.dismiss()
	return nil
}

// dismiss closes the leader popup.
func (l *leaderModel) dismiss() {
	l.active = false
	l.level = 0
	l.group = ""
}

// currentGroupLabel returns the display label for the current group (for breadcrumbs).
func (l *leaderModel) currentGroupLabel() string {
	if l.level == 0 || l.group == "" {
		return ""
	}
	for _, g := range leaderGroups() {
		if g.Key == l.group {
			return g.Label
		}
	}
	return ""
}

// currentItems returns the items to display in the current state.
func (l *leaderModel) currentItems() []LeaderItem {
	groups := leaderGroups()

	if l.level == 0 {
		// Root: show all groups as items
		items := make([]LeaderItem, 0, len(groups))
		for _, g := range groups {
			items = append(items, LeaderItem{
				Key:     g.Key,
				Label:   g.Label,
				IsGroup: len(g.Items) > 0,
			})
		}
		return items
	}

	// Sub-group level
	for _, g := range groups {
		if g.Key == l.group {
			return g.Items
		}
	}
	return nil
}

// leaderGroups defines the full set of leader key groups and actions.
func leaderGroups() []LeaderGroup {
	return []LeaderGroup{
		{
			Key:   "c",
			Label: "Connect",
			Items: []LeaderItem{
				{Key: "c", Label: "Connect selected", Action: "connect-selected"},
				{Key: "q", Label: "Quick connect", Action: "quick-connect"},
				{Key: "a", Label: "Add connection", Action: "add-connection"},
				{Key: "e", Label: "Edit connection", Action: "edit-connection"},
				{Key: "d", Label: "Delete connection", Action: "delete-connection"},
			},
		},
		{
			Key:   "f",
			Label: "Find",
			Items: []LeaderItem{
				{Key: "f", Label: "Fuzzy find", Action: "fuzzy-find"},
				{Key: "s", Label: "Sessions", Action: "find-sessions"},
				{Key: "t", Label: "By tag", Action: "find-by-tag"},
				{Key: "g", Label: "By group", Action: "find-by-group"},
				{Key: "r", Label: "Recent", Action: "find-recent"},
				{Key: "q", Label: "Frequent", Action: "show-frequent"},
				{Key: "b", Label: "Favorites", Action: "show-favorites"},
				{Key: "a", Label: "All", Action: "show-all"},
				{Key: "v", Label: "Toggle favorite", Action: "toggle-favorite"},
			},
		},
		{
			Key:   "s",
			Label: "Sessions",
			Items: []LeaderItem{
				{Key: "l", Label: "List sessions", Action: "sessions"},
				{Key: "k", Label: "Kill session", Action: "kill-session-prompt"},
				{Key: "a", Label: "Kill all", Action: "kill-all-sessions"},
			},
		},
		{
			Key:   "g",
			Label: "Groups",
			Items: []LeaderItem{
				{Key: "c", Label: "Create group", Action: "create-group"},
				{Key: "l", Label: "List groups", Action: "group-list"},
				{Key: "m", Label: "Move to group", Action: "move-to-group"},
				{Key: "f", Label: "Filter by group", Action: "filter-by-group"},
			},
		},
		{
			Key:   "i",
			Label: "Import",
			Items: []LeaderItem{
				{Key: "s", Label: "SSH config", Action: "import-ssh"},
				{Key: "c", Label: "CSV", Action: "import-csv"},
				{Key: "j", Label: "JSON", Action: "import-json"},
			},
		},
		{
			Key:   "e",
			Label: "Export",
			Items: []LeaderItem{
				{Key: "a", Label: "All", Action: "export-all"},
				{Key: "s", Label: "Selection", Action: "export-selection"},
				{Key: "g", Label: "Group", Action: "export-group"},
			},
		},
		{
			Key:   "t",
			Label: "Tags",
			Items: []LeaderItem{
				{Key: "f", Label: "Filter by tag", Action: "filter-by-tag"},
				{Key: "a", Label: "Add tag", Action: "add-tag"},
				{Key: "r", Label: "Remove tag", Action: "remove-tag"},
			},
		},
		{
			Key:   "v",
			Label: "View",
			Items: []LeaderItem{
				{Key: "l", Label: "Table view", Action: "view-table"},
				{Key: "d", Label: "Detail panel", Action: "view-detail"},
				{Key: "w", Label: "Wide mode", Action: "view-wide"},
				{Key: "e", Label: "Logs", Action: "view-log"},
			},
		},
		{
			Key:   "x",
			Label: "Sort",
			Items: []LeaderItem{
				{Key: "n", Label: "By name", Action: "sort-name"},
				{Key: "h", Label: "By host", Action: "sort-host"},
				{Key: "g", Label: "By group", Action: "sort-group"},
				{Key: "p", Label: "By protocol", Action: "sort-protocol"},
				{Key: "s", Label: "By status", Action: "sort-status"},
				{Key: "l", Label: "By latency", Action: "sort-latency"},
				{Key: "f", Label: "By favorite", Action: "sort-fav"},
			},
		},
		{
			Key:   "h",
			Label: "Health",
			Items: []LeaderItem{
				{Key: "a", Label: "Check all", Action: "check-all"},
				{Key: "p", Label: "Pulse dashboard", Action: "pulse-view"},
			},
		},
		{
			Key:   "p",
			Label: "Cred Profiles",
			Items: []LeaderItem{
				{Key: "l", Label: "List cred profiles", Action: "profile-list"},
				{Key: "c", Label: "Create cred profile", Action: "profile-create"},
				{Key: "e", Label: "Edit cred profile", Action: "profile-edit"},
				{Key: "d", Label: "Delete cred profile", Action: "profile-delete"},
				{Key: "a", Label: "Assign to session", Action: "profile-assign"},
				{Key: "s", Label: "Save from session", Action: "profile-save-from"},
				{Key: "r", Label: "Remove from session", Action: "profile-remove"},
				{Key: "w", Label: "Who uses cred profile", Action: "profile-who"},
			},
		},
		{
			Key:   "d",
			Label: "Data",
			Items: []LeaderItem{
				{Key: "n", Label: "Set note", Action: "set-note"},
				{Key: "f", Label: "Manage fields", Action: "manage-fields"},
				{Key: "p", Label: "Copy password", Action: "copy-password"},
			},
		},
		{
			Key:   "o",
			Label: "Options",
			Items: []LeaderItem{
				{Key: "h", Label: "Health monitoring", Action: "toggle-health"},
				{Key: "k", Label: "Keybindings", Action: "keybindings"},
			},
		},
		{
			Key:   "w",
			Label: "Window",
			Items: []LeaderItem{
				{Key: "v", Label: "Split vertical", Action: "split-vertical"},
				{Key: "s", Label: "Split horizontal", Action: "split-horizontal"},
				{Key: "h", Label: "Focus left", Action: "focus-left"},
				{Key: "j", Label: "Focus down", Action: "focus-down"},
				{Key: "k", Label: "Focus up", Action: "focus-up"},
				{Key: "l", Label: "Focus right", Action: "focus-right"},
				{Key: "c", Label: "Close pane", Action: "close-pane"},
				{Key: "b", Label: "Broadcast toggle", Action: "broadcast-toggle"},
				{Key: "=", Label: "Equalize panes", Action: "equalize-panes"},
				{Key: "z", Label: "Zoom pane", Action: "zoom-pane"},
				{Key: ">", Label: "Resize right", Action: "resize-right"},
				{Key: "<", Label: "Resize left", Action: "resize-left"},
				{Key: "+", Label: "Resize down", Action: "resize-down"},
				{Key: "-", Label: "Resize up", Action: "resize-up"},
				// Preset layouts
				{Key: "2", Label: "Layout 2 columns", Action: "preset-2v"},
				{Key: "3", Label: "Layout 3 columns", Action: "preset-3v"},
				{Key: "4", Label: "Layout 2x2 grid", Action: "preset-2x2"},
				{Key: "m", Label: "Layout main+side", Action: "preset-main-side"},
			},
		},
		{
			Key:   "?",
			Label: "Help",
			Items: nil, // Direct action, no sub-items
		},
	}
}
