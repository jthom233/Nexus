package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/theme"
	"github.com/dr4zz/nexus/internal/vault"
)

// --- Message types ---

// VaultCreateMsg is sent when the user requests creating a new credential profile.
type VaultCreateMsg struct{}

// VaultEditMsg is sent when the user requests editing the given profile.
type VaultEditMsg struct{ Profile vault.CredentialProfile }

// VaultDeleteMsg is sent when the user requests deleting the given profile.
type VaultDeleteMsg struct{ Profile vault.CredentialProfile }

// VaultCloseMsg is sent when the user dismisses the vault list view.
type VaultCloseMsg struct{}

// VaultRefreshMsg is sent to supply an updated list of profiles to the vault view.
type VaultRefreshMsg struct{ Profiles []vault.CredentialProfile }

// --- Model ---

type vaultModel struct {
	profiles     []vault.CredentialProfile
	connCounts   map[string]int // profile name → connection count using it
	cursor       int
	width        int
	height       int
	showDetail   bool
	showPassword bool
}

func newVaultModel() vaultModel {
	return vaultModel{
		connCounts: make(map[string]int),
	}
}

func (m *vaultModel) setProfiles(profiles []vault.CredentialProfile) {
	m.profiles = profiles
	// Clamp cursor
	if m.cursor >= len(m.profiles) {
		m.cursor = len(m.profiles) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *vaultModel) setConnCounts(counts map[string]int) {
	if counts == nil {
		m.connCounts = make(map[string]int)
	} else {
		m.connCounts = counts
	}
}

// selectedProfile returns a pointer to the profile at the current cursor, or nil
// if there are no profiles.
func (m vaultModel) selectedProfile() *vault.CredentialProfile {
	if len(m.profiles) == 0 || m.cursor < 0 || m.cursor >= len(m.profiles) {
		return nil
	}
	return &m.profiles[m.cursor]
}

// --- Update ---

func (m vaultModel) Update(msg tea.Msg) (vaultModel, tea.Cmd) {
	switch msg := msg.(type) {
	case VaultRefreshMsg:
		m.setProfiles(msg.Profiles)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		// Navigation
		case "j", "down":
			if m.cursor < len(m.profiles)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "g":
			m.cursor = 0
		case "G":
			if len(m.profiles) > 0 {
				m.cursor = len(m.profiles) - 1
			}

		// Toggle detail / escape
		case "enter":
			if !m.showDetail && len(m.profiles) > 0 {
				m.showDetail = true
			}
		case "esc":
			if m.showDetail {
				m.showDetail = false
				return m, nil
			}
			return m, func() tea.Msg { return VaultCloseMsg{} }

		// Password toggle (only meaningful in detail view)
		case "p":
			m.showPassword = !m.showPassword

		// Actions (list view only)
		case "c":
			if !m.showDetail {
				return m, func() tea.Msg { return VaultCreateMsg{} }
			}
		case "e":
			if !m.showDetail {
				if p := m.selectedProfile(); p != nil {
					prof := *p
					return m, func() tea.Msg { return VaultEditMsg{Profile: prof} }
				}
			}
		case "d":
			if !m.showDetail {
				if p := m.selectedProfile(); p != nil {
					prof := *p
					return m, func() tea.Msg { return VaultDeleteMsg{Profile: prof} }
				}
			}
		}
	}

	return m, nil
}

// --- View ---

func (m vaultModel) View() string {
	if m.showDetail {
		return m.renderDetail()
	}
	return m.renderList()
}

// --- List rendering ---

