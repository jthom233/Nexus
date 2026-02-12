package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConfirmResultMsg carries the user's yes/no decision.
type ConfirmResultMsg struct {
	Confirmed bool
	Action    string
	ID        string
}

type confirmModel struct {
	active  bool
	prompt  string
	action  string
	id      string
	focused bool // true = yes, false = no
	width   int
	height  int
}

func newConfirm() confirmModel {
	return confirmModel{}
}

func (c *confirmModel) show(prompt, action, id string) {
	c.active = true
	c.prompt = prompt
	c.action = action
	c.id = id
	c.focused = false // default to "No"
}

func (c *confirmModel) hide() {
	c.active = false
}

func (c confirmModel) Update(msg tea.Msg) (confirmModel, tea.Cmd) {
	if !c.active {
		return c, nil
	}

	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "y", "Y":
			c.hide()
			return c, func() tea.Msg {
				return ConfirmResultMsg{Confirmed: true, Action: c.action, ID: c.id}
			}
		case "n", "N", "esc":
			c.hide()
			return c, func() tea.Msg {
				return ConfirmResultMsg{Confirmed: false, Action: c.action, ID: c.id}
			}
		case "left", "h", "right", "l", "tab":
			c.focused = !c.focused
		case "enter":
			confirmed := c.focused
			c.hide()
			return c, func() tea.Msg {
				return ConfirmResultMsg{Confirmed: confirmed, Action: c.action, ID: c.id}
			}
		}
	}

	return c, nil
}

func (c confirmModel) View() string {
	if !c.active {
		return ""
	}

	prompt := ConfirmPromptStyle.Render(c.prompt)

	yesStyle := lipgloss.NewStyle().Foreground(ColorSubtle).Padding(0, 2)
	noStyle := lipgloss.NewStyle().Foreground(ColorSubtle).Padding(0, 2)

	if c.focused {
		yesStyle = yesStyle.Foreground(ColorGreen).Bold(true).Underline(true)
	} else {
		noStyle = noStyle.Foreground(ColorRed).Bold(true).Underline(true)
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center,
		yesStyle.Render("[Y]es"),
		noStyle.Render("[N]o"),
	)

	hint := ConfirmHintStyle.Render("y/n or enter to confirm")
	content := lipgloss.JoinVertical(lipgloss.Center, prompt, "", buttons, "", hint)

	box := ConfirmBoxStyle.Render(content)

	// Center on screen
	return lipgloss.Place(c.width, c.height, lipgloss.Center, lipgloss.Center, box)
}
