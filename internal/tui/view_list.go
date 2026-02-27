package tui

import (
	"fmt"
	"strings"

	"github.com/dr4zz/nexus/internal/termcap"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/key"
	"github.com/dr4zz/nexus/internal/health"
	"github.com/dr4zz/nexus/internal/launcher"
)

func (a App) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()

	// --- Bracket-pending state handling (two-key sequences: ]x / [x) ---
	if a.bracketPending != 0 {
		pending := a.bracketPending
		a.bracketPending = 0
		return a.handleBracketMotion(pending, k)
	}

	// --- Bracket: initiate two-key sequences ---
	switch k {
	case "]":
		a.bracketPending = ']'
		return a, nil
	case "[":
		a.bracketPending = '['
		return a, nil
	}


	// Handle Visual mode keys first if visual mode is active.
	if a.selectionSet.VisualActive() {
		return a.handleVisualKey(msg)
	}

	// --- Bulk operation intercept (Normal mode with a picker selection) ---
	// When items are selected via Tab/Ctrl+A and no motion sequence is pending,
	// the d and y keys operate on the full selection instead of the single cursor row.
	if a.selectionSet.HasSelection() && !a.list.table.motion.Pending() {
		switch {
		case key.Matches(msg, a.keys.Delete):
			return a.bulkDeleteSelected()
		case key.Matches(msg, a.keys.Yank):
			return a.bulkYankSelected()
		}
	}

	motion := a.list.table.motion

	// Capture the pending operator before the engine processes this key,
	// because HandleKey resets the engine upon producing a result.
	pendingOp := motion.PendingOperator()

	// Pass key events to the motion engine first.
	result := motion.HandleKey(msg, a.list.table.cursor, len(a.list.table.rows), a.list.table.height)

	if result != nil {
		// Store the operator that was active when this result was produced.
		a.list.table.lastOperator = pendingOp
		// Motion sequence completed — apply the result.
		return a.applyMotionResult(result)
	}

	// If the engine is now pending (accumulating a sequence), consume the key.
	if motion.Pending() {
		return a, nil
	}

	// --- App-level key handlers (non-motion keys) ---
	switch {
	case k == "ctrl+a":
		// Toggle select-all / deselect-all for the visible (filtered) rows.
		rowCount := len(a.list.table.rows)
		if a.selectionSet.HasSelection() {
			a.selectionSet.DeselectAll()
			a.list.table.selectionSet = nil
		} else {
			a.selectionSet.SelectAll(rowCount)
			if a.selectionSet.HasSelection() {
				a.list.table.selectionSet = &a.selectionSet
			}
		}
		a.statusBar.visualCount = a.selectionSet.Count()
		return a, nil

	case key.Matches(msg, a.keys.Escape):
		// In Normal mode with a picker selection (but not visual mode), clear the selection.
		if a.selectionSet.HasSelection() && !a.selectionSet.VisualActive() {
			a.selectionSet.DeselectAll()
			a.list.table.selectionSet = nil
			a.statusBar.visualCount = 0
			return a, nil
		}
		// Otherwise fall through (no further esc handling at list level in normal mode).

	case k == "tab": // Toggle selection on current row
		if len(a.list.table.rows) == 0 {
			return a, nil
		}
		cursor := a.list.table.Cursor()
		a.selectionSet.Toggle(cursor)
		if a.selectionSet.HasSelection() {
			a.list.table.selectionSet = &a.selectionSet
		} else {
			a.list.table.selectionSet = nil
		}
		a.statusBar.visualCount = a.selectionSet.Count()
		return a, nil

	case k == "shift+tab": // Deselect current row (idempotent; no-op on empty list)
		if len(a.list.table.rows) == 0 {
			return a, nil
		}
		cursor := a.list.table.Cursor()
		a.selectionSet.Deselect(cursor)
		if a.selectionSet.HasSelection() {
			a.list.table.selectionSet = &a.selectionSet
		} else {
			a.list.table.selectionSet = nil
		}
		a.statusBar.visualCount = a.selectionSet.Count()
		return a, nil

	case k == " ": // Space = leader key
		if !a.leader.active {
			cmd := a.leader.activate()
			return a, cmd
		}
	case key.Matches(msg, a.keys.Quit):
		a.confirmQuit()
		return a, nil
	case k == "/":
		a.search.activate(SearchForward)
		a.search.width = a.width
		a.list.table.highlightText = ""
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.setMode(ModeInsert)
	case k == "?":
		a.search.activate(SearchReverse)
		a.search.width = a.width
		a.list.table.highlightText = ""
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.setMode(ModeInsert)
	case k == "n":
		return a.searchNextMatch(true)
	case k == "*":
		return a.searchUnderCursor(SearchForward)
	case k == "#":
		return a.searchUnderCursor(SearchReverse)
	case key.Matches(msg, a.keys.Command):
		a.command.activate()
		a.mode = ModeCommand
		a.statusBar.mode = ModeCommand
		return a, a.command.input.Focus()
	case key.Matches(msg, a.keys.Enter):
		return a.connectSelected()
	case key.Matches(msg, a.keys.Add):
		a.form.startAdd(a.cfg.GroupNames(), a.profileNames())
		a.form.width = a.width
		a.form.height = a.contentHeight()
		a.pushView(viewForm)
		a.help.view = "form"
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.form.form.Init()
	case key.Matches(msg, a.keys.Edit):
		if c := a.list.selectedConnection(); c != nil {
			conn := *c
			a.inheritGroupProfile(&conn)
			a.form.startEdit(conn, a.cfg.GroupNames(), a.profileNames())
			a.form.width = a.width
			a.form.height = a.contentHeight()
			a.pushView(viewForm)
			a.help.view = "form"
			a.mode = ModeInsert
			a.statusBar.mode = ModeInsert
			return a, a.form.form.Init()
		}
		return a, nil
	case key.Matches(msg, a.keys.Detail):
		if c := a.list.selectedConnection(); c != nil {
			st := a.list.statuses[c.ID]
			latStr := ""
			if st.status == health.Online || st.status == health.Degraded {
				latStr = st.latency.String()
			}
			a.setDetailConnection(c, st.status, latStr)
			a.detail.setSize(a.width, a.contentHeight())
			a.pushView(viewDetail)
			a.help.view = "detail"
		}
		return a, nil
	case key.Matches(msg, a.keys.Refresh):
		a.log.info("Manual health check refresh")
		a.statusBar.setFlash("Refreshing...", flashInfo)
		return a, tea.Batch(
			a.checker.CheckAll(a.list.healthTargets()),
			scheduleFlashClear(),
		)
	case key.Matches(msg, a.keys.Sessions):
		a.sessionsView.setSessions(a.allSessions())
		a.sessionsView.setSize(a.width, a.contentHeight())
		a.pushView(viewSessions)
		a.help.view = "sessions"
		return a, nil

	case k == "ctrl+l": // View event log
		a.log.setSize(a.width, a.contentHeight())
		a.pushView(viewLog)
		return a, nil

	case k == "V": // Enter line-visual mode
		return a.enterVisualMode()

	case k == "N": // Jump to previous search match (opposite direction of n)
		return a.searchNextMatch(false)
	case k == "S": // Sort by Status
		if idx := a.list.sortKeyToColumnIndex("status"); idx >= 0 {
			a.list.table.CycleSort(idx)
		}
		return a, nil
	case k == "P": // Sort by Protocol
		if idx := a.list.sortKeyToColumnIndex("protocol"); idx >= 0 {
			a.list.table.CycleSort(idx)
		}
		return a, nil

	// Wide mode toggle

	case k == "u": // Undo
		return a.performUndo()
	case k == "ctrl+r": // Redo
		return a.performRedo()
	case k == "ctrl+w":
		a.list.toggleWideMode()
		return a, nil
	case k == "ctrl+p":
		a.finder.SetSize(a.width, a.height)
		a.finder.Activate(PickerConnections, a.cfg.Connections)
		a.mode = ModeInsert
		a.statusBar.mode = ModeInsert
		return a, a.finder.prompt.Focus()
	}

	return a, nil
}

