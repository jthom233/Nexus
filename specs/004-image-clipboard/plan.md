# Implementation Plan: Image Clipboard Support for RDP

**Branch**: `004-image-clipboard` | **Date**: 2026-02-17 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/004-image-clipboard/spec.md`

## Summary

Add bidirectional image clipboard support for RDP sessions via the CLIPRDR CF_DIB format. Extend the existing `clipboardChannel` in `internal/gui/clipboard.go` to detect local image clipboard content, advertise CF_DIB alongside CF_UNICODETEXT, convert between PNG and the Windows DIB wire format, and handle both local-to-remote and remote-to-local image transfers. Platform-specific image clipboard I/O is implemented via shell-out on Linux (Wayland/X11) and Win32 API on Windows.

## Technical Context

**Language/Version**: Go 1.25 (per go.mod)
**Primary Dependencies**: github.com/tomatome/grdp (RDP protocol), github.com/atotto/clipboard (text-only, supplemented), github.com/tomatome/win (Win32 API, transitive dep), standard library `image`, `image/png`
**Storage**: N/A
**Testing**: Manual integration testing (no existing test framework in internal/gui/)
**Target Platform**: Linux (Wayland + X11), Windows
**Project Type**: Single Go project — CLI/GUI application using Ebitengine
**Performance Goals**: <2s for images under 2MB (SC-001), no hang for images up to 20MB
**Constraints**: 20MB max transfer size, silent failure on errors, focused session only
**Scale/Scope**: Single user, single focused RDP session at a time

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Constitution is an unfilled template — no project-specific gates defined. Proceeding without violations.

**Post-Phase 1 re-check**: No violations. The design adds 3 new files and modifies 1 existing file, all within the established `internal/gui/` package. No new external dependencies. No new abstractions — follows the existing flat-file, direct-implementation pattern.

## Project Structure

### Documentation (this feature)

```text
specs/004-image-clipboard/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 output — research findings
├── data-model.md        # Phase 1 output — entity model
├── quickstart.md        # Phase 1 output — developer quickstart
├── checklists/          # Quality checklists
│   └── requirements.md
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code (repository root)

```text
internal/gui/
├── clipboard.go                  # MODIFY — add CF_DIB constant, extend format list,
│                                 #   add format dispatch in request/response handlers,
│                                 #   extend poll loop for image detection
├── clipboard_image.go            # NEW — shared image logic: DIB↔PNG conversion,
│                                 #   BITMAPINFOHEADER encode/decode, RGBA↔BGRA swap
├── clipboard_image_other.go      # NEW — Linux image clipboard I/O
│                                 #   (wl-paste/wl-copy on Wayland, xclip on X11)
│                                 #   Build tag: //go:build !windows
├── clipboard_image_windows.go    # NEW — Windows image clipboard I/O
│                                 #   (Win32 OpenClipboard/GetClipboardData/SetClipboardData)
│                                 #   Build tag: //go:build windows
├── app.go                        # NO CHANGES — active session routing already correct
├── rdp.go                        # NO CHANGES — clipboard channel wiring already correct
├── input.go                      # NO CHANGES — Ctrl+V forwarded as scancodes, correct as-is
└── tabs.go                       # NO CHANGES — Active() returns focused session
```

**Structure Decision**: Flat file additions within the existing `internal/gui/` package, following the established `_other.go`/`_windows.go` build tag convention (see `keyhook_other.go`).

## Implementation Design

### Component 1: DIB Codec (`clipboard_image.go`)

Pure Go, no platform dependencies. Two functions:

- `encodeDIB(img image.Image) ([]byte, error)` — Convert Go image to CF_DIB wire format:
  1. Convert to RGBA if not already
  2. Write 40-byte BITMAPINFOHEADER (LE)
  3. Write pixel rows bottom-to-top, swapping R↔B for BGRA order
  4. Return complete DIB byte slice

- `decodeDIB(data []byte) (image.Image, error)` — Convert CF_DIB wire format to Go image:
  1. Parse BITMAPINFOHEADER (validate biBitCount=32, biCompression=0)
  2. Read pixel rows (handle both positive/negative biHeight)
  3. Swap B↔R for RGBA order, flip to top-down
  4. Return `*image.RGBA`

Size guard: reject if total data > 20MB before allocating buffers.

### Component 2: Platform Image I/O (`clipboard_image_other.go`, `clipboard_image_windows.go`)

