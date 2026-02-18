package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/theme"
)

// FinderPicker represents the type of data the finder is searching through.
type FinderPicker int

const (
	PickerConnections FinderPicker = iota
	PickerSessions
	PickerTags
	PickerGroups
	PickerRecent
	PickerCommands
)

// String returns a human-readable label for the picker type.
func (p FinderPicker) String() string {
	switch p {
	case PickerConnections:
		return "Connections"
	case PickerSessions:
		return "Sessions"
	case PickerTags:
		return "Tags"
	case PickerGroups:
		return "Groups"
	case PickerRecent:
		return "Recent"
	case PickerCommands:
		return "Commands"
	default:
		return "Unknown"
	}
}

// FinderEntry represents a single result in the finder list.
type FinderEntry struct {
	Display        string
	Score          int
	Connection     config.Connection
	MatchPositions []int
}

// FinderModel is the Telescope-style fuzzy finder overlay.
type FinderModel struct {
	prompt      textinput.Model
	entries     []FinderEntry // all entries (unfiltered)
	results     []FinderEntry // filtered + scored results
	cursor      int
	multiSelect map[int]bool
	active      bool
	picker      FinderPicker
	width       int
	height      int
}

// NewFinder creates a new FinderModel.
func NewFinder() *FinderModel {
	ti := textinput.New()
	ti.Prompt = "Find> "
	ti.Placeholder = "type to search..."
	ti.CharLimit = 200
	return &FinderModel{
		prompt:      ti,
		multiSelect: make(map[int]bool),
	}
}

// Activate opens the finder with the given picker type and data.
func (f *FinderModel) Activate(picker FinderPicker, connections []config.Connection) {
	f.active = true
	f.picker = picker
	f.cursor = 0
	f.multiSelect = make(map[int]bool)
	f.prompt.SetValue("")
	f.prompt.Focus()

	switch picker {
	case PickerGroups:
		// Build deduplicated group entries
		seen := make(map[string]bool)
		f.entries = nil
		for _, c := range connections {
			if c.Group != "" && !seen[c.Group] {
				seen[c.Group] = true
				f.entries = append(f.entries, FinderEntry{
					Display:    c.Group,
					Connection: c,
				})
			}
		}
		sort.Slice(f.entries, func(i, j int) bool {
			return f.entries[i].Display < f.entries[j].Display
		})
	case PickerTags:
		// Build deduplicated tag entries
		seen := make(map[string]bool)
		f.entries = nil
		for _, c := range connections {
			for _, tag := range c.Tags {
				if !seen[tag] {
					seen[tag] = true
					f.entries = append(f.entries, FinderEntry{
						Display:    tag,
						Connection: c,
					})
				}
			}
		}
		sort.Slice(f.entries, func(i, j int) bool {
			return f.entries[i].Display < f.entries[j].Display
		})
	default:
		// Build entries from connections
		f.entries = make([]FinderEntry, len(connections))
		for i, c := range connections {
			f.entries[i] = FinderEntry{
				Display:    c.Name,
				Connection: c,
			}
		}
	}
	f.filterResults()
}

// Deactivate closes the finder.
func (f *FinderModel) Deactivate() {
	f.active = false
	f.prompt.Blur()
	f.entries = nil
	f.results = nil
	f.multiSelect = make(map[int]bool)
}

// SetSize updates the finder dimensions.
func (f *FinderModel) SetSize(w, h int) {
	f.width = w
	f.height = h
}

// SelectedConnection returns the connection at the cursor, or nil.
func (f *FinderModel) SelectedConnection() *config.Connection {
	if len(f.results) == 0 || f.cursor < 0 || f.cursor >= len(f.results) {
		return nil
	}
	c := f.results[f.cursor].Connection
	return &c
}

