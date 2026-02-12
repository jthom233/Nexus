package tui

import (
	"testing"
	"time"

	"github.com/dr4zz/nexus/internal/config"
)

func testConnections() []config.Connection {
	t1 := time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	t3 := time.Date(2025, 1, 5, 12, 0, 0, 0, time.UTC)
	return []config.Connection{
		{ID: "a", Name: "alpha", Favorite: true, LastConnectedAt: &t1, ConnectCount: 5},
		{ID: "b", Name: "bravo", Favorite: false, LastConnectedAt: &t2, ConnectCount: 10},
		{ID: "c", Name: "charlie", Favorite: true, LastConnectedAt: &t3, ConnectCount: 1},
		{ID: "d", Name: "delta", Favorite: false, LastConnectedAt: nil, ConnectCount: 0},
	}
}

func TestFilterFavorites(t *testing.T) {
	conns := testConnections()
	result := FilterFavorites(conns)

	if len(result) != 2 {
		t.Fatalf("expected 2 favorites, got %d", len(result))
	}
	if result[0].ID != "a" {
		t.Errorf("expected first favorite to be 'a', got %q", result[0].ID)
	}
	if result[1].ID != "c" {
		t.Errorf("expected second favorite to be 'c', got %q", result[1].ID)
	}
}

func TestFilterFavoritesEmpty(t *testing.T) {
	conns := []config.Connection{
		{ID: "x", Name: "xray", Favorite: false},
		{ID: "y", Name: "yankee", Favorite: false},
	}
	result := FilterFavorites(conns)
	if len(result) != 0 {
		t.Fatalf("expected 0 favorites, got %d", len(result))
	}
}

func TestSortByRecent(t *testing.T) {
	conns := testConnections()
	result := SortByRecent(conns)

	if len(result) != 4 {
		t.Fatalf("expected 4 connections, got %d", len(result))
	}

	// Most recent first: bravo (Jan 15), alpha (Jan 10), charlie (Jan 5), delta (nil)
	expected := []string{"b", "a", "c", "d"}
	for i, id := range expected {
		if result[i].ID != id {
			t.Errorf("position %d: expected %q, got %q", i, id, result[i].ID)
		}
	}
}

func TestSortByRecentAllNil(t *testing.T) {
	conns := []config.Connection{
		{ID: "x", Name: "xray"},
		{ID: "y", Name: "yankee"},
	}
	result := SortByRecent(conns)
	// Order should be preserved (stable sort, all nil)
	if result[0].ID != "x" || result[1].ID != "y" {
		t.Errorf("expected stable order [x,y], got [%s,%s]", result[0].ID, result[1].ID)
	}
}

func TestSortByFrequent(t *testing.T) {
	conns := testConnections()
	result := SortByFrequent(conns)

	if len(result) != 4 {
		t.Fatalf("expected 4 connections, got %d", len(result))
	}

	// Most frequent first: bravo (10), alpha (5), charlie (1), delta (0)
	expected := []string{"b", "a", "c", "d"}
	for i, id := range expected {
		if result[i].ID != id {
			t.Errorf("position %d: expected %q, got %q", i, id, result[i].ID)
		}
	}
}

func TestSortByFrequentAllZero(t *testing.T) {
	conns := []config.Connection{
		{ID: "x", Name: "xray", ConnectCount: 0},
		{ID: "y", Name: "yankee", ConnectCount: 0},
	}
	result := SortByFrequent(conns)
	// Order should be preserved (stable sort, all zero)
	if result[0].ID != "x" || result[1].ID != "y" {
		t.Errorf("expected stable order [x,y], got [%s,%s]", result[0].ID, result[1].ID)
	}
}

func TestPinFavorites(t *testing.T) {
	conns := testConnections()
	result := PinFavorites(conns)

	if len(result) != 4 {
		t.Fatalf("expected 4 connections, got %d", len(result))
	}

	// Favorites first (a, c), then non-favorites (b, d)
	expected := []string{"a", "c", "b", "d"}
	for i, id := range expected {
		if result[i].ID != id {
			t.Errorf("position %d: expected %q, got %q", i, id, result[i].ID)
		}
	}
}

func TestPinFavoritesNoFavorites(t *testing.T) {
	conns := []config.Connection{
		{ID: "x", Name: "xray", Favorite: false},
		{ID: "y", Name: "yankee", Favorite: false},
	}
	result := PinFavorites(conns)
	// No favorites, order unchanged
	if result[0].ID != "x" || result[1].ID != "y" {
		t.Errorf("expected [x,y], got [%s,%s]", result[0].ID, result[1].ID)
	}
}

func TestSortByRecentDoesNotMutateInput(t *testing.T) {
	conns := testConnections()
	origFirst := conns[0].ID
	_ = SortByRecent(conns)
	if conns[0].ID != origFirst {
		t.Error("SortByRecent mutated the input slice")
	}
}

func TestSortByFrequentDoesNotMutateInput(t *testing.T) {
	conns := testConnections()
	origFirst := conns[0].ID
	_ = SortByFrequent(conns)
	if conns[0].ID != origFirst {
		t.Error("SortByFrequent mutated the input slice")
	}
}

func TestFavoriteToggle(t *testing.T) {
	conns := testConnections()
	// Toggle alpha (currently favorite) to non-favorite
	conns[0].Favorite = !conns[0].Favorite
	if conns[0].Favorite {
		t.Error("expected alpha to be unfavorited after toggle")
	}
	// Toggle delta (currently not favorite) to favorite
	conns[3].Favorite = !conns[3].Favorite
	if !conns[3].Favorite {
		t.Error("expected delta to be favorited after toggle")
	}

	// Verify filter reflects the change
	favs := FilterFavorites(conns)
	if len(favs) != 2 {
		t.Fatalf("expected 2 favorites after toggle, got %d", len(favs))
	}
	// Should be charlie and delta now
	ids := map[string]bool{}
	for _, f := range favs {
		ids[f.ID] = true
	}
	if !ids["c"] || !ids["d"] {
		t.Errorf("expected favorites to be [c,d], got %v", ids)
	}
}
