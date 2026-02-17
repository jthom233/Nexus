# Research: Paned Sessions with Broadcast

**Feature**: 003-paned-sessions
**Date**: 2026-02-16

## R1: Session Execution Model — tea.Exec vs Background Goroutines

**Decision**: Refactor from `tea.Exec()` to background goroutines with channel-based I/O.

**Rationale**: `tea.Exec()` surrenders terminal control entirely — the TUI freezes, no rendering or message handling occurs. This is fundamentally incompatible with paned sessions where the TUI must remain active to composite multiple outputs. The existing ManagedSession already runs background goroutines (`readOutput()`, `readStderr()`, `waitDone()`) that persist across detach/reattach cycles. The refactoring extends this pattern: sessions always run in background, I/O is routed through channels, and the `attachLoop()` raw-mode terminal takeover becomes optional (single-session legacy mode only).

**Alternatives considered**:
- **Keep tea.Exec for single pane, custom for multi**: Adds two code paths, doubles maintenance burden. Rejected.
- **PTY pairs per session**: Each session gets a pseudo-terminal. Cleaner separation but adds OS-level complexity and dependency on pty allocation. Deferred — may be needed later for full ANSI compatibility but not for MVP.

## R2: Terminal Output Rendering in Panes

**Decision**: Use a lightweight virtual terminal (VTerm) buffer per session to interpret ANSI escape sequences and produce renderable cell grids.

**Rationale**: SSH sessions emit raw ANSI escape sequences (cursor movement, colors, screen clearing, scrolling regions). Simply dumping this output into a lipgloss-rendered sub-region would corrupt the layout. A VTerm buffer interprets these sequences into a 2D cell grid (character + style per cell) that can be rendered cleanly within a bounded pane region. The `replayBuffer` (256KB) provides history that can bootstrap the VTerm on session start.

**Alternatives considered**:
- **Raw output with escape filtering**: Strip/ignore cursor positioning. Too lossy — breaks vim, htop, and any full-screen TUI running inside the session. Rejected.
- **Go VTE library (github.com/danielgatis/go-vte)**: Low-level parser only, no screen buffer management. Would need to build the cell grid on top. Partial solution.
- **Custom cell-grid VTerm**: Build a minimal terminal emulator that tracks cursor, handles basic CSI sequences (SGR colors, cursor move, erase, scroll). Best fit for MVP — scope can be bounded to common sequences.

## R3: Input Routing and Broadcast Architecture

**Decision**: Bubbletea's `tea.KeyMsg` is routed by the pane layout model to the active pane's session stdin channel. In broadcast mode, the same input is fanned out to all panes with active sessions.

**Rationale**: Bubbletea already captures all keyboard input as `tea.KeyMsg`. The current `attachLoop()` bypasses this by reading stdin directly in a goroutine. In the new architecture, the TUI remains in control of stdin (via Bubbletea's event loop), and the pane layout model decides where to route each keystroke. This is simpler and avoids stdin contention.

**Alternatives considered**:
- **Raw stdin multiplexing outside Bubbletea**: Intercept stdin before Bubbletea. Fragile, conflicts with Bubbletea's own input handling. Rejected.
- **tea.KeyMsg → byte conversion**: Convert Bubbletea key messages back to terminal bytes for session stdin. This is the chosen approach — straightforward for printable characters and common control sequences.

## R4: Pane Layout Data Structure

**Decision**: Binary split tree where each internal node is a split (horizontal/vertical with ratio) and each leaf is a pane.

**Rationale**: A binary tree naturally represents recursive splitting (split a pane → two children). It supports arbitrary nesting, proportional resize (adjust split ratio at any node), and clean recursive rendering via lipgloss `JoinHorizontal`/`JoinVertical`. tmux and vim both use this model internally.

**Alternatives considered**:
- **Flat grid layout (columns only)**: Simpler but limits layout flexibility. The Zellij "Stacks and Stages" creative concept uses this. Could be an MVP simplification, but the tree is not much harder and is more extensible.
- **Tiling window manager model**: Auto-arranging tiles. Over-engineered for the use case — users expect explicit splits.

## R5: Leader Key Integration (`Space w`)

**Decision**: Add a new `w` (Window) leader group with sub-keys for all pane operations.

**Rationale**: The existing leader key system in `leader.go` supports arbitrary groups via the `leaderGroups()` function. Adding `Space w` follows the established pattern exactly: define a `LeaderGroup` with `Key: "w"` and `Items` for each pane action. Actions are dispatched through `executeLeaderAction()` in app.go. No architectural changes needed — pure additive.

**Key mappings**:
- `Space w v` — vertical split
- `Space w s` — horizontal split
- `Space w h/j/k/l` — focus left/down/up/right pane
- `Space w c` — close pane
- `Space w b` — toggle broadcast mode
- `Space w =` — equalize pane sizes
- `Space w z` — zoom (maximize/restore) active pane
- `Space w 1/2/3/4` — preset layouts

## R6: Backward Compatibility — Single Session Mode

**Decision**: When only one pane exists, the UX is identical to the current single-session workflow. `tea.Exec()` path is preserved as a fallback for non-pane connections.

**Rationale**: SC-006 requires existing single-session workflows to function unchanged. The simplest approach: if a user connects to a session without being in pane mode, use the existing `tea.Exec()` path. Pane mode is entered explicitly via `Space w v/s` or preset layouts. This avoids breaking the proven detach/reattach flow for users who don't want panes.

**Alternatives considered**:
- **Always use pane mode (single pane = full screen)**: Cleaner architecture but risks regressions in the existing flow. Deferred to a future release once pane rendering is proven stable.

## R7: Resize Coordination

**Decision**: Disable ManagedSession's 250ms terminal polling. Pane layout model calculates per-pane dimensions on `WindowSizeMsg` and pushes `session.WindowChange(rows, cols)` directly.

**Rationale**: The current resize watcher polls `term.GetSize()` every 250ms and sends SSH window change requests. In pane mode, each session sees a subset of the terminal, not the full dimensions. The pane layout model is the single source of truth for pane dimensions and must coordinate resize signals to avoid conflicting size reports.
