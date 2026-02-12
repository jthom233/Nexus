package tui

import "testing"

func TestHistoryAddAndPrev(t *testing.T) {
	ce := NewCommandEngine()
	// Override history file to prevent file I/O in tests
	ce.historyFile = ""
	ce.history = nil

	ce.AddToHistory("connect server1")
	ce.AddToHistory("quit")
	ce.AddToHistory("help")

	// History should be [help, quit, connect server1] (most recent first)
	if len(ce.history) != 3 {
		t.Fatalf("expected 3 history entries, got %d", len(ce.history))
	}
	if ce.history[0] != "help" {
		t.Errorf("expected history[0]='help', got %q", ce.history[0])
	}
	if ce.history[1] != "quit" {
		t.Errorf("expected history[1]='quit', got %q", ce.history[1])
	}
	if ce.history[2] != "connect server1" {
		t.Errorf("expected history[2]='connect server1', got %q", ce.history[2])
	}
}

func TestHistoryDeduplication(t *testing.T) {
	ce := NewCommandEngine()
	ce.historyFile = ""
	ce.history = nil

	ce.AddToHistory("connect server1")
	ce.AddToHistory("quit")
	ce.AddToHistory("connect server1") // duplicate, should move to front

	if len(ce.history) != 2 {
		t.Fatalf("expected 2 history entries after dedup, got %d", len(ce.history))
	}
	if ce.history[0] != "connect server1" {
		t.Errorf("expected history[0]='connect server1', got %q", ce.history[0])
	}
	if ce.history[1] != "quit" {
		t.Errorf("expected history[1]='quit', got %q", ce.history[1])
	}
}

func TestHistoryPrevNext(t *testing.T) {
	ce := NewCommandEngine()
	ce.historyFile = ""
	ce.history = nil
	ce.AddToHistory("cmd1")
	ce.AddToHistory("cmd2")
	ce.AddToHistory("cmd3")
	// History: [cmd3, cmd2, cmd1]

	// Start with current input "typing"
	got := ce.HistoryPrev("typing")
	if got != "cmd3" {
		t.Errorf("HistoryPrev(1): expected 'cmd3', got %q", got)
	}

	got = ce.HistoryPrev("typing")
	if got != "cmd2" {
		t.Errorf("HistoryPrev(2): expected 'cmd2', got %q", got)
	}

	got = ce.HistoryPrev("typing")
	if got != "cmd1" {
		t.Errorf("HistoryPrev(3): expected 'cmd1', got %q", got)
	}

	// At oldest, should stay at cmd1
	got = ce.HistoryPrev("typing")
	if got != "cmd1" {
		t.Errorf("HistoryPrev(4): expected 'cmd1' (oldest), got %q", got)
	}

	// Now go forward
	got = ce.HistoryNext()
	if got != "cmd2" {
		t.Errorf("HistoryNext(1): expected 'cmd2', got %q", got)
	}

	got = ce.HistoryNext()
	if got != "cmd3" {
		t.Errorf("HistoryNext(2): expected 'cmd3', got %q", got)
	}

	got = ce.HistoryNext()
	if got != "typing" {
		t.Errorf("HistoryNext(3): expected saved input 'typing', got %q", got)
	}
}

func TestHistoryEmptyInput(t *testing.T) {
	ce := NewCommandEngine()
	ce.historyFile = ""
	ce.history = nil

	// Adding empty should be a no-op
	ce.AddToHistory("")
	ce.AddToHistory("   ")
	if len(ce.history) != 0 {
		t.Errorf("expected 0 history entries for blank input, got %d", len(ce.history))
	}
}

func TestHistoryPrevOnEmpty(t *testing.T) {
	ce := NewCommandEngine()
	ce.historyFile = ""
	ce.history = nil

	got := ce.HistoryPrev("current")
	if got != "current" {
		t.Errorf("HistoryPrev on empty should return current, got %q", got)
	}
}

func TestCompletionMatching(t *testing.T) {
	ce := NewCommandEngine()
	ce.historyFile = ""

	matches := ce.Complete("con")
	if len(matches) == 0 {
		t.Fatal("expected matches for 'con', got none")
	}
	found := false
	for _, m := range matches {
		if m == "connect" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'connect' in matches, got %v", matches)
	}
}

func TestCompletionEmpty(t *testing.T) {
	ce := NewCommandEngine()
	ce.historyFile = ""

	matches := ce.Complete("")
	if len(matches) != 0 {
		t.Errorf("expected no matches for empty input, got %v", matches)
	}
}

func TestCompletionNoMatch(t *testing.T) {
	ce := NewCommandEngine()
	ce.historyFile = ""

	matches := ce.Complete("zzz_nonexistent")
	if len(matches) != 0 {
		t.Errorf("expected no matches for 'zzz_nonexistent', got %v", matches)
	}
}

