package tui

import (
	"testing"

	"github.com/dr4zz/nexus/internal/config"
)

func treeTestConnections() []config.Connection {
	return []config.Connection{
		{ID: "c1", Name: "prod-web-1", Group: "prod/web", Protocol: config.ProtoSSH, Host: "10.0.1.1"},
		{ID: "c2", Name: "prod-web-2", Group: "prod/web", Protocol: config.ProtoSSH, Host: "10.0.1.2"},
		{ID: "c3", Name: "prod-api-1", Group: "prod/api", Protocol: config.ProtoSSH, Host: "10.0.2.1"},
		{ID: "c4", Name: "staging-1", Group: "staging", Protocol: config.ProtoSSH, Host: "10.0.3.1"},
		{ID: "c5", Name: "dev-local", Group: "", Protocol: config.ProtoSSH, Host: "localhost"},
	}
}

func TestTreeBuildFromGroups(t *testing.T) {
	tm := NewTreeModel(treeTestConnections())

	// Root should have 3 top-level children: (ungrouped), prod, staging
	if len(tm.root.Children) != 3 {
		t.Fatalf("expected 3 top-level children, got %d", len(tm.root.Children))
	}

	// Check that we can find the expected folders.
	names := make(map[string]bool)
	for _, child := range tm.root.Children {
		names[child.Name] = true
	}
	for _, expected := range []string{"(ungrouped)", "prod", "staging"} {
		if !names[expected] {
			t.Errorf("expected top-level folder %q, not found", expected)
		}
	}

	// Find the "prod" node and verify it has children "api" and "web".
	var prodNode *TreeNode
	for _, child := range tm.root.Children {
		if child.Name == "prod" {
			prodNode = child
			break
		}
	}
	if prodNode == nil {
		t.Fatal("prod node not found")
	}
	if len(prodNode.Children) != 2 {
		t.Fatalf("expected prod to have 2 children, got %d", len(prodNode.Children))
	}

	childNames := make(map[string]bool)
	for _, child := range prodNode.Children {
		childNames[child.Name] = true
	}
	if !childNames["api"] || !childNames["web"] {
		t.Errorf("expected prod to have children 'api' and 'web', got %v", childNames)
	}

	// Check "prod/web" has 2 connections.
	var webNode *TreeNode
	for _, child := range prodNode.Children {
		if child.Name == "web" {
			webNode = child
			break
		}
	}
	if webNode == nil {
		t.Fatal("prod/web node not found")
	}
	if len(webNode.Connections) != 2 {
		t.Errorf("expected prod/web to have 2 connections, got %d", len(webNode.Connections))
	}
}

func TestTreeFlattenOrder(t *testing.T) {
	tm := NewTreeModel(treeTestConnections())

	// All folders start expanded, so the flat view should contain all entries.
	// Expected order (sorted alphabetically):
	//   (ungrouped)        folder, depth 0
	//     dev-local        conn,   depth 1
	//   prod               folder, depth 0
	//     api              folder, depth 1
	//       prod-api-1     conn,   depth 2
	//     web              folder, depth 1
	//       prod-web-1     conn,   depth 2
	//       prod-web-2     conn,   depth 2
	//   staging            folder, depth 0
	//     staging-1        conn,   depth 1

	expected := []struct {
		isFolder bool
		depth    int
		name     string // folder name or conn name
	}{
		{true, 0, "(ungrouped)"},
		{false, 1, "dev-local"},
		{true, 0, "prod"},
		{true, 1, "api"},
		{false, 2, "prod-api-1"},
		{true, 1, "web"},
		{false, 2, "prod-web-1"},
		{false, 2, "prod-web-2"},
		{true, 0, "staging"},
		{false, 1, "staging-1"},
	}

	if len(tm.flatView) != len(expected) {
		t.Fatalf("expected %d flat entries, got %d", len(expected), len(tm.flatView))
	}

	for i, exp := range expected {
		entry := tm.flatView[i]
		if entry.IsFolder != exp.isFolder {
			t.Errorf("entry[%d]: expected IsFolder=%v, got %v", i, exp.isFolder, entry.IsFolder)
		}
		if entry.Depth != exp.depth {
			t.Errorf("entry[%d]: expected Depth=%d, got %d", i, exp.depth, entry.Depth)
		}
		name := entry.ConnName
		if entry.IsFolder {
			name = entry.Node.Name
		}
		if name != exp.name {
			t.Errorf("entry[%d]: expected name=%q, got %q", i, exp.name, name)
		}
	}
}

func TestTreeToggle(t *testing.T) {
	tm := NewTreeModel(treeTestConnections())

	// Find "prod" folder index in flat view.
	prodIdx := -1
	for i, entry := range tm.flatView {
		if entry.IsFolder && entry.Node.Name == "prod" {
			prodIdx = i
			break
		}
	}
	if prodIdx < 0 {
		t.Fatal("prod folder not found in flat view")
	}

	initialLen := len(tm.flatView)

	// Collapse prod — should hide api, web, and their connections (5 entries).
	tm.Toggle(prodIdx)

	if tm.flatView[prodIdx].Node.Expanded {
		t.Error("expected prod to be collapsed after toggle")
	}

	// After collapsing prod, we lose: api folder, prod-api-1, web folder, prod-web-1, prod-web-2 = 5 entries.
	expectedLen := initialLen - 5
	if len(tm.flatView) != expectedLen {
		t.Errorf("after collapse: expected %d entries, got %d", expectedLen, len(tm.flatView))
	}

	// Toggle again to expand.
	tm.Toggle(prodIdx)
	if !tm.flatView[prodIdx].Node.Expanded {
		t.Error("expected prod to be expanded after second toggle")
	}
	if len(tm.flatView) != initialLen {
		t.Errorf("after re-expand: expected %d entries, got %d", initialLen, len(tm.flatView))
	}
}

