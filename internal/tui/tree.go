package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/theme"
)

// TreeNode represents a folder in the hierarchical tree.
type TreeNode struct {
	Name        string
	Path        string
	Expanded    bool
	Children    []*TreeNode
	Connections []string // connection IDs
}

// TreeEntry represents a single line in the flattened tree view.
type TreeEntry struct {
	Node         *TreeNode
	Depth        int
	IsFolder     bool
	ConnectionID string // non-empty if this entry is a connection
	ConnName     string // display name for connections
}

// TreeModel manages the hierarchical folder tree view.
type TreeModel struct {
	root     *TreeNode
	flatView []TreeEntry
	cursor   int
	width    int
	height   int
	offset   int
	connMap  map[string]config.Connection // ID -> Connection for display
}

// NewTreeModel builds a tree from connections grouped by their Group field.
// Group paths use "/" as separator (e.g. "prod/web" creates prod -> web).
func NewTreeModel(connections []config.Connection) *TreeModel {
	t := &TreeModel{
		root: &TreeNode{
			Name:     "(root)",
			Path:     "",
			Expanded: true,
		},
		connMap: make(map[string]config.Connection),
	}

	for _, c := range connections {
		t.connMap[c.ID] = c
	}

	// Build the tree structure from connection groups.
	for _, c := range connections {
		group := c.Group
		if group == "" {
			group = "(ungrouped)"
		}
		node := t.ensurePath(group)
		node.Connections = append(node.Connections, c.ID)
	}

	// Sort children and connections at every level.
	t.sortTree(t.root)

	// Start with all folders expanded.
	t.setExpandedAll(t.root, true)

	t.Flatten()
	return t
}

// ensurePath creates or retrieves the TreeNode for the given slash-separated path.
func (t *TreeModel) ensurePath(path string) *TreeNode {
	parts := strings.Split(path, "/")
	current := t.root
	accumulated := ""

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if accumulated == "" {
			accumulated = part
		} else {
			accumulated = accumulated + "/" + part
		}

		found := false
		for _, child := range current.Children {
			if child.Name == part {
				current = child
				found = true
				break
			}
		}
		if !found {
			newNode := &TreeNode{
				Name:     part,
				Path:     accumulated,
				Expanded: true,
			}
			current.Children = append(current.Children, newNode)
			current = newNode
		}
	}
	return current
}

// sortTree recursively sorts children and connections alphabetically.
func (t *TreeModel) sortTree(node *TreeNode) {
	sort.Slice(node.Children, func(i, j int) bool {
		return strings.ToLower(node.Children[i].Name) < strings.ToLower(node.Children[j].Name)
	})
	sort.Slice(node.Connections, func(i, j int) bool {
		a := t.connMap[node.Connections[i]].Name
		b := t.connMap[node.Connections[j]].Name
		return strings.ToLower(a) < strings.ToLower(b)
	})
	for _, child := range node.Children {
		t.sortTree(child)
	}
}

// setExpandedAll recursively sets the Expanded state of all nodes.
func (t *TreeModel) setExpandedAll(node *TreeNode, expanded bool) {
	node.Expanded = expanded
	for _, child := range node.Children {
		t.setExpandedAll(child, expanded)
	}
}

// Toggle toggles the expand/collapse state of the folder at the given flat index.
func (t *TreeModel) Toggle(idx int) {
	if idx < 0 || idx >= len(t.flatView) {
		return
	}
	entry := t.flatView[idx]
	if !entry.IsFolder {
		return
	}
	entry.Node.Expanded = !entry.Node.Expanded
	t.Flatten()
	t.clampCursor()
}

// Expand expands the folder at the given flat index.
func (t *TreeModel) Expand(idx int) {
	if idx < 0 || idx >= len(t.flatView) {
		return
	}
	entry := t.flatView[idx]
	if !entry.IsFolder {
		return
	}
	entry.Node.Expanded = true
	t.Flatten()
}

// Collapse collapses the folder at the given flat index.
func (t *TreeModel) Collapse(idx int) {
	if idx < 0 || idx >= len(t.flatView) {
		return
	}
	entry := t.flatView[idx]
	if !entry.IsFolder {
		return
	}
	entry.Node.Expanded = false
	t.Flatten()
	t.clampCursor()
}

// ExpandAll expands all folders in the tree.
func (t *TreeModel) ExpandAll() {
	t.setExpandedAll(t.root, true)
	t.Flatten()
}

// CollapseAll collapses all folders in the tree (root stays expanded).
func (t *TreeModel) CollapseAll() {
	for _, child := range t.root.Children {
		t.setExpandedAll(child, false)
	}
	t.Flatten()
	t.clampCursor()
}

// Flatten rebuilds the flat view from the current tree state.
func (t *TreeModel) Flatten() {
	t.flatView = t.flatView[:0]
	t.flattenNode(t.root, -1) // root itself is not shown, start at depth -1
}

// flattenNode recursively flattens a node and its children into flatView.
func (t *TreeModel) flattenNode(node *TreeNode, depth int) {
	// Don't show the root node itself, just its children.
	if depth >= 0 {
		t.flatView = append(t.flatView, TreeEntry{
			Node:     node,
			Depth:    depth,
			IsFolder: true,
		})
	}

	if !node.Expanded && depth >= 0 {
		return
	}

	// Add child folders first, then connections.
	for _, child := range node.Children {
		t.flattenNode(child, depth+1)
	}

	// Add connections at this level.
	connDepth := depth + 1
	if depth < 0 {
		connDepth = 0
	}
	for _, connID := range node.Connections {
		name := connID
		if c, ok := t.connMap[connID]; ok {
			name = c.Name
		}
		t.flatView = append(t.flatView, TreeEntry{
			Node:         node,
			Depth:        connDepth,
			IsFolder:     false,
			ConnectionID: connID,
			ConnName:     name,
		})
	}
}

