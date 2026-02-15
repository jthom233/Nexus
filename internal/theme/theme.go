package theme

import (
	"sort"

	"github.com/charmbracelet/lipgloss"
)

// Theme defines a complete color scheme for Nexus.
type Theme struct {
	Name string

	// Core semantic colors
	Bg        lipgloss.Color
	Fg        lipgloss.Color
	Accent    lipgloss.Color
	Error     lipgloss.Color
	Warning   lipgloss.Color
	Success   lipgloss.Color
	Info      lipgloss.Color
	Subtle    lipgloss.Color
	Highlight lipgloss.Color
	Cursor    lipgloss.Color
	Selection lipgloss.Color
	Border    lipgloss.Color
	Header    lipgloss.Color
	Muted     lipgloss.Color

	// Mode colors (for statusbar mode indicator)
	ModeNormal  lipgloss.Color
	ModeInsert  lipgloss.Color
	ModeVisual  lipgloss.Color
	ModeCommand lipgloss.Color

	// Protocol colors
	ProtoSSH    lipgloss.Color
	ProtoRDP    lipgloss.Color
	ProtoVNC    lipgloss.Color
	ProtoTelnet lipgloss.Color

	// Status colors
	StatusOnline   lipgloss.Color
	StatusOffline  lipgloss.Color
	StatusDegraded lipgloss.Color
	StatusUnknown  lipgloss.Color
}

var (
	current      *Theme
	themes       map[string]*Theme
	colorProfile string = "truecolor" // default
)

// SetProfile configures the color rendering profile.
// Values: "truecolor", "256", "16", "mono"
func SetProfile(profile string) {
	colorProfile = profile
}

// Profile returns the current color profile.
func Profile() string {
	return colorProfile
}

func init() {
	themes = map[string]*Theme{
		"tokyonight-storm": tokyonightStorm(),
		"catppuccin-mocha": catppuccinMocha(),
		"dracula":          dracula(),
		"gruvbox-dark":     gruvboxDark(),
		"nord":             nord(),
	}
	current = themes["tokyonight-storm"]
}

// Current returns the active theme.
func Current() *Theme {
	return current
}

// Set switches the active theme by name. Returns true if the theme was found.
func Set(name string) bool {
	t, ok := themes[name]
	if !ok {
		return false
	}
	current = t
	return true
}

