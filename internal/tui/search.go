package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/theme"
)

// SearchMode represents the type of active search.
type SearchMode int

const (
	SearchNone    SearchMode = iota
	SearchForward            // / -- forward search
	SearchReverse            // ? -- reverse search
)

const maxSearchHistory = 50

// parsedSearch holds the decomposed search query after prefix parsing.
type parsedSearch struct {
	mode    string // "fuzzy", "inverse", "strict-fuzzy", "tag", "group"
	pattern string // the actual search text after prefix
}

// searchModel manages incremental search state.
type searchModel struct {
	mode       SearchMode // current search mode (forward/reverse/none)
	query      string     // current search text being typed
	savedQuery string     // saved input when browsing history
	history    []string   // search history (most recent first)
	historyIdx int        // -1 = current input, 0+ = history entry
	matches    []int      // indices of matching rows in the table (into allRows)
	matchIdx   int        // current match position in matches slice
	active     bool       // is search input active (cursor in search bar)
	confirmed  bool       // search was confirmed (for n/N navigation)
	width      int        // available width for rendering
}

func newSearch() searchModel {
	return searchModel{historyIdx: -1}
}

// activate starts a new search session with the given mode.
func (s *searchModel) activate(mode SearchMode) {
	s.mode = mode
	s.active = true
	s.query = ""
	s.savedQuery = ""
	s.historyIdx = -1
	s.matches = nil
	s.matchIdx = 0
	s.confirmed = false
}

// deactivate cancels the search and clears state.
func (s *searchModel) deactivate() {
	s.active = false
	s.query = ""
	s.savedQuery = ""
	s.historyIdx = -1
	s.matches = nil
	s.matchIdx = 0
	s.confirmed = false
	s.mode = SearchNone
}

// confirm locks in the current search, saves to history, and deactivates input.
func (s *searchModel) confirm() {
	s.active = false
	s.confirmed = true
	if s.query != "" {
		s.pushHistory(s.query)
	}
	s.historyIdx = -1
	s.savedQuery = ""
}

// pushHistory adds a query to the front of the history, deduplicating.
func (s *searchModel) pushHistory(q string) {
	// Remove duplicate if it exists
	for i, h := range s.history {
		if h == q {
			s.history = append(s.history[:i], s.history[i+1:]...)
			break
		}
	}
	// Prepend
	s.history = append([]string{q}, s.history...)
	if len(s.history) > maxSearchHistory {
		s.history = s.history[:maxSearchHistory]
	}
}

// handleRune appends a character to the query.
func (s *searchModel) handleRune(r rune) {
	s.query += string(r)
	s.historyIdx = -1
}

// handleBackspace removes the last character from the query.
func (s *searchModel) handleBackspace() {
	if len(s.query) > 0 {
		runes := []rune(s.query)
		s.query = string(runes[:len(runes)-1])
		s.historyIdx = -1
	}
}

// historyUp cycles to an older history entry.
func (s *searchModel) historyUp() {
	if len(s.history) == 0 {
		return
	}
	if s.historyIdx == -1 {
		// Save current input before browsing history
		s.savedQuery = s.query
	}
	if s.historyIdx+1 < len(s.history) {
		s.historyIdx++
		s.query = s.history[s.historyIdx]
	}
}

// historyDown cycles to a newer history entry or back to current input.
func (s *searchModel) historyDown() {
	if s.historyIdx <= -1 {
		return
	}
	s.historyIdx--
	if s.historyIdx < 0 {
		s.query = s.savedQuery
	} else {
		s.query = s.history[s.historyIdx]
	}
}

// parseSearchQuery parses the search query into mode and pattern.
func parseSearchQuery(query string) parsedSearch {
	if strings.HasPrefix(query, "!") {
		return parsedSearch{mode: "inverse", pattern: query[1:]}
	}
	if strings.HasPrefix(query, "-f ") {
		return parsedSearch{mode: "strict-fuzzy", pattern: query[3:]}
	}
	if strings.HasPrefix(query, "-t ") {
		return parsedSearch{mode: "tag", pattern: query[3:]}
	}
	if strings.HasPrefix(query, "-g ") {
		return parsedSearch{mode: "group", pattern: query[3:]}
	}
	return parsedSearch{mode: "fuzzy", pattern: query}
}

