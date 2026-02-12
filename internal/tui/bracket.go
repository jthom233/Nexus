package tui

import (
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/health"
	"github.com/dr4zz/nexus/internal/session"
)

// BracketNav implements bracket-style navigation motions ([x / ]x) that jump
// between connections matching specific criteria (group boundaries, errors,
// active sessions, favorites).
type BracketNav struct{}

// NewBracketNav creates a new BracketNav instance.
func NewBracketNav() *BracketNav {
	return &BracketNav{}
}

// NextConnection returns cursor+1, clamped to bounds. (]c)
func (b *BracketNav) NextConnection(cursor int, connections []config.Connection) int {
	if len(connections) == 0 {
		return 0
	}
	next := cursor + 1
	if next >= len(connections) {
		return cursor
	}
	return next
}

// PrevConnection returns cursor-1, clamped to bounds. ([c)
func (b *BracketNav) PrevConnection(cursor int, connections []config.Connection) int {
	if len(connections) == 0 {
		return 0
	}
	prev := cursor - 1
	if prev < 0 {
		return cursor
	}
	return prev
}

// NextGroup jumps to the first item of the next group. (]g)
// Groups are determined by contiguous runs of the same Group field.
// Returns cursor unchanged if no next group exists.
func (b *BracketNav) NextGroup(cursor int, connections []config.Connection) int {
	if len(connections) == 0 || cursor < 0 || cursor >= len(connections) {
		return cursor
	}
	currentGroup := connections[cursor].Group
	// Skip past remaining items in the current group.
	i := cursor + 1
	for i < len(connections) && connections[i].Group == currentGroup {
		i++
	}
	// i is now at the first item of the next group, or past the end.
	if i >= len(connections) {
		return cursor // no next group
	}
	return i
}

// PrevGroup jumps to the first item of the previous group. ([g)
// Returns cursor unchanged if no previous group exists.
func (b *BracketNav) PrevGroup(cursor int, connections []config.Connection) int {
	if len(connections) == 0 || cursor < 0 || cursor >= len(connections) {
		return cursor
	}
	if cursor == 0 {
		return cursor
	}
	// Find the start of the current group.
	currentGroup := connections[cursor].Group
	start := cursor
	for start > 0 && connections[start-1].Group == currentGroup {
		start--
	}
	if start == 0 && connections[0].Group == currentGroup {
		// We're already at the beginning of the first group.
		return cursor
	}
	// start > 0 means connections[start-1] is the last item of the previous group.
	// Now find the beginning of that group.
	if start > 0 {
		prevGroup := connections[start-1].Group
		begin := start - 1
		for begin > 0 && connections[begin-1].Group == prevGroup {
			begin--
		}
		return begin
	}
	return cursor
}

// NextError jumps to the next offline connection. (]e)
// statuses maps connection ID to health status.
// Returns cursor unchanged if no offline connection is found after the cursor.
func (b *BracketNav) NextError(cursor int, connections []config.Connection, statuses map[string]health.Status) int {
	if len(connections) == 0 {
		return cursor
	}
	for i := cursor + 1; i < len(connections); i++ {
		if statuses[connections[i].ID] == health.Offline {
			return i
		}
	}
	return cursor // no match found
}

// PrevError jumps to the previous offline connection. ([e)
// Returns cursor unchanged if no offline connection is found before the cursor.
func (b *BracketNav) PrevError(cursor int, connections []config.Connection, statuses map[string]health.Status) int {
	if len(connections) == 0 {
		return cursor
	}
	for i := cursor - 1; i >= 0; i-- {
		if statuses[connections[i].ID] == health.Offline {
			return i
		}
	}
	return cursor // no match found
}

// NextSession jumps to the next connection that has an active session. (]s)
// Returns cursor unchanged if no match is found.
func (b *BracketNav) NextSession(cursor int, connections []config.Connection, sessions []*session.ManagedSession) int {
	if len(connections) == 0 {
		return cursor
	}
	activeIDs := sessionConnIDs(sessions)
	for i := cursor + 1; i < len(connections); i++ {
		if activeIDs[connections[i].ID] {
			return i
		}
	}
	return cursor
}

// PrevSession jumps to the previous connection that has an active session. ([s)
// Returns cursor unchanged if no match is found.
func (b *BracketNav) PrevSession(cursor int, connections []config.Connection, sessions []*session.ManagedSession) int {
	if len(connections) == 0 {
		return cursor
	}
	activeIDs := sessionConnIDs(sessions)
	for i := cursor - 1; i >= 0; i-- {
		if activeIDs[connections[i].ID] {
			return i
		}
	}
	return cursor
}

// NextFavorite jumps to the next favorited connection. (]f)
// Returns cursor unchanged if no match is found.
func (b *BracketNav) NextFavorite(cursor int, connections []config.Connection) int {
	if len(connections) == 0 {
		return cursor
	}
	for i := cursor + 1; i < len(connections); i++ {
		if connections[i].Favorite {
			return i
		}
	}
	return cursor
}

// PrevFavorite jumps to the previous favorited connection. ([f)
// Returns cursor unchanged if no match is found.
func (b *BracketNav) PrevFavorite(cursor int, connections []config.Connection) int {
	if len(connections) == 0 {
		return cursor
	}
	for i := cursor - 1; i >= 0; i-- {
		if connections[i].Favorite {
			return i
		}
	}
	return cursor
}

// sessionConnIDs builds a set of connection IDs that have active sessions.
func sessionConnIDs(sessions []*session.ManagedSession) map[string]bool {
	ids := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		ids[s.ConnID] = true
	}
	return ids
}
