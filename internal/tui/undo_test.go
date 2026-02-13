package tui

import (
	"testing"

	"github.com/dr4zz/nexus/internal/config"
)

func makeTestConn(id, name string) config.Connection {
	return config.Connection{
		ID:       id,
		Name:     name,
		Protocol: config.ProtoSSH,
		Host:     "example.com",
	}
}

func TestPushAndUndo(t *testing.T) {
	s := NewUndoStack()

	conn := makeTestConn("c1", "Server A")
	s.Push(Operation{
		Type:  UndoOpDelete,
		Name:  conn.Name,
		Index: 0,
		Before: conn,
	})

	if !s.CanUndo() {
		t.Fatal("expected CanUndo to be true after Push")
	}

	op, ok := s.Undo()
	if !ok {
		t.Fatal("expected Undo to succeed")
	}
	if op.Type != UndoOpDelete {
		t.Fatalf("expected UndoOpDelete, got %d", op.Type)
	}
	if op.Before.ID != "c1" {
		t.Fatalf("expected connection ID c1, got %s", op.Before.ID)
	}
	if s.CanUndo() {
		t.Fatal("expected CanUndo to be false after undoing the only operation")
	}
}

func TestUndoAndRedoCycle(t *testing.T) {
	s := NewUndoStack()

	before := makeTestConn("c1", "Old Name")
	after := makeTestConn("c1", "New Name")
	s.Push(Operation{
		Type:   UndoOpEdit,
		ConnID: "c1",
		Name:   "Old Name",
		Before: before,
		After:  after,
	})

	// Undo should move it to redo stack.
	op, ok := s.Undo()
	if !ok {
		t.Fatal("Undo should succeed")
	}
	if op.Before.Name != "Old Name" {
		t.Fatalf("expected Before.Name 'Old Name', got %q", op.Before.Name)
	}
	if !s.CanRedo() {
		t.Fatal("expected CanRedo to be true after Undo")
	}
	if s.CanUndo() {
		t.Fatal("expected CanUndo to be false after Undo")
	}

	// Redo should move it back to undo stack.
	op, ok = s.Redo()
	if !ok {
		t.Fatal("Redo should succeed")
	}
	if op.After.Name != "New Name" {
		t.Fatalf("expected After.Name 'New Name', got %q", op.After.Name)
	}
	if !s.CanUndo() {
		t.Fatal("expected CanUndo to be true after Redo")
	}
	if s.CanRedo() {
		t.Fatal("expected CanRedo to be false after Redo")
	}
}

func TestStackDepthLimit(t *testing.T) {
	s := NewUndoStack()

	for i := 0; i < 60; i++ {
		s.Push(Operation{
			Type: UndoOpAdd,
			Name: "conn",
			After: makeTestConn("c", "conn"),
		})
	}

	if s.UndoLen() != 50 {
		t.Fatalf("expected undo stack depth 50, got %d", s.UndoLen())
	}
}

func TestRedoClearedOnNewPush(t *testing.T) {
	s := NewUndoStack()

	s.Push(Operation{Type: UndoOpAdd, Name: "a", After: makeTestConn("a", "A")})
	s.Push(Operation{Type: UndoOpAdd, Name: "b", After: makeTestConn("b", "B")})

	// Undo one — moves to redo.
	s.Undo()
	if !s.CanRedo() {
		t.Fatal("expected CanRedo after Undo")
	}
	if s.RedoLen() != 1 {
		t.Fatalf("expected redo stack length 1, got %d", s.RedoLen())
	}

	// Push a new operation — redo should be cleared.
	s.Push(Operation{Type: UndoOpAdd, Name: "c", After: makeTestConn("c", "C")})
	if s.CanRedo() {
		t.Fatal("expected CanRedo to be false after new Push")
	}
	if s.RedoLen() != 0 {
		t.Fatalf("expected redo stack length 0, got %d", s.RedoLen())
	}
}

func TestEmptyStackReturnsFalse(t *testing.T) {
	s := NewUndoStack()

	_, ok := s.Undo()
	if ok {
		t.Fatal("Undo on empty stack should return false")
	}
	_, ok = s.Redo()
	if ok {
		t.Fatal("Redo on empty stack should return false")
	}
	if s.CanUndo() {
		t.Fatal("CanUndo on empty stack should be false")
	}
	if s.CanRedo() {
		t.Fatal("CanRedo on empty stack should be false")
	}
}