// matchRow checks whether a single row matches the parsed search query.
// The cells slice order is: [indicator, name, host, protocol, group, latency, ...].
// In wide mode additional columns include port, username, tags, identity.
func matchRow(row Row, ps parsedSearch) bool {
	switch ps.mode {
	case "inverse":
		searchable := strings.Join(row.Cells, " ")
		return !fuzzyMatch(ps.pattern, searchable)
	case "strict-fuzzy":
		searchable := strings.Join(row.Cells, " ")
		return strictFuzzyMatch(ps.pattern, searchable)
	case "tag":
		// Tags are typically in the TAGS column (index 8 in wide mode).
		// In normal mode, tags are not displayed but we search all cells.
		searchable := strings.Join(row.Cells, " ")
		return containsTag(ps.pattern, searchable)
	case "group":
		// Group column is index 4 in both normal and wide layouts.
		if len(row.Cells) > 4 {
			return strings.EqualFold(strings.TrimSpace(row.Cells[4]), strings.TrimSpace(ps.pattern))
		}
		return false
	default: // "fuzzy"
		searchable := strings.Join(row.Cells, " ")
		return fuzzyMatch(ps.pattern, searchable)
	}
}

// strictFuzzyMatch checks that all query characters appear in order in the target.
// Unlike fuzzyMatch, this does NOT check for substring containment first.
func strictFuzzyMatch(query, target string) bool {
	query = strings.ToLower(query)
	target = strings.ToLower(target)
	qi := 0
	for ti := 0; ti < len(target) && qi < len(query); ti++ {
		if target[ti] == query[qi] {
			qi++
		}
	}
	return qi == len(query)
}

// containsTag checks whether any comma- or space-separated tag in the target
// matches the given tag pattern (case-insensitive).
func containsTag(tag, cellText string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return true
	}
	// Split on commas, spaces, and common separators
	for _, sep := range []string{",", " "} {
		for _, part := range strings.Split(cellText, sep) {
			if strings.ToLower(strings.TrimSpace(part)) == tag {
				return true
			}
		}
	}
	return false
}

// computeMatches finds all row indices in the given rows slice that match the query.
func (s *searchModel) computeMatches(rows []Row) {
	s.matches = nil
	if s.query == "" {
		return
	}
	ps := parseSearchQuery(s.query)
	if ps.pattern == "" && ps.mode != "inverse" {
		return
	}
	for i, row := range rows {
		if matchRow(row, ps) {
			s.matches = append(s.matches, i)
		}
	}
}

// nextMatch returns the index of the next match after the given cursor position.
// forward=true searches forward; forward=false searches backward.
// Returns -1 if no matches exist.
func (s *searchModel) nextMatch(cursor int, forward bool) int {
	if len(s.matches) == 0 {
		return -1
	}
	if forward {
		// Find first match with index > cursor
		for _, idx := range s.matches {
			if idx > cursor {
				return idx
			}
		}
		// Wrap around
		return s.matches[0]
	}
	// Reverse: find last match with index < cursor
	for i := len(s.matches) - 1; i >= 0; i-- {
		if s.matches[i] < cursor {
			return s.matches[i]
		}
	}
	// Wrap around
	return s.matches[len(s.matches)-1]
}

// currentMatchNumber returns the 1-based ordinal of the given cursor position
// within the match list, or 0 if not on a match.
func (s *searchModel) currentMatchNumber(cursor int) int {
	for i, idx := range s.matches {
		if idx == cursor {
			return i + 1
		}
	}
	return 0
}

// searchForName initiates a search for the given name string.
func (s *searchModel) searchForName(name string, mode SearchMode) {
	s.mode = mode
	s.query = name
	s.active = false
	s.confirmed = true
	if name != "" {
		s.pushHistory(name)
	}
	s.historyIdx = -1
	s.savedQuery = ""
}

// View renders the search bar.
func (s searchModel) View() string {
	if !s.active {
		return ""
	}

	th := theme.Current()

	prefixChar := "/"
	if s.mode == SearchReverse {
		prefixChar = "?"
	}

	prefixStyle := lipgloss.NewStyle().
		Foreground(th.Accent).
		Bold(true)
	textStyle := lipgloss.NewStyle().
		Foreground(th.Fg)
	cursorStyle := lipgloss.NewStyle().
		Background(th.Fg).
		Foreground(th.Bg)

	prefix := prefixStyle.Render(prefixChar)
	text := textStyle.Render(s.query)
	cursor := cursorStyle.Render(" ")

	left := prefix + text + cursor

	// Match count indicator on the right
	right := ""
	if s.query != "" {
		matchNum := 0
		total := len(s.matches)
		if total > 0 {
			// Find which match we're closest to
			matchNum = s.currentMatchNumber(s.matchIdx)
			if matchNum == 0 && total > 0 {
				matchNum = 1
			}
		}
		countStyle := lipgloss.NewStyle().
			Foreground(th.Subtle)
		if total == 0 {
			right = countStyle.Render("[0/0]")
		} else {
			right = countStyle.Render(fmt.Sprintf("[%d/%d]", matchNum, total))
		}
	}

	gap := s.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 0 {
		gap = 0
	}

	barStyle := lipgloss.NewStyle().
		Width(s.width).
		Padding(0, 1)

	line := left + strings.Repeat(" ", gap) + right
	return barStyle.Render(line)
}
