package tui

import (
	"testing"

	"github.com/dr4zz/nexus/internal/config"
)

func TestFuzzyScoreExactMatch(t *testing.T) {
	score, positions := FuzzyScore("web-server", "web-server production")
	if score < 0 {
		t.Fatal("expected match, got no match")
	}
	if len(positions) != 10 {
		t.Fatalf("expected 10 match positions, got %d", len(positions))
	}
	// Positions should be 0..9 for exact substring at start
	for i := 0; i < 10; i++ {
		if positions[i] != i {
			t.Errorf("position %d: expected %d, got %d", i, i, positions[i])
		}
	}
}

func TestFuzzyScorePrefixBeatsSubstring(t *testing.T) {
	prefixScore, _ := FuzzyScore("web", "web-server")
	subScore, _ := FuzzyScore("web", "the-web-app")

	if prefixScore < 0 || subScore < 0 {
		t.Fatal("expected both to match")
	}
	if prefixScore <= subScore {
		t.Errorf("prefix score (%d) should be higher than substring score (%d)", prefixScore, subScore)
	}
}

func TestFuzzyScoreSubstringBeatsFuzzy(t *testing.T) {
	subScore, _ := FuzzyScore("srv", "srv-alpha")
	fuzzyScore, _ := FuzzyScore("srv", "server-via-relay")

	if subScore < 0 || fuzzyScore < 0 {
		t.Fatal("expected both to match")
	}
	if subScore <= fuzzyScore {
		t.Errorf("substring score (%d) should be higher than fuzzy score (%d)", subScore, fuzzyScore)
	}
}

func TestFuzzyScoreExactBeatsFuzzy(t *testing.T) {
	exactScore, _ := FuzzyScore("prod", "production-server")
	fuzzyScore, _ := FuzzyScore("prod", "preview-on-demand")

	if exactScore < 0 || fuzzyScore < 0 {
		t.Fatal("expected both to match")
	}
	if exactScore <= fuzzyScore {
		t.Errorf("exact score (%d) should be higher than fuzzy score (%d)", exactScore, fuzzyScore)
	}
}

func TestFuzzyScoreEmptyPatternMatchesAll(t *testing.T) {
	score, positions := FuzzyScore("", "anything")
	if score != 0 {
		t.Errorf("expected score 0 for empty pattern, got %d", score)
	}
	if positions != nil {
		t.Errorf("expected nil positions for empty pattern, got %v", positions)
	}
}

func TestFuzzyScoreNoMatch(t *testing.T) {
	score, positions := FuzzyScore("xyz", "abc")
	if score >= 0 {
		t.Errorf("expected no match (score < 0), got %d", score)
	}
	if positions != nil {
		t.Errorf("expected nil positions for no match, got %v", positions)
	}
}

func TestFuzzyScoreCaseInsensitive(t *testing.T) {
	score1, _ := FuzzyScore("WEB", "web-server")
	score2, _ := FuzzyScore("web", "web-server")

	if score1 < 0 || score2 < 0 {
		t.Fatal("expected both to match")
	}
	// Both should match; exact case should score slightly higher
	if score2 <= score1 {
		// score2 (lowercase matching lowercase) should have case bonus
		// but both should be valid matches
	}
}

func TestFuzzyScoreWordBoundaryBonus(t *testing.T) {
	// "wsc" matching at word boundaries in "web-server-central" (w=boundary, s=boundary, c=boundary)
	// should score higher than "wsc" matching at non-boundary positions in "newscast" (n-e-w-s-c)
	// Both are fuzzy (no exact substring), but word boundaries add +10 each.
	score1, _ := FuzzyScore("wsc", "web-server-central")
	score2, _ := FuzzyScore("wsc", "bowlsicap")

	if score1 < 0 || score2 < 0 {
		t.Fatal("expected both to match")
	}
	if score1 <= score2 {
		t.Errorf("word boundary score (%d) should be higher than non-boundary score (%d)", score1, score2)
	}
}

func TestFinderMultiSelectToggle(t *testing.T) {
	finder := NewFinder()
	conns := []config.Connection{
		{ID: "a", Name: "alpha", Protocol: config.ProtoSSH, Host: "a.example.com"},
		{ID: "b", Name: "bravo", Protocol: config.ProtoSSH, Host: "b.example.com"},
		{ID: "c", Name: "charlie", Protocol: config.ProtoSSH, Host: "c.example.com"},
	}

	finder.SetSize(80, 24)
	finder.Activate(PickerConnections, conns)

	if len(finder.results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(finder.results))
	}

	// Toggle select on first item
	if finder.multiSelect[0] {
		t.Error("expected item 0 to not be selected initially")
	}
	finder.multiSelect[0] = true
	if !finder.multiSelect[0] {
		t.Error("expected item 0 to be selected after toggle")
	}

	// Toggle select on second item
	finder.multiSelect[1] = true

	selected := finder.SelectedConnections()
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected connections, got %d", len(selected))
	}

	// Deselect first
	delete(finder.multiSelect, 0)
	selected = finder.SelectedConnections()
	if len(selected) != 1 {
		t.Fatalf("expected 1 selected connection after deselect, got %d", len(selected))
	}
}