// Names returns all registered theme names in sorted order.
func Names() []string {
	names := make([]string, 0, len(themes))
	for name := range themes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func tokyonightStorm() *Theme {
	return &Theme{
		Name:      "tokyonight-storm",
		Bg:        lipgloss.Color("#24283b"),
		Fg:        lipgloss.Color("#c0caf5"),
		Accent:    lipgloss.Color("#7aa2f7"),
		Error:     lipgloss.Color("#f7768e"),
		Warning:   lipgloss.Color("#e0af68"),
		Success:   lipgloss.Color("#9ece6a"),
		Info:      lipgloss.Color("#7dcfff"),
		Subtle:    lipgloss.Color("#565f89"),
		Highlight: lipgloss.Color("#292e42"),
		Cursor:    lipgloss.Color("#c0caf5"),
		Selection: lipgloss.Color("#364A82"),
		Border:    lipgloss.Color("#3b4261"),
		Header:    lipgloss.Color("#bb9af7"),
		Muted:     lipgloss.Color("#545c7e"),

		ModeNormal:  lipgloss.Color("#7aa2f7"),
		ModeInsert:  lipgloss.Color("#9ece6a"),
		ModeVisual:  lipgloss.Color("#bb9af7"),
		ModeCommand: lipgloss.Color("#e0af68"),

		ProtoSSH:    lipgloss.Color("#9ece6a"),
		ProtoRDP:    lipgloss.Color("#7aa2f7"),
		ProtoVNC:    lipgloss.Color("#ff9e64"),
		ProtoTelnet: lipgloss.Color("#e0af68"),

		StatusOnline:   lipgloss.Color("#9ece6a"),
		StatusOffline:  lipgloss.Color("#f7768e"),
		StatusDegraded: lipgloss.Color("#e0af68"),
		StatusUnknown:  lipgloss.Color("#565f89"),
	}
}

func catppuccinMocha() *Theme {
	return &Theme{
		Name:      "catppuccin-mocha",
		Bg:        lipgloss.Color("#1e1e2e"),
		Fg:        lipgloss.Color("#cdd6f4"),
		Accent:    lipgloss.Color("#89b4fa"),
		Error:     lipgloss.Color("#f38ba8"),
		Warning:   lipgloss.Color("#f9e2af"),
		Success:   lipgloss.Color("#a6e3a1"),
		Info:      lipgloss.Color("#89dceb"),
		Subtle:    lipgloss.Color("#6c7086"),
		Highlight: lipgloss.Color("#313244"),
		Cursor:    lipgloss.Color("#f5e0dc"),
		Selection: lipgloss.Color("#45475a"),
		Border:    lipgloss.Color("#585b70"),
		Header:    lipgloss.Color("#cba6f7"),
		Muted:     lipgloss.Color("#585b70"),

		ModeNormal:  lipgloss.Color("#89b4fa"),
		ModeInsert:  lipgloss.Color("#a6e3a1"),
		ModeVisual:  lipgloss.Color("#cba6f7"),
		ModeCommand: lipgloss.Color("#f9e2af"),

		ProtoSSH:    lipgloss.Color("#a6e3a1"),
		ProtoRDP:    lipgloss.Color("#89b4fa"),
		ProtoVNC:    lipgloss.Color("#fab387"),
		ProtoTelnet: lipgloss.Color("#f9e2af"),

		StatusOnline:   lipgloss.Color("#a6e3a1"),
		StatusOffline:  lipgloss.Color("#f38ba8"),
		StatusDegraded: lipgloss.Color("#f9e2af"),
		StatusUnknown:  lipgloss.Color("#6c7086"),
	}
}

func dracula() *Theme {
	return &Theme{
		Name:      "dracula",
		Bg:        lipgloss.Color("#282a36"),
		Fg:        lipgloss.Color("#f8f8f2"),
		Accent:    lipgloss.Color("#bd93f9"),
		Error:     lipgloss.Color("#ff5555"),
		Warning:   lipgloss.Color("#f1fa8c"),
		Success:   lipgloss.Color("#50fa7b"),
		Info:      lipgloss.Color("#8be9fd"),
		Subtle:    lipgloss.Color("#6272a4"),
		Highlight: lipgloss.Color("#44475a"),
		Cursor:    lipgloss.Color("#f8f8f2"),
		Selection: lipgloss.Color("#44475a"),
		Border:    lipgloss.Color("#6272a4"),
		Header:    lipgloss.Color("#ff79c6"),
		Muted:     lipgloss.Color("#6272a4"),

		ModeNormal:  lipgloss.Color("#bd93f9"),
		ModeInsert:  lipgloss.Color("#50fa7b"),
		ModeVisual:  lipgloss.Color("#ff79c6"),
		ModeCommand: lipgloss.Color("#f1fa8c"),

		ProtoSSH:    lipgloss.Color("#50fa7b"),
		ProtoRDP:    lipgloss.Color("#8be9fd"),
		ProtoVNC:    lipgloss.Color("#ffb86c"),
		ProtoTelnet: lipgloss.Color("#f1fa8c"),

		StatusOnline:   lipgloss.Color("#50fa7b"),
		StatusOffline:  lipgloss.Color("#ff5555"),
		StatusDegraded: lipgloss.Color("#f1fa8c"),
		StatusUnknown:  lipgloss.Color("#6272a4"),
	}
}

func gruvboxDark() *Theme {
	return &Theme{
		Name:      "gruvbox-dark",
		Bg:        lipgloss.Color("#282828"),
		Fg:        lipgloss.Color("#ebdbb2"),
		Accent:    lipgloss.Color("#458588"),
		Error:     lipgloss.Color("#cc241d"),
		Warning:   lipgloss.Color("#d79921"),
		Success:   lipgloss.Color("#98971a"),
		Info:      lipgloss.Color("#83a598"),
		Subtle:    lipgloss.Color("#928374"),
		Highlight: lipgloss.Color("#3c3836"),
		Cursor:    lipgloss.Color("#ebdbb2"),
		Selection: lipgloss.Color("#504945"),
		Border:    lipgloss.Color("#665c54"),
		Header:    lipgloss.Color("#b16286"),
		Muted:     lipgloss.Color("#665c54"),

		ModeNormal:  lipgloss.Color("#458588"),
		ModeInsert:  lipgloss.Color("#98971a"),
		ModeVisual:  lipgloss.Color("#b16286"),
		ModeCommand: lipgloss.Color("#d79921"),

		ProtoSSH:    lipgloss.Color("#98971a"),
		ProtoRDP:    lipgloss.Color("#83a598"),
		ProtoVNC:    lipgloss.Color("#d65d0e"),
		ProtoTelnet: lipgloss.Color("#d79921"),

		StatusOnline:   lipgloss.Color("#98971a"),
		StatusOffline:  lipgloss.Color("#cc241d"),
		StatusDegraded: lipgloss.Color("#d79921"),
		StatusUnknown:  lipgloss.Color("#928374"),
	}
}

func nord() *Theme {
	return &Theme{
		Name:      "nord",
		Bg:        lipgloss.Color("#2e3440"),
		Fg:        lipgloss.Color("#eceff4"),
		Accent:    lipgloss.Color("#88c0d0"),
		Error:     lipgloss.Color("#bf616a"),
		Warning:   lipgloss.Color("#ebcb8b"),
		Success:   lipgloss.Color("#a3be8c"),
		Info:      lipgloss.Color("#81a1c1"),
		Subtle:    lipgloss.Color("#4c566a"),
		Highlight: lipgloss.Color("#3b4252"),
		Cursor:    lipgloss.Color("#eceff4"),
		Selection: lipgloss.Color("#434c5e"),
		Border:    lipgloss.Color("#4c566a"),
		Header:    lipgloss.Color("#b48ead"),
		Muted:     lipgloss.Color("#4c566a"),

		ModeNormal:  lipgloss.Color("#88c0d0"),
		ModeInsert:  lipgloss.Color("#a3be8c"),
		ModeVisual:  lipgloss.Color("#b48ead"),
		ModeCommand: lipgloss.Color("#ebcb8b"),

		ProtoSSH:    lipgloss.Color("#a3be8c"),
		ProtoRDP:    lipgloss.Color("#81a1c1"),
		ProtoVNC:    lipgloss.Color("#d08770"),
		ProtoTelnet: lipgloss.Color("#ebcb8b"),

		StatusOnline:   lipgloss.Color("#a3be8c"),
		StatusOffline:  lipgloss.Color("#bf616a"),
		StatusDegraded: lipgloss.Color("#ebcb8b"),
		StatusUnknown:  lipgloss.Color("#4c566a"),
	}
}
