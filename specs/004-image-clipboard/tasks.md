# Tasks: Image Clipboard Support for RDP

**Input**: Design documents from `/specs/004-image-clipboard/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md

**Tests**: No automated tests requested. Manual integration testing per quickstart.md.

**Organization**: Tasks grouped by user story. Each story is independently implementable and testable after foundational phase completes.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: Create new files with correct build tags and package declarations

- [ ] T001 [P] Create `internal/gui/clipboard_image.go` with package declaration, imports (`image`, `image/png`, `image/draw`, `bytes`, `encoding/binary`, `fmt`), and `const dibHeaderSize = 40` placeholder
- [ ] T002 [P] Create `internal/gui/clipboard_image_other.go` with `//go:build !windows` build tag, package declaration, and stub functions `detectImageClipboard() bool`, `readImageClipboard() ([]byte, error)`, `writeImageClipboard([]byte) error` returning false/nil
- [ ] T003 [P] Create `internal/gui/clipboard_image_windows.go` with `//go:build windows` build tag, package declaration, and stub functions `detectImageClipboard() bool`, `readImageClipboard() ([]byte, error)`, `writeImageClipboard([]byte) error` returning false/nil

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure required by ALL user stories — DIB codec and platform clipboard I/O

**CRITICAL**: No user story work can begin until this phase is complete

- [ ] T004 Add `cfDIB uint32 = 8` constant and `maxImageSize = 20 * 1024 * 1024` constant to `internal/gui/clipboard.go` alongside existing `cfUnicodeText`
- [ ] T005 Add `lastImageAvailable bool` and `pendingFormat uint32` fields to `clipboardChannel` struct in `internal/gui/clipboard.go`
- [ ] T006 Implement `encodeDIB(img image.Image) ([]byte, error)` in `internal/gui/clipboard_image.go` — convert Go image to CF_DIB wire format: RGBA→BGRA channel swap, top-down→bottom-up row flip, 40-byte BITMAPINFOHEADER (LE), size guard for maxImageSize
- [ ] T007 Implement `decodeDIB(data []byte) (*image.RGBA, error)` in `internal/gui/clipboard_image.go` — parse BITMAPINFOHEADER, validate biBitCount=32 and biCompression=0, read pixel rows handling positive/negative biHeight, BGRA→RGBA swap, bottom-up→top-down flip
- [ ] T008 [P] Implement Linux `detectImageClipboard()` in `internal/gui/clipboard_image_other.go` — check `$WAYLAND_DISPLAY` env var: if set, run `wl-paste --list-types` and check for `image/png`; otherwise run `xclip -o -selection clipboard -t TARGETS` and check for `image/png`. Return true if image available. 5s timeout on subprocess, return false on error.
- [ ] T009 [P] Implement Linux `readImageClipboard()` in `internal/gui/clipboard_image_other.go` — if Wayland, run `wl-paste --no-newline --type image/png`; if X11, run `xclip -o -selection clipboard -t image/png`. Return raw PNG bytes. 5s timeout, return error on failure.
- [ ] T010 [P] Implement Linux `writeImageClipboard(pngData []byte)` in `internal/gui/clipboard_image_other.go` — if Wayland, pipe pngData to `wl-copy --type image/png` via stdin; if X11, pipe to `xclip -i -selection clipboard -t image/png` via stdin. 5s timeout, return error on failure.
- [ ] T011 [P] Implement Windows `detectImageClipboard()` in `internal/gui/clipboard_image_windows.go` — use `win.OpenClipboard(0)`, `win.IsClipboardFormatAvailable(cfDIB)`, `win.CloseClipboard()`. Return true if CF_DIB available.
- [ ] T012 [P] Implement Windows `readImageClipboard()` in `internal/gui/clipboard_image_windows.go` — `win.OpenClipboard(0)`, `win.GetClipboardData(cfDIB)`, `win.GlobalLock`, copy raw DIB bytes (BITMAPINFOHEADER + pixels), `win.GlobalUnlock`, `win.CloseClipboard`. Return DIB bytes directly (not PNG — caller handles conversion on Windows).
- [ ] T013 [P] Implement Windows `writeImageClipboard(pngData []byte)` in `internal/gui/clipboard_image_windows.go` — decode PNG, call `encodeDIB()`, `win.OpenClipboard(0)`, `win.EmptyClipboard`, `win.GlobalAlloc` + `win.GlobalLock`, copy DIB bytes, `win.SetClipboardData(cfDIB, hGlobal)`, `win.CloseClipboard`.

**Checkpoint**: DIB codec and platform I/O are ready. `go build ./...` should pass on Linux. User story implementation can begin.

---

