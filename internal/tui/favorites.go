package tui

import (
	"sort"

	"github.com/dr4zz/nexus/internal/config"
)

// FilterFavorites returns only connections marked as favorites.
func FilterFavorites(conns []config.Connection) []config.Connection {
	var result []config.Connection
	for _, c := range conns {
		if c.Favorite {
			result = append(result, c)
		}
	}
	return result
}

// SortByRecent returns a copy of connections sorted by LastConnectedAt descending.
// Connections without a LastConnectedAt are placed at the end.
func SortByRecent(conns []config.Connection) []config.Connection {
	result := make([]config.Connection, len(conns))
	copy(result, conns)
	sort.SliceStable(result, func(i, j int) bool {
		a := result[i].LastConnectedAt
		b := result[j].LastConnectedAt
		if a == nil && b == nil {
			return false
		}
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		return a.After(*b)
	})
	return result
}

// SortByFrequent returns a copy of connections sorted by ConnectCount descending.
func SortByFrequent(conns []config.Connection) []config.Connection {
	result := make([]config.Connection, len(conns))
	copy(result, conns)
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].ConnectCount > result[j].ConnectCount
	})
	return result
}

// PinFavorites returns a copy with favorites pinned to the top, preserving
// relative order within each group.
func PinFavorites(conns []config.Connection) []config.Connection {
	var favs, rest []config.Connection
	for _, c := range conns {
		if c.Favorite {
			favs = append(favs, c)
		} else {
			rest = append(rest, c)
		}
	}
	return append(favs, rest...)
}
