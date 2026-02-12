package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/health"
)

type connStatus struct {
	status  health.Status
	latency time.Duration
}

type listModel struct {
	cfg          *config.Config
	table        table.Model
	statuses     map[string]connStatus
	filtered     []config.Connection
	groupFilter  string
	width        int
	height       int
	ready        bool
}

func newList(cfg *config.Config) listModel {
	m := listModel{
		cfg:      cfg,
		statuses: make(map[string]connStatus),
	}
	m.filtered = cfg.Connections
	return m
}

func (l *listModel) setSize(width, height int) {
	l.width = width
	l.height = height
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
	row := l.table.Cursor()
	if row < 0 || row >= len(l.filtered) {
		return nil
	}
	return &l.filtered[row]
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
	columns := []table.Column{
		{Title: "", Width: 3},          // status
		{Title: "NAME", Width: 0},      // calculated
		{Title: "PROTO", Width: 5},
		{Title: "HOST", Width: 0},      // calculated
		{Title: "GROUP", Width: 0},     // calculated
		{Title: "LATENCY", Width: 8},
	}

	// Calculate dynamic widths
	nameW := 20
	groupW := 12
	hostW := 18
	fixed := 3 + 5 + 8 + 12 // status + proto + latency + padding
	remaining := l.width - fixed
	if remaining > 0 {
		nameW = remaining * 35 / 100
		hostW = remaining * 35 / 100
		groupW = remaining * 30 / 100
	}
	columns[1].Width = nameW
	columns[3].Width = hostW
	columns[4].Width = groupW

	rows := make([]table.Row, len(l.filtered))
	for i, c := range l.filtered {
		st := l.statuses[c.ID]
		indicator := statusIndicator(st.status)

		latStr := ""
		if st.status == health.Online || st.status == health.Degraded {
			latStr = fmt.Sprintf("%dms", st.latency.Milliseconds())
		}

		rows[i] = table.Row{
			indicator,
			truncate(c.Name, nameW),
			c.Protocol.Label(),
			truncate(c.HostPort(), hostW),
			truncate(c.Group, groupW),
			latStr,
		}
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(l.height),
	)

	s := table.DefaultStyles()
	s.Header = TableHeaderStyle
	s.Selected = TableSelectedStyle
	s.Cell = TableRowStyle
	t.SetStyles(s)

	// Preserve cursor position
	cursor := 0
	if l.ready {
		cursor = l.table.Cursor()
		if cursor >= len(rows) {
			cursor = len(rows) - 1
		}
		if cursor < 0 {
			cursor = 0
		}
	}
	t.SetCursor(cursor)
	l.table = t
	l.ready = true
}

func (l listModel) Update(msg tea.Msg) (listModel, tea.Cmd) {
	var cmd tea.Cmd
	l.table, cmd = l.table.Update(msg)
	return l, cmd
}

func (l listModel) View() string {
	if !l.ready {
		return ""
	}
	return l.table.View()
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	// Rough byte-level truncation for ASCII-heavy content
	if len(s) > max-1 {
		return s[:max-1] + "~"
	}
	return s
}
