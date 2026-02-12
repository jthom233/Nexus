package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

type headerModel struct {
	breadcrumbs  []string
	sessionCount int
	width        int
}

func newHeader() headerModel {
	return headerModel{
		breadcrumbs: []string{"All Connections"},
	}
}

func (h headerModel) View() string {
	logo := LogoStyle.Render("NEXUS")

	crumbs := ""
	for i, b := range h.breadcrumbs {
		if i > 0 {
			crumbs += BreadcrumbStyle.Render(" > ")
		}
		crumbs += BreadcrumbStyle.Render(b)
	}

	left := lipgloss.JoinHorizontal(lipgloss.Center, logo, crumbs)
	hintText := "?:help  L:logs  q:quit"
	if h.sessionCount > 0 {
		hintText = fmt.Sprintf("s:sessions(%d)  ", h.sessionCount) + hintText
	}
	hint := HintStyle.Render(hintText)

	gap := h.width - lipgloss.Width(left) - lipgloss.Width(hint)
	if gap < 0 {
		gap = 0
	}
	padding := lipgloss.NewStyle().Width(gap).Render("")

	row := lipgloss.JoinHorizontal(lipgloss.Center, left, padding, hint)
	return HeaderStyle.Width(h.width).Render(row)
}
