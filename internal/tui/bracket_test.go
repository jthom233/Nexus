package tui

import (
	"testing"

	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/health"
)

func testConnections() []config.Connection {
	return []config.Connection{
		{ID: "1", Name: "web-1", Group: "web", Favorite: true},
		{ID: "2", Name: "web-2", Group: "web", Favorite: false},
		{ID: "3", Name: "db-1", Group: "database", Favorite: false},
		{ID: "4", Name: "db-2", Group: "database", Favorite: true},
		{ID: "5", Name: "cache-1", Group: "cache", Favorite: false},
		{ID: "6", Name: "cache-2", Group: "cache", Favorite: false},
		{ID: "7", Name: "api-1", Group: "api", Favorite: true},
	}
}

func testStatuses() map[string]health.Status {
	return map[string]health.Status{
		"1": health.Online,
		"2": health.Online,
		"3": health.Offline,
		"4": health.Online,
		"5": health.Offline,
		"6": health.Online,
		"7": health.Offline,
	}
}

func TestNextConnection(t *testing.T) {
	b := NewBracketNav()
	conns := testConnections()

	tests := []struct {
		name   string
		cursor int
		want   int
	}{
		{"move from 0 to 1", 0, 1},
		{"move from 3 to 4", 3, 4},
		{"at last stays", 6, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.NextConnection(tt.cursor, conns)
			if got != tt.want {
				t.Errorf("NextConnection(%d) = %d, want %d", tt.cursor, got, tt.want)
			}
		})
	}
}

func TestPrevConnection(t *testing.T) {
	b := NewBracketNav()
	conns := testConnections()

	tests := []struct {
		name   string
		cursor int
		want   int
	}{
		{"move from 3 to 2", 3, 2},
		{"move from 1 to 0", 1, 0},
		{"at first stays", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.PrevConnection(tt.cursor, conns)
			if got != tt.want {
				t.Errorf("PrevConnection(%d) = %d, want %d", tt.cursor, got, tt.want)
			}
		})
	}
}

func TestNextGroup(t *testing.T) {
	b := NewBracketNav()
	conns := testConnections()

	tests := []struct {
		name   string
		cursor int
		want   int
	}{
		{"from web group to database", 0, 2},
		{"from second web to database", 1, 2},
		{"from database to cache", 2, 4},
		{"from cache to api", 4, 6},
		{"at last group stays", 6, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.NextGroup(tt.cursor, conns)
			if got != tt.want {
				t.Errorf("NextGroup(%d) = %d, want %d", tt.cursor, got, tt.want)
			}
		})
	}
}

func TestPrevGroup(t *testing.T) {
	b := NewBracketNav()
	conns := testConnections()

	tests := []struct {
		name   string
		cursor int
		want   int
	}{
		{"from database to web", 2, 0},
		{"from second database to web", 3, 0},
		{"from cache to database", 4, 2},
		{"from api to cache", 6, 4},
		{"at first group stays", 0, 0},
		{"from second item in first group stays", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.PrevGroup(tt.cursor, conns)
			if got != tt.want {
				t.Errorf("PrevGroup(%d) = %d, want %d", tt.cursor, got, tt.want)
			}
		})
	}
}

func TestNextError(t *testing.T) {
	b := NewBracketNav()
	conns := testConnections()
	statuses := testStatuses()

	tests := []struct {
		name   string
		cursor int
		want   int
	}{
		{"from 0 finds db-1 offline at 2", 0, 2},
		{"from 2 finds cache-1 offline at 4", 2, 4},
		{"from 4 finds api-1 offline at 6", 4, 6},
		{"from 6 no more errors stays", 6, 6},
		{"skips online connections", 1, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.NextError(tt.cursor, conns, statuses)
			if got != tt.want {
				t.Errorf("NextError(%d) = %d, want %d", tt.cursor, got, tt.want)
			}
		})
	}
}

func TestPrevError(t *testing.T) {
	b := NewBracketNav()
	conns := testConnections()
	statuses := testStatuses()

	tests := []struct {
		name   string
		cursor int
		want   int
	}{
		{"from 6 finds cache-1 offline at 4", 6, 4},
		{"from 4 finds db-1 offline at 2", 4, 2},
		{"from 2 no prev errors stays", 2, 2},
		{"from 0 no prev errors stays", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.PrevError(tt.cursor, conns, statuses)
			if got != tt.want {
				t.Errorf("PrevError(%d) = %d, want %d", tt.cursor, got, tt.want)
			}
		})
	}
}

func TestNextFavorite(t *testing.T) {
	b := NewBracketNav()
	conns := testConnections()
	// Favorites: index 0 (web-1), 3 (db-2), 6 (api-1)

	tests := []struct {
		name   string
		cursor int
		want   int
	}{
		{"from 0 finds db-2 at 3", 0, 3},
		{"from 1 finds db-2 at 3", 1, 3},
		{"from 3 finds api-1 at 6", 3, 6},
		{"from 6 no more favorites stays", 6, 6},
		{"from 5 finds api-1 at 6", 5, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.NextFavorite(tt.cursor, conns)
			if got != tt.want {
				t.Errorf("NextFavorite(%d) = %d, want %d", tt.cursor, got, tt.want)
			}
		})
	}
}

