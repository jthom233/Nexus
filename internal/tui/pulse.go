package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/health"
	"github.com/dr4zz/nexus/internal/theme"
)

// PulseSummary holds aggregated counts for the pulse dashboard.
type PulseSummary struct {
	Total          int
	Online         int
	Offline        int
	Degraded       int
	Unknown        int
	ActiveSessions int
}

// GroupHealth holds per-group online/offline counts.
type GroupHealth struct {
	Name    string
	Online  int
	Offline int
	Total   int
}

// ProtoDist holds protocol distribution data.
type ProtoDist struct {
	Protocol string
	Count    int
}

// LatencyEntry holds a connection name and its latency.
type LatencyEntry struct {
	Name    string
	Latency time.Duration
}

// PulseModel is the bubbletea sub-model for the Pulse health dashboard.
type PulseModel struct {
	summary  PulseSummary
	groups   []GroupHealth
	protos   []ProtoDist
	latency  []LatencyEntry
	width    int
	height   int
}

// NewPulseModel creates a new empty PulseModel.
func NewPulseModel() *PulseModel {
	return &PulseModel{}
}

// SetSize sets the available render area.
func (p *PulseModel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// Refresh recalculates all pulse data from the current connections and health state.
func (p *PulseModel) Refresh(connections []config.Connection, statuses map[string]connStatus, activeSessions int) {
	p.summary = PulseSummary{
		Total:          len(connections),
		ActiveSessions: activeSessions,
	}

	// Count by status
	for _, c := range connections {
		st, ok := statuses[c.ID]
		if !ok {
			p.summary.Unknown++
			continue
		}
		switch st.status {
		case health.Online:
			p.summary.Online++
		case health.Offline:
			p.summary.Offline++
		case health.Degraded:
			p.summary.Degraded++
		default:
			p.summary.Unknown++
		}
	}

	// Group health
	groupMap := make(map[string]*GroupHealth)
	for _, c := range connections {
		g := c.Group
		if g == "" {
			g = "(ungrouped)"
		}
		gh, ok := groupMap[g]
		if !ok {
			gh = &GroupHealth{Name: g}
			groupMap[g] = gh
		}
		gh.Total++
		st, ok := statuses[c.ID]
		if ok && (st.status == health.Online || st.status == health.Degraded) {
			gh.Online++
		} else if ok && st.status == health.Offline {
			gh.Offline++
		}
	}
	p.groups = make([]GroupHealth, 0, len(groupMap))
	for _, gh := range groupMap {
		p.groups = append(p.groups, *gh)
	}
	sort.Slice(p.groups, func(i, j int) bool {
		return p.groups[i].Name < p.groups[j].Name
	})

	// Protocol distribution
	protoMap := make(map[string]int)
	for _, c := range connections {
		protoMap[string(c.Protocol)]++
	}
	p.protos = make([]ProtoDist, 0, len(protoMap))
	for proto, count := range protoMap {
		p.protos = append(p.protos, ProtoDist{Protocol: strings.ToUpper(proto), Count: count})
	}
	sort.Slice(p.protos, func(i, j int) bool {
		return p.protos[i].Count > p.protos[j].Count
	})

	// Latency: top 10 slowest connections (that responded)
	var latencies []LatencyEntry
	for _, c := range connections {
		st, ok := statuses[c.ID]
		if ok && st.latency > 0 && (st.status == health.Online || st.status == health.Degraded) {
			latencies = append(latencies, LatencyEntry{Name: c.Name, Latency: st.latency})
		}
	}
	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i].Latency > latencies[j].Latency
	})
	if len(latencies) > 10 {
		latencies = latencies[:10]
	}
	p.latency = latencies
}

// View renders the pulse dashboard.
func (p *PulseModel) View() string {
	t := theme.Current()

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Info).
		Padding(0, 1)

	sectionTitle := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Header).
		Padding(0, 0, 0, 1)

	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Padding(0, 1)

	labelStyle := lipgloss.NewStyle().
		Foreground(t.Subtle)

	valueStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Fg)

	hintStyle := lipgloss.NewStyle().
		Foreground(t.Muted).
		Italic(true).
		Padding(0, 1)

	var sections []string

	// Title
	title := titleStyle.Render("Pulse Dashboard")
	hint := hintStyle.Render("esc to return  r to refresh")
	sections = append(sections, lipgloss.JoinHorizontal(lipgloss.Center, title, hint))
	sections = append(sections, "")

	// Summary cards row
	cards := p.renderSummaryCards(t, cardStyle, labelStyle, valueStyle)
	sections = append(sections, cards)
	sections = append(sections, "")

	// Calculate available width for two-column layout
	colWidth := (p.width - 6) / 2
	if colWidth < 30 {
		colWidth = 30
	}

	// Left column: Group Health + Protocol Distribution
	leftParts := []string{
		sectionTitle.Render("Group Health"),
		p.renderGroupTable(t, colWidth),
		"",
		sectionTitle.Render("Protocol Distribution"),
		p.renderProtoDist(t, colWidth),
	}
	leftCol := lipgloss.JoinVertical(lipgloss.Left, leftParts...)

	// Right column: Latency Top 10
	rightParts := []string{
		sectionTitle.Render("Slowest Connections"),
		p.renderLatencyTable(t, colWidth),
	}
	rightCol := lipgloss.JoinVertical(lipgloss.Left, rightParts...)

	// Join columns
	cols := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(colWidth+2).Render(leftCol),
		lipgloss.NewStyle().Width(colWidth+2).Render(rightCol),
	)
	sections = append(sections, cols)

	return lipgloss.NewStyle().Width(p.width).Render(
		lipgloss.JoinVertical(lipgloss.Left, sections...),
	)
}

