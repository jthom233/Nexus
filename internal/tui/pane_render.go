package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/session"
	"github.com/dr4zz/nexus/internal/theme"
)

// panePickerMsg is sent when the user selects a connection in the empty pane picker.
type panePickerMsg struct {
	PaneID     string
	Connection config.Connection
}

// panePickerCancelMsg is sent when the user cancels the connection picker.
type panePickerCancelMsg struct {
	PaneID string
}

// PaneConnectionPicker is a simple inline picker shown inside an empty pane.
// When the user presses Enter on a highlighted connection, a background session
// is started and the pane transitions Empty→Connecting→Active.
type PaneConnectionPicker struct {
	PaneID      string
	Connections []config.Connection
	Cursor      int
	Width       int
	Height      int
	Active      bool
}

// NewPaneConnectionPicker creates a picker for the given pane ID.
func NewPaneConnectionPicker(paneID string, conns []config.Connection, width, height int) *PaneConnectionPicker {
	return &PaneConnectionPicker{
		PaneID:      paneID,
		Connections: conns,
		Cursor:      0,
		Width:       width,
		Height:      height,
		Active:      true,
	}
}

// MoveUp moves the selection cursor up one entry.
func (p *PaneConnectionPicker) MoveUp() {
	if p.Cursor > 0 {
		p.Cursor--
	}
}

// MoveDown moves the selection cursor down one entry.
func (p *PaneConnectionPicker) MoveDown() {
	if p.Cursor < len(p.Connections)-1 {
		p.Cursor++
	}
}

// Selected returns a pointer to the currently highlighted connection, or nil.
func (p *PaneConnectionPicker) Selected() *config.Connection {
	if p.Cursor >= 0 && p.Cursor < len(p.Connections) {
		c := p.Connections[p.Cursor]
		return &c
	}
	return nil
}

// Confirm deactivates the picker and returns the selected connection (or nil).
func (p *PaneConnectionPicker) Confirm() *config.Connection {
	c := p.Selected()
	p.Active = false
	return c
}

// Cancel deactivates the picker without selecting a connection.
func (p *PaneConnectionPicker) Cancel() {
	p.Active = false
}

// View renders the picker list for embedding inside an empty pane's content area.
func (p *PaneConnectionPicker) View() string {
	t := theme.Current()
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
	cursorStyle := lipgloss.NewStyle().Background(t.Accent).Foreground(lipgloss.Color("15"))
	dimStyle := lipgloss.NewStyle().Foreground(t.Muted)

	lines := []string{
		titleStyle.Render("Select connection  (j/k, Enter, Esc)"),
		"",
	}

	maxVisible := p.Height - 4
	if maxVisible < 1 {
		maxVisible = 1
	}

	start := 0
	if p.Cursor >= maxVisible {
		start = p.Cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(p.Connections) {
		end = len(p.Connections)
	}

	if len(p.Connections) == 0 {
		lines = append(lines, dimStyle.Render("No connections configured"))
	} else {
		for i := start; i < end; i++ {
			c := p.Connections[i]
			label := fmt.Sprintf("%-28s  %-5s  %s", c.Name, c.Protocol.Label(), c.HostPort())
			if i == p.Cursor {
				lines = append(lines, cursorStyle.Render("> "+label))
			} else {
				lines = append(lines, "  "+label)
			}
		}
	}

	return strings.Join(lines, "\n")
}

// startBackgroundSession creates and starts a ManagedSession in background mode
// for the given connection. The caller must wire up output forwarding.
func startBackgroundSession(c config.Connection) (*session.ManagedSession, error) {
	sess := session.NewManagedSession(session.ManagedSessionOptions{
		ID:           "",
		Name:         c.Name,
		ConnID:       c.ID,
		Protocol:     string(c.Protocol),
		Host:         c.Host,
		Port:         c.EffectivePort(),
		Username:     c.Username,
		Password:     c.Password,
		IdentityFile: c.IdentityFile,
		ProxyJump:    c.ProxyJump,
		ProxyCommand: c.ProxyCommand,
	})
	sess.PortForwards = c.PortForwards
	if err := sess.StartBackground(); err != nil {
		return nil, err
	}
	return sess, nil
}

// renderPane produces the styled string representation of a single pane.
//
// When solo is true (only one pane in the layout), no border is drawn.
// When focused is true, the border uses the accent color; unfocused panes use
// the muted border color.
// When broadcasting is true and the pane is active, the border is styled with
// the warning color. The focused pane additionally shows a "[BROADCAST]" label
// injected into the top border line.
func renderPane(pane *Pane, focused bool, solo bool, broadcasting bool, zoomed bool) string {
	content := paneContent(pane)

	if solo {
		// Single-pane layout: no border, just the raw content.
		return content
	}

	t := theme.Current()
	borderColor := t.Border
	if broadcasting && pane.State == PaneActive {
		borderColor = t.Warning
	} else if focused {
		borderColor = t.Accent
	}

	// Compute inner dimensions: lipgloss border consumes 2 cols and 2 rows.
	innerW := pane.Width - 2
	innerH := pane.Height - 2
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}

	rendered := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		Width(innerW).
		Height(innerH).
		Render(content)

	// When broadcasting on the focused active pane, inject "[BROADCAST]" into
	// the top border line. lipgloss v1.1.0 has no native border-title support,
	// so we rewrite the first visible line of the rendered output.
	if broadcasting && focused && pane.State == PaneActive {
		rendered = injectBorderTitle(rendered, "[BROADCAST]")
	}

	// When zoomed, inject "[ZOOM]" into the top border line to indicate
	// that the pane is currently expanded to fill the full layout area.
	if zoomed {
		rendered = injectBorderTitle(rendered, "[ZOOM]")
	}

	return rendered
}