// SelectedConnections returns all multi-selected connections, or just the cursor one.
func (f *FinderModel) SelectedConnections() []config.Connection {
	if len(f.multiSelect) > 0 {
		var conns []config.Connection
		for idx := range f.multiSelect {
			if idx < len(f.results) {
				conns = append(conns, f.results[idx].Connection)
			}
		}
		return conns
	}
	if c := f.SelectedConnection(); c != nil {
		return []config.Connection{*c}
	}
	return nil
}

// filterResults applies the current query to entries and scores them.
func (f *FinderModel) filterResults() {
	query := f.prompt.Value()
	f.results = nil

	if query == "" {
		// Show all entries with score 0
		for _, e := range f.entries {
			entry := e
			entry.Score = 0
			entry.MatchPositions = nil
			f.results = append(f.results, entry)
		}
	} else {
		for _, e := range f.entries {
			// Build a searchable string from connection fields
			searchable := e.Connection.Name + " " + e.Connection.Host + " " +
				string(e.Connection.Protocol) + " " + e.Connection.Group + " " +
				strings.Join(e.Connection.Tags, " ")
			score, positions := FuzzyScore(query, searchable)
			if score >= 0 {
				entry := e
				entry.Score = score
				entry.MatchPositions = positions
				f.results = append(f.results, entry)
			}
		}
		// Sort by score descending (higher is better)
		sort.SliceStable(f.results, func(i, j int) bool {
			return f.results[i].Score > f.results[j].Score
		})
	}

	// Clamp cursor
	if f.cursor >= len(f.results) {
		f.cursor = len(f.results) - 1
	}
	if f.cursor < 0 {
		f.cursor = 0
	}
}

// FuzzyScore computes a fuzzy match score between pattern and target.
// Returns (score, matchPositions). Returns (-1, nil) if no match.
// Scoring bonuses:
//   - Consecutive match: +5
//   - Word boundary match: +10
//   - Case-exact match: +1
//   - Exact substring match: +50
//   - Prefix match: +25
func FuzzyScore(pattern, target string) (int, []int) {
	if pattern == "" {
		return 0, nil
	}

	lowerPattern := strings.ToLower(pattern)
	lowerTarget := strings.ToLower(target)

	// Check for exact substring match (highest bonus)
	if idx := strings.Index(lowerTarget, lowerPattern); idx >= 0 {
		positions := make([]int, len(lowerPattern))
		for i := range positions {
			positions[i] = idx + i
		}
		score := 50 * len(pattern) // base score for exact substring
		if idx == 0 {
			score += 25 // prefix bonus
		}
		// Case-exact bonus
		matched := target[idx : idx+len(pattern)]
		for i := 0; i < len(pattern); i++ {
			if pattern[i] == matched[i] {
				score++ // exact case match
			}
		}
		return score, positions
	}

	// Fuzzy match: all pattern chars must appear in order
	positions := make([]int, 0, len(lowerPattern))
	pi := 0
	lastMatchIdx := -1
	score := 0

	targetRunes := []rune(lowerTarget)
	patternRunes := []rune(lowerPattern)
	origRunes := []rune(target)
	origPattern := []rune(pattern)

	for ti := 0; ti < len(targetRunes) && pi < len(patternRunes); ti++ {
		if targetRunes[ti] == patternRunes[pi] {
			positions = append(positions, ti)

			// Consecutive match bonus
			if lastMatchIdx >= 0 && ti == lastMatchIdx+1 {
				score += 5
			}

			// Word boundary bonus (start of string, after space/underscore/hyphen/dot)
			if ti == 0 || isWordBoundary(targetRunes, ti) {
				score += 10
			}

			// Case-exact match bonus
			if pi < len(origPattern) && ti < len(origRunes) && origRunes[ti] == origPattern[pi] {
				score++
			}

			lastMatchIdx = ti
			pi++
		}
	}

	if pi < len(patternRunes) {
		return -1, nil // not all pattern characters matched
	}

	// Base score for matching at all
	score += len(pattern)

	return score, positions
}

