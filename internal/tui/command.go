package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CommandMsg is sent when a command is submitted from the command bar.
type CommandMsg struct {
	Name string
	Args string
}

var knownCommands = []string{
	"connect", "group", "all", "add", "edit", "delete", "import-ssh", "sessions", "logs", "help", "q", "quit",
}

type commandModel struct {
	input  textinput.Model
	active bool
	width  int
}

func newCommand() commandModel {
	ti := textinput.New()
	ti.Prompt = ": "
	ti.PromptStyle = CommandPromptStyle
	ti.Placeholder = ""
	ti.CharLimit = 200
	return commandModel{input: ti}
}

func (c *commandModel) activate() {
	c.active = true
	c.input.Focus()
	c.input.SetValue("")
}

func (c *commandModel) deactivate() {
	c.active = false
	c.input.Blur()
	c.input.SetValue("")
}

func (c commandModel) Update(msg tea.Msg) (commandModel, tea.Cmd) {
	if !c.active {
		return c, nil
	}

	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "enter":
			cmd := c.parseCommand()
			c.deactivate()
			return c, func() tea.Msg { return cmd }
		case "esc":
			c.deactivate()
			return c, nil
		case "tab":
			c.autocomplete()
			return c, nil
		}
	}

	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return c, cmd
}

func (c commandModel) View() string {
	if !c.active {
		return ""
	}
	return lipgloss.NewStyle().Width(c.width).Padding(0, 1).Render(c.input.View())
}

func (c commandModel) parseCommand() CommandMsg {
	raw := strings.TrimSpace(c.input.Value())
	parts := strings.SplitN(raw, " ", 2)
	name := parts[0]
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}
	return CommandMsg{Name: name, Args: args}
}

func (c *commandModel) autocomplete() {
	val := c.input.Value()
	if val == "" {
		return
	}
	for _, cmd := range knownCommands {
		if strings.HasPrefix(cmd, val) {
			c.input.SetValue(cmd)
			c.input.SetCursor(len(cmd))
			return
		}
	}
}
