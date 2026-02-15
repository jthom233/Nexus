# Research: TUI Command & Keybinding Cleanup

## Decision 1: Group Storage Model

**Decision**: Use existing `Config.Groups []Group` slice for explicit group management.

**Rationale**: Config already stores groups as a separate `[]Group` slice (config.go:97) with `Name` and `Color` fields. Connections reference groups by string. This model supports explicit group lifecycle (create before use, persist empty groups) without schema changes.

**Alternatives considered**:
- Derive groups from connections (implicit) — rejected: can't have empty groups, can't pre-create groups
- Separate groups config file — rejected: over-engineering, groups are lightweight

## Decision 2: Group Command Behavior

**Decision**: `:mkdir` creates explicit group entries, `:rmdir` only deletes if no connections reference it, `:mv` auto-creates target group if missing.

**Rationale**: Explicit groups match user mental model (mkdir/rmdir from filesystem). Auto-creation on `:mv` reduces friction — users shouldn't need two commands to move a connection to a new group. Safety check on `:rmdir` prevents orphaning connections.

**Alternatives considered**:
- Require group to exist before `:mv` — rejected: too much friction for simple reorganization
- Allow `:rmdir` with force flag to reassign connections — rejected: scope creep, edit connections individually

## Decision 3: Removal Approach for Dead Code

**Decision**: Delete files entirely (marks.go, tree.go), remove all references in app.go, remove entries from command registry and leader menu in single atomic changes per file.

**Rationale**: Clean deletion is safer than commenting out. Dead code removal should be complete — no partial references left. Each file change should compile independently to allow incremental verification.

**Alternatives considered**:
- Feature-flag the removed code — rejected: code is genuinely unused/broken, no future use planned
- Move to `internal/tui/deprecated/` — rejected: dead code should be deleted, git preserves history

## Decision 4: Leader Menu Organization

**Decision**: Restructure leader menu to match command categories. Add new groups where needed, remove stub items, wire missing actions to existing command handlers.

**Rationale**: Leader menu should be 1:1 with available commands. Every `:` command should have a leader path. Organization should be intuitive (neovim which-key style).

**New leader structure** (post-cleanup):

| Group | Key | Items |
|-------|-----|-------|
| Connections | c | connect, quick-connect, add, edit, delete |
| Find | f | fuzzy-find, sessions, by-tag, by-group, recent, frequent, favorites, all, toggle-fav |
| Sessions | s | list, kill, kill-all |
| Groups | g | create, delete, move, filter |
| Import | i | ssh, csv, json |
| Export | e | all, selection, group |
| Tags | t | filter, add, remove |
| View | v | table, detail, wide, log |
| Sort | x | name, host, group, protocol, status, latency, fav |
| Health | h | check-all, pulse |
| Password | p | show, copy |
| Data | d | note, field, import-ssh |
| Options | o | keybindings |
| Help | ? | help |

**Changes from current**:
- Connections group: added add, edit, delete (were missing)
- Find group: added frequent, favorites, all (were missing)
- Groups group: replaced stubs with create/delete/move/filter (all functional)
- View group: removed tree (dead)
- Health group: removed check-selected stub
- Password group: keep copy-password (implement or remove)
- Options group: removed theme (stub)
- Data group: new group for note/field/import-ssh (were unreachable from leader)

## Decision 5: Undo Integration for Group Operations

**Decision**: Register `:mv` operations with the undo system using existing `UndoOpEdit` type (connection edit). `:mkdir`/`:rmdir` are not undoable (matches filesystem semantics — mkdir/rmdir are not undoable in shells either).

**Rationale**: `:mv` changes a connection's Group field, which is semantically an edit. Existing undo infrastructure handles connection edits. Group creation/deletion are structural changes that don't fit the connection-centric undo model.

**Alternatives considered**:
- Add new UndoOp types for group operations — rejected: over-engineering for rare operations
- Make all group operations undoable — rejected: complexity not justified

## Decision 6: copy-password and check-selected Stubs

**Decision**: Remove `check-selected` from leader menu (health checking is already available via `r` key and `:pulse`). Keep `copy-password` but implement it (clipboard copy using existing `atotto/clipboard` dependency from GUI module).

**Rationale**: check-selected is redundant. copy-password is genuinely useful — users need to copy credentials without showing them on screen.

**Alternatives considered**:
- Keep both as stubs — rejected: violates "no stubs" principle
- Remove both — rejected: copy-password has clear user value