func TestTreeExpandCollapse(t *testing.T) {
	tm := NewTreeModel(treeTestConnections())

	// Find "prod" folder index.
	prodIdx := -1
	for i, entry := range tm.flatView {
		if entry.IsFolder && entry.Node.Name == "prod" {
			prodIdx = i
			break
		}
	}
	if prodIdx < 0 {
		t.Fatal("prod folder not found")
	}

	// Collapse with Collapse().
	tm.Collapse(prodIdx)
	if tm.flatView[prodIdx].Node.Expanded {
		t.Error("expected prod to be collapsed")
	}

	// Expand with Expand().
	tm.Expand(prodIdx)
	if !tm.flatView[prodIdx].Node.Expanded {
		t.Error("expected prod to be expanded")
	}
}

func TestTreeExpandAllCollapseAll(t *testing.T) {
	tm := NewTreeModel(treeTestConnections())

	allExpanded := len(tm.flatView)

	// CollapseAll should collapse everything except root.
	tm.CollapseAll()
	// Only top-level folders should remain: (ungrouped), prod, staging.
	if len(tm.flatView) != 3 {
		t.Errorf("after CollapseAll: expected 3 entries, got %d", len(tm.flatView))
	}

	// ExpandAll should restore everything.
	tm.ExpandAll()
	if len(tm.flatView) != allExpanded {
		t.Errorf("after ExpandAll: expected %d entries, got %d", allExpanded, len(tm.flatView))
	}
}

func TestTreeCursorNavigation(t *testing.T) {
	tm := NewTreeModel(treeTestConnections())
	tm.setSize(80, 20)

	// Cursor starts at 0.
	if tm.Cursor() != 0 {
		t.Errorf("expected initial cursor=0, got %d", tm.Cursor())
	}

	// Move down.
	tm.CursorDown()
	if tm.Cursor() != 1 {
		t.Errorf("expected cursor=1 after CursorDown, got %d", tm.Cursor())
	}

	// Move up.
	tm.CursorUp()
	if tm.Cursor() != 0 {
		t.Errorf("expected cursor=0 after CursorUp, got %d", tm.Cursor())
	}

	// Move up at top stays at 0.
	tm.CursorUp()
	if tm.Cursor() != 0 {
		t.Errorf("expected cursor=0 when already at top, got %d", tm.Cursor())
	}

	// Move to last entry.
	tm.MoveCursor(len(tm.flatView) - 1)
	if tm.Cursor() != len(tm.flatView)-1 {
		t.Errorf("expected cursor=%d, got %d", len(tm.flatView)-1, tm.Cursor())
	}

	// CursorDown at bottom stays.
	tm.CursorDown()
	if tm.Cursor() != len(tm.flatView)-1 {
		t.Errorf("expected cursor to stay at bottom, got %d", tm.Cursor())
	}
}

func TestTreeSelectedConnection(t *testing.T) {
	tm := NewTreeModel(treeTestConnections())
	tm.setSize(80, 20)

	// Cursor on a folder should not return a connection.
	if _, ok := tm.SelectedConnection(); ok {
		t.Error("expected no connection selected when on a folder")
	}

	// Move cursor to a connection entry. Entry 1 should be "dev-local" (conn under ungrouped).
	tm.SetCursor(1)
	id, ok := tm.SelectedConnection()
	if !ok {
		t.Error("expected connection selected on entry 1")
	}
	if id != "c5" {
		t.Errorf("expected connection ID 'c5', got %q", id)
	}
}

func TestTreeSelectedFolder(t *testing.T) {
	tm := NewTreeModel(treeTestConnections())
	tm.setSize(80, 20)

	// Cursor on entry 0 should be a folder.
	path, ok := tm.SelectedFolder()
	if !ok {
		t.Error("expected folder selected on entry 0")
	}
	if path != "(ungrouped)" {
		t.Errorf("expected path '(ungrouped)', got %q", path)
	}

	// Move cursor to a connection — should not return a folder.
	tm.SetCursor(1)
	if _, ok := tm.SelectedFolder(); ok {
		t.Error("expected no folder selected when on a connection")
	}
}

func TestTreeEmptyConnections(t *testing.T) {
	tm := NewTreeModel(nil)
	if len(tm.flatView) != 0 {
		t.Errorf("expected 0 flat entries for nil connections, got %d", len(tm.flatView))
	}
	// Cursor operations should not panic.
	tm.CursorUp()
	tm.CursorDown()
	tm.Toggle(0)
	tm.ExpandAll()
	tm.CollapseAll()
}

func TestTreeView(t *testing.T) {
	tm := NewTreeModel(treeTestConnections())
	tm.setSize(80, 20)

	view := tm.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
	// The view should contain folder indicators and connection names.
	// We check for the presence of key strings (without ANSI codes, look for raw text).
	// Note: The rendered output contains ANSI escape codes, so we cannot do simple string matching.
	// Just verify it renders without panicking and produces output.
	if len(view) < 10 {
		t.Error("view output seems too short")
	}
}

func TestTreeCountConnections(t *testing.T) {
	tm := NewTreeModel(treeTestConnections())

	// Find prod node.
	var prodNode *TreeNode
	for _, child := range tm.root.Children {
		if child.Name == "prod" {
			prodNode = child
			break
		}
	}
	if prodNode == nil {
		t.Fatal("prod node not found")
	}

	// prod should have 3 total connections (2 in web, 1 in api).
	count := countConnections(prodNode)
	if count != 3 {
		t.Errorf("expected 3 connections under prod, got %d", count)
	}
}
