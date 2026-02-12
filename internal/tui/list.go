package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/health"
)

// listViewMode represents the current view/sort mode for the connection list.
type listViewMode int

const (
	listViewDefault  listViewMode = iota // default: favorites pinned to top
	listViewFavorites                     // filter: only favorites
	listViewRecent                        // sort: by LastConnectedAt desc
	listViewFrequent                      // sort: by ConnectCount desc
)

type connStatus struct {
	status  health.Status
	latency time.Duration
}

type listModel struct {
	cfg         *config.Config
	table       tableModel
	statuses    map[string]connStatus
	filtered    []config.Connection
	groupFilter string
	viewMode    listViewMode
	width       int
	height      int
	ready       bool
}

func newList(cfg *config.Config) listModel {
	m := listModel{
		cfg:      cfg,
		statuses: make(map[string]connStatus),
		table:    newTableModel(normalColumns()),
	}
	m.filtered = cfg.Connections
	return m
}

// normalColumns returns the standard set of columns.
func normalColumns() []Column {
	return []Column{
		{Title: "", MinWidth: 3, Flex: 0, SortKey: "status", Align: 2},                 // status indicator
		{Title: "\u2605", MinWidth: 1, MaxWidth: 1, Flex: 0, SortKey: "fav", Align: 2}, // favorite indicator
		{Title: "NAME", MinWidth: 10, Flex: 35, SortKey: "name", Align: 0},             // name
		{Title: "HOST", MinWidth: 10, Flex: 30, SortKey: "host", Align: 0},             // host
		{Title: "PROTOCOL", MinWidth: 5, Flex: 0, SortKey: "protocol", Align: 0},       // protocol (fixed)
		{Title: "GROUP", MinWidth: 8, Flex: 20, SortKey: "group", Align: 0},            // group
		{Title: "LATENCY", MinWidth: 8, Flex: 0, SortKey: "latency", Align: 1},         // latency (right-aligned)
	}
}

// wideColumns returns the extended set of columns for wide mode.
func wideColumns() []Column {
	return []Column{
		{Title: "", MinWidth: 3, Flex: 0, SortKey: "status", Align: 2},
		{Title: "\u2605", MinWidth: 1, MaxWidth: 1, Flex: 0, SortKey: "fav", Align: 2},
		{Title: "NAME", MinWidth: 8, Flex: 20, SortKey: "name", Align: 0},
		{Title: "HOST", MinWidth: 8, Flex: 15, SortKey: "host", Align: 0},
		{Title: "PROTOCOL", MinWidth: 5, Flex: 0, SortKey: "protocol", Align: 0},
		{Title: "GROUP", MinWidth: 6, Flex: 10, SortKey: "group", Align: 0},
		{Title: "LATENCY", MinWidth: 8, Flex: 0, SortKey: "latency", Align: 1},
		{Title: "PORT", MinWidth: 5, Flex: 0, SortKey: "port", Align: 1},
		{Title: "USERNAME", MinWidth: 8, Flex: 10, SortKey: "username", Align: 0},
		{Title: "TAGS", MinWidth: 8, Flex: 15, SortKey: "tags", Align: 0},
		{Title: "IDENTITY", MinWidth: 8, Flex: 10, SortKey: "identity", Align: 0},
	}
}

// sortKeyToColumnIndex finds the column index matching the given sort key.
func (l *listModel) sortKeyToColumnIndex(key string) int {
	for i, col := range l.table.columns {
		if col.SortKey == key {
			return i
		}
	}
	return -1
}

func (l *listModel) setSize(width, height int) {
	l.width = width
	l.height = height
	l.table.setSize(width, height)
	l.rebuildTable()
}

func (l *listModel) applyFilter(query string) {
	if query == "" {
		l.applyGroupFilter()
		return
	}

	var result []config.Connection
	base := l.groupFilteredConns()
	for _, c := range base {
		searchable := strings.Join([]string{c.Name, c.Host, string(c.Protocol), c.Group, strings.Join(c.Tags, " ")}, " ")
		if fuzzyMatch(query, searchable) {
			result = append(result, c)
		}
	}
	l.filtered = result
	l.rebuildTable()
}

func (l *listModel) applyGroupFilter() {
	l.filtered = l.groupFilteredConns()
	l.applyViewMode()
	l.rebuildTable()
}

func (l *listModel) groupFilteredConns() []config.Connection {
	if l.groupFilter == "" {
		return l.cfg.Connections
	}
	var result []config.Connection
	for _, c := range l.cfg.Connections {
		if c.Group == l.groupFilter {
			result = append(result, c)
		}
	}
	return result
}

