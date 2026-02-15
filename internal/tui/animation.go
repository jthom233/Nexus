package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// transitionTickMsg drives animation frames.
type transitionTickMsg struct{}

// transition manages a view fade animation.
type transition struct {
	active   bool
	pending  bool // set by pushView/popView, consumed by Update
	frame    int
	total    int
	interval time.Duration
}

// newTransition creates a transition with the given total frames.
func newTransition(frames int) transition {
	return transition{
		total:    frames,
		interval: 16 * time.Millisecond,
	}
}

// start begins the transition animation.
// Returns a tea.Cmd that ticks the first frame.
func (t *transition) start() tea.Cmd {
	if t.total <= 0 {
		return nil
	}
	t.active = true
	t.frame = 0
	return t.tick()
}

// markPending flags the transition to start on the next Update cycle.
// This is used by pushView/popView which cannot return tea.Cmd directly.
func (t *transition) markPending() {
	if t.total <= 0 {
		return
	}
	t.pending = true
	t.active = true
	t.frame = 0
}

// consumePending returns a tick command if a transition start is pending,
// and clears the pending flag. Called at the end of Update.
func (t *transition) consumePending() tea.Cmd {
	if !t.pending {
		return nil
	}
	t.pending = false
	return t.tick()
}

// tick returns a tea.Cmd for the next frame.
func (t *transition) tick() tea.Cmd {
	return tea.Tick(t.interval, func(time.Time) tea.Msg {
		return transitionTickMsg{}
	})
}

// advance moves to the next frame. Returns a tea.Cmd if more frames remain.
func (t *transition) advance() tea.Cmd {
	t.frame++
	if t.frame >= t.total {
		t.active = false
		t.frame = 0
		return nil
	}
	return t.tick()
}

// skip jumps to the end of the transition.
func (t *transition) skip() {
	t.active = false
	t.frame = 0
	t.pending = false
}

// progress returns the animation progress from 0.0 to 1.0.
func (t *transition) progress() float64 {
	if t.total <= 0 {
		return 1.0
	}
	return float64(t.frame) / float64(t.total)
}

// fadeStyle applies a fade effect by blending the foreground color toward the
// background. progress 0.0 = fully visible, 1.0 = fully faded to background.
func fadeStyle(s lipgloss.Style, bgColor lipgloss.Color, progress float64) lipgloss.Style {
	if progress <= 0 {
		return s
	}
	if progress >= 1.0 {
		return s.Foreground(bgColor)
	}
	// At intermediate values, return the style unmodified.
	// Full color interpolation would require parsing hex colors;
	// callers can use the Muted theme color as a midpoint instead.
	return s
}