func (m vaultModel) renderList() string {
	th := theme.Current()

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(th.Header)
	countStyle := lipgloss.NewStyle().Foreground(th.Muted)
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(th.Header)
	cursorStyle := lipgloss.NewStyle().Foreground(th.Cursor).Background(th.Selection).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(th.Muted)
	hintStyle := lipgloss.NewStyle().Foreground(th.Subtle)

	var sb strings.Builder

	// Title line
	count := fmt.Sprintf("%d cred profiles", len(m.profiles))
	titleLine := titleStyle.Render("  Credential Vault")
	countRendered := countStyle.Render(count)
	gap := m.width - lipgloss.Width(titleLine) - lipgloss.Width(countRendered) - 2
	if gap < 1 {
		gap = 1
	}
	sb.WriteString(titleLine + strings.Repeat(" ", gap) + countRendered)
	sb.WriteString("\n\n")

	if len(m.profiles) == 0 {
		empty := lipgloss.NewStyle().
			Foreground(th.Muted).
			Width(m.width).
			Align(lipgloss.Center).
			Padding(2, 0).
			Render("No credential profiles. Press c to create one.")
		sb.WriteString(empty)
		sb.WriteString("\n")
	} else {
		// Column widths
		nameW, userW, groupW, usedW, updW := m.columnWidths()

		// Header
		hdr := fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s  %-*s",
			nameW, "NAME",
			userW, "USERNAME",
			groupW, "GROUP",
			usedW, "USED BY",
			updW, "UPDATED",
		)
		sb.WriteString(headerStyle.Render(hdr))
		sb.WriteString("\n")

		sep := lipgloss.NewStyle().Foreground(th.Subtle).Render(strings.Repeat("─", m.width))
		sb.WriteString(sep)
		sb.WriteString("\n")

		// Rows
		contentHeight := m.height - 6 // title(1) + blank(1) + header(1) + sep(1) + hints(2)
		if contentHeight < 1 {
			contentHeight = 1
		}

		// Viewport scrolling
		offset := 0
		if m.cursor >= contentHeight {
			offset = m.cursor - contentHeight + 1
		}
		end := offset + contentHeight
		if end > len(m.profiles) {
			end = len(m.profiles)
		}

		for i := offset; i < end; i++ {
			p := m.profiles[i]
			isCursor := i == m.cursor

			group := p.Group
			if group == "" {
				group = "-"
			}
			used := m.connCounts[p.Name]
			usedStr := fmt.Sprintf("%d conns", used)
			updStr := relativeTime(p.UpdatedAt)

			nameCell := truncateToWidth(p.Name, nameW)
			userCell := truncateToWidth(p.Username, userW)
			groupCell := truncateToWidth(group, groupW)

			prefix := "  "
			if isCursor {
				prefix = "> "
			}

			line := fmt.Sprintf("%s%-*s  %-*s  %-*s  %-*s  %-*s",
				prefix,
				nameW, nameCell,
				userW, userCell,
				groupW, groupCell,
				usedW, usedStr,
				updW, updStr,
			)

			// Pad to full width
			lineW := lipgloss.Width(line)
			if lineW < m.width {
				line += strings.Repeat(" ", m.width-lineW)
			}

			if isCursor {
				sb.WriteString(cursorStyle.Render(line))
			} else {
				// Alternate row shading
				if (i-offset)%2 == 1 {
					sb.WriteString(lipgloss.NewStyle().Background(th.Highlight).Render(line))
				} else {
					sb.WriteString(line)
				}
			}
			sb.WriteString("\n")
		}
	}

	// Fill remaining space
	rendered := strings.Count(sb.String(), "\n")
	for rendered < m.height-2 {
		sb.WriteString("\n")
		rendered++
	}

	// Hint bar
	hints := "c:create  e:edit  d:delete  Enter:detail  Esc:back"
	hintLine := "  " + hintStyle.Render(hints)
	// Pad hint line to full width so it looks anchored
	hintLineW := lipgloss.Width(hintLine)
	if hintLineW < m.width {
		hintLine += mutedStyle.Render(strings.Repeat(" ", m.width-hintLineW))
	}
	sb.WriteString(hintLine)

	return sb.String()
}

