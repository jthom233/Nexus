package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/theme"
)

// --- Message types ---

// GroupListDeleteMsg is sent when the user requests deleting the selected group.
type GroupListDeleteMsg struct{ GroupName string }

// GroupListCloseMsg is sent when the user dismisses the group list view.
type GroupListCloseMsg struct{}

// --- Model ---

type groupListModel struct {
	groups     []config.Group
	connCounts map[string]int // group name → connection count
	cursor     int
	width      int
	height     int
}

func newGroupListModel() groupListModel {
	return groupListModel{
		connCounts: make(map[string]int),
	}
}

func (m *groupListModel) setGroups(groups []config.Group) {
	m.groups = groups
	// Clamp cursor
	if m.cursor >= len(m.groups) {
		m.cursor = len(m.groups) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *groupListModel) setConnCounts(counts map[string]int) {
	if counts == nil {
		m.connCounts = make(map[string]int)
	} else {
		m.connCounts = counts
	}
}

// selectedGroup returns a pointer to the group at the current cursor, or nil
// if there are no groups.
func (m groupListModel) selectedGroup() *config.Group {
	if len(m.groups) == 0 || m.cursor < 0 || m.cursor >= len(m.groups) {
		return nil
	}
	return &m.groups[m.cursor]
}

// --- Update ---

func (m groupListModel) Update(msg tea.Msg) (groupListModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		// Navigation
		case "j", "down":
			if m.cursor < len(m.groups)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "g":
			m.cursor = 0
		case "G":
			if len(m.groups) > 0 {
				m.cursor = len(m.groups) - 1
			}

		// Actions
		case "d":
			if g := m.selectedGroup(); g != nil {
				name := g.Name
				return m, func() tea.Msg { return GroupListDeleteMsg{GroupName: name} }
			}
		case "esc":
			return m, func() tea.Msg { return GroupListCloseMsg{} }
		}
	}

	return m, nil
}

// --- View ---

func (m groupListModel) View() string {
	th := theme.Current()

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(th.Header)
	countStyle := lipgloss.NewStyle().Foreground(th.Muted)
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(th.Header)
	cursorStyle := lipgloss.NewStyle().Foreground(th.Cursor).Background(th.Selection).Bold(true)
	hintStyle := lipgloss.NewStyle().Foreground(th.Subtle)
	mutedStyle := lipgloss.NewStyle().Foreground(th.Muted)

	var sb strings.Builder

	// Title line
	count := fmt.Sprintf("%d groups", len(m.groups))
	titleLine := titleStyle.Render("  Groups")
	countRendered := countStyle.Render(count)
	gap := m.width - lipgloss.Width(titleLine) - lipgloss.Width(countRendered) - 2
	if gap < 1 {
		gap = 1
	}
	sb.WriteString(titleLine + strings.Repeat(" ", gap) + countRendered)
	sb.WriteString("\n\n")

	if len(m.groups) == 0 {
		empty := lipgloss.NewStyle().
			Foreground(th.Muted).
			Width(m.width).
			Align(lipgloss.Center).
			Padding(2, 0).
			Render("No groups defined.")
		sb.WriteString(empty)
		sb.WriteString("\n")
	} else {
		// Column widths
		nameW, connW := m.columnWidths()

		// Header
		hdr := fmt.Sprintf("  %-*s  %-*s",
			nameW, "NAME",
			connW, "CONNECTIONS",
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
		if end > len(m.groups) {
			end = len(m.groups)
		}

		for i := offset; i < end; i++ {
			g := m.groups[i]
			isCursor := i == m.cursor

			connCount := m.connCounts[g.Name]
			connStr := fmt.Sprintf("%d", connCount)

			nameCell := truncateToWidth(g.Name, nameW)

			prefix := "  "
			if isCursor {
				prefix = "> "
			}

			line := fmt.Sprintf("%s%-*s  %-*s",
				prefix,
				nameW, nameCell,
				connW, connStr,
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
	hints := "d:delete  Esc:back"
	hintLine := "  " + hintStyle.Render(hints)
	// Pad hint line to full width so it looks anchored
	hintLineW := lipgloss.Width(hintLine)
	if hintLineW < m.width {
		hintLine += mutedStyle.Render(strings.Repeat(" ", m.width-hintLineW))
	}
	sb.WriteString(hintLine)

	return sb.String()
}

// columnWidths computes fixed column widths for the group list view based on available width.
func (m groupListModel) columnWidths() (nameW, connW int) {
	// Fixed columns
	connW = 12 // "CONNECTIONS"

	// Overhead: "  " prefix (2) + separator between 2 cols (1×2=2) = 4
	overhead := 4
	remaining := m.width - connW - overhead
	if remaining < 8 {
		remaining = 8
	}
	nameW = remaining
	return
}
