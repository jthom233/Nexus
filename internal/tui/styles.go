package tui

import "github.com/charmbracelet/lipgloss"

// Color palette (Dracula-inspired)
var (
	ColorBg        = lipgloss.Color("#282a36")
	ColorFg        = lipgloss.Color("#f8f8f2")
	ColorSubtle    = lipgloss.Color("#6272a4")
	ColorHighlight = lipgloss.Color("#44475a")
	ColorGreen     = lipgloss.Color("#50fa7b")
	ColorRed       = lipgloss.Color("#ff5555")
	ColorYellow    = lipgloss.Color("#f1fa8c")
	ColorCyan      = lipgloss.Color("#8be9fd")
	ColorPurple    = lipgloss.Color("#bd93f9")
	ColorOrange    = lipgloss.Color("#ffb86c")
	ColorPink      = lipgloss.Color("#ff79c6")
	ColorGrey      = lipgloss.Color("#6272a4")
)

// Status indicators
const (
	StatusOnline  = "●"
	StatusOffline = "○"
	StatusDegraded = "◐"
	StatusUnknown = "?"
)

// Header styles
var (
	LogoStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorCyan).
			Padding(0, 1)

	BreadcrumbStyle = lipgloss.NewStyle().
			Foreground(ColorSubtle).
			Padding(0, 1)

	HeaderStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(ColorHighlight)

	HintStyle = lipgloss.NewStyle().
			Foreground(ColorSubtle).
			Padding(0, 1)
)

// Table styles
var (
	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorPurple).
				Padding(0, 1)

	TableRowStyle = lipgloss.NewStyle().
			Foreground(ColorFg).
			Padding(0, 1)

	TableSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorFg).
				Background(ColorHighlight).
				Bold(true).
				Padding(0, 1)
)

// Status bar styles
var (
	StatusBarStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderTop(true).
			BorderForeground(ColorHighlight).
			Foreground(ColorSubtle).
			Padding(0, 1)

	FlashStyle = lipgloss.NewStyle().
			Foreground(ColorGreen)

	FlashErrorStyle = lipgloss.NewStyle().
			Foreground(ColorRed)

	KeyHintKeyStyle = lipgloss.NewStyle().
				Foreground(ColorPink).
				Bold(true)

	KeyHintDescStyle = lipgloss.NewStyle().
				Foreground(ColorSubtle)

	KeyHintBarStyle = lipgloss.NewStyle().
			Foreground(ColorSubtle).
			Padding(0, 1)
)

// Status indicator styles
var (
	StatusOnlineStyle  = lipgloss.NewStyle().Foreground(ColorGreen)
	StatusOfflineStyle = lipgloss.NewStyle().Foreground(ColorRed)
	StatusDegradedStyle = lipgloss.NewStyle().Foreground(ColorYellow)
	StatusUnknownStyle = lipgloss.NewStyle().Foreground(ColorGrey)
)

// Filter bar style
var (
	FilterPromptStyle = lipgloss.NewStyle().
				Foreground(ColorCyan).
				Bold(true)

	FilterInputStyle = lipgloss.NewStyle().
				Foreground(ColorFg)
)

// Command bar style
var (
	CommandPromptStyle = lipgloss.NewStyle().
				Foreground(ColorOrange).
				Bold(true)
)

// Help overlay styles
var (
	HelpTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorCyan).
			Padding(0, 1)

	HelpKeyStyle = lipgloss.NewStyle().
			Foreground(ColorPink).
			Bold(true).
			Width(12)

	HelpDescStyle = lipgloss.NewStyle().
			Foreground(ColorFg)

	HelpOverlayStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPurple).
				Padding(1, 2)
)

// Detail view styles
var (
	DetailLabelStyle = lipgloss.NewStyle().
				Foreground(ColorPurple).
				Bold(true).
				Width(18)

	DetailValueStyle = lipgloss.NewStyle().
				Foreground(ColorFg)

	DetailTitleStyle = lipgloss.NewStyle().
				Foreground(ColorCyan).
				Bold(true).
				Padding(0, 0, 1, 0)
)

// Confirm dialog styles
var (
	ConfirmBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorRed).
			Padding(1, 3)

	ConfirmPromptStyle = lipgloss.NewStyle().
				Foreground(ColorFg).
				Bold(true)

	ConfirmHintStyle = lipgloss.NewStyle().
				Foreground(ColorSubtle)
)

// Group tag style
func GroupTagStyle(color string) lipgloss.Style {
	c := lipgloss.Color(color)
	if color == "" {
		c = ColorSubtle
	}
	return lipgloss.NewStyle().
		Foreground(c).
		Bold(true)
}

// Protocol label style
func ProtoStyle(proto string) lipgloss.Style {
	switch proto {
	case "SSH":
		return lipgloss.NewStyle().Foreground(ColorGreen)
	case "RDP":
		return lipgloss.NewStyle().Foreground(ColorCyan)
	case "VNC":
		return lipgloss.NewStyle().Foreground(ColorOrange)
	case "TEL":
		return lipgloss.NewStyle().Foreground(ColorYellow)
	default:
		return lipgloss.NewStyle().Foreground(ColorFg)
	}
}