func TestPrevFavorite(t *testing.T) {
	b := NewBracketNav()
	conns := testConnections()
	// Favorites: index 0 (web-1), 3 (db-2), 6 (api-1)

	tests := []struct {
		name   string
		cursor int
		want   int
	}{
		{"from 6 finds db-2 at 3", 6, 3},
		{"from 5 finds db-2 at 3", 5, 3},
		{"from 3 finds web-1 at 0", 3, 0},
		{"from 0 no prev favorites stays", 0, 0},
		{"from 2 finds web-1 at 0", 2, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.PrevFavorite(tt.cursor, conns)
			if got != tt.want {
				t.Errorf("PrevFavorite(%d) = %d, want %d", tt.cursor, got, tt.want)
			}
		})
	}
}

func TestBracketNavEmptyConnections(t *testing.T) {
	b := NewBracketNav()
	var empty []config.Connection
	emptyStatuses := map[string]health.Status{}

	if got := b.NextConnection(0, empty); got != 0 {
		t.Errorf("NextConnection on empty = %d, want 0", got)
	}
	if got := b.PrevConnection(0, empty); got != 0 {
		t.Errorf("PrevConnection on empty = %d, want 0", got)
	}
	if got := b.NextGroup(0, empty); got != 0 {
		t.Errorf("NextGroup on empty = %d, want 0", got)
	}
	if got := b.PrevGroup(0, empty); got != 0 {
		t.Errorf("PrevGroup on empty = %d, want 0", got)
	}
	if got := b.NextError(0, empty, emptyStatuses); got != 0 {
		t.Errorf("NextError on empty = %d, want 0", got)
	}
	if got := b.PrevError(0, empty, emptyStatuses); got != 0 {
		t.Errorf("PrevError on empty = %d, want 0", got)
	}
	if got := b.NextFavorite(0, empty); got != 0 {
		t.Errorf("NextFavorite on empty = %d, want 0", got)
	}
	if got := b.PrevFavorite(0, empty); got != 0 {
		t.Errorf("PrevFavorite on empty = %d, want 0", got)
	}
}

func TestNextGroupSingleGroup(t *testing.T) {
	b := NewBracketNav()
	conns := []config.Connection{
		{ID: "1", Name: "a", Group: "same"},
		{ID: "2", Name: "b", Group: "same"},
		{ID: "3", Name: "c", Group: "same"},
	}

	// No next group exists — should stay at cursor.
	if got := b.NextGroup(0, conns); got != 0 {
		t.Errorf("NextGroup single group from 0 = %d, want 0", got)
	}
	if got := b.NextGroup(2, conns); got != 2 {
		t.Errorf("NextGroup single group from 2 = %d, want 2", got)
	}
}

func TestPrevGroupSingleGroup(t *testing.T) {
	b := NewBracketNav()
	conns := []config.Connection{
		{ID: "1", Name: "a", Group: "same"},
		{ID: "2", Name: "b", Group: "same"},
	}

	if got := b.PrevGroup(0, conns); got != 0 {
		t.Errorf("PrevGroup single group from 0 = %d, want 0", got)
	}
	if got := b.PrevGroup(1, conns); got != 1 {
		t.Errorf("PrevGroup single group from 1 = %d, want 1", got)
	}
}

func TestTagFilterParsing(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantOp  TagFilterOp
		wantLen int
	}{
		{"single tag", "web", TagFilterAnd, 1},
		{"AND operator", "web AND production", TagFilterAnd, 2},
		{"OR operator", "web OR api", TagFilterOr, 2},
		{"multiple AND", "web AND production AND critical", TagFilterAnd, 3},
		{"multiple OR", "web OR api OR cache", TagFilterOr, 3},
		{"empty", "", TagFilterAnd, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTagFilter(tt.expr)
			if got.Op != tt.wantOp {
				t.Errorf("ParseTagFilter(%q).Op = %d, want %d", tt.expr, got.Op, tt.wantOp)
			}
			if len(got.Tags) != tt.wantLen {
				t.Errorf("ParseTagFilter(%q) has %d tags, want %d", tt.expr, len(got.Tags), tt.wantLen)
			}
		})
	}
}

func TestTagFilterMatch(t *testing.T) {
	conn := config.Connection{
		Tags: []string{"web", "production", "critical"},
	}

	tests := []struct {
		name string
		expr string
		want bool
	}{
		{"single match", "web", true},
		{"single no match", "database", false},
		{"AND all present", "web AND production", true},
		{"AND one missing", "web AND staging", false},
		{"OR one present", "web OR database", true},
		{"OR none present", "database OR cache", false},
		{"empty matches all", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := ParseTagFilter(tt.expr)
			got := filter.Match(conn)
			if got != tt.want {
				t.Errorf("TagFilter(%q).Match() = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}
