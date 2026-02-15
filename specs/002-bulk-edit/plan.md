# Implementation Plan: Bulk Edit Operations

**Branch**: `002-bulk-edit` | **Date**: 2026-02-14 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/002-bulk-edit/spec.md`

## Summary

Add batch undo support to the undo system (`UndoOpBatch` type containing child operations), wire `ExecuteBulkMove` to the `:mv`/`:move` command handler when visual mode is active, and retrofit existing bulk tag/delete operations to use batch undo entries. Net result: all bulk operations are undoable in a single keystroke.

## Technical Context

**Language/Version**: Go 1.21+
**Primary Dependencies**: charmbracelet/bubbletea (TUI framework), charmbracelet/huh (forms), gopkg.in/yaml.v3 (config)
**Storage**: YAML config file (`~/.config/nexus/config.yaml`)
**Testing**: `go test`, `go vet`, `go build`
**Target Platform**: Linux (primary), cross-platform terminal
**Project Type**: Single Go project
**Performance Goals**: N/A (undo stack operations, not performance-critical)
**Constraints**: Must not break existing single-item undo/redo behavior
**Scale/Scope**: ~3 files modified, ~150 lines added, ~40 lines modified

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Constitution not yet customized for this project (placeholder template). No gates to enforce. Proceeding with standard engineering practices.

**Post-design re-check**: Plan adds `UndoOpBatch` type and `Children` field to Operation struct — backward-compatible addition. Existing undo/redo paths remain unchanged for non-batch operations. No architectural concerns.

## Project Structure

### Documentation (this feature)

```text
specs/002-bulk-edit/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (files to modify)

```text
internal/
├── tui/
│   ├── undo.go          # Add UndoOpBatch type, Children field, batch-aware Undo/Redo
│   ├── undo_test.go     # Add tests for batch undo/redo
│   ├── app.go           # Wire bulk move to :mv command, retrofit tag/delete handlers
│   └── bulk.go          # No changes needed (ExecuteBulkMove already exists)
```

**Structure Decision**: Existing Go project structure. All changes within `internal/tui/`. No new files created.

## Complexity Tracking

No constitution violations. No complexity justifications needed.
