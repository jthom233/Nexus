package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dr4zz/nexus/internal/config"
)

// TagManager provides tag manipulation and query operations on connections.
type TagManager struct{}

// NewTagManager creates a new TagManager.
func NewTagManager() *TagManager {
	return &TagManager{}
}

// AddTag adds a tag to a connection if not already present.
// Returns true if the tag was added, false if it already existed.
func (tm *TagManager) AddTag(conn *config.Connection, tag string) bool {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return false
	}
	for _, t := range conn.Tags {
		if strings.EqualFold(t, tag) {
			return false
		}
	}
	conn.Tags = append(conn.Tags, tag)
	return true
}

// RemoveTag removes a tag from a connection.
// Returns true if the tag was removed, false if it was not found.
func (tm *TagManager) RemoveTag(conn *config.Connection, tag string) bool {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return false
	}
	for i, t := range conn.Tags {
		if strings.EqualFold(t, tag) {
			conn.Tags = append(conn.Tags[:i], conn.Tags[i+1:]...)
			return true
		}
	}
	return false
}

// AllTags returns all unique tags across the given connections with their counts,
// sorted alphabetically.
func (tm *TagManager) AllTags(connections []config.Connection) []TagInfo {
	counts := make(map[string]int)
	for _, c := range connections {
		for _, t := range c.Tags {
			t = strings.TrimSpace(t)
			if t != "" {
				counts[strings.ToLower(t)]++
			}
		}
	}

	result := make([]TagInfo, 0, len(counts))
	for tag, count := range counts {
		result = append(result, TagInfo{Name: tag, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// TagInfo holds a tag name and the count of connections with that tag.
type TagInfo struct {
	Name  string
	Count int
}

// FormatTagList formats the tag list for display.
func (tm *TagManager) FormatTagList(tags []TagInfo) string {
	if len(tags) == 0 {
		return "No tags found"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tags (%d):\n", len(tags)))
	for _, t := range tags {
		sb.WriteString(fmt.Sprintf("  %-20s %d connection(s)\n", t.Name, t.Count))
	}
	return sb.String()
}

// TagFilterOp represents a boolean operator in a tag filter expression.
type TagFilterOp int

const (
	TagFilterAnd TagFilterOp = iota
	TagFilterOr
)

// TagFilter represents a parsed tag filter expression with boolean logic.
type TagFilter struct {
	Tags []string
	Op   TagFilterOp
}

// ParseTagFilter parses a tag filter expression like "web AND production" or "web OR api".
// Default operator is AND if no explicit operator is found.
func ParseTagFilter(expr string) TagFilter {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return TagFilter{}
	}

	// Check for explicit OR operator
	if idx := strings.Index(strings.ToUpper(expr), " OR "); idx >= 0 {
		parts := splitTagExpr(expr, " OR ")
		return TagFilter{Tags: parts, Op: TagFilterOr}
	}

	// Check for explicit AND operator
	if idx := strings.Index(strings.ToUpper(expr), " AND "); idx >= 0 {
		parts := splitTagExpr(expr, " AND ")
		return TagFilter{Tags: parts, Op: TagFilterAnd}
	}

	// Single tag or space-separated tags (implicit AND)
	parts := strings.Fields(expr)
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			cleaned = append(cleaned, strings.ToLower(p))
		}
	}
	if len(cleaned) == 0 {
		return TagFilter{}
	}
	return TagFilter{Tags: cleaned, Op: TagFilterAnd}
}

// splitTagExpr splits an expression by a case-insensitive operator string.
func splitTagExpr(expr, op string) []string {
	upper := strings.ToUpper(expr)
	var parts []string
	for {
		idx := strings.Index(upper, op)
		if idx < 0 {
			part := strings.TrimSpace(expr)
			if part != "" {
				parts = append(parts, strings.ToLower(part))
			}
			break
		}
		part := strings.TrimSpace(expr[:idx])
		if part != "" {
			parts = append(parts, strings.ToLower(part))
		}
		expr = expr[idx+len(op):]
		upper = upper[idx+len(op):]
	}
	return parts
}

// Match returns true if the connection's tags satisfy the filter.
func (tf TagFilter) Match(conn config.Connection) bool {
	if len(tf.Tags) == 0 {
		return true
	}
	connTags := make(map[string]bool, len(conn.Tags))
	for _, t := range conn.Tags {
		connTags[strings.ToLower(strings.TrimSpace(t))] = true
	}

	switch tf.Op {
	case TagFilterAnd:
		for _, tag := range tf.Tags {
			if !connTags[tag] {
				return false
			}
		}
		return true
	case TagFilterOr:
		for _, tag := range tf.Tags {
			if connTags[tag] {
				return true
			}
		}
		return false
	}
	return false
}

// FilterByTags filters connections using a tag filter expression.
func FilterByTags(connections []config.Connection, filter TagFilter) []config.Connection {
	if len(filter.Tags) == 0 {
		return connections
	}
	var result []config.Connection
	for _, c := range connections {
		if filter.Match(c) {
			result = append(result, c)
		}
	}
	return result
}
