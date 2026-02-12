package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all application keybindings.
type KeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Enter   key.Binding
	Filter  key.Binding
	Command key.Binding
	Add     key.Binding
	Edit    key.Binding
	Delete  key.Binding
	Detail  key.Binding
	Group   key.Binding
	Refresh key.Binding
	Help    key.Binding
	Yank         key.Binding
	Sessions     key.Binding
	ShowPassword key.Binding
	Escape       key.Binding
	Quit         key.Binding
}

// DefaultKeyMap returns the default set of keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("j/k", "navigate"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("", ""),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "connect"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		Command: key.NewBinding(
			key.WithKeys(":"),
			key.WithHelp(":", "command"),
		),
		Add: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "add"),
		),
		Edit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		Detail: key.NewBinding(
			key.WithKeys("D"),
			key.WithHelp("D", "detail"),
		),
		Group: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "group"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Yank: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy cmd"),
		),
		Sessions: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "sessions"),
		),
		ShowPassword: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "show password"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "quit"),
		),
	}
}
