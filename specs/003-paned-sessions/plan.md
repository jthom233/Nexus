# Implementation Plan: Paned Sessions with Broadcast

**Branch**: `003-paned-sessions` | **Date**: 2026-02-16 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/003-paned-sessions/spec.md`

## Summary

Add tmux-style paned sessions to the Nexus TUI, allowing users to split the terminal into multiple panes each running an independent connection session, with an all-or-nothing broadcast mode to send keystrokes to all panes simultaneously. The core architectural change is refactoring SSH sessions from blocking `tea.Exec()` terminal takeover to background goroutines with channel-based I/O, enabling the TUI to composite multiple session outputs into a split layout using lipgloss.

## Technical Context

**Language/Version**: Go 1.25
**Primary Dependencies**: Bubbletea v1.3.10, Bubbles v1.0.0, Lipgloss v1.1.0, charmbracelet/huh v0.8.0, golang.org/x/term
**Storage**: YAML config (`~/.config/nexus/config.yaml`), SQLite store
**Testing**: `go test ./...`
**Target Platform**: Linux (terminal-based TUI)
**Project Type**: Single Go project with TUI + GUI binaries
**Performance Goals**: Pane operations with no visible rendering delay; broadcast input delivered within same input cycle
**Constraints**: 256KB replay buffer per session; minimum pane size ~20 cols x 5 rows; no hard pane count limit
**Scale/Scope**: Typical use: 2-8 concurrent panes; app.go is ~3000 lines

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Constitution is a blank template — no gates defined. PASSED (no violations possible).

## Project Structure

### Documentation (this feature)

```text
specs/003-paned-sessions/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (internal API contracts)
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
internal/
├── tui/
│   ├── app.go           # MODIFY: Add viewPaneLayout, paneLayout field, routing
│   ├── panes.go         # NEW: PaneLayoutModel, Pane, split tree, rendering
│   ├── pane_render.go   # NEW: Per-pane terminal output rendering
│   ├── leader.go        # MODIFY: Add Space w group
│   └── keys.go          # MODIFY: Add pane-related key bindings (if needed)
├── session/
│   ├── managed.go       # MODIFY: Add channel-based I/O, background mode
│   └── vterm.go         # NEW: Virtual terminal buffer for ANSI interpretation
└── ...
```

**Structure Decision**: This feature extends the existing Go project structure. New files are added under `internal/tui/` for pane layout management and `internal/session/` for the refactored session I/O model. No new top-level directories needed.

## Complexity Tracking

> No constitution violations to justify.
