package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

// handleTagCommand processes :tag add/remove commands.
func (a App) handleTagCommand(args string) (tea.Model, tea.Cmd) {
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		a.statusBar.setFlash("Usage: :tag <add|remove|list> <tag>", flashError)
		return a, scheduleFlashClear()
	}

	subcmd := strings.ToLower(parts[0])
	tagArg := strings.TrimSpace(parts[1])

	switch subcmd {
	case "add":
		if tagArg == "" {
			a.statusBar.setFlash("Usage: :tag add <tag>", flashError)
			return a, scheduleFlashClear()
		}
		// Apply to selection if any, otherwise to current connection.
		if a.selectionSet.HasSelection() {
			return a.tagAddVisual(tagArg)
		}
		c := a.list.selectedConnection()
		if c == nil {
			a.statusBar.setFlash("No connection selected", flashError)
			return a, scheduleFlashClear()
		}
		before := *c
		if a.tagManager.AddTag(c, tagArg) {
			// Persist the change.
			if err := a.cfg.UpdateConnection(*c); err != nil {
				a.log.error("Failed to save tag: %v", err)
				a.statusBar.setFlash("Error saving tag", flashError)
			} else {
				a.undoStack.Push(Operation{
					Type:   UndoOpTagChange,
					ConnID: c.ID,
					Name:   c.Name,
					Before: before,
					After:  *c,
				})
				a.log.info("Added tag '%s' to %s", tagArg, c.Name)
				a.statusBar.setFlash(fmt.Sprintf("Added tag '%s' to %s", tagArg, c.Name), flashInfo)
			}
			a.list.rebuildTable()
		} else {
			a.statusBar.setFlash(fmt.Sprintf("Tag '%s' already exists on %s", tagArg, c.Name), flashInfo)
		}
		return a, scheduleFlashClear()

	case "remove":
		if tagArg == "" {
			a.statusBar.setFlash("Usage: :tag remove <tag>", flashError)
			return a, scheduleFlashClear()
		}
		if a.selectionSet.HasSelection() {
			return a.tagRemoveVisual(tagArg)
		}
		c := a.list.selectedConnection()
		if c == nil {
			a.statusBar.setFlash("No connection selected", flashError)
			return a, scheduleFlashClear()
		}
		before := *c
		if a.tagManager.RemoveTag(c, tagArg) {
			if err := a.cfg.UpdateConnection(*c); err != nil {
				a.log.error("Failed to save tag removal: %v", err)
				a.statusBar.setFlash("Error saving tag removal", flashError)
			} else {
				a.undoStack.Push(Operation{
					Type:   UndoOpTagChange,
					ConnID: c.ID,
					Name:   c.Name,
					Before: before,
					After:  *c,
				})
				a.log.info("Removed tag '%s' from %s", tagArg, c.Name)
				a.statusBar.setFlash(fmt.Sprintf("Removed tag '%s' from %s", tagArg, c.Name), flashInfo)
			}
			a.list.rebuildTable()
		} else {
			a.statusBar.setFlash(fmt.Sprintf("Tag '%s' not found on %s", tagArg, c.Name), flashInfo)
		}
		return a, scheduleFlashClear()

	case "list":
		return a.handleTagsListCommand()

	default:
		a.statusBar.setFlash("Usage: :tag <add|remove|list> <tag>", flashError)
		return a, scheduleFlashClear()
	}
}

// tagAddVisual adds a tag to all visually selected connections.
func (a App) tagAddVisual(tag string) (tea.Model, tea.Cmd) {
	ids := a.selectionSet.SelectedIDs(a.list.table.rows)
	var children []Operation
	for _, id := range ids {
		c := a.cfg.FindConnection(id)
		if c == nil {
			continue
		}
		before := *c
		if a.tagManager.AddTag(c, tag) {
			children = append(children, Operation{
				Type:   UndoOpTagChange,
				ConnID: c.ID,
				Name:   c.Name,
				Before: before,
				After:  *c,
			})
		}
	}
	if len(children) > 0 {
		if err := config.Save(a.cfg); err != nil {
			a.log.error("Failed to save tags: %v", err)
			a.statusBar.setFlash("Error saving tags", flashError)
			return a, scheduleFlashClear()
		}
		a.undoStack.PushBatch("bulk tag add: "+tag, children)
		a.log.info("Added tag '%s' to %d connections", tag, len(children))
		a.statusBar.setFlash(fmt.Sprintf("Added tag '%s' to %d connections", tag, len(children)), flashInfo)
		a.list.rebuildTable()
	} else {
		a.statusBar.setFlash(fmt.Sprintf("Tag '%s' already exists on all selected connections", tag), flashInfo)
	}
	return a, scheduleFlashClear()
}

// tagRemoveVisual removes a tag from all visually selected connections.
func (a App) tagRemoveVisual(tag string) (tea.Model, tea.Cmd) {
	ids := a.selectionSet.SelectedIDs(a.list.table.rows)
	var children []Operation
	for _, id := range ids {
		c := a.cfg.FindConnection(id)
		if c == nil {
			continue
		}
		before := *c
		if a.tagManager.RemoveTag(c, tag) {
			children = append(children, Operation{
				Type:   UndoOpTagChange,
				ConnID: c.ID,
				Name:   c.Name,
				Before: before,
				After:  *c,
			})
		}
	}
	if len(children) > 0 {
		if err := config.Save(a.cfg); err != nil {
			a.log.error("Failed to save tag removal: %v", err)
			a.statusBar.setFlash("Error saving tag removal", flashError)
			return a, scheduleFlashClear()
		}
		a.undoStack.PushBatch("bulk tag remove: "+tag, children)
		a.log.info("Removed tag '%s' from %d connections", tag, len(children))
		a.statusBar.setFlash(fmt.Sprintf("Removed tag '%s' from %d connections", tag, len(children)), flashInfo)
		a.list.rebuildTable()
	} else {
		a.statusBar.setFlash(fmt.Sprintf("Tag '%s' not found on any selected connections", tag), flashInfo)
	}
	return a, scheduleFlashClear()
}

func (a App) handleTagsListCommand() (tea.Model, tea.Cmd) {
	tags := a.tagManager.AllTags(a.cfg.Connections)
	if len(tags) == 0 {
		a.statusBar.setFlash("No tags found", flashInfo)
		return a, scheduleFlashClear()
	}
	// Format a compact summary for the status bar.
	var parts []string
	for _, t := range tags {
		parts = append(parts, fmt.Sprintf("%s(%d)", t.Name, t.Count))
	}
	summary := "Tags: " + strings.Join(parts, ", ")
	if len(summary) > 120 {
		summary = summary[:117] + "..."
	}
	a.log.info("%s", a.tagManager.FormatTagList(tags))
	a.statusBar.setFlash(summary, flashInfo)
	return a, scheduleFlashClear()
}
