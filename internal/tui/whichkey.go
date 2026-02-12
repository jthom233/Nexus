package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/theme"
)

// View renders the which-key popup overlay.
// Returns an empty string if the leader is not active.
func (l leaderModel) View() string {
	if !l.active {
		return ""
	}

	t := theme.Current()

	// --- Styles ---
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Info).
		Padding(0, 1)

	keyStyle := lipgloss.NewStyle().
		Foreground(t.Accent).
		Bold(true).
		Width(3).
		Align(lipgloss.Right)

	labelStyle := lipgloss.NewStyle().
		Foreground(t.Fg).
		PaddingLeft(1)

	separatorStyle := lipgloss.NewStyle().
		Foreground(t.Subtle)

	hintStyle := lipgloss.NewStyle().
		Foreground(t.Muted).
		Italic(true)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Padding(1, 2)

	// --- Title ---
	title := "Leader"
	if l.level > 0 && l.group != "" {
		groupLabel := l.currentGroupLabel()
		if groupLabel != "" {
			title = "Leader" + separatorStyle.Render(" > ") + titleStyle.Render(groupLabel)
		}
	}
	titleLine := titleStyle.Render(title)

	// --- Items ---
	items := l.currentItems()
	if len(items) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left,
			titleLine,
			"",
			hintStyle.Render("  No items available"),
		)
		box := boxStyle.Render(content)
		return lipgloss.Place(l.width, l.height, lipgloss.Center, lipgloss.Center, box)
	}

	// Determine column layout: aim for 2-3 columns depending on item count
	cols := 2
	if len(items) > 8 {
		cols = 3
	}
	if len(items) <= 4 {
		cols = 1
	}

	// Build cells: each cell is "key  label"
	cells := make([]string, len(items))
	maxCellWidth := 0
	for i, item := range items {
		indicator := " "
		if item.IsGroup {
			indicator = separatorStyle.Render("+")
		}
		cell := lipgloss.JoinHorizontal(lipgloss.Top,
			keyStyle.Render(item.Key),
			indicator,
			labelStyle.Render(item.Label),
		)
		cells[i] = cell
		w := lipgloss.Width(cell)
		if w > maxCellWidth {
			maxCellWidth = w
		}
	}

	// Pad all cells to the same width for alignment
	cellStyle := lipgloss.NewStyle().Width(maxCellWidth + 2)
	for i := range cells {
		cells[i] = cellStyle.Render(cells[i])
	}

	// Arrange into rows
	rows := make([]string, 0)
	for i := 0; i < len(cells); i += cols {
		end := i + cols
		if end > len(cells) {
			end = len(cells)
		}
		row := lipgloss.JoinHorizontal(lipgloss.Top, cells[i:end]...)
		rows = append(rows, row)
	}

	grid := strings.Join(rows, "\n")

	// --- Hint ---
	hint := hintStyle.Render("esc to cancel")

	// --- Assemble ---
	content := lipgloss.JoinVertical(lipgloss.Left,
		titleLine,
		"",
		grid,
		"",
		hint,
	)

	box := boxStyle.Render(content)

	return lipgloss.Place(l.width, l.height, lipgloss.Center, lipgloss.Center, box)
}