// CursorUp moves the cursor up by one.
func (t *TreeModel) CursorUp() {
	if t.cursor > 0 {
		t.cursor--
	}
	t.clampViewport()
}

// CursorDown moves the cursor down by one.
func (t *TreeModel) CursorDown() {
	if t.cursor < len(t.flatView)-1 {
		t.cursor++
	}
	t.clampViewport()
}

// MoveCursor moves the cursor to an absolute position, clamping to bounds.
func (t *TreeModel) MoveCursor(target int) {
	t.cursor = clampInt(target, 0, max(len(t.flatView)-1, 0))
	t.clampViewport()
}

// Cursor returns the current cursor position.
func (t *TreeModel) Cursor() int {
	return t.cursor
}

// SetCursor sets the cursor to the given index, clamping to bounds.
func (t *TreeModel) SetCursor(idx int) {
	t.cursor = clampInt(idx, 0, max(len(t.flatView)-1, 0))
	t.clampViewport()
}

// RowCount returns the number of visible entries in the flat view.
func (t *TreeModel) RowCount() int {
	return len(t.flatView)
}

// SelectedConnection returns the connection ID if the cursor is on a connection line.
func (t *TreeModel) SelectedConnection() (string, bool) {
	if t.cursor < 0 || t.cursor >= len(t.flatView) {
		return "", false
	}
	entry := t.flatView[t.cursor]
	if entry.IsFolder {
		return "", false
	}
	return entry.ConnectionID, true
}

// SelectedFolder returns the folder path if the cursor is on a folder line.
func (t *TreeModel) SelectedFolder() (string, bool) {
	if t.cursor < 0 || t.cursor >= len(t.flatView) {
		return "", false
	}
	entry := t.flatView[t.cursor]
	if !entry.IsFolder {
		return "", false
	}
	return entry.Node.Path, true
}

// setSize updates the available width and content height for rendering.
func (t *TreeModel) setSize(width, height int) {
	t.width = width
	t.height = height
	t.clampViewport()
}

// countConnections recursively counts all connections under a node.
func countConnections(node *TreeNode) int {
	count := len(node.Connections)
	for _, child := range node.Children {
		count += countConnections(child)
	}
	return count
}

// View renders the tree view to a string.
func (t *TreeModel) View() string {
	if len(t.flatView) == 0 {
		return "  No connections"
	}

	th := theme.Current()

	folderStyle := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	connStyle := lipgloss.NewStyle().Foreground(th.Fg)
	countStyle := lipgloss.NewStyle().Foreground(th.Muted)
	cursorBg := lipgloss.NewStyle().Background(th.Selection).Foreground(th.Cursor).Bold(true)

	visibleHeight := t.height
	if visibleHeight <= 0 {
		visibleHeight = 1
	}
	endIdx := t.offset + visibleHeight
	if endIdx > len(t.flatView) {
		endIdx = len(t.flatView)
	}

	var sb strings.Builder

	for i := t.offset; i < endIdx; i++ {
		entry := t.flatView[i]
		isCursor := i == t.cursor
		indent := strings.Repeat("  ", entry.Depth)

		var line string
		if entry.IsFolder {
			icon := "\u25b8" // ▸ collapsed
			if entry.Node.Expanded {
				icon = "\u25be" // ▾ expanded
			}
			connCount := countConnections(entry.Node)
			countStr := countStyle.Render(fmt.Sprintf(" (%d)", connCount))
			if isCursor {
				// Render folder name without styling, apply cursor style to whole line
				line = fmt.Sprintf("%s%s %s%s", indent, icon, entry.Node.Name, countStr)
			} else {
				line = fmt.Sprintf("%s%s %s%s", indent, folderStyle.Render(icon), folderStyle.Render(entry.Node.Name), countStr)
			}
		} else {
			if isCursor {
				line = fmt.Sprintf("%s  %s", indent, entry.ConnName)
			} else {
				line = fmt.Sprintf("%s  %s", indent, connStyle.Render(entry.ConnName))
			}
		}

		// Pad to full width
		lineW := lipgloss.Width(line)
		if lineW < t.width {
			line += strings.Repeat(" ", t.width-lineW)
		}

		if isCursor {
			line = cursorBg.Render(line)
		}

		sb.WriteString(line)
		if i < endIdx-1 {
			sb.WriteString("\n")
		}
	}

	// Fill remaining lines if tree is shorter than height.
	rendered := endIdx - t.offset
	for rendered < visibleHeight {
		sb.WriteString("\n")
		sb.WriteString(strings.Repeat(" ", t.width))
		rendered++
	}

	return sb.String()
}

// clampCursor ensures cursor is within bounds after flatten changes.
func (t *TreeModel) clampCursor() {
	total := len(t.flatView)
	if total == 0 {
		t.cursor = 0
		return
	}
	if t.cursor >= total {
		t.cursor = total - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
	t.clampViewport()
}

// clampViewport ensures the cursor is visible within the scrolling window.
func (t *TreeModel) clampViewport() {
	total := len(t.flatView)
	if total == 0 {
		t.cursor = 0
		t.offset = 0
		return
	}
	t.cursor = clampInt(t.cursor, 0, total-1)

	visibleHeight := t.height
	if visibleHeight <= 0 {
		visibleHeight = 1
	}

	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+visibleHeight {
		t.offset = t.cursor - visibleHeight + 1
	}
	maxOffset := total - visibleHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	t.offset = clampInt(t.offset, 0, maxOffset)
}
