package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/health"
)

type detailModel struct {
	conn     *config.Connection
	status   health.Status
	latency  string
	viewport viewport.Model
	ready    bool
	width    int
	height   int
}

func newDetail() detailModel {
	return detailModel{}
}

func (d *detailModel) setConnection(conn *config.Connection, status health.Status, latency string) {
	d.conn = conn
	d.status = status
	d.latency = latency
	d.updateContent()
}

func (d *detailModel) setSize(width, height int) {
	d.width = width
	d.height = height
	if !d.ready {
		d.viewport = viewport.New(width, height)
		d.ready = true
	} else {
		d.viewport.Width = width
		d.viewport.Height = height
	}
	d.updateContent()
}

func (d *detailModel) updateContent() {
	if d.conn == nil || !d.ready {
		return
	}

	c := d.conn
	var b strings.Builder

	title := DetailTitleStyle.Render(fmt.Sprintf("  %s", c.Name))
	b.WriteString(title + "\n\n")

	row := func(label, value string) {
		b.WriteString(DetailLabelStyle.Render(label))
		b.WriteString(DetailValueStyle.Render(value))
		b.WriteString("\n")
	}

	row("ID:", c.ID)
	row("Protocol:", c.Protocol.Label())
	row("Host:", c.Host)
	row("Port:", fmt.Sprintf("%d", c.EffectivePort()))

	if c.Username != "" {
		row("Username:", c.Username)
	}
	if c.Password != "" {
		row("Password:", "****")
	}
	if c.Domain != "" {
		row("Domain:", c.Domain)
	}
	if c.IdentityFile != "" {
		row("Identity File:", c.IdentityFile)
	}
	if c.Group != "" {
		row("Group:", c.Group)
	}
	if len(c.Tags) > 0 {
		row("Tags:", strings.Join(c.Tags, ", "))
	}

	b.WriteString("\n")

	// Status
	statusStr := statusIndicator(d.status) + " " + d.status.String()
	if d.latency != "" {
		statusStr += " (" + d.latency + ")"
	}
	row("Status:", statusStr)

	// Protocol-specific options
	if c.Protocol == config.ProtoRDP {
		b.WriteString("\n")
		b.WriteString(DetailTitleStyle.Render("  RDP Options") + "\n\n")
		if c.RDPOptions.Resolution != "" {
			row("Resolution:", c.RDPOptions.Resolution)
		}
		row("Fullscreen:", fmt.Sprintf("%v", c.RDPOptions.Fullscreen))
		row("Dynamic Res:", fmt.Sprintf("%v", c.RDPOptions.DynamicResolution))
	}
	if c.Protocol == config.ProtoVNC && c.VNCPassword != "" {
		b.WriteString("\n")
		b.WriteString(DetailTitleStyle.Render("  VNC Options") + "\n\n")
		row("VNC Password:", "****")
	}

	d.viewport.SetContent(b.String())
}

func (d detailModel) Update(msg tea.Msg) (detailModel, tea.Cmd) {
	var cmd tea.Cmd
	d.viewport, cmd = d.viewport.Update(msg)
	return d, cmd
}

func (d detailModel) View() string {
	if d.conn == nil {
		return lipgloss.NewStyle().Width(d.width).Height(d.height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(ColorSubtle).
			Render("No connection selected")
	}
	return d.viewport.View()
}

func statusIndicator(s health.Status) string {
	switch s {
	case health.Online:
		return StatusOnlineStyle.Render(StatusOnline)
	case health.Offline:
		return StatusOfflineStyle.Render(StatusOffline)
	case health.Degraded:
		return StatusDegradedStyle.Render(StatusDegraded)
	default:
		return StatusUnknownStyle.Render(StatusUnknown)
	}
}