// applyViewMode applies the current view mode filter/sort to l.filtered.
func (l *listModel) applyViewMode() {
	switch l.viewMode {
	case listViewFavorites:
		l.filtered = FilterFavorites(l.filtered)
	case listViewRecent:
		l.filtered = SortByRecent(l.filtered)
	case listViewFrequent:
		l.filtered = SortByFrequent(l.filtered)
	case listViewDefault:
		l.filtered = PinFavorites(l.filtered)
	}
}

// setViewMode sets the view mode and refreshes the list.
func (l *listModel) setViewMode(mode listViewMode) {
	l.viewMode = mode
	l.filtered = l.groupFilteredConns()
	l.applyViewMode()
	l.rebuildTable()
}

func (l *listModel) cycleGroup() {
	groups := l.cfg.GroupNames()
	if len(groups) == 0 {
		return
	}

	if l.groupFilter == "" {
		l.groupFilter = groups[0]
	} else {
		for i, g := range groups {
			if g == l.groupFilter {
				if i+1 < len(groups) {
					l.groupFilter = groups[i+1]
				} else {
					l.groupFilter = "" // cycle back to "all"
				}
				break
			}
		}
	}
	l.applyGroupFilter()
}

func (l *listModel) updateHealthResults(results []health.Result) {
	for _, r := range results {
		l.statuses[r.ID] = connStatus{status: r.Status, latency: r.Latency}
	}
	l.rebuildTable()
}

func (l *listModel) selectedConnection() *config.Connection {
	if len(l.filtered) == 0 {
		return nil
	}
	r := l.table.selectedRow()
	if r == nil {
		return nil
	}
	// Look up the connection by ID.
	for i := range l.filtered {
		if l.filtered[i].ID == r.ID {
			return &l.filtered[i]
		}
	}
	return nil
}

func (l *listModel) healthTargets() map[string]string {
	targets := make(map[string]string)
	for _, c := range l.cfg.Connections {
		targets[c.ID] = c.HostPort()
	}
	return targets
}

func (l *listModel) countsByStatus() (total, online, offline int) {
	total = len(l.cfg.Connections)
	for _, c := range l.cfg.Connections {
		if s, ok := l.statuses[c.ID]; ok {
			switch s.status {
			case health.Online:
				online++
			case health.Offline:
				offline++
			}
		}
	}
	return
}

func (l *listModel) rebuildTable() {
	rows := l.buildRows()
	if l.ready {
		l.table.updateRows(rows)
	} else {
		l.table.setRows(rows)
	}
	l.table.setSize(l.width, l.height)
	l.ready = true
}

func (l *listModel) buildRows() []Row {
	rows := make([]Row, len(l.filtered))
	for i, c := range l.filtered {
		st := l.statuses[c.ID]
		indicator := statusChar(st.status)

		latStr := ""
		if st.status == health.Online || st.status == health.Degraded {
			latStr = fmt.Sprintf("%dms", st.latency.Milliseconds())
		}

		statusStr := st.status.String()

		// Favorite indicator
		favStr := ""
		if c.Favorite {
			favStr = "\u2605" // filled star
		}

		// Add jump host indicator to host display
		hostDisplay := c.HostPort()
		if c.ProxyJump != "" || c.ProxyCommand != "" {
			hostDisplay = "\u21e2 " + hostDisplay
		}

		if l.table.wideMode {
			rows[i] = Row{
				Cells: []string{
					indicator,
					favStr,
					c.Name,
					hostDisplay,
					c.Protocol.Label(),
					c.Group,
					latStr,
					fmt.Sprintf("%d", c.EffectivePort()),
					c.Username,
					strings.Join(c.Tags, ","),
					c.IdentityFile,
				},
				ID:     c.ID,
				Status: statusStr,
			}
		} else {
			rows[i] = Row{
				Cells: []string{
					indicator,
					favStr,
					c.Name,
					hostDisplay,
					c.Protocol.Label(),
					c.Group,
					latStr,
				},
				ID:     c.ID,
				Status: statusStr,
			}
		}
	}
	return rows
}

// statusChar returns the raw status character (without styling — styling is done by the table renderer).
func statusChar(s health.Status) string {
	switch s {
	case health.Online:
		return StatusOnline
	case health.Offline:
		return StatusOffline
	case health.Degraded:
		return StatusDegraded
	default:
		return StatusUnknown
	}
}

// toggleWideMode switches between normal and wide column layouts.
func (l *listModel) toggleWideMode() {
	l.table.ToggleWideMode()
	if l.table.WideMode() {
		l.table.SetColumns(wideColumns())
	} else {
		l.table.SetColumns(normalColumns())
	}
	l.rebuildTable()
}

func (l listModel) Update(msg tea.Msg) (listModel, tea.Cmd) {
	// The table model doesn't need Update for bubbletea messages —
	// navigation is handled through the motion engine in app.go.
	return l, nil
}

func (l listModel) View() string {
	if !l.ready {
		return ""
	}
	return l.table.View()
}