## Phase 3: User Story 1 — Paste Local Image into RDP Session (Priority: P1) MVP

**Goal**: Users can copy a screenshot locally and paste it into a remote RDP application via CF_DIB

**Independent Test**: Take a screenshot locally, switch to RDP session, paste into Paint. Image should appear at full fidelity. Text clipboard must still work unchanged.

### Implementation for User Story 1

- [ ] T014 [US1] Extend `pollClipboard()` in `internal/gui/clipboard.go` — after existing text change detection, call `detectImageClipboard()`. If `imageAvailable != c.lastImageAvailable`, update `c.lastImageAvailable` and trigger `sendFormatList`. Also trigger `sendFormatList` when text changes and image is available (to re-advertise both formats).
- [ ] T015 [US1] Extend `sendFormatList()` in `internal/gui/clipboard.go` — build format list dynamically: if text available (existing `lastContent != ""`), include CF_UNICODETEXT entry; if `lastImageAvailable`, include CF_DIB entry. Support emitting both formats in a single FORMAT_LIST PDU when both are present (FR-003). Preserve existing PDU serialization pattern using `bytes.Buffer` and `core.WriteUInt32LE`/`core.WriteUInt16LE`.
- [ ] T016 [US1] Extend `processFormatDataRequest()` in `internal/gui/clipboard.go` — add dispatch for `cfDIB`: call `readImageClipboard()` to get PNG bytes (Linux) or DIB bytes (Windows), decode PNG to `image.Image` if needed, call `encodeDIB(img)`, send DIB bytes via `sendFormatDataResponse`. On any error, log at debug level and send `CB_RESPONSE_FAIL`. Keep existing `cfUnicodeText` path unchanged.
- [ ] T017 [US1] Verify text clipboard regression — ensure that when clipboard contains only text (no image), the format list still advertises only CF_UNICODETEXT and the existing `processFormatDataRequest` text path is unchanged. Confirm `isTextContent()` guard still filters binary content for the text path in `internal/gui/clipboard.go`.

**Checkpoint**: Local-to-remote image paste works. Text clipboard unaffected. This is the MVP.

---

## Phase 4: User Story 2 — Copy Image from RDP Session to Local Machine (Priority: P2)

**Goal**: Users can copy an image inside the RDP session and paste it locally

**Independent Test**: Copy an image in the RDP session (e.g., right-click image in browser → Copy), switch to local image editor, paste. Image should appear intact.

### Implementation for User Story 2

- [ ] T018 [US2] Extend `processFormatList()` in `internal/gui/clipboard.go` — when scanning server's advertised formats, also check for `cfDIB` (format ID 8). If CF_DIB found, call `sendFormatDataRequest(cfDIB)` and set `c.pendingFormat = cfDIB`. If both CF_UNICODETEXT and CF_DIB are advertised, request CF_DIB (image is the higher-value format from the remote side). Keep existing CF_UNICODETEXT request path as fallback when no CF_DIB is present.
- [ ] T019 [US2] Extend `processFormatDataResponse()` in `internal/gui/clipboard.go` — route by `c.pendingFormat`: if `cfUnicodeText`, use existing text decode path (`core.UnicodeDecode` → `clipboard.WriteAll`); if `cfDIB`, call `decodeDIB(data)` to get `*image.RGBA`, encode as PNG via `png.Encode()`, call `writeImageClipboard(pngBytes)` to place on local clipboard. Reset `c.pendingFormat` after handling. On any error, log at debug level and discard (silent failure per FR-008).
- [ ] T020 [US2] Verify bidirectional non-interference — ensure that receiving a CF_DIB from the server does not corrupt or clear the local text clipboard, and that receiving CF_UNICODETEXT still works via the existing path in `internal/gui/clipboard.go`.

**Checkpoint**: Bidirectional image clipboard works. Both local→remote and remote→local. Text clipboard unaffected.

---

## Phase 5: User Story 3 — Large Image Handling (Priority: P3)

**Goal**: Images up to 20MB transfer without corruption; images exceeding the limit degrade gracefully

**Independent Test**: Copy a 4K+ resolution screenshot (>10MB) and paste into RDP session. Verify it arrives intact. Copy a very large image (>20MB DIB size) and verify graceful failure (no crash, no hang).

### Implementation for User Story 3

