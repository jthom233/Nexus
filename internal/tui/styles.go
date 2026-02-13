package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/theme"
)

// Color palette — derived from the active theme.
// These variables are kept for backward compatibility; they delegate to
// the current theme so that a theme switch is reflected everywhere.
var (
	ColorBg        = theme.Current().Bg
	ColorFg        = theme.Current().Fg
	ColorSubtle    = theme.Current().Subtle
	ColorHighlight = theme.Current().Highlight
	ColorGreen     = theme.Current().Success
	ColorRed       = theme.Current().Error
	ColorYellow    = theme.Current().Warning
	ColorCyan      = theme.Current().Info
	ColorPurple    = theme.Current().Accent
	ColorOrange    = theme.Current().Warning
	ColorPink      = theme.Current().Header
	ColorGrey      = theme.Current().Muted
)

// Status indicators
const (
	StatusOnline   = "●"
	StatusOffline  = "○"
	StatusDegraded = "◐"
	StatusUnknown  = "?"
)

// Header styles
var (
	LogoStyle       = theme.LogoStyle()
	BreadcrumbStyle = theme.BreadcrumbStyle()
	HeaderStyle     = theme.HeaderStyle()
	HintStyle       = theme.HintStyle()
)

// Table styles
var (
	TableHeaderStyle   = theme.TableHeaderStyle()
	TableRowStyle      = theme.TableRowStyle()
	TableSelectedStyle = theme.TableSelectedStyle()
)

// Status bar styles
var (
	StatusBarStyle   = theme.StatusBarStyle()
	FlashStyle       = theme.FlashStyle()
	FlashErrorStyle  = theme.FlashErrorStyle()
	KeyHintKeyStyle  = theme.KeyHintKeyStyle()
	KeyHintDescStyle = theme.KeyHintDescStyle()
	KeyHintBarStyle  = theme.KeyHintBarStyle()
)

// Status indicator styles
var (
	StatusOnlineStyle   = theme.StatusOnlineStyle()
	StatusOfflineStyle  = theme.StatusOfflineStyle()
	StatusDegradedStyle = theme.StatusDegradedStyle()
	StatusUnknownStyle  = theme.StatusUnknownStyle()
)

// Filter bar style
var (
	FilterPromptStyle = theme.FilterPromptStyle()
	FilterInputStyle  = theme.FilterInputStyle()
)

// Command bar style
var (
	CommandPromptStyle = theme.CommandPromptStyle()
)

// Help overlay styles
var (
	HelpTitleStyle   = theme.HelpTitleStyle()
	HelpKeyStyle     = theme.HelpKeyStyle()
	HelpDescStyle    = theme.HelpDescStyle()
	HelpOverlayStyle = theme.HelpOverlayStyle()
)

// Detail view styles
var (
	DetailLabelStyle = theme.DetailLabelStyle()
	DetailValueStyle = theme.DetailValueStyle()
	DetailTitleStyle = theme.DetailTitleStyle()
)

// Confirm dialog styles
var (
	ConfirmBoxStyle    = theme.ConfirmBoxStyle()
	ConfirmPromptStyle = theme.ConfirmPromptStyle()
	ConfirmHintStyle   = theme.ConfirmHintStyle()
)

// GroupTagStyle returns a style for group tags with the given color.
func GroupTagStyle(color string) lipgloss.Style {
	return theme.GroupTagStyle(color)
}

// ProtoStyle returns a style colored per protocol.
func ProtoStyle(proto string) lipgloss.Style {
	return theme.ProtoStyle(proto)
}
