package theme

import "github.com/charmbracelet/lipgloss"

// --- Header styles ---

func LogoStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(current.Info).
		Padding(0, 1)
}

func BreadcrumbStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Subtle).
		Padding(0, 1)
}

func HeaderStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(current.Highlight)
}

func HintStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Subtle).
		Padding(0, 1)
}

// --- Table styles ---

func TableHeaderStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(current.Header).
		Padding(0, 1)
}

func TableRowStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Fg).
		Padding(0, 1)
}

func TableSelectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Fg).
		Background(current.Highlight).
		Bold(true).
		Padding(0, 1)
}

// --- Status bar styles ---

func StatusBarStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderTop(true).
		BorderForeground(current.Highlight).
		Foreground(current.Subtle).
		Padding(0, 1)
}

func FlashStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Success)
}

func FlashErrorStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Error)
}

func KeyHintKeyStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Header).
		Bold(true)
}

func KeyHintDescStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Subtle)
}

func KeyHintBarStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Subtle).
		Padding(0, 1)
}

// --- Status indicator styles ---

func StatusOnlineStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(current.StatusOnline)
}

func StatusOfflineStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(current.StatusOffline)
}

func StatusDegradedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(current.StatusDegraded)
}

func StatusUnknownStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(current.StatusUnknown)
}

// --- Filter bar style ---

func FilterPromptStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Info).
		Bold(true)
}

func FilterInputStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Fg)
}

// --- Command bar style ---

func CommandPromptStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Warning).
		Bold(true)
}

// --- Help overlay styles ---

func HelpTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(current.Info).
		Padding(0, 1)
}

func HelpKeyStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Header).
		Bold(true).
		Width(12)
}

func HelpDescStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Fg)
}

func HelpOverlayStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(current.Header).
		Padding(1, 2)
}

// --- Detail view styles ---

func DetailLabelStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Header).
		Bold(true).
		Width(18)
}

func DetailValueStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Fg)
}

func DetailTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Info).
		Bold(true).
		Padding(0, 0, 1, 0)
}

// --- Confirm dialog styles ---

func ConfirmBoxStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(current.Error).
		Padding(1, 3)
}

func ConfirmPromptStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Fg).
		Bold(true)
}

func ConfirmHintStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(current.Subtle)
}

// --- Group tag style ---

func GroupTagStyle(color string) lipgloss.Style {
	c := lipgloss.Color(color)
	if color == "" {
		c = current.Subtle
	}
	return lipgloss.NewStyle().
		Foreground(c).
		Bold(true)
}

// --- Protocol label style ---

func ProtoStyle(proto string) lipgloss.Style {
	switch proto {
	case "SSH":
		return lipgloss.NewStyle().Foreground(current.ProtoSSH)
	case "RDP":
		return lipgloss.NewStyle().Foreground(current.ProtoRDP)
	case "VNC":
		return lipgloss.NewStyle().Foreground(current.ProtoVNC)
	case "TEL":
		return lipgloss.NewStyle().Foreground(current.ProtoTelnet)
	default:
		return lipgloss.NewStyle().Foreground(current.Fg)
	}
}
