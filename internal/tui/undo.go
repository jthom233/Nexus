package tui

import "github.com/dr4zz/nexus/internal/config"

// OpType represents the kind of undoable operation.
type OpType int

const (
	UndoOpAdd       OpType = iota // A connection was added
	UndoOpDelete                  // A connection was deleted
	UndoOpEdit                    // A connection was edited
	UndoOpTagChange               // Tags were changed on a connection
	UndoOpBatch                   // Batch operation containing multiple child operations
)

// Operation records a single undoable action.
type Operation struct {
	Type   OpType
	ConnID string            // The connection ID this operation relates to
	Name   string            // Human-readable name (for flash messages)
	Index  int               // Original index in cfg.Connections (used for UndoOpDelete reinsertion)
	Before   config.Connection // Snapshot before the operation (UndoOpDelete, UndoOpEdit, UndoOpTagChange)
	After    config.Connection // Snapshot after the operation (UndoOpAdd, UndoOpEdit, UndoOpTagChange)
	Children []Operation       // Child operations for batch (nil for non-batch)
}

const maxUndoDepth = 50

// UndoStack manages undo/redo history for destructive operations.
type UndoStack struct {
	undoStack []Operation
	redoStack []Operation
}

// NewUndoStack creates an empty undo/redo stack.
func NewUndoStack() *UndoStack {
	return &UndoStack{}
}

// Push records a new operation on the undo stack and clears the redo stack.
func (u *UndoStack) Push(op Operation) {
	u.undoStack = append(u.undoStack, op)
	if len(u.undoStack) > maxUndoDepth {
		// Trim the oldest entries.
		excess := len(u.undoStack) - maxUndoDepth
		u.undoStack = u.undoStack[excess:]
	}
	// Any new operation invalidates the redo history.
	u.redoStack = nil
}

// PushBatch records a batch of operations as a single undo entry.
func (u *UndoStack) PushBatch(name string, children []Operation) {
	u.Push(Operation{
		Type:     UndoOpBatch,
		Name:     name,
		Children: children,
	})
}

// Undo pops the most recent operation from the undo stack,
// pushes its reverse onto the redo stack, and returns it.
func (u *UndoStack) Undo() (Operation, bool) {
	if len(u.undoStack) == 0 {
		return Operation{}, false
	}
	op := u.undoStack[len(u.undoStack)-1]
	u.undoStack = u.undoStack[:len(u.undoStack)-1]
	u.redoStack = append(u.redoStack, op)
	return op, true
}

// Redo pops the most recent operation from the redo stack,
// pushes it back onto the undo stack, and returns it.
func (u *UndoStack) Redo() (Operation, bool) {
	if len(u.redoStack) == 0 {
		return Operation{}, false
	}
	op := u.redoStack[len(u.redoStack)-1]
	u.redoStack = u.redoStack[:len(u.redoStack)-1]
	u.undoStack = append(u.undoStack, op)
	return op, true
}

// CanUndo reports whether there is an operation to undo.
func (u *UndoStack) CanUndo() bool {
	return len(u.undoStack) > 0
}

// CanRedo reports whether there is an operation to redo.
func (u *UndoStack) CanRedo() bool {
	return len(u.redoStack) > 0
}

// UndoLen returns the number of operations on the undo stack (for testing).
func (u *UndoStack) UndoLen() int {
	return len(u.undoStack)
}

// RedoLen returns the number of operations on the redo stack (for testing).
func (u *UndoStack) RedoLen() int {
	return len(u.redoStack)
}
