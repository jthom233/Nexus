package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/theme"
)

type headerModel struct {
	viewName   string
	itemCount  int
	filterText string
	activeMenu int // 1=connections, 2=sessions, 3=logs
	configPath string
	width      int
}

func newHeader() headerModel {
	return headerModel{
		viewName:   "Connections",
		activeMenu: 1,
		configPath: "~/.config/nexus/config.yaml",
	}
}

// setView updates the header for the current view.
func (h *headerModel) setView(name string, menu int) {
	h.viewName = name
	h.activeMenu = menu
}

// setItemCount updates the item count shown in the title bar.
func (h *headerModel) setItemCount(n int) {
	h.itemCount = n
}

// setFilter updates the active filter text shown in the title bar.
func (h *headerModel) setFilter(text string) {
	h.filterText = text
}

// setConfigPath sets the config file path displayed in the header.
func (h *headerModel) setConfigPath(path string) {
	h.configPath = path
}

func (h headerModel) View() string {
	t := theme.Current()

	// --- Line 1: Logo (left) + Menu items (right) ---
	logoStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Info)
	logo := logoStyle.Render("NEXUS")

	menu := h.renderMenu(t)

	gap1 := h.width - lipgloss.Width(logo) - lipgloss.Width(menu)
	if gap1 < 1 {
		gap1 = 1
	}
	pad1 := strings.Repeat(" ", gap1)
	line1 := logo + pad1 + menu

	// --- Line 2: Config path (right-aligned) ---
	configStyle := lipgloss.NewStyle().
		Foreground(t.Subtle)
	configText := configStyle.Render(h.configPath)

	gap2 := h.width - lipgloss.Width(configText)
	if gap2 < 0 {
		gap2 = 0
	}
	pad2 := strings.Repeat(" ", gap2)
	line2 := pad2 + configText

	// --- Line 3: Title bar ---
	line3 := h.renderTitleBar(t)

	return lipgloss.JoinVertical(lipgloss.Left, line1, line2, line3)
}

// renderMenu renders the menu items for line 1.
func (h headerModel) renderMenu(t *theme.Theme) string {
	type menuItem struct {
		key   string
		label string
		id    int
	}
	items := []menuItem{
		{"1", "Connections", 1},
		{"2", "Sessions", 2},
		{"3", "Logs", 3},
	}

	accentStyle := lipgloss.NewStyle().Foreground(t.Accent)
	labelStyle := lipgloss.NewStyle().Foreground(t.Fg)
	activeLabelStyle := lipgloss.NewStyle().Foreground(t.Fg).Bold(true)
	helpAccentStyle := lipgloss.NewStyle().Foreground(t.Accent)
	helpLabelStyle := lipgloss.NewStyle().Foreground(t.Fg)

	var parts []string
	for _, item := range items {
		key := accentStyle.Render("<" + item.key + ">")
		var label string
		if item.id == h.activeMenu {
			label = activeLabelStyle.Render(item.label)
		} else {
			label = labelStyle.Render(item.label)
		}
		parts = append(parts, key+label)
	}

	helpKey := helpAccentStyle.Render("?")
	helpLabel := helpLabelStyle.Render("Help")
	parts = append(parts, helpKey+helpLabel)

	return strings.Join(parts, "  ")
}

// renderTitleBar renders line 3 — the title bar with view name, count, and filter.
func (h headerModel) renderTitleBar(t *theme.Theme) string {
	titleBarBg := lipgloss.NewStyle().
		Background(t.Highlight).
		Width(h.width)

	// Left side: view name + item count
	nameStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Header).
		Background(t.Highlight)
	countStyle := lipgloss.NewStyle().
		Foreground(t.Fg).
		Background(t.Highlight)

	left := nameStyle.Render(h.viewName)
	if h.itemCount > 0 {
		left += countStyle.Render(fmt.Sprintf(" [%d]", h.itemCount))
	}

	// Right side: filter indicator
	right := ""
	if h.filterText != "" {
		filterStyle := lipgloss.NewStyle().
			Foreground(t.Accent).
			Background(t.Highlight)
		right = filterStyle.Render("Filter: /" + h.filterText)
	}

	gap := h.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 0 {
		gap = 0
	}
	padding := strings.Repeat(" ", gap)

	content := " " + left + padding + right + " "
	return titleBarBg.Render(content)
}