func TestCompletionCycling(t *testing.T) {
	ce := NewCommandEngine()
	ce.historyFile = ""

	// There should be multiple commands starting with "re" (recent, recordings)
	first := ce.NextCompletion("re")
	second := ce.NextCompletion(first)

	// If there are multiple candidates, they should cycle
	matches := ce.Complete("re")
	if len(matches) >= 2 {
		if first == second {
			t.Errorf("expected cycling to return different values, both got %q", first)
		}
	}
}

func TestAliasResolution(t *testing.T) {
	ce := NewCommandEngine()
	ce.historyFile = ""

	ce.SetAlias("c", "connect")
	ce.SetAlias("ss", "sessions")

	got := ce.ResolveAlias("c server1")
	if got != "connect server1" {
		t.Errorf("expected 'connect server1', got %q", got)
	}

	got = ce.ResolveAlias("ss")
	if got != "sessions" {
		t.Errorf("expected 'sessions', got %q", got)
	}

	// Non-alias should pass through
	got = ce.ResolveAlias("help")
	if got != "help" {
		t.Errorf("expected 'help', got %q", got)
	}
}

func TestBuiltinAliasResolution(t *testing.T) {
	ce := NewCommandEngine()
	ce.historyFile = ""

	got := ce.ResolveBuiltinAlias("q")
	if got != "quit" {
		t.Errorf("expected 'quit' for alias 'q', got %q", got)
	}

	got = ce.ResolveBuiltinAlias("fav")
	if got != "favorites" {
		t.Errorf("expected 'favorites' for alias 'fav', got %q", got)
	}

	got = ce.ResolveBuiltinAlias("connect")
	if got != "connect" {
		t.Errorf("expected 'connect' unchanged, got %q", got)
	}
}

func TestRangeParsingValid(t *testing.T) {
	start, end, rest := ParseRange("1,10 delete")
	if start != 0 || end != 9 {
		t.Errorf("expected range 0-9, got %d-%d", start, end)
	}
	if rest != "delete" {
		t.Errorf("expected rest='delete', got %q", rest)
	}
}

func TestRangeParsingWithSpaces(t *testing.T) {
	start, end, rest := ParseRange("5 , 20 tag add prod")
	if start != 4 || end != 19 {
		t.Errorf("expected range 4-19, got %d-%d", start, end)
	}
	if rest != "tag add prod" {
		t.Errorf("expected rest='tag add prod', got %q", rest)
	}
}

func TestRangeParsingNoRange(t *testing.T) {
	start, end, rest := ParseRange("delete server1")
	if start != -1 || end != -1 {
		t.Errorf("expected no range (-1,-1), got %d,%d", start, end)
	}
	if rest != "delete server1" {
		t.Errorf("expected rest='delete server1', got %q", rest)
	}
}

func TestRangeParsingEmpty(t *testing.T) {
	start, end, rest := ParseRange("")
	if start != -1 || end != -1 {
		t.Errorf("expected no range, got %d,%d", start, end)
	}
	if rest != "" {
		t.Errorf("expected empty rest, got %q", rest)
	}
}

func TestFindCommand(t *testing.T) {
	ce := NewCommandEngine()
	ce.historyFile = ""

	def := ce.FindCommand("connect")
	if def == nil {
		t.Fatal("expected to find 'connect' command")
	}
	if def.Name != "connect" {
		t.Errorf("expected Name='connect', got %q", def.Name)
	}

	// Find by alias
	def = ce.FindCommand("dc")
	if def == nil {
		t.Fatal("expected to find 'dc' alias")
	}
	if def.Name != "disconnect" {
		t.Errorf("expected Name='disconnect' for alias 'dc', got %q", def.Name)
	}

	// Not found
	def = ce.FindCommand("nonexistent")
	if def != nil {
		t.Errorf("expected nil for nonexistent command, got %+v", def)
	}
}

func TestHistoryCap(t *testing.T) {
	ce := NewCommandEngine()
	ce.historyFile = ""
	ce.history = nil

	for i := 0; i < 600; i++ {
		ce.AddToHistory("cmd" + string(rune('A'+i%26)) + string(rune('0'+i%10)))
	}
	if len(ce.history) > maxHistoryEntries {
		t.Errorf("history exceeded max: got %d, max %d", len(ce.history), maxHistoryEntries)
	}
}

func TestAllNamesIncludesAliases(t *testing.T) {
	ce := NewCommandEngine()
	ce.historyFile = ""
	ce.SetAlias("c", "connect")

	names := ce.AllNames()
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}

	// Should include canonical names
	if !found["connect"] {
		t.Error("expected 'connect' in AllNames")
	}
	// Should include built-in aliases
	if !found["q"] {
		t.Error("expected 'q' (alias for quit) in AllNames")
	}
	// Should include user aliases
	if !found["c"] {
		t.Error("expected 'c' (user alias) in AllNames")
	}
}