// applyMotionResult handles a completed motion/operator result from the motion engine.
func (a App) applyMotionResult(result *MotionResult) (tea.Model, tea.Cmd) {
	switch result.Action {
	case ActionMove:
		target := result.Target
		// Resolve H/M/L sentinel values
		if target < 0 {
			target = a.list.table.ResolveScreenTarget(target)
		}
		a.list.table.MoveCursor(target)
		a.syncCursorPosition()
		return a, nil

	case ActionPageDown:
		a.list.table.PageDown(result.Count)
		a.syncCursorPosition()
		return a, nil

	case ActionPageUp:
		a.list.table.PageUp(result.Count)
		a.syncCursorPosition()
		return a, nil

	case ActionOpLine:
		// dd = delete, yy = yank — operator was captured before engine reset
		return a.handleOperatorLine(result)

	case ActionOpRange:
		return a.handleOperatorRange(result)
	}
	return a, nil
}

// handleOperatorLine handles dd/yy/cc operator-on-line results.
// The operator type is tracked via table.lastOperator, which is captured
// from the motion engine's pending state before HandleKey resets it.
func (a App) handleOperatorLine(result *MotionResult) (tea.Model, tea.Cmd) {
	op := a.list.table.lastOperator
	switch op {
	case OpDelete:
		if c := a.list.selectedConnection(); c != nil {
			a.confirm.show(
				"Delete connection '"+c.Name+"'?",
				"delete",
				c.ID,
			)
			a.confirm.width = a.width
			a.confirm.height = a.height
		}
	case OpYank:
		return a.yankCommand()
	}
	return a, nil
}

