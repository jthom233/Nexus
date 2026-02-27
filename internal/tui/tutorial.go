package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/theme"
)

// tutorialStep holds the title and body text for one step.
type tutorialStep struct {
	title string
	body  string
}

var tutorialSteps = []tutorialStep{
	{
		title: "Welcome to Nexus!",
		body:  "Navigate the connection list with  j/k  or the arrow keys.\nUse  g/G  to jump to the top or bottom.",
	},
	{
		title: "Connecting",
		body:  "Press  Enter  to connect to the highlighted server.\nNexus supports SSH, RDP, VNC, and Telnet.",
	},
	{
		title: "Command Menu",
		body:  "Press  Space  to open the which-key command menu.\nIt shows every available action grouped by context.",
	},
	{
		title: "Command Palette",
		body:  "Type  :  to open the command palette.\nTry  :add ,  :edit ,  :delete ,  or  :help .",
	},
	{
		title: "You're Ready!",
		body:  "Press  ?  at any time to show the full keybinding help.\nPress  q  to quit.  Enjoy using Nexus!",
	},
}

// tutorialModel is a 5-step interactive walkthrough overlay.
type tutorialModel struct {
	active bool
	step   int
	width  int
	height int
}

func newTutorial() tutorialModel {
	return tutorialModel{}
}

// activate starts the tutorial from the beginning.
func (t *tutorialModel) activate() {
	t.active = true
	t.step = 0
}

// advance moves to the next step, or deactivates when finished.
// Returns true when the tutorial has just finished.
func (t *tutorialModel) advance() bool {
	t.step++
	if t.step >= len(tutorialSteps) {
		t.active = false
		return true
	}
	return false
}

// Update handles key messages while the tutorial is active.
// Any key press advances to the next step (or closes the tutorial).
func (t tutorialModel) Update(msg tea.Msg) (tutorialModel, bool) {
	if !t.active {
		return t, false
	}
	if _, ok := msg.(tea.KeyMsg); ok {
		done := t.advance()
		return t, done
	}
	return t, false
}

// View renders the tutorial card centred on screen.
func (t tutorialModel) View() string {
	if !t.active || t.step >= len(tutorialSteps) {
		return ""
	}

	s := tutorialSteps[t.step]
	total := len(tutorialSteps)

	col := theme.Current()

	// Progress indicator: "Step 1 / 5"
	progress := lipgloss.NewStyle().
		Foreground(col.Subtle).
		Render(itoa(t.step+1) + " / " + itoa(total))

	// Title
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(col.Info).
		Render(s.title)

	// Body
	body := lipgloss.NewStyle().
		Foreground(col.Fg).
		Render(s.body)

	// Footer hint
	var hint string
	if t.step == total-1 {
		hint = "Press any key to start using Nexus."
	} else {
		hint = "Press any key to continue..."
	}
	footer := lipgloss.NewStyle().
		Foreground(col.Subtle).
		Italic(true).
		Render(hint)

	content := lipgloss.JoinVertical(lipgloss.Center,
		progress,
		"",
		title,
		"",
		body,
		"",
		footer,
	)

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(col.Header).
		Padding(1, 3).
		Width(54).
		Render(content)

	return lipgloss.Place(t.width, t.height, lipgloss.Center, lipgloss.Center, card)
}