func (p *PulseModel) renderSummaryCards(t *theme.Theme, cardStyle, labelStyle, valueStyle lipgloss.Style) string {
	type card struct {
		label string
		value int
		color lipgloss.Color
	}
	cards := []card{
		{"Total", p.summary.Total, t.Fg},
		{"Online", p.summary.Online, t.StatusOnline},
		{"Offline", p.summary.Offline, t.StatusOffline},
		{"Degraded", p.summary.Degraded, t.StatusDegraded},
		{"Unknown", p.summary.Unknown, t.StatusUnknown},
		{"Sessions", p.summary.ActiveSessions, t.Accent},
	}

	var rendered []string
	for _, c := range cards {
		val := valueStyle.Foreground(c.color).Render(fmt.Sprintf("%d", c.value))
		lbl := labelStyle.Render(c.label)
		content := lipgloss.JoinVertical(lipgloss.Center, val, lbl)
		rendered = append(rendered, cardStyle.Width(12).Align(lipgloss.Center).Render(content))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

func (p *PulseModel) renderGroupTable(t *theme.Theme, width int) string {
	if len(p.groups) == 0 {
		return lipgloss.NewStyle().Foreground(t.Subtle).Padding(0, 2).Render("No groups")
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(t.Header)
	rowStyle := lipgloss.NewStyle().Foreground(t.Fg)
	onStyle := lipgloss.NewStyle().Foreground(t.StatusOnline)
	offStyle := lipgloss.NewStyle().Foreground(t.StatusOffline)

	nameW := width - 24
	if nameW < 10 {
		nameW = 10
	}

	var rows []string
	rows = append(rows, fmt.Sprintf("  %s %s %s",
		headerStyle.Width(nameW).Render("GROUP"),
		headerStyle.Width(10).Align(lipgloss.Right).Render("ONLINE"),
		headerStyle.Width(10).Align(lipgloss.Right).Render("OFFLINE"),
	))

	for _, g := range p.groups {
		name := g.Name
		if len(name) > nameW {
			name = name[:nameW-1] + "~"
		}
		rows = append(rows, fmt.Sprintf("  %s %s %s",
			rowStyle.Width(nameW).Render(name),
			onStyle.Width(10).Align(lipgloss.Right).Render(fmt.Sprintf("%d", g.Online)),
			offStyle.Width(10).Align(lipgloss.Right).Render(fmt.Sprintf("%d", g.Offline)),
		))
	}

	return strings.Join(rows, "\n")
}

func (p *PulseModel) renderProtoDist(t *theme.Theme, width int) string {
	if len(p.protos) == 0 {
		return lipgloss.NewStyle().Foreground(t.Subtle).Padding(0, 2).Render("No connections")
	}

	maxCount := 0
	for _, pd := range p.protos {
		if pd.Count > maxCount {
			maxCount = pd.Count
		}
	}

	barMaxW := width - 20
	if barMaxW < 10 {
		barMaxW = 10
	}

	labelStyle := lipgloss.NewStyle().Foreground(t.Fg).Width(8)
	countStyle := lipgloss.NewStyle().Foreground(t.Subtle).Width(5).Align(lipgloss.Right)

	var rows []string
	for _, pd := range p.protos {
		barLen := 0
		if maxCount > 0 {
			barLen = (pd.Count * barMaxW) / maxCount
		}
		if barLen < 1 && pd.Count > 0 {
			barLen = 1
		}

		barColor := t.Accent
		switch pd.Protocol {
		case "SSH":
			barColor = t.ProtoSSH
		case "RDP":
			barColor = t.ProtoRDP
		case "VNC":
			barColor = t.ProtoVNC
		case "TELNET":
			barColor = t.ProtoTelnet
		}

		bar := lipgloss.NewStyle().Foreground(barColor).Render(strings.Repeat("█", barLen))
		row := fmt.Sprintf("  %s %s %s",
			labelStyle.Render(pd.Protocol),
			bar,
			countStyle.Render(fmt.Sprintf("%d", pd.Count)),
		)
		rows = append(rows, row)
	}

	return strings.Join(rows, "\n")
}

func (p *PulseModel) renderLatencyTable(t *theme.Theme, width int) string {
	if len(p.latency) == 0 {
		return lipgloss.NewStyle().Foreground(t.Subtle).Padding(0, 2).Render("No latency data")
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(t.Header)
	rowStyle := lipgloss.NewStyle().Foreground(t.Fg)

	nameW := width - 16
	if nameW < 10 {
		nameW = 10
	}

	var rows []string
	rows = append(rows, fmt.Sprintf("  %s %s",
		headerStyle.Width(nameW).Render("CONNECTION"),
		headerStyle.Width(12).Align(lipgloss.Right).Render("LATENCY"),
	))

	for _, entry := range p.latency {
		name := entry.Name
		if len(name) > nameW {
			name = name[:nameW-1] + "~"
		}

		latStr := formatLatency(entry.Latency)
		latColor := t.StatusOnline
		if entry.Latency > 2*time.Second {
			latColor = t.StatusOffline
		} else if entry.Latency > 500*time.Millisecond {
			latColor = t.StatusDegraded
		}

		rows = append(rows, fmt.Sprintf("  %s %s",
			rowStyle.Width(nameW).Render(name),
			lipgloss.NewStyle().Foreground(latColor).Width(12).Align(lipgloss.Right).Render(latStr),
		))
	}

	return strings.Join(rows, "\n")
}

func formatLatency(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
