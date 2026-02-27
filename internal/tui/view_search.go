package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dr4zz/nexus/internal/config"
)

// handleSearchInput processes key events while the search bar is active.
func (a App) handleSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch k {
	case "esc":
		// Cancel search, clear filter and highlight
		a.search.deactivate()
		a.list.table.setFilter("")
		a.list.table.highlightText = ""
		a.list.applyFilter("")
		a.header.setFilter("")
		a.header.setItemCount(len(a.list.filtered))
		a.statusBar.mode = ModeNormal
		a.syncCursorPosition()
		return a, a.setMode(ModeNormal)

	case "enter":
		// Confirm search
		query := a.search.query
		a.search.computeMatches(a.list.table.rows)
		a.search.confirm()

		if query != "" {
			ps := parseSearchQuery(query)
			a.list.table.highlightText = ps.pattern
			a.header.setFilter(query)
		} else {
			a.list.table.highlightText = ""
			a.header.setFilter("")
		}
		a.statusBar.mode = ModeNormal
		a.syncCursorPosition()
		return a, a.setMode(ModeNormal)

	case "up":
		a.search.historyUp()
		a.applyIncrementalSearch()
		return a, nil

	case "down":
		a.search.historyDown()
		a.applyIncrementalSearch()
		return a, nil

	case "backspace", "ctrl+h":
		a.search.handleBackspace()
		a.applyIncrementalSearch()
		return a, nil

	default:
		// Handle regular character input
		if len(k) == 1 {
			a.search.handleRune(rune(k[0]))
			a.applyIncrementalSearch()
			return a, nil
		}
		// Handle rune messages for multi-byte characters
		if msg.Type == tea.KeyRunes {
			for _, r := range msg.Runes {
				a.search.handleRune(r)
			}
			a.applyIncrementalSearch()
			return a, nil
		}
	}
	return a, nil
}

// applyIncrementalSearch updates the table filter and match indices as the user types.
func (a *App) applyIncrementalSearch() {
	query := a.search.query
	if query == "" {
		a.list.applyFilter("")
		a.list.table.highlightText = ""
		a.header.setFilter("")
		a.header.setItemCount(len(a.list.filtered))
		a.search.matches = nil
		a.syncCursorPosition()
		return
	}

	ps := parseSearchQuery(query)
	a.list.table.highlightText = ps.pattern

	// Apply filter based on parsed search mode
	a.applySearchFilter(ps)
	a.header.setFilter(query)
	a.header.setItemCount(len(a.list.filtered))

	// Compute matches on the filtered rows
	a.search.computeMatches(a.list.table.rows)

	// Jump to first match if there are any
	if len(a.search.matches) > 0 {
		var target int
		if a.search.mode == SearchForward {
			target = a.search.matches[0]
		} else {
			target = a.search.matches[len(a.search.matches)-1]
		}
		a.list.table.MoveCursor(target)
		a.search.matchIdx = target
	}
	a.syncCursorPosition()
}

// applySearchFilter filters the list based on the parsed search query.
func (a *App) applySearchFilter(ps parsedSearch) {
	if ps.pattern == "" && ps.mode != "inverse" {
		a.list.applyFilter("")
		return
	}

	switch ps.mode {
	case "tag":
		a.applyTagFilter(ps.pattern)
	case "group":
		a.applyGroupSearchFilter(ps.pattern)
	default:
		// For fuzzy, inverse, strict-fuzzy we apply at the row level
		a.applyRowFilter(ps)
	}
}

// applyRowFilter filters list rows using the search matching logic.
func (a *App) applyRowFilter(ps parsedSearch) {
	base := a.list.groupFilteredConns()
	if ps.pattern == "" && ps.mode != "inverse" {
		a.list.filtered = base
		a.list.rebuildTable()
		return
	}

	// Build temporary rows for matching, then filter connections
	var result []config.Connection
	for _, c := range base {
		searchable := strings.Join([]string{c.Name, c.Host, string(c.Protocol), c.Group, strings.Join(c.Tags, " ")}, " ")
		tempRow := Row{Cells: []string{"", c.Name, c.Host, c.Protocol.Label(), c.Group, ""}}

		switch ps.mode {
		case "inverse":
			if !fuzzyMatch(ps.pattern, searchable) {
				result = append(result, c)
			}
		case "strict-fuzzy":
			if strictFuzzyMatch(ps.pattern, searchable) {
				result = append(result, c)
			}
		default: // "fuzzy"
			_ = tempRow // satisfy compiler
			if fuzzyMatch(ps.pattern, searchable) {
				result = append(result, c)
			}
		}
	}
	a.list.filtered = result
	a.list.rebuildTable()
}

// applyTagFilter filters connections by a specific tag.
func (a *App) applyTagFilter(tag string) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	base := a.list.groupFilteredConns()
	if tag == "" {
		a.list.filtered = base
		a.list.rebuildTable()
		return
	}

	var result []config.Connection
	for _, c := range base {
		for _, t := range c.Tags {
			if strings.ToLower(strings.TrimSpace(t)) == tag {
				result = append(result, c)
				break
			}
		}
	}
	a.list.filtered = result
	a.list.rebuildTable()
}

// applyGroupSearchFilter filters connections by group name.
func (a *App) applyGroupSearchFilter(group string) {
	group = strings.TrimSpace(group)
	base := a.list.groupFilteredConns()
	if group == "" {
		a.list.filtered = base
		a.list.rebuildTable()
		return
	}

	var result []config.Connection
	for _, c := range base {
		if strings.EqualFold(strings.TrimSpace(c.Group), group) {
			result = append(result, c)
		}
	}
	a.list.filtered = result
	a.list.rebuildTable()
}

// searchNextMatch jumps to the next (or previous) search match.
// sameDirection=true means follow the original search direction, false means opposite.
func (a App) searchNextMatch(sameDirection bool) (tea.Model, tea.Cmd) {
	if !a.search.confirmed || a.search.query == "" {
		return a, nil
	}

	// Recompute matches on current rows
	a.search.computeMatches(a.list.table.rows)
	if len(a.search.matches) == 0 {
		a.statusBar.setFlash("Pattern not found: "+a.search.query, flashError)
		return a, scheduleFlashClear()
	}

	cursor := a.list.table.Cursor()
	forward := a.search.mode == SearchForward
	if !sameDirection {
		forward = !forward
	}

	target := a.search.nextMatch(cursor, forward)
	if target >= 0 {
		a.list.table.MoveCursor(target)
		a.search.matchIdx = target
		a.syncCursorPosition()
	}
	return a, nil
}

// searchUnderCursor searches for the name of the connection under the cursor.
func (a App) searchUnderCursor(mode SearchMode) (tea.Model, tea.Cmd) {
	c := a.list.selectedConnection()
	if c == nil {
		return a, nil
	}

	name := c.Name
	a.search.searchForName(name, mode)
	a.search.computeMatches(a.list.table.rows)
	a.list.table.highlightText = name
	a.header.setFilter(name)

	// Jump to next/previous match from current position
	cursor := a.list.table.Cursor()
	forward := mode == SearchForward
	target := a.search.nextMatch(cursor, forward)
	if target >= 0 && target != cursor {
		a.list.table.MoveCursor(target)
		a.search.matchIdx = target
	}
	a.syncCursorPosition()
	return a, nil
}