func TestOperationTypes(t *testing.T) {
	s := NewUndoStack()

	// UndoOpAdd: undo should return the added connection.
	added := makeTestConn("add1", "Added Server")
	s.Push(Operation{
		Type:  UndoOpAdd,
		Name:  added.Name,
		After: added,
	})

	op, ok := s.Undo()
	if !ok || op.Type != UndoOpAdd {
		t.Fatal("expected UndoOpAdd undo")
	}
	if op.After.ID != "add1" {
		t.Fatalf("expected After.ID 'add1', got %q", op.After.ID)
	}

	// Clear for next test.
	s = NewUndoStack()

	// UndoOpDelete: undo should return the deleted connection and its index.
	deleted := makeTestConn("del1", "Deleted Server")
	s.Push(Operation{
		Type:   UndoOpDelete,
		Name:   deleted.Name,
		Index:  3,
		Before: deleted,
	})

	op, ok = s.Undo()
	if !ok || op.Type != UndoOpDelete {
		t.Fatal("expected UndoOpDelete undo")
	}
	if op.Before.ID != "del1" {
		t.Fatalf("expected Before.ID 'del1', got %q", op.Before.ID)
	}
	if op.Index != 3 {
		t.Fatalf("expected Index 3, got %d", op.Index)
	}

	// Clear for next test.
	s = NewUndoStack()

	// UndoOpEdit: undo should return both before and after snapshots.
	before := makeTestConn("edit1", "Before Edit")
	after := makeTestConn("edit1", "After Edit")
	after.Host = "new-host.com"
	s.Push(Operation{
		Type:   UndoOpEdit,
		ConnID: "edit1",
		Name:   before.Name,
		Before: before,
		After:  after,
	})

	op, ok = s.Undo()
	if !ok || op.Type != UndoOpEdit {
		t.Fatal("expected UndoOpEdit undo")
	}
	if op.Before.Name != "Before Edit" {
		t.Fatalf("expected Before.Name 'Before Edit', got %q", op.Before.Name)
	}
	if op.After.Host != "new-host.com" {
		t.Fatalf("expected After.Host 'new-host.com', got %q", op.After.Host)
	}
}

func TestMultipleUndoRedo(t *testing.T) {
	s := NewUndoStack()

	s.Push(Operation{Type: UndoOpAdd, Name: "A", After: makeTestConn("a", "A")})
	s.Push(Operation{Type: UndoOpAdd, Name: "B", After: makeTestConn("b", "B")})
	s.Push(Operation{Type: UndoOpAdd, Name: "C", After: makeTestConn("c", "C")})

	if s.UndoLen() != 3 {
		t.Fatalf("expected undo len 3, got %d", s.UndoLen())
	}

	// Undo all three.
	for i := 0; i < 3; i++ {
		_, ok := s.Undo()
		if !ok {
			t.Fatalf("Undo %d should succeed", i+1)
		}
	}
	if s.UndoLen() != 0 {
		t.Fatalf("expected undo len 0 after undoing all, got %d", s.UndoLen())
	}
	if s.RedoLen() != 3 {
		t.Fatalf("expected redo len 3, got %d", s.RedoLen())
	}

	// Redo all three.
	for i := 0; i < 3; i++ {
		_, ok := s.Redo()
		if !ok {
			t.Fatalf("Redo %d should succeed", i+1)
		}
	}
	if s.UndoLen() != 3 {
		t.Fatalf("expected undo len 3 after redoing all, got %d", s.UndoLen())
	}
	if s.RedoLen() != 0 {
		t.Fatalf("expected redo len 0 after redoing all, got %d", s.RedoLen())
	}
}

func TestTagChangeOperation(t *testing.T) {
	s := NewUndoStack()

	before := makeTestConn("tc1", "Tag Server")
	before.Tags = []string{"prod", "web"}
	after := makeTestConn("tc1", "Tag Server")
	after.Tags = []string{"prod", "web", "critical"}

	s.Push(Operation{
		Type:   UndoOpTagChange,
		ConnID: "tc1",
		Name:   before.Name,
		Before: before,
		After:  after,
	})

	op, ok := s.Undo()
	if !ok || op.Type != UndoOpTagChange {
		t.Fatal("expected UndoOpTagChange undo")
	}
	if len(op.Before.Tags) != 2 {
		t.Fatalf("expected 2 before tags, got %d", len(op.Before.Tags))
	}
	if len(op.After.Tags) != 3 {
		t.Fatalf("expected 3 after tags, got %d", len(op.After.Tags))
	}
}