// injectBorderTitle rewrites the first line of a lipgloss-bordered string,
// inserting title text immediately after the top-left corner character.
// The title replaces the same number of horizontal border characters (─) so
// the overall line width is preserved.
func injectBorderTitle(rendered string, title string) string {
	lines := strings.SplitN(rendered, "\n", 2)
	if len(lines) == 0 {
		return rendered
	}

	topLine := lines[0]
	rest := ""
	if len(lines) > 1 {
		rest = "\n" + lines[1]
	}

	// The top-left corner of NormalBorder is "┌" (U+250C, 3 UTF-8 bytes).
	// We want to replace the horizontal chars right after it with the title.
	// Find the corner rune and inject title after it.
	runes := []rune(topLine)
	if len(runes) < 2 {
		return rendered
	}

	// runes[0] should be "┌"; skip it and replace following runes with title.
	titleRunes := []rune(title)
	titleLen := len(titleRunes)

	// Ensure we don't exceed the line length.
	available := len(runes) - 2 // exclude corner chars at both ends
	if titleLen > available {
		titleLen = available
	}

	newLine := make([]rune, len(runes))
	copy(newLine, runes)
	for i := 0; i < titleLen; i++ {
		newLine[1+i] = titleRunes[i]
	}

	return string(newLine) + rest
}

// paneContent returns the text content to display inside a pane, based on its
// current state. When the pane has an active connection picker it is rendered
// regardless of the pane state.
func paneContent(pane *Pane) string {
	// If a picker is open (and active), render it instead of the normal state.
	if pane.Picker != nil && pane.Picker.Active {
		return pane.Picker.View()
	}

	switch pane.State {
	case PaneActive:
		if pane.VTerm != nil {
			return pane.VTerm.Render()
		}
		return ""

	case PaneConnecting:
		return centeredText(pane.Width, pane.Height, "Connecting...")

	case PaneGUISession:
		return centeredText(pane.Width, pane.Height, "Session running in GUI window")

	case PaneDisconnected:
		return renderDisconnected(pane)

	default: // PaneEmpty
		return centeredText(pane.Width, pane.Height, "")
	}
}

// renderDisconnected renders the content for a pane in the PaneDisconnected state.
// It shows the disconnect reason (if any) and reconnect instructions.
func renderDisconnected(pane *Pane) string {
	t := theme.Current()
	errorStyle := lipgloss.NewStyle().Foreground(t.Error)
	hintStyle := lipgloss.NewStyle().Foreground(t.Muted)

	var lines []string

	if pane.DisconnectErr != nil {
		errMsg := pane.DisconnectErr.Error()
		lines = append(lines, errorStyle.Render(errMsg))
	} else {
		lines = append(lines, errorStyle.Render("Disconnected"))
	}
	lines = append(lines, "")
	lines = append(lines, hintStyle.Render("Enter to reconnect  ·  Space w c to close"))

	content := strings.Join(lines, "\n")

	// Center vertically within the pane.
	lineCount := len(lines)
	padTop := (pane.Height - lineCount) / 2
	if padTop < 0 {
		padTop = 0
	}

	var sb strings.Builder
	for i := 0; i < padTop; i++ {
		sb.WriteByte('\n')
	}
	sb.WriteString(content)
	return sb.String()
}

// centeredText returns a string with text centered within the given dimensions.
// The vertical centering is approximate: it pads with newlines above and below.
func centeredText(width, height int, text string) string {
	t := theme.Current()
	styled := lipgloss.NewStyle().
		Foreground(t.Muted).
		Render(text)

	textWidth := len([]rune(text))
	padLeft := (width - textWidth) / 2
	if padLeft < 0 {
		padLeft = 0
	}
	line := strings.Repeat(" ", padLeft) + styled

	padTop := (height - 1) / 2
	if padTop < 0 {
		padTop = 0
	}

	var sb strings.Builder
	for i := 0; i < padTop; i++ {
		sb.WriteByte('\n')
	}
	sb.WriteString(line)
	return sb.String()
}