// columnWidths computes fixed column widths for the list view based on available width.
func (m vaultModel) columnWidths() (nameW, userW, groupW, usedW, updW int) {
	// Fixed columns
	usedW = 8  // "12 conns"
	updW = 7   // "3mo ago"
	groupW = 12

	// Overhead: "  " prefix (2) + separators between 5 cols (4×2=8) = 10
	overhead := 10
	remaining := m.width - usedW - updW - groupW - overhead
	if remaining < 20 {
		remaining = 20
	}
	nameW = remaining * 55 / 100
	userW = remaining - nameW
	if nameW < 8 {
		nameW = 8
	}
	if userW < 8 {
		userW = 8
	}
	return
}

// --- Detail rendering ---

func (m vaultModel) renderDetail() string {
	p := m.selectedProfile()
	if p == nil {
		return lipgloss.NewStyle().
			Width(m.width).
			Height(m.height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(ColorSubtle).
			Render("No profile selected")
	}

	th := theme.Current()
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(th.Header)
	descStyle := lipgloss.NewStyle().Foreground(th.Muted)
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(th.Info)
	hintStyle := lipgloss.NewStyle().Foreground(th.Subtle)

	field := func(label, value string) string {
		if value == "" {
			value = "-"
		}
		return DetailLabelStyle.Render(label) + DetailValueStyle.Render(value) + "\n"
	}

	masked := func(label, value string) string {
		if value == "" {
			return field(label, "-")
		}
		if m.showPassword {
			return field(label, value)
		}
		return field(label, "****")
	}

	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(titleStyle.Render("  "+p.Name) + "\n")
	if p.Description != "" {
		sb.WriteString(descStyle.Render("  "+p.Description) + "\n")
	}
	sb.WriteString("\n")

	sb.WriteString(sectionStyle.Render("  Credentials") + "\n\n")
	sb.WriteString("  " + field("Username:     ", p.Username))
	sb.WriteString("  " + masked("Password:     ", p.Password))
	sb.WriteString("  " + field("Identity:     ", p.IdentityFile))
	sb.WriteString("  " + masked("Passphrase:   ", p.Passphrase))
	sb.WriteString("  " + field("Domain:       ", p.Domain))
	sb.WriteString("  " + masked("VNC Password: ", p.VNCPassword))
	sb.WriteString("\n")

	sb.WriteString("  " + field("Group:        ", p.Group))
	if len(p.Tags) > 0 {
		sb.WriteString("  " + field("Tags:         ", strings.Join(p.Tags, ", ")))
	} else {
		sb.WriteString("  " + field("Tags:         ", "-"))
	}
	sb.WriteString("  " + field("Created:      ", p.CreatedAt.UTC().Format("2006-01-02 15:04 UTC")))
	sb.WriteString("  " + field("Updated:      ", p.UpdatedAt.UTC().Format("2006-01-02 15:04 UTC")))
	sb.WriteString("\n")

	pwHint := "p:show password"
	if m.showPassword {
		pwHint = "p:hide password"
	}
	hints := pwHint + "  Esc:back"
	sb.WriteString("  " + hintStyle.Render(hints))

	return sb.String()
}

// --- Helpers ---

// relativeTime returns a human-readable relative time string such as "2d ago",
// "1w ago", or "3mo ago".
func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}

	minutes := int(d.Minutes())
	hours := int(d.Hours())
	days := hours / 24
	weeks := days / 7
	months := days / 30
	years := days / 365

	switch {
	case years >= 1:
		return fmt.Sprintf("%dyr ago", years)
	case months >= 1:
		return fmt.Sprintf("%dmo ago", months)
	case weeks >= 1:
		return fmt.Sprintf("%dw ago", weeks)
	case days >= 1:
		return fmt.Sprintf("%dd ago", days)
	case hours >= 1:
		return fmt.Sprintf("%dh ago", hours)
	case minutes >= 1:
		return fmt.Sprintf("%dm ago", minutes)
	default:
		return "just now"
	}
}