// handleOperatorRange handles operator+motion range results (e.g., dG, ygg).
func (a App) handleOperatorRange(result *MotionResult) (tea.Model, tea.Cmd) {
	op := a.list.table.lastOperator
	switch op {
	case OpDelete:
		// For range delete, still confirm for the selected connection
		if c := a.list.selectedConnection(); c != nil {
			a.confirm.show(
				"Delete connection '"+c.Name+"'?",
				"delete",
				c.ID,
			)
			a.confirm.width = a.width
			a.confirm.height = a.height
		}
	case OpYank:
		return a.yankCommand()
	}
	return a, nil
}

// enterVisualMode switches to Visual mode with the current cursor as anchor.
func (a App) enterVisualMode() (tea.Model, tea.Cmd) {
	cursor := a.list.table.Cursor()
	a.selectionSet.EnterVisual(cursor)
	a.list.table.selectionSet = &a.selectionSet
	a.mode = ModeVisual
	a.statusBar.mode = ModeVisual
	a.statusBar.visualCount = a.selectionSet.Count()
	return a, a.setMode(ModeVisual)
}

// exitVisualMode returns to Normal mode but preserves any picker selection (Tab-selected items).
// The status bar is updated to reflect the remaining selection count.
func (a *App) exitVisualMode() tea.Cmd {
	a.selectionSet.ExitVisual()
	if !a.selectionSet.HasSelection() {
		a.list.table.selectionSet = nil
	}
	a.mode = ModeNormal
	a.statusBar.mode = ModeNormal
	// Show "N selected" in Normal mode if items are still Tab-selected after leaving Visual.
	a.statusBar.visualCount = a.selectionSet.Count()
	return a.setMode(ModeNormal)
}

// clearAllSelection fully resets all selection state: exits Visual mode, deselects all items,
// clears the table's selection pointer, resets mode to Normal, and zeroes the status-bar count.
// Use this after bulk operations where selections must no longer persist.
func (a *App) clearAllSelection() tea.Cmd {
	a.selectionSet.ExitVisual()
	a.selectionSet.DeselectAll()
	a.list.table.selectionSet = nil
	a.statusBar.mode = ModeNormal
	a.statusBar.visualCount = 0
	return a.setMode(ModeNormal)
}

// syncVisualStatus updates the statusbar with the current visual selection count.
func (a *App) syncVisualStatus() {
	a.statusBar.visualCount = a.selectionSet.Count()
}

