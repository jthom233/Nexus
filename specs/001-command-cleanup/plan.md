# Implementation Plan: TUI Command & Keybinding Cleanup

**Branch**: `001-command-cleanup` | **Date**: 2026-02-14 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/001-command-cleanup/spec.md`

## Summary

Remove 11 dead/stub commands from the registry, delete marks system (~115 lines), tree model (~440 lines), and change operator. Implement 3 group management commands (`:mkdir`, `:rmdir`, `:mv`) backed by new Config methods. Expand the leader menu to cover all working commands with no stubs.

## Technical Context

**Language/Version**: Go 1.21+
**Primary Dependencies**: charmbracelet/bubbletea (TUI framework), charmbracelet/huh (forms), gopkg.in/yaml.v3 (config)
**Storage**: YAML config file (`~/.config/nexus/config.yaml`)
**Testing**: `go test`, `go vet`, `go build`
**Target Platform**: Linux (primary), cross-platform terminal
**Project Type**: Single Go project
**Performance Goals**: N/A (TUI cleanup, no performance-critical paths)
**Constraints**: Must not break existing keybindings or commands that work correctly
**Scale/Scope**: ~8 files modified, ~2 files deleted, ~600 lines removed, ~200 lines added

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Constitution not yet customized for this project (placeholder template). No gates to enforce. Proceeding with standard engineering practices: no new dependencies, backward-compatible config changes, clean compilation.

**Post-design re-check**: Plan adds `AddGroup`/`DeleteGroup` methods to Config — minimal additions following existing patterns (`AddConnection`/`DeleteConnection`). No architectural concerns.

## Project Structure

### Documentation (this feature)

```text
specs/001-command-cleanup/
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
├── config/
│   └── config.go           # Add group management methods (AddGroup, DeleteGroup, FindGroup, HasConnectionsInGroup)
├── tui/
│   ├── app.go              # Remove marks/tree/fold refs, add group command handlers, update leader dispatch
│   ├── command.go           # Remove 11 stub commands from registry
│   ├── leader.go            # Restructure leader groups, add missing actions
│   ├── motion.go            # Remove OpChange operator
│   ├── marks.go             # DELETE entire file
│   └── tree.go              # DELETE entire file
```

**Structure Decision**: Existing Go project structure. All changes are within `internal/tui/` and `internal/config/`. No new files created — only modifications and deletions.

## Complexity Tracking

No constitution violations. No complexity justifications needed.
