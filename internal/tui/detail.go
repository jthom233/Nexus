package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/health"
)

type detailModel struct {
	conn         *config.Connection
	credSources  map[string]string
	status       health.Status
	latency      string
	showPassword bool
	viewport     viewport.Model
	ready        bool
	width        int
	height       int
}

func newDetail() detailModel {
	return detailModel{}
}

func (d *detailModel) setConnection(conn *config.Connection, status health.Status, latency string) {
	d.conn = conn
	d.credSources = nil
	d.status = status
	d.latency = latency
	d.showPassword = false
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

	// row renders a label/value pair with an optional provenance annotation.
	row := func(label, value string) {
		b.WriteString(DetailLabelStyle.Render(label))
		b.WriteString(DetailValueStyle.Render(value))
		b.WriteString("\n")
	}

	// rowWithSource renders a label/value pair followed by a dim provenance label.
	rowWithSource := func(label, value, sourceKey string) {
		b.WriteString(DetailLabelStyle.Render(label))
		b.WriteString(DetailValueStyle.Render(value))
		if src, ok := d.credSources[sourceKey]; ok {
			b.WriteString("  " + DetailProvenanceStyle.Render(formatSource(src)))
		}
		b.WriteString("\n")
	}

	row("ID:", c.ID)
	row("Protocol:", c.Protocol.Label())
	row("Host:", c.Host)
	row("Port:", fmt.Sprintf("%d", c.EffectivePort()))

	// Credential profile assignment with dangling-reference warning.
	if c.CredentialProfile != "" {
		profileLabel := c.CredentialProfile
		// If credSources is non-nil (resolved) but none of the credential keys
		// came from this profile, the reference is dangling.
		if d.credSources != nil {
			hasSrc := false
			for _, v := range d.credSources {
				if v == "profile:"+c.CredentialProfile {
					hasSrc = true
					break
				}
			}
			if !hasSrc {
				profileLabel += "  " + DetailWarnStyle.Render("[NOT FOUND]")
			}
		}
		row("Cred Profile:", profileLabel)
	} else if d.credSources != nil {
		// Show group-inherited cred profile when no explicit profile is set.
		for _, v := range d.credSources {
			if strings.HasPrefix(v, "group:") {
				profileName := strings.TrimPrefix(v, "group:")
				row("Cred Profile:", profileName+"  "+DetailProvenanceStyle.Render("inherited from group"))
				break
			}
		}
	}

	if c.Username != "" {
		rowWithSource("Username:", c.Username, "username")
	}
	if c.Password != "" {
		if d.showPassword {
			rowWithSource("Password:", c.Password, "password")
		} else {
			rowWithSource("Password:", "****", "password")
		}
	}
	if c.Domain != "" {
		rowWithSource("Domain:", c.Domain, "domain")
	}
	if c.IdentityFile != "" {
		rowWithSource("Identity File:", c.IdentityFile, "identity_file")
	}
	if c.Group != "" {
		row("Group:", c.Group)
	}
	if len(c.Tags) > 0 {
		row("Tags:", strings.Join(c.Tags, ", "))
	}

	// Port forwards
	if len(c.PortForwards) > 0 {
		b.WriteString("\n")
		b.WriteString(DetailTitleStyle.Render("  Port Forwards") + "\n\n")
		for _, pf := range c.PortForwards {
			row("Forward:", pf.String())
		}
	}

	// Proxy settings
	if c.ProxyJump != "" {
		row("Proxy Jump:", c.ProxyJump)
	}
	if c.ProxyCommand != "" {
		row("Proxy Cmd:", c.ProxyCommand)
	}

	// Notes
	if c.Notes != "" {
		b.WriteString("\n")
		b.WriteString(DetailTitleStyle.Render("  Notes") + "\n\n")
		b.WriteString(DetailValueStyle.Render("  "+c.Notes) + "\n")
	}

	// Custom Fields
	if len(c.CustomFields) > 0 {
		b.WriteString("\n")
		b.WriteString(DetailTitleStyle.Render("  Custom Fields") + "\n\n")
		// Sort keys for deterministic display
		cfKeys := make([]string, 0, len(c.CustomFields))
		for k := range c.CustomFields {
			cfKeys = append(cfKeys, k)
		}
		sort.Strings(cfKeys)
		for _, k := range cfKeys {
			row(k+":", c.CustomFields[k])
		}
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
		if d.showPassword {
			rowWithSource("VNC Password:", c.VNCPassword, "vnc_password")
		} else {
			rowWithSource("VNC Password:", "****", "vnc_password")
		}
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

// formatSource converts a raw source string from ResolveCredentials into a
// human-readable provenance label.
//
//	"direct"      → "direct"
//	"profile:X"   → "from profile"
//	"group:X"     → "from group: X"
func formatSource(src string) string {
	if src == "direct" {
		return "direct"
	}
	if strings.HasPrefix(src, "profile:") {
		return "from cred profile"
	}
	if strings.HasPrefix(src, "group:") {
		name := strings.TrimPrefix(src, "group:")
		return "from group: " + name
	}
	return src
}