func (a App) handleVisualKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	cursor := a.list.table.Cursor()
	total := len(a.list.table.rows)

	switch {
	case key.Matches(msg, a.keys.Escape):
		cmd := a.exitVisualMode()
		return a, cmd

	case key.Matches(msg, a.keys.Down):
		// Move cursor down and extend selection range
		newCursor := min(cursor+1, total-1)
		a.list.table.MoveCursor(newCursor)
		a.selectionSet.UpdateRange(newCursor)
		a.list.table.selectionSet = &a.selectionSet
		a.syncVisualStatus()
		a.syncCursorPosition()
		return a, nil

	case key.Matches(msg, a.keys.Up):
		// Move cursor up and extend/contract selection range
		newCursor := max(cursor-1, 0)
		a.list.table.MoveCursor(newCursor)
		a.selectionSet.UpdateRange(newCursor)
		a.list.table.selectionSet = &a.selectionSet
		a.syncVisualStatus()
		a.syncCursorPosition()
		return a, nil

	case k == "G":
		// Jump to last row, extend selection
		a.list.table.MoveCursor(total - 1)
		a.selectionSet.UpdateRange(total - 1)
		a.list.table.selectionSet = &a.selectionSet
		a.syncVisualStatus()
		a.syncCursorPosition()
		return a, nil

	case k == "tab":
		// Cherry-pick / deselect individual items while staying in Visual mode.
		a.selectionSet.Toggle(cursor)
		a.list.table.selectionSet = &a.selectionSet
		a.syncVisualStatus()
		return a, nil

	case k == "ctrl+d":
		// Half-page down; extend selection range addditively.
		pageHalf := max(a.list.table.height/2, 1)
		newCursor := min(cursor+pageHalf, total-1)
		a.list.table.MoveCursor(newCursor)
		a.selectionSet.UpdateRange(newCursor)
		a.list.table.selectionSet = &a.selectionSet
		a.syncVisualStatus()
		a.syncCursorPosition()
		return a, nil

	case k == "ctrl+u":
		// Half-page up; extend selection range additively.
		pageHalf := max(a.list.table.height/2, 1)
		newCursor := max(cursor-pageHalf, 0)
		a.list.table.MoveCursor(newCursor)
		a.selectionSet.UpdateRange(newCursor)
		a.list.table.selectionSet = &a.selectionSet
		a.syncVisualStatus()
		a.syncCursorPosition()
		return a, nil

	case k == "v":
		// Toggle current item in/out of selection (non-contiguous)
		a.selectionSet.Toggle(cursor)
		a.list.table.selectionSet = &a.selectionSet
		a.syncVisualStatus()
		return a, nil

	case key.Matches(msg, a.keys.Delete):
		// Delete all selected connections (with confirmation)
		count := a.selectionSet.Count()
		if count == 0 {
			return a, nil
		}
		ids := a.selectionSet.SelectedIDs(a.list.table.rows)
		if len(ids) == 0 {
			return a, nil
		}
		// Store selected IDs for the confirm handler
		prompt := fmt.Sprintf("Delete %d selected connections?", count)
		// Join IDs with comma for the confirm action
		idStr := strings.Join(ids, ",")
		a.confirm.show(prompt, "delete-visual", idStr)
		a.confirm.width = a.width
		a.confirm.height = a.height
		return a, nil

	case key.Matches(msg, a.keys.Yank):
		// Yank all selected connection details to clipboard
		return a.yankVisualSelection()
	}

	return a, nil
}

// bulkDeleteSelected shows a bulk-delete confirmation for all Tab-selected connections
// (Normal mode, non-visual). After confirmation the confirm handler calls exitVisualMode
// (via "delete-visual" action) which also calls DeselectAll.
func (a App) bulkDeleteSelected() (tea.Model, tea.Cmd) {
	count := a.selectionSet.Count()
	if count == 0 {
		return a, nil
	}
	ids := a.selectionSet.SelectedIDs(a.list.table.rows)
	if len(ids) == 0 {
		return a, nil
	}
	prompt := fmt.Sprintf("Delete %d selected connections?", count)
	idStr := strings.Join(ids, ",")
	a.confirm.show(prompt, "delete-visual", idStr)
	a.confirm.width = a.width
	a.confirm.height = a.height
	return a, nil
}