// isWordBoundary checks if position i in runes is at a word boundary.
func isWordBoundary(runes []rune, i int) bool {
	if i == 0 {
		return true
	}
	prev := runes[i-1]
	curr := runes[i]
	// After separator characters
	if prev == ' ' || prev == '_' || prev == '-' || prev == '.' || prev == '/' || prev == '@' {
		return true
	}
	// camelCase boundary: lowercase followed by uppercase
	if unicode.IsLower(prev) && unicode.IsUpper(curr) {
		return true
	}
	return false
}

// Update processes messages for the finder.
func (f *FinderModel) Update(msg tea.Msg) tea.Cmd {
	if !f.active {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			f.Deactivate()
			return nil
		case "enter":
			// Selection is handled by the caller
			return nil
		case "up", "ctrl+k":
			if f.cursor > 0 {
				f.cursor--
			}
			return nil
		case "down", "ctrl+j":
			if f.cursor < len(f.results)-1 {
				f.cursor++
			}
			return nil
		case "tab":
			// Toggle multi-select on current item
			if len(f.results) > 0 {
				if f.multiSelect[f.cursor] {
					delete(f.multiSelect, f.cursor)
				} else {
					f.multiSelect[f.cursor] = true
				}
				// Move cursor down after toggle
				if f.cursor < len(f.results)-1 {
					f.cursor++
				}
			}
			return nil
		default:
			// Pass to text input
			var cmd tea.Cmd
			f.prompt, cmd = f.prompt.Update(msg)
			f.filterResults()
			return cmd
		}
	}

	var cmd tea.Cmd
	f.prompt, cmd = f.prompt.Update(msg)
	return cmd
}

// View renders the three-panel finder layout.
func (f *FinderModel) View() string {
	if !f.active {
		return ""
	}

	t := theme.Current()

	// Calculate dimensions
	boxW := f.width - 4
	if boxW < 40 {
		boxW = 40
	}
	boxH := f.height - 4
	if boxH < 10 {
		boxH = 10
	}

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Accent).
		Padding(0, 1)
	title := titleStyle.Render(fmt.Sprintf(" %s", f.picker.String()))

	// Prompt
	promptStyle := lipgloss.NewStyle().
		Width(boxW - 4).
		Padding(0, 1)
	promptView := promptStyle.Render(f.prompt.View())

	// Calculate panel dimensions
	contentH := boxH - 5 // title + prompt + borders + hint
	if contentH < 3 {
		contentH = 3
	}
	leftW := (boxW - 4) * 60 / 100
	rightW := (boxW - 4) - leftW - 1 // -1 for separator

	// Left panel: results list
	leftContent := f.renderResults(leftW, contentH, t)

	// Right panel: preview
	rightContent := f.renderPreview(rightW, contentH, t)

	// Separator
	sep := lipgloss.NewStyle().
		Foreground(t.Border).
		Render(strings.Repeat("│\n", contentH))

	panels := lipgloss.JoinHorizontal(lipgloss.Top, leftContent, sep, rightContent)

	// Count + hint
	countStyle := lipgloss.NewStyle().Foreground(t.Subtle)
	hintStyle := lipgloss.NewStyle().Foreground(t.Muted).Italic(true)
	multiCount := len(f.multiSelect)
	countStr := fmt.Sprintf("%d/%d", len(f.results), len(f.entries))
	if multiCount > 0 {
		countStr += fmt.Sprintf(" [%d selected]", multiCount)
	}
	hint := lipgloss.JoinHorizontal(lipgloss.Top,
		countStyle.Render(countStr),
		"  ",
		hintStyle.Render("enter:select  tab:multi  esc:close"),
	)

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		promptView,
		"",
		panels,
		"",
		hint,
	)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Padding(0, 1).
		Width(boxW).
		Height(boxH)

	box := boxStyle.Render(content)
	return lipgloss.Place(f.width, f.height, lipgloss.Center, lipgloss.Center, box)
}

