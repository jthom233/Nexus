# Tasks: Paned Sessions with Broadcast

**Input**: Design documents from `/specs/003-paned-sessions/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: Verify build and prepare new file scaffolds

- [x] T001 Verify existing project builds cleanly with `go build ./cmd/nexus/` and `go test ./...`
- [x] T002 [P] Create scaffold file `internal/session/vterm.go` with package declaration, VTermBuffer struct, Cell struct, CellStyle struct, and constructor stubs per contracts/vterm.md
- [x] T003 [P] Create scaffold file `internal/tui/panes.go` with package declaration, Pane struct, SplitNode struct, PaneLayoutModel struct, PaneState enum, Direction enum, and constructor stubs per contracts/pane-layout.md and data-model.md

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**CRITICAL**: No user story work can begin until this phase is complete

### VTermBuffer — ANSI Terminal Emulator

- [x] T004 [P] Implement VTermBuffer cell grid initialization, Clear(), and Resize() in `internal/session/vterm.go` — allocate [][]Cell grid, fill with spaces, handle width/height changes preserving visible content per contracts/vterm.md
- [x] T005 [P] Implement VTermBuffer ANSI escape sequence parser in `internal/session/vterm.go` — state machine for ESC [ CSI sequences, parse numeric parameters, dispatch to handler functions for cursor movement (CUU/CUD/CUF/CUB/CUP/HVP), erase (ED/EL), and character rendering (printable, newline, CR, tab, backspace)
- [x] T006 Implement VTermBuffer SGR color handling in `internal/session/vterm.go` — parse SGR parameters (0-49 basic, 38;5;N 256-color, 38;2;R;G;B truecolor) and map to CellStyle FG/BG/Bold/Underline/Reverse attributes
- [x] T007 Implement VTermBuffer scroll region support in `internal/session/vterm.go` — DECSTBM (set scroll region), SU/SD (scroll up/down within region), line feed behavior respecting scroll region boundaries
- [x] T008 Implement VTermBuffer Render() method in `internal/session/vterm.go` — iterate cell grid, convert CellStyle to lipgloss inline styles, produce string output suitable for lipgloss composition. Render() must be pure (no state mutation) and use internal mutex for thread safety with concurrent Write() calls
- [x] T009 Implement VTermBuffer Write(data []byte) as io.Writer in `internal/session/vterm.go` — entry point that feeds bytes to the ANSI parser, updates cell grid, handles unknown sequences by silently dropping them. Must be thread-safe via mutex

### ManagedSession Extensions — Channel-Based I/O

- [x] T010 [P] Add InputCh, OutputCh, BackgroundMode, PaneWidth, PaneHeight fields to ManagedSession struct in `internal/session/managed.go` per data-model.md. Initialize channels in constructor (OutputCh buffered size 256)
- [x] T011 Implement StartBackground() method on ManagedSession in `internal/session/managed.go` — connect SSH, start shell, launch readOutput/readStderr/waitDone goroutines (reuse existing), but route output to OutputCh instead of outputBuf when BackgroundMode is true. Must be idempotent (no-op if already connected). Mutually exclusive with Run() per contracts/managed-session-ext.md
- [x] T012 Implement WriteInput(data []byte) method on ManagedSession in `internal/session/managed.go` — thread-safe write to sshStdin. Used by PaneLayoutModel.RouteInput() for both single-pane and broadcast input delivery
- [x] T013 Implement SetPaneSize(width, height int) method on ManagedSession in `internal/session/managed.go` — update PaneWidth/PaneHeight, send SSH WindowChange request to session. Debounce rapid calls (max 1 WindowChange per 100ms) per contracts/managed-session-ext.md
- [x] T014 Implement OutputChan() method on ManagedSession in `internal/session/managed.go` — return read-only channel (<-chan []byte). Close OutputCh when SSH session ends (in waitDone goroutine)

**Checkpoint**: Foundation ready — VTermBuffer can parse ANSI and render cells, ManagedSession can run in background with channel I/O. User story implementation can now begin.

---

## Phase 3: User Story 1 — Split Terminal into Panes (Priority: P1) MVP

**Goal**: Users can split the terminal into multiple panes, each running an independent session, with vim-style navigation between panes.

**Independent Test**: Open a connection, split vertically (`Space w v`), connect second session in new pane, verify both sessions render independently, navigate between panes with `Space w h/l`.

### Pane Layout Model — Split Tree & Focus

- [x] T015 [US1] Implement binary split tree operations in `internal/tui/panes.go` — SplitVertical() and SplitHorizontal() on PaneLayoutModel: find active pane's leaf node, replace with SplitNode containing original pane + new empty pane (State=Empty), enforce minimum size constraint (20x5), return error flash if too small. Per contracts/pane-layout.md behavioral contracts 1 and 5
- [x] T016 [US1] Implement SetSize(width, height int) on PaneLayoutModel in `internal/tui/panes.go` — recursive dimension calculation: traverse split tree, allocate width/height to children based on Ratio (clamped 0.1-0.9), account for 1-char border between panes. Call SetPaneSize() on each pane's ManagedSession for SSH WindowChange coordination
- [x] T017 [US1] Implement FocusDirection(dir Direction) on PaneLayoutModel in `internal/tui/panes.go` — given Left/Right/Up/Down, find the adjacent pane by walking the split tree geometry. Update ActivePaneID and Focused flags. Per contracts/pane-layout.md behavioral contract 3 (exactly one pane focused at all times)
- [x] T018 [US1] Implement RouteInput(data []byte) on PaneLayoutModel in `internal/tui/panes.go` — when not broadcasting, call WriteInput() on active pane's ManagedSession. Skip if active pane State != Active

### Pane Rendering

- [x] T019 [US1] Implement pane View() rendering in `internal/tui/pane_render.go` — for each leaf pane: if State==Active, call pane's VTermBuffer.Render(); if State==Empty, render connection picker placeholder; if State==Disconnected, render status message. Wrap each pane in a lipgloss border (highlighted for focused pane, dim for unfocused). Per contracts/pane-layout.md behavioral contract 6 (single pane = no borders)
- [x] T020 [US1] Implement PaneLayoutModel View() in `internal/tui/panes.go` — recursive tree rendering: for leaf nodes call pane render, for split nodes use lipgloss.JoinHorizontal (vertical split) or lipgloss.JoinVertical (horizontal split) to composite children. Return single string for app.go View() dispatch
- [x] T021 [US1] Implement PaneOutputMsg handling in PaneLayoutModel Update() in `internal/tui/panes.go` — when PaneOutputMsg arrives, find target pane by PaneID, write data to pane's VTermBuffer. Set up a tea.Tick (e.g., 16ms) to drain OutputCh from each active session and emit PaneOutputMsg messages for continuous rendering

### App Integration — View, Messages, Leader Keys

- [x] T022 [US1] Add viewPaneLayout constant to viewKind enum in `internal/tui/app.go`. Add paneLayout *PaneLayoutModel field to App struct. Initialize in app constructor
- [x] T023 [US1] Add `Space w` leader group in `internal/tui/leader.go` — add LeaderGroup with Key:"w", Label:"Window" and LeaderItems for: v (split vertical), s (split horizontal), h (focus left), j (focus down), k (focus up), l (focus right), c (close pane), b (broadcast toggle), = (equalize), z (zoom). IMPORTANT: `s` is horizontal split, `v` is vertical split — `h/j/k/l` are exclusively directional focus. Follow existing leaderGroups() pattern
- [x] T024 [US1] Implement executeLeaderAction() cases for pane actions in `internal/tui/app.go` — handle "split-vertical", "split-horizontal", "focus-left", "focus-right", "focus-up", "focus-down" actions. For splits: if not in viewPaneLayout, create initial pane from current session and pushView(viewPaneLayout), then split. For focus: delegate to paneLayout.FocusDirection()
- [x] T025 [US1] Add viewPaneLayout case in App.View() in `internal/tui/app.go` — render paneLayout.View() as content between header and statusbar. Add viewPaneLayout case in App.Update() tea.KeyMsg routing to call handlePaneLayoutKey()
- [x] T026 [US1] Implement handlePaneLayoutKey() in `internal/tui/app.go` — in Normal mode, route Space to leader key. Implement a keyMsgToBytes(msg tea.KeyMsg) []byte utility function that converts Bubbletea key messages to raw terminal bytes: printable runes → UTF-8 bytes, "enter" → \r, "tab" → \t, "backspace" → \x7f, "ctrl+c" → \x03, arrow keys → ESC [ A/B/C/D sequences, etc. Call paneLayout.RouteInput(keyMsgToBytes(msg)) for session input delivery. Handle Esc to pop view if in pane management context
- [x] T027 [US1] Add PaneOutputMsg, PaneConnectedMsg, PaneDisconnectedMsg handling in App.Update() in `internal/tui/app.go` — route PaneOutputMsg to paneLayout.Update(). Handle PaneDisconnectedMsg by updating pane state. Handle PaneConnectedMsg by transitioning pane from Connecting to Active

### Connection Picker in New Panes

- [x] T028 [US1] Implement connection picker for empty panes in `internal/tui/pane_render.go` — when a pane has State==Empty, render a compact connection list inside the pane region. Handle Enter key to initiate connection: create ManagedSession via SessionManager, call StartBackground(), transition pane state Empty→Connecting→Active, attach VTermBuffer to pane

### Window Resize Coordination

- [x] T029 [US1] Update App.layout() in `internal/tui/app.go` to call paneLayout.SetSize(width, contentHeight) when viewPaneLayout is active. Update WindowSizeMsg handler to propagate to pane layout. Disable ManagedSession's 250ms terminal polling for sessions in BackgroundMode (skip resize goroutine launch in attachLoop when BackgroundMode is true in `internal/session/managed.go`)

**Checkpoint**: User Story 1 complete — users can split into panes, navigate between them, each pane runs an independent session with VTerm rendering. Verify with `go build ./cmd/nexus/` and manual test: open connection, `Space w v`, connect second session, `Space w h/l` to navigate.

---

## Phase 4: User Story 2 — Broadcast Input to All Panes (Priority: P2)

**Goal**: Toggle broadcast mode to send keystrokes to all panes with active sessions simultaneously, with clear visual feedback.

**Independent Test**: Open 2+ panes with active sessions, press `Space w b`, verify border color changes and BROADCAST indicator appears, type a command, verify it appears in all panes, press `Space w b` again to toggle off.

- [x] T030 [US2] Implement ToggleBroadcast() and IsBroadcasting() on PaneLayoutModel in `internal/tui/panes.go` — toggle BroadcastMode flag. When toggling on, set flash message "Broadcasting to N sessions". When toggling off, set flash "Broadcast off"
- [x] T031 [US2] Update RouteInput() in `internal/tui/panes.go` to fan out input to all panes with State==Active when BroadcastMode is true — call WriteInput() on each active pane's ManagedSession. Skip panes with State != Active per contracts/pane-layout.md behavioral contract 4
- [x] T032 [US2] Update pane border rendering in `internal/tui/pane_render.go` — when BroadcastMode is active, change all active pane borders to a warning/accent color from theme. Add "[BROADCAST]" label to focused pane border title
- [x] T033 [US2] Update statusbar rendering in `internal/tui/app.go` — when paneLayout.IsBroadcasting() is true, display persistent "BROADCAST" indicator in status bar (similar to existing mode indicator). Use theme accent/warning color for visibility
- [x] T034 [US2] Wire `Space w b` leader action "broadcast-toggle" to paneLayout.ToggleBroadcast() in executeLeaderAction() in `internal/tui/app.go`

**Checkpoint**: User Story 2 complete — broadcast mode sends input to all active panes with clear visual indicators. Verify: open 2 panes, `Space w b`, type `whoami`, see output in both panes.

---

## Phase 5: User Story 3 — Resize and Close Panes (Priority: P2)

**Goal**: Users can resize panes using keybindings and close individual panes with space redistribution.

**Independent Test**: Open 3 panes, resize one larger, verify adjacent panes shrink proportionally. Close middle pane, verify remaining panes expand to fill space. Close all panes, verify return to connection list.

- [x] T035 [US3] Implement ResizeActive(direction Direction, amount int) on PaneLayoutModel in `internal/tui/panes.go` — find the SplitNode parent of the active pane, adjust Ratio by amount (e.g., ±0.05 per step), clamp to [0.1, 0.9], recalculate all pane dimensions via SetSize(). Amount is in percentage steps
- [x] T036 [US3] Implement ClosePane() on PaneLayoutModel in `internal/tui/panes.go` — remove active pane's leaf node from split tree, replace parent SplitNode with the sibling subtree (sibling expands to fill space), update focus to nearest remaining pane. If last pane closed, return a command to popView() back to connection list. Per contracts/pane-layout.md behavioral contract 2
- [x] T037 [US3] Implement EqualizeAll() on PaneLayoutModel in `internal/tui/panes.go` — recursively set all SplitNode Ratios to 0.5, then recalculate dimensions
- [x] T038 [US3] Implement ZoomToggle() on PaneLayoutModel in `internal/tui/panes.go` — store pre-zoom tree state, temporarily set active pane to full size (hide other panes). Toggle back restores original layout. Add zoomed state flag and zoomed border indicator
- [x] T039 [US3] Wire leader actions in `internal/tui/app.go` executeLeaderAction() — "close-pane" → ClosePane(), "equalize-panes" → EqualizeAll(), "zoom-pane" → ZoomToggle(). Add resize keybindings: `Space w >` (grow right), `Space w <` (grow left), `Space w +` (grow down), `Space w -` (grow up) mapped to ResizeActive() calls
- [x] T040 [US3] Add resize leader items to `Space w` group in `internal/tui/leader.go` — add LeaderItems for > (resize right), < (resize left), + (resize down), - (resize up) mapped to resize actions

**Checkpoint**: User Story 3 complete — panes can be resized, equalized, zoomed, and closed with proper space redistribution. Verify: split into 3 panes, resize, close one, verify layout adjusts.

---

## Phase 6: User Story 4 — Pane Layouts and Presets (Priority: P3)

**Goal**: Users can select preset layouts (2x2 grid, 3-column, main+sidebar) and assign connections to panes.

**Independent Test**: From connection list, press `Space w 2` for 2-column layout, verify 2 empty panes appear side-by-side with connection pickers. Select connections for each pane.

- [x] T041 [US4] Implement ApplyPreset(name string) on PaneLayoutModel in `internal/tui/panes.go` — define preset layouts: "2h" (2 horizontal), "2v" (2 vertical), "3v" (3 columns), "2x2" (4-pane grid), "main-side" (large left + small right). Build split tree programmatically for each preset, create empty panes with connection pickers
- [x] T042 [US4] Wire leader actions for presets in `internal/tui/app.go` executeLeaderAction() — "preset-2h", "preset-2v", "preset-3v", "preset-2x2", "preset-main-side". If not in viewPaneLayout, pushView first, then apply preset
- [x] T043 [US4] Add preset leader items to `Space w` group in `internal/tui/leader.go` — add LeaderItems: 1 (single/restore), 2 (2-column), 3 (3-column), 4 (2x2 grid) mapped to preset actions
- [x] T044 [US4] Add `:layout` command to command bar in `internal/tui/command.go` (or equivalent command handler) — parse `:layout 2x2`, `:layout 3v`, `:layout main-side` and delegate to paneLayout.ApplyPreset()

**Checkpoint**: User Story 4 complete — users can quickly set up common pane arrangements via leader keys or command bar.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Edge cases, backward compatibility, and integration quality

- [x] T045 Handle non-SSH connection types in panes per FR-009 in `internal/tui/pane_render.go` and `internal/tui/panes.go` — when a pane initiates an RDP or VNC connection, delegate to existing launcher (opens GUI window via IPC) and display "Session running in GUI window" in the pane region. For Telnet, evaluate whether to use existing launcher or extend ManagedSession with Telnet support. At minimum, all four protocols must be selectable from the pane connection picker without errors
- [x] T046 Handle pane disconnection gracefully in `internal/tui/pane_render.go` — when PaneDisconnectedMsg received, render disconnection status in pane (show error message, offer reconnect via Enter key or close via `Space w c`). Do not disrupt other panes per FR-011
- [x] T047 Ensure backward compatibility for single-session workflow in `internal/tui/app.go` — when user connects from connection list without being in pane mode, use existing tea.Exec() path unchanged. Verify detach/reattach (Ctrl+\) still works for non-pane sessions per SC-006 and research.md R6
- [x] T048 [P] Handle terminal too-small-to-split edge case in `internal/tui/panes.go` — when SplitVertical/SplitHorizontal would create panes below 20x5, display flash message "Terminal too small to split" and abort the operation per FR-010
- [x] T049 [P] Handle external terminal resize in `internal/tui/panes.go` — ensure WindowSizeMsg propagation recalculates all pane dimensions proportionally with no rendering artifacts per FR-014 and SC-005
- [x] T050 Verify `go build ./cmd/nexus/` compiles without errors and `go vet ./...` passes (pre-existing warnings in rdp.go:83 and vnc.go:52 are acceptable)
- [x] T051 Manual integration test: open nexus, connect SSH session, split into 4 panes via `Space w v` then `Space w s` in each, connect sessions, toggle broadcast, type commands, resize, close panes, verify all acceptance scenarios from spec.md

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **US1 (Phase 3)**: Depends on Foundational — core split/navigate/render
- **US2 (Phase 4)**: Depends on US1 (needs working panes to broadcast to)
- **US3 (Phase 5)**: Depends on US1 (needs working panes to resize/close)
- **US4 (Phase 6)**: Depends on US1 (needs working split tree to apply presets)
- **Polish (Phase 7)**: Depends on all user stories being complete

### User Story Dependencies

- **US1 (P1)**: Depends on Foundational only — no other story dependencies
- **US2 (P2)**: Depends on US1 (broadcast requires panes to exist)
- **US3 (P2)**: Depends on US1 (resize/close requires panes to exist). **Can run in parallel with US2** — they modify different aspects of PaneLayoutModel
- **US4 (P3)**: Depends on US1 (presets build split trees). Can run after US1, in parallel with US2/US3

### Within Each Phase

- Foundational: T004-T009 (VTerm) can run in parallel with T010-T014 (ManagedSession)
- US1: T015-T018 (layout model) → T019-T021 (rendering) → T022-T029 (app integration)
- US2: T030-T031 (model) → T032-T033 (visual) → T034 (wiring)
- US3: T035-T038 (model) → T039-T040 (wiring)
- US4: T041 (model) → T042-T044 (wiring)

### Parallel Opportunities

- **Phase 2**: VTerm (T004-T009) and ManagedSession (T010-T014) are independent packages — full parallelism
- **Phase 3**: T015/T016/T017/T018 can partially overlap (different methods, same file)
- **Phase 4+5**: US2 and US3 can run in parallel after US1 completes (different functional areas)
- **Phase 6**: US4 can run in parallel with US2/US3
- **Phase 7**: T047 and T048 are independent [P] tasks

---

## Parallel Example: Foundational Phase

```bash
# Launch VTerm and ManagedSession work in parallel:
Agent A: "T004-T009: Implement VTermBuffer in internal/session/vterm.go"
Agent B: "T010-T014: Implement ManagedSession extensions in internal/session/managed.go"
```

## Parallel Example: User Stories 2 & 3

```bash
# After US1 is complete, launch US2 and US3 in parallel:
Agent A: "T030-T034: Implement broadcast mode (US2)"
Agent B: "T035-T040: Implement resize and close (US3)"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T003)
2. Complete Phase 2: Foundational (T004-T014) — **parallelize VTerm + ManagedSession**
3. Complete Phase 3: User Story 1 (T015-T029)
4. **STOP and VALIDATE**: Test pane splitting, navigation, and independent session rendering
5. Build and demo if ready

### Incremental Delivery

1. Setup + Foundational → Foundation ready
2. Add US1 → Test independently → Demo (MVP — pane splitting works!)
3. Add US2 + US3 in parallel → Test independently → Demo (broadcast + resize)
4. Add US4 → Test independently → Demo (preset layouts)
5. Polish → Final validation → Release

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- VTerm is the highest-risk component — if ANSI parsing proves insufficient, consider adopting a Go VTE library
- Backward compatibility (T046) is critical — existing single-session tea.Exec path must remain untouched
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