// bulkYankSelected copies commands for all Tab-selected connections to the clipboard
// (Normal mode, non-visual). Clears the selection after yanking.
func (a App) bulkYankSelected() (tea.Model, tea.Cmd) {
	ids := a.selectionSet.SelectedIDs(a.list.table.rows)
	if len(ids) == 0 {
		return a, nil
	}

	var lines []string
	for _, id := range ids {
		c := a.cfg.FindConnection(id)
		if c == nil {
			continue
		}
		l, err := launcher.ForProtocol(c.Protocol)
		if err != nil {
			continue
		}
		lines = append(lines, l.Command(*c))
	}

	if len(lines) == 0 {
		return a, nil
	}

	text := strings.Join(lines, "\n")
	if err := termcap.DefaultClipboard().WriteAll(text); err != nil {
		a.log.error("Clipboard error: %v", err)
		a.statusBar.setFlash("Clipboard error: "+err.Error(), flashError)
	} else {
		a.log.info("Copied %d connection commands to clipboard", len(lines))
		a.statusBar.setFlash(fmt.Sprintf("Copied %d commands", len(lines)), flashInfo)
	}

	// Clear selection after yanking.
	clearCmd := a.clearAllSelection()
	return a, tea.Batch(clearCmd, scheduleFlashClear())
}

// yankVisualSelection copies the details of all visually selected connections to the clipboard.
func (a App) yankVisualSelection() (tea.Model, tea.Cmd) {
	ids := a.selectionSet.SelectedIDs(a.list.table.rows)
	if len(ids) == 0 {
		return a, nil
	}

	var lines []string
	for _, id := range ids {
		c := a.cfg.FindConnection(id)
		if c == nil {
			continue
		}
		l, err := launcher.ForProtocol(c.Protocol)
		if err != nil {
			continue
		}
		lines = append(lines, l.Command(*c))
	}

	if len(lines) == 0 {
		return a, nil
	}

	text := strings.Join(lines, "\n")
	if err := termcap.DefaultClipboard().WriteAll(text); err != nil {
		a.log.error("Clipboard error: %v", err)
		a.statusBar.setFlash("Clipboard error: "+err.Error(), flashError)
	} else {
		a.log.info("Copied %d connection commands to clipboard", len(lines))
		a.statusBar.setFlash(fmt.Sprintf("Copied %d commands", len(lines)), flashInfo)
	}

	// Clear all selection after yanking (spec: selections must clear after bulk operations).
	clearCmd := a.clearAllSelection()
	return a, tea.Batch(clearCmd, scheduleFlashClear())
}

// handleBracketMotion processes the second key in a bracket motion sequence.
// pending is ']' or '[', key is the follow-up character (c/g/e/s/f).
func (a App) handleBracketMotion(pending rune, key string) (tea.Model, tea.Cmd) {
	if len(key) != 1 {
		return a, nil // invalid follow-up, cancel silently
	}

	conns := a.list.filtered
	cursor := a.list.table.Cursor()
	var target int

	// Build a health status map for error navigation.
	healthStatuses := make(map[string]health.Status, len(a.list.statuses))
	for id, cs := range a.list.statuses {
		healthStatuses[id] = cs.status
	}

	switch key {
	case "c":
		if pending == ']' {
			target = a.bracketNav.NextConnection(cursor, conns)
		} else {
			target = a.bracketNav.PrevConnection(cursor, conns)
		}
	case "g":
		if pending == ']' {
			target = a.bracketNav.NextGroup(cursor, conns)
		} else {
			target = a.bracketNav.PrevGroup(cursor, conns)
		}
	case "e":
		if pending == ']' {
			target = a.bracketNav.NextError(cursor, conns, healthStatuses)
		} else {
			target = a.bracketNav.PrevError(cursor, conns, healthStatuses)
		}
	case "s":
		sessions := a.sessions.All()
		if pending == ']' {
			target = a.bracketNav.NextSession(cursor, conns, sessions)
		} else {
			target = a.bracketNav.PrevSession(cursor, conns, sessions)
		}
	case "f":
		if pending == ']' {
			target = a.bracketNav.NextFavorite(cursor, conns)
		} else {
			target = a.bracketNav.PrevFavorite(cursor, conns)
		}
	default:
		return a, nil // unknown follow-up, cancel silently
	}

	if target != cursor {
		a.list.table.MoveCursor(target)
		a.syncCursorPosition()
	}
	return a, nil
}