// renderResults renders the left panel with scored results.
func (f *FinderModel) renderResults(width, height int, t *theme.Theme) string {
	if len(f.results) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Width(width).
			Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(t.Subtle)
		return emptyStyle.Render("No matches")
	}

	var lines []string
	// Calculate visible window
	start := 0
	if f.cursor >= height {
		start = f.cursor - height + 1
	}
	end := start + height
	if end > len(f.results) {
		end = len(f.results)
	}

	for i := start; i < end; i++ {
		entry := f.results[i]
		isSelected := i == f.cursor
		isMulti := f.multiSelect[i]

		// Indicator
		indicator := "  "
		if isMulti {
			indicator = lipgloss.NewStyle().Foreground(t.Accent).Render("● ")
		}

		// Display text with match highlighting
		display := f.highlightMatches(entry.Display, entry.MatchPositions, t)

		// Connection info
		info := lipgloss.NewStyle().Foreground(t.Subtle).Render(
			fmt.Sprintf(" %s@%s", entry.Connection.Username, entry.Connection.Host))

		line := indicator + display + info

		// Truncate to width
		if lipgloss.Width(line) > width {
			line = line[:width]
		}

		if isSelected {
			lineStyle := lipgloss.NewStyle().
				Background(t.Selection).
				Width(width)
			line = lineStyle.Render(line)
		} else {
			lineStyle := lipgloss.NewStyle().Width(width)
			line = lineStyle.Render(line)
		}

		lines = append(lines, line)
	}

	// Pad remaining lines
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}

	return strings.Join(lines, "\n")
}

// highlightMatches renders text with match positions highlighted.
func (f *FinderModel) highlightMatches(text string, positions []int, t *theme.Theme) string {
	if len(positions) == 0 {
		return lipgloss.NewStyle().Foreground(t.Fg).Render(text)
	}

	matchStyle := lipgloss.NewStyle().Foreground(t.Warning).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(t.Fg)

	posSet := make(map[int]bool)
	for _, p := range positions {
		posSet[p] = true
	}

	runes := []rune(text)
	var result strings.Builder
	for i, r := range runes {
		if posSet[i] {
			result.WriteString(matchStyle.Render(string(r)))
		} else {
			result.WriteString(normalStyle.Render(string(r)))
		}
	}
	return result.String()
}

// renderPreview renders the right panel with connection details.
func (f *FinderModel) renderPreview(width, height int, t *theme.Theme) string {
	if len(f.results) == 0 || f.cursor < 0 || f.cursor >= len(f.results) {
		emptyStyle := lipgloss.NewStyle().
			Width(width).
			Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(t.Subtle)
		return emptyStyle.Render("No preview")
	}

	c := f.results[f.cursor].Connection
	labelStyle := lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Width(12)
	valueStyle := lipgloss.NewStyle().Foreground(t.Fg)
	titleStyle := lipgloss.NewStyle().Foreground(t.Info).Bold(true)

	var lines []string
	lines = append(lines, titleStyle.Render(c.Name))
	lines = append(lines, "")

	row := func(label, value string) {
		if value != "" {
			lines = append(lines, labelStyle.Render(label)+valueStyle.Render(value))
		}
	}

	row("Protocol:", c.Protocol.Label())
	row("Host:", c.Host)
	row("Port:", fmt.Sprintf("%d", c.EffectivePort()))
	row("User:", c.Username)
	row("Group:", c.Group)
	if len(c.Tags) > 0 {
		row("Tags:", strings.Join(c.Tags, ", "))
	}
	if c.Notes != "" {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(t.Subtle).Italic(true).Render(c.Notes))
	}
	if len(c.CustomFields) > 0 {
		lines = append(lines, "")
		cfKeys := make([]string, 0, len(c.CustomFields))
		for k := range c.CustomFields {
			cfKeys = append(cfKeys, k)
		}
		sort.Strings(cfKeys)
		for _, k := range cfKeys {
			row(k+":", c.CustomFields[k])
		}
	}

	// Pad/truncate to height
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	// Ensure width
	style := lipgloss.NewStyle().Width(width)
	for i, line := range lines {
		lines[i] = style.Render(line)
	}

	return strings.Join(lines, "\n")
}