- [ ] T021 [US3] Add size guard in `encodeDIB()` in `internal/gui/clipboard_image.go` — before allocating the output buffer, calculate `totalSize = dibHeaderSize + width*height*4`. If `totalSize > maxImageSize`, return an error `"image too large: %d bytes exceeds %d byte limit"`. Caller (`processFormatDataRequest`) sends CB_RESPONSE_FAIL.
- [ ] T022 [US3] Add size guard in `decodeDIB()` in `internal/gui/clipboard_image.go` — validate `len(data) <= maxImageSize + dibHeaderSize` before parsing. If data exceeds limit, return error. Also validate that `biWidth` and `biHeight` produce reasonable dimensions (no integer overflow on `width*height*4`).
- [ ] T023 [US3] Add size guard in `processFormatDataRequest()` for CF_DIB path in `internal/gui/clipboard.go` — if `readImageClipboard()` returns data larger than `maxImageSize`, log and send CB_RESPONSE_FAIL instead of attempting conversion.
- [ ] T024 [US3] Add size guard in `processFormatDataResponse()` for CF_DIB path in `internal/gui/clipboard.go` — if received DIB data exceeds `maxImageSize`, log and discard instead of attempting decode. Ensures no hang on oversized server-side images.

**Checkpoint**: Large images up to 20MB work correctly. Oversized images fail gracefully with no crash or hang.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Build verification, error handling consistency, edge cases

- [ ] T025 Run `go build ./...` to verify all new files compile correctly on Linux (build tag `!windows`)
- [ ] T026 Run `go vet ./...` to check for any new warnings introduced by image clipboard code (pre-existing IPv6 warnings in rdp.go:83 and vnc.go:52 are expected)
- [ ] T027 Review all error paths in `internal/gui/clipboard.go` and `internal/gui/clipboard_image.go` — ensure every error is logged at debug level with `log.Printf` and results in either CB_RESPONSE_FAIL (for requests) or silent discard (for responses), per FR-008
- [ ] T028 Verify debounce behavior — ensure rapid clipboard changes (text→image→text) in `pollClipboard()` in `internal/gui/clipboard.go` result in only the latest format list being sent, per FR-010

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately. All 3 tasks are parallel (different files).
- **Foundational (Phase 2)**: Depends on Setup. T006/T007 (DIB codec) are sequential (T006 before T007 or parallel since different functions). T008-T013 (platform I/O) are parallel with T006/T007 (different files). T004/T005 (constants/struct) should be first in this phase.
- **User Story 1 (Phase 3)**: Depends on Foundational (Phase 2) completion. T014→T015→T016 are sequential (poll detects → format list advertises → request handler serves). T017 can run after T016.
- **User Story 2 (Phase 4)**: Depends on Foundational (Phase 2) completion. Can run in parallel with US1 if needed, but recommended after US1 since US1 validates the core format list mechanism that US2 builds on. T018→T019 sequential. T020 after T019.
- **User Story 3 (Phase 5)**: Depends on US1 and US2 completion (adds guards to code written in those phases). T021-T024 are parallel (different functions/locations).
- **Polish (Phase 6)**: Depends on all user stories being complete.

### User Story Dependencies

- **US1 (P1)**: After Foundational — no dependencies on other stories. **This is the MVP.**
- **US2 (P2)**: After Foundational — can technically start in parallel with US1, but recommended sequential since T015 (sendFormatList changes) informs T018 (processFormatList changes).
- **US3 (P3)**: After US1 + US2 — adds size guards to functions created in those stories.

### Parallel Opportunities

**Phase 1** (all parallel):
```
T001 ─┐
T002 ─┼─ All create different files
T003 ─┘
```

**Phase 2** (mixed):
```
T004, T005 ─→ T006, T007 (codec depends on constants)
T008, T009, T010 ─┐
T011, T012, T013 ─┘ (platform files parallel with each other AND with codec)
```

**Phase 5** (all parallel):
```
T021 ─┐
T022 ─┼─ All guard different functions
T023 ─┤
T024 ─┘
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (3 tasks, all parallel)
2. Complete Phase 2: Foundational (10 tasks, mostly parallel)
3. Complete Phase 3: User Story 1 (4 tasks, sequential)
4. **STOP and VALIDATE**: Take screenshot locally → paste into RDP Paint → verify image appears. Also verify text paste still works.
5. This delivers the core "paste local image into remote" workflow.

### Incremental Delivery

1. Setup + Foundational → DIB codec and platform I/O ready
2. User Story 1 → Local→remote image paste (MVP!)
3. User Story 2 → Remote→local image paste (bidirectional complete)
4. User Story 3 → Large image size guards (robustness)
5. Polish → Build verification, error handling review

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story
- No automated tests — manual testing per quickstart.md
- All error handling is silent (debug log only, no user-facing dialogs)
- Pre-existing `go vet` warnings in rdp.go:83 and vnc.go:52 are known and unrelated
- Windows platform files (T003, T011-T013) can be stubbed initially and completed later if Linux is the primary development platform
