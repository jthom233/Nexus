package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// performUndo undoes the most recent destructive operation.
func (a App) performUndo() (tea.Model, tea.Cmd) {
	op, ok := a.undoStack.Undo()
	if !ok {
		a.statusBar.setFlash("Nothing to undo", flashError)
		return a, scheduleFlashClear()
	}

	var flashMsg string
	switch op.Type {
	case UndoOpAdd:
		// Undo add = delete the connection that was added.
		if err := a.cfg.DeleteConnection(op.After.ID); err != nil {
			a.log.error("Undo add failed: %v", err)
			a.statusBar.setFlash("Undo failed: "+err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		flashMsg = fmt.Sprintf("Undo: removed '%s'", op.Name)

	case UndoOpDelete:
		// Undo delete = re-insert the connection at its original index.
		if err := a.cfg.InsertConnectionAt(op.Before, op.Index); err != nil {
			a.log.error("Undo delete failed: %v", err)
			a.statusBar.setFlash("Undo failed: "+err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		flashMsg = fmt.Sprintf("Undo: restored '%s'", op.Name)

	case UndoOpEdit:
		// Undo edit = restore the before snapshot.
		if err := a.cfg.UpdateConnection(op.Before); err != nil {
			a.log.error("Undo edit failed: %v", err)
			a.statusBar.setFlash("Undo failed: "+err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		flashMsg = fmt.Sprintf("Undo: reverted '%s'", op.Name)

	case UndoOpTagChange:
		// Undo tag change = restore the before snapshot.
		if err := a.cfg.UpdateConnection(op.Before); err != nil {
			a.log.error("Undo tag change failed: %v", err)
			a.statusBar.setFlash("Undo failed: "+err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		flashMsg = fmt.Sprintf("Undo: restored tags on '%s'", op.Name)

	case UndoOpBatch:
		// Undo batch = undo all children in reverse order.
		for i := len(op.Children) - 1; i >= 0; i-- {
			if err := a.undoSingleOp(op.Children[i]); err != nil {
				a.log.error("Undo batch child failed: %v", err)
			}
		}
		flashMsg = fmt.Sprintf("Undo: %s (%d items)", op.Name, len(op.Children))
	}

	a.log.info("%s", flashMsg)
	a.statusBar.setFlash(flashMsg, flashInfo)
	a.list.filtered = a.list.groupFilteredConns()
	a.list.rebuildTable()
	total, online, offline := a.list.countsByStatus()
	a.statusBar.total = total
	a.statusBar.online = online
	a.statusBar.offline = offline
	a.syncHeaderView()
	a.syncCursorPosition()

	return a, tea.Batch(
		a.checker.CheckAll(a.list.healthTargets()),
		scheduleFlashClear(),
	)
}

// undoSingleOp applies undo logic for a single (non-batch) operation.
func (a *App) undoSingleOp(op Operation) error {
	switch op.Type {
	case UndoOpAdd:
		return a.cfg.DeleteConnection(op.After.ID)
	case UndoOpDelete:
		return a.cfg.InsertConnectionAt(op.Before, op.Index)
	case UndoOpEdit, UndoOpTagChange:
		return a.cfg.UpdateConnection(op.Before)
	}
	return nil
}

// redoSingleOp applies redo logic for a single (non-batch) operation.
func (a *App) redoSingleOp(op Operation) error {
	switch op.Type {
	case UndoOpAdd:
		return a.cfg.AddConnection(op.After)
	case UndoOpDelete:
		return a.cfg.DeleteConnection(op.Before.ID)
	case UndoOpEdit, UndoOpTagChange:
		return a.cfg.UpdateConnection(op.After)
	}
	return nil
}

// performRedo re-applies the most recently undone operation.
func (a App) performRedo() (tea.Model, tea.Cmd) {
	op, ok := a.undoStack.Redo()
	if !ok {
		a.statusBar.setFlash("Nothing to redo", flashError)
		return a, scheduleFlashClear()
	}

	var flashMsg string
	switch op.Type {
	case UndoOpAdd:
		// Redo add = re-add the connection.
		if err := a.cfg.AddConnection(op.After); err != nil {
			a.log.error("Redo add failed: %v", err)
			a.statusBar.setFlash("Redo failed: "+err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		flashMsg = fmt.Sprintf("Redo: added '%s'", op.Name)

	case UndoOpDelete:
		// Redo delete = delete the connection again.
		if err := a.cfg.DeleteConnection(op.Before.ID); err != nil {
			a.log.error("Redo delete failed: %v", err)
			a.statusBar.setFlash("Redo failed: "+err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		flashMsg = fmt.Sprintf("Redo: deleted '%s'", op.Name)

	case UndoOpEdit:
		// Redo edit = re-apply the after snapshot.
		if err := a.cfg.UpdateConnection(op.After); err != nil {
			a.log.error("Redo edit failed: %v", err)
			a.statusBar.setFlash("Redo failed: "+err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		flashMsg = fmt.Sprintf("Redo: re-applied edit on '%s'", op.Name)

	case UndoOpTagChange:
		// Redo tag change = re-apply the after snapshot.
		if err := a.cfg.UpdateConnection(op.After); err != nil {
			a.log.error("Redo tag change failed: %v", err)
			a.statusBar.setFlash("Redo failed: "+err.Error(), flashError)
			return a, scheduleFlashClear()
		}
		flashMsg = fmt.Sprintf("Redo: re-applied tags on '%s'", op.Name)

	case UndoOpBatch:
		// Redo batch = redo all children in forward order.
		for _, child := range op.Children {
			if err := a.redoSingleOp(child); err != nil {
				a.log.error("Redo batch child failed: %v", err)
			}
		}
		flashMsg = fmt.Sprintf("Redo: %s (%d items)", op.Name, len(op.Children))
	}

	a.log.info("%s", flashMsg)
	a.statusBar.setFlash(flashMsg, flashInfo)
	a.list.filtered = a.list.groupFilteredConns()
	a.list.rebuildTable()
	total, online, offline := a.list.countsByStatus()
	a.statusBar.total = total
	a.statusBar.online = online
	a.statusBar.offline = offline
	a.syncHeaderView()
	a.syncCursorPosition()

	return a, tea.Batch(
		a.checker.CheckAll(a.list.healthTargets()),
		scheduleFlashClear(),
	)
}
