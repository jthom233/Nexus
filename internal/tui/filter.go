package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// filterDebounceMsg signals that the debounce period has elapsed and the filter
// should be applied. The seq field prevents stale ticks from triggering a filter.
type filterDebounceMsg struct{ seq int }

const filterDebounceInterval = 30 * time.Millisecond

type filterModel struct {
	input      textinput.Model
	active     bool
	width      int
	debounceSeq int // incremented on each keystroke; only the latest tick fires
}

func newFilter() filterModel {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.PromptStyle = FilterPromptStyle
	ti.TextStyle = FilterInputStyle
	ti.Placeholder = "filter..."
	ti.CharLimit = 100
	return filterModel{input: ti}
}

func (f *filterModel) activate() {
	f.active = true
	f.input.Focus()
}

func (f *filterModel) deactivate() {
	f.active = false
	f.input.Blur()
	f.input.SetValue("")
}

func (f filterModel) value() string {
	return f.input.Value()
}

func (f filterModel) Update(msg tea.Msg) (filterModel, tea.Cmd) {
	if !f.active {
		return f, nil
	}
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return f, cmd
}

func (f filterModel) View() string {
	if !f.active {
		return ""
	}
	return lipgloss.NewStyle().Width(f.width).Padding(0, 1).Render(f.input.View())
}

// fuzzyMatch returns true if the query matches the target using a simple fuzzy algorithm.
func fuzzyMatch(query, target string) bool {
	query = strings.ToLower(query)
	target = strings.ToLower(target)

	if strings.Contains(target, query) {
		return true
	}

	// Simple fuzzy: all query chars must appear in order in target
	qi := 0
	for ti := 0; ti < len(target) && qi < len(query); ti++ {
		if target[ti] == query[qi] {
			qi++
		}
	}
	return qi == len(query)
}
