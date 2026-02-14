package tui

import (
	"github.com/charmbracelet/lipgloss"
)

type helpModel struct {
	active bool
	view   string // current view name for context-sensitive help
	width  int
	height int
}

func newHelp() helpModel {
	return helpModel{view: "list"}
}

func (h *helpModel) toggle() {
	h.active = !h.active
}

type helpEntry struct {
	key  string
	desc string
}

func (h helpModel) View() string {
	if !h.active {
		return ""
	}

	title := HelpTitleStyle.Render("Keybindings")

	entries := h.entriesForView()
	rows := ""
	for _, e := range entries {
		row := lipgloss.JoinHorizontal(lipgloss.Top,
			HelpKeyStyle.Render(e.key),
			HelpDescStyle.Render(e.desc),
		)
		rows += row + "\n"
	}

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", rows)
	box := HelpOverlayStyle.Render(content)

	return lipgloss.Place(h.width, h.height, lipgloss.Center, lipgloss.Center, box)
}

func (h helpModel) entriesForView() []helpEntry {
	common := []helpEntry{
		{"?", "Toggle help"},
		{"esc", "Back / cancel"},
		{"q", "Quit"},
	}

	switch h.view {
	case "list":
		return append([]helpEntry{
			{"j/k ↑/↓", "Navigate list"},
			{"enter", "Connect to selected"},
			{"Space", "Leader key (command palette)"},
			{"/", "Filter connections"},
			{":", "Command mode"},
			{"a", "Add new connection"},
			{"e", "Edit selected"},
			{"d", "Delete selected"},
			{"D", "Show detail view"},
			{"tab", "Cycle group filter"},
			{"Space x", "Sort menu"},
			{"r", "Refresh health checks"},
			{"y", "Copy command to clipboard"},
			{"s", "Active sessions"},
			{"Ctrl+l", "View event log"},
		}, common...)
	case "detail":
		return append([]helpEntry{
			{"enter", "Connect"},
			{"Space", "Leader key (command palette)"},
			{"e", "Edit connection"},
			{"p", "Toggle password visibility"},
			{"j/k ↑/↓", "Scroll"},
			{"Ctrl+l", "View event log"},
		}, common...)
	case "log":
		return append([]helpEntry{
			{"j/k ↑/↓", "Scroll log"},
		}, common...)
	case "sessions":
		return append([]helpEntry{
			{"j/k ↑/↓", "Navigate sessions"},
			{"enter", "Reattach to session"},
			{"Space", "Leader key (command palette)"},
			{"d", "Kill/disconnect session"},
			{"Ctrl+l", "View event log"},
		}, common...)
	case "form":
		return append([]helpEntry{
			{"tab", "Next field"},
			{"shift+tab", "Previous field"},
			{"enter", "Submit / select"},
		}, common...)
	default:
		return common
	}
}