Three platform functions each:

- `detectImageClipboard() bool` — Is image content available on the local clipboard?
  - Linux: `wl-paste --list-types` or `xclip -o -selection clipboard -t TARGETS`, check for `image/png`
  - Windows: `IsClipboardFormatAvailable(CF_DIB)`

- `readImageClipboard() ([]byte, error)` — Read image as PNG bytes from local clipboard
  - Linux: `wl-paste --no-newline --type image/png` or `xclip -o -selection clipboard -t image/png`
  - Windows: `OpenClipboard` → `GetClipboardData(CF_DIB)` → `GlobalLock` → copy bytes → encode as DIB directly (skip PNG intermediary on Windows)

- `writeImageClipboard(pngData []byte) error` — Write PNG image to local clipboard
  - Linux: pipe to `wl-copy --type image/png` or `xclip -i -selection clipboard -t image/png`
  - Windows: decode PNG → `SetClipboardData(CF_DIB, hGlobal)` with DIB bytes

Platform detection: reuse existing `isWayland()` check from `app.go` or replicate the `$WAYLAND_DISPLAY` env var check.

### Component 3: Protocol Extensions (`clipboard.go` modifications)

Changes to the existing `clipboardChannel`:

1. **New constant**: `cfDIB = 8` alongside existing `cfUnicodeText = 13`

2. **Struct extension**: Add `lastImageAvailable bool` and `pendingFormat uint32` fields to `clipboardChannel`

3. **`pollClipboard` extension**: After text check, call `detectImageClipboard()`. If image availability changed, or text changed, call `sendFormatList` with the current format set.

4. **`sendFormatList` extension**: Build format list dynamically:
   - If text available: include CF_UNICODETEXT entry
   - If image available: include CF_DIB entry
   - Can include both when both are available

5. **`processFormatDataRequest` extension**: Dispatch on `fmtID`:
   - `cfUnicodeText` (13): existing text path
   - `cfDIB` (8): call `readImageClipboard()` → if PNG, decode PNG → `encodeDIB()` → send response. On Windows, `readImageClipboard()` returns DIB directly.
   - Other: existing `CB_RESPONSE_FAIL` path

6. **`processFormatList` extension**: When scanning server's format list, also check for `cfDIB`. If found and no text format available, request CF_DIB. If both, prefer CF_UNICODETEXT (text is cheaper). Store requested format in `pendingFormat`.

7. **`processFormatDataResponse` extension**: Route by `pendingFormat`:
   - `cfUnicodeText`: existing text decode path
   - `cfDIB`: call `decodeDIB()` → encode as PNG → `writeImageClipboard(pngBytes)`. Reset `pendingFormat`.

### Data Flow

```
LOCAL → REMOTE:
  pollClipboard() ──→ detectImageClipboard() = true
    │                      │
    └─→ sendFormatList([CF_UNICODETEXT, CF_DIB])
                           │
                 server sends FORMAT_DATA_REQUEST(8)
                           │
         processFormatDataRequest(8)
           ├─→ readImageClipboard() → PNG bytes
           ├─→ png.Decode() → image.RGBA
           ├─→ encodeDIB(img) → DIB bytes
           └─→ sendFormatDataResponse(dibBytes)

REMOTE → LOCAL:
  server sends FORMAT_LIST([CF_UNICODETEXT, CF_DIB])
    │
    └─→ processFormatList() → sendFormatDataRequest(8)
                                     │
                     server sends FORMAT_DATA_RESPONSE(dibBytes)
                                     │
            processFormatDataResponse()
              ├─→ decodeDIB(dibBytes) → image.RGBA
              ├─→ png.Encode() → PNG bytes
              └─→ writeImageClipboard(pngBytes)
```

### Error Handling

All errors are handled silently with debug-level logging (per FR-008 and clarification):

- Image clipboard read failure → log, send CB_RESPONSE_FAIL
- DIB encode/decode failure → log, send CB_RESPONSE_FAIL or discard
- Image exceeds 20MB → log, send CB_RESPONSE_FAIL
- Platform tool not available (`wl-paste`/`xclip` missing) → log, image clipboard disabled, text continues working
- Subprocess timeout → 5s timeout on all shell-out commands, log on timeout

## Complexity Tracking

No constitution violations to justify — design stays within existing patterns.