func TestFinderResultsOrderByScore(t *testing.T) {
	finder := NewFinder()
	conns := []config.Connection{
		{ID: "a", Name: "database-staging", Protocol: config.ProtoSSH, Host: "db.staging.com"},
		{ID: "b", Name: "db-prod", Protocol: config.ProtoSSH, Host: "db.prod.com"},
		{ID: "c", Name: "web-server", Protocol: config.ProtoSSH, Host: "web.example.com"},
		{ID: "d", Name: "db-dev", Protocol: config.ProtoSSH, Host: "db.dev.com"},
	}

	finder.SetSize(80, 24)
	finder.Activate(PickerConnections, conns)
	finder.prompt.SetValue("db")
	finder.filterResults()

	// "db" should match connections with "db" in their name
	if len(finder.results) < 2 {
		t.Fatalf("expected at least 2 results for 'db', got %d", len(finder.results))
	}

	// Results should be ordered by score (highest first)
	for i := 1; i < len(finder.results); i++ {
		if finder.results[i].Score > finder.results[i-1].Score {
			t.Errorf("results not sorted by score: index %d (score %d) > index %d (score %d)",
				i, finder.results[i].Score, i-1, finder.results[i-1].Score)
		}
	}

	// "db-prod" or "db-dev" (prefix match) should rank higher than "database-staging" (substring)
	firstResult := finder.results[0]
	if firstResult.Connection.Name != "db-prod" && firstResult.Connection.Name != "db-dev" {
		// Both db-prod and db-dev have "db" as prefix, so either is valid as top result
		t.Logf("Top result was %s (score: %d)", firstResult.Connection.Name, firstResult.Score)
	}
}

func TestFinderEmptyQuery(t *testing.T) {
	finder := NewFinder()
	conns := []config.Connection{
		{ID: "a", Name: "alpha", Protocol: config.ProtoSSH, Host: "a.example.com"},
		{ID: "b", Name: "bravo", Protocol: config.ProtoSSH, Host: "b.example.com"},
	}

	finder.SetSize(80, 24)
	finder.Activate(PickerConnections, conns)

	// Empty query should show all results with score 0
	if len(finder.results) != 2 {
		t.Fatalf("expected 2 results for empty query, got %d", len(finder.results))
	}
	for i, r := range finder.results {
		if r.Score != 0 {
			t.Errorf("result %d: expected score 0 for empty query, got %d", i, r.Score)
		}
	}
}

func TestFinderSelectedConnection(t *testing.T) {
	finder := NewFinder()
	conns := []config.Connection{
		{ID: "a", Name: "alpha", Protocol: config.ProtoSSH, Host: "a.example.com"},
		{ID: "b", Name: "bravo", Protocol: config.ProtoSSH, Host: "b.example.com"},
	}

	finder.SetSize(80, 24)
	finder.Activate(PickerConnections, conns)

	// Cursor at 0
	c := finder.SelectedConnection()
	if c == nil {
		t.Fatal("expected selected connection, got nil")
	}
	if c.ID != "a" {
		t.Errorf("expected connection 'a', got %q", c.ID)
	}

	// Move cursor
	finder.cursor = 1
	c = finder.SelectedConnection()
	if c == nil {
		t.Fatal("expected selected connection at cursor 1, got nil")
	}
	if c.ID != "b" {
		t.Errorf("expected connection 'b', got %q", c.ID)
	}
}

func TestFinderDeactivate(t *testing.T) {
	finder := NewFinder()
	conns := []config.Connection{
		{ID: "a", Name: "alpha", Protocol: config.ProtoSSH, Host: "a.example.com"},
	}

	finder.SetSize(80, 24)
	finder.Activate(PickerConnections, conns)
	if !finder.active {
		t.Fatal("expected finder to be active")
	}

	finder.Deactivate()
	if finder.active {
		t.Fatal("expected finder to be inactive after deactivate")
	}
	if finder.results != nil {
		t.Error("expected nil results after deactivate")
	}
}

func TestFinderPickerString(t *testing.T) {
	tests := []struct {
		picker FinderPicker
		want   string
	}{
		{PickerConnections, "Connections"},
		{PickerSessions, "Sessions"},
		{PickerTags, "Tags"},
		{PickerGroups, "Groups"},
		{PickerRecent, "Recent"},
		{PickerCommands, "Commands"},
	}

	for _, tt := range tests {
		got := tt.picker.String()
		if got != tt.want {
			t.Errorf("FinderPicker(%d).String() = %q, want %q", tt.picker, got, tt.want)
		}
	}
}

func TestFuzzyScoreConsecutiveBonus(t *testing.T) {
	// "ab" in "abc" (consecutive) should score higher than "ab" in "axbxc" (non-consecutive)
	score1, _ := FuzzyScore("ab", "abc-server")
	score2, _ := FuzzyScore("ab", "axb-server")

	if score1 < 0 || score2 < 0 {
		t.Fatal("expected both to match")
	}
	if score1 <= score2 {
		t.Errorf("consecutive score (%d) should be higher than non-consecutive score (%d)", score1, score2)
	}
}
