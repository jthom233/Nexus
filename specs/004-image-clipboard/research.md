# Research: Image Clipboard Support for RDP

**Feature Branch**: `004-image-clipboard`
**Date**: 2026-02-17

## Decision 1: CLIPRDR Implementation Approach

**Decision**: Extend the existing custom `clipboardChannel` in `internal/gui/clipboard.go` rather than adopting grdp's built-in `cliprdr` package.

**Rationale**: Nexus already hand-rolls all CLIPRDR PDU serialization and does not use grdp's `cliprdr` package. The grdp cliprdr package is Windows-only (uses Win32 `OpenClipboard`, hidden window message loop) and cannot work cross-platform. The existing nexus code is clean, well-structured, and only needs format-dispatch extensions.

**Alternatives considered**:
- Use grdp's `cliprdr` package: Rejected — Windows-only, would require major forking or rewrite.
- Write a new standalone CLIPRDR library: Rejected — unnecessary abstraction; the existing code is already correct and isolated.

## Decision 2: Platform Image Clipboard Access

**Decision**: Shell-out to platform tools (`wl-paste`/`wl-copy` on Wayland, `xclip` on X11) and use Win32 API via `github.com/tomatome/win` on Windows. Create platform-split files using Go build tags.

**Rationale**: This mirrors how `atotto/clipboard` already works for text (shell-out on Linux, Win32 on Windows). The `tomatome/win` package is already a transitive dependency via grdp and provides `OpenClipboard`, `GetClipboardData`, `SetClipboardData`, `GlobalAlloc`, `GlobalLock` — sufficient for CF_DIB without new dependencies.

**Alternatives considered**:
- Pure Go image clipboard library: No mature cross-platform Go library exists for image clipboard.
- CGo bindings: Rejected — adds build complexity, shell-out is simpler and proven.
- Extend `atotto/clipboard`: Rejected — text-only by design, would require forking.

## Decision 3: Image Format Conversion

**Decision**: Use Go's standard `image` and `image/png` packages for PNG↔RGBA conversion. Write custom DIB encoder/decoder for the CF_DIB wire format (40-byte BITMAPINFOHEADER + bottom-up BGRA pixel data).

**Rationale**: CF_DIB is a simple uncompressed format — no third-party library needed. Go's `image.RGBA` stores pixels top-down in RGBA order; conversion to bottom-up BGRA is a straightforward row-flip + channel-swap. PNG is the universal image clipboard format on Linux.

**Alternatives considered**:
- Use CF_DIBV5 (format 17): Rejected — less widely supported by Windows applications; CF_DIB (format 8) is the standard.
- Use third-party BMP library: Rejected — CF_DIB is not a full BMP file (no file header), custom code is simpler and more correct.

## Decision 4: Clipboard Change Detection for Images

**Decision**: Poll clipboard MIME types (not image data) to detect image presence changes, then read full image data only when the server requests it (lazy loading).

**Rationale**: Reading large image data on every 500ms poll tick is wasteful. Instead, poll `wl-paste --list-types` (Wayland) or `xclip -t TARGETS` (X11) to detect whether image content is available, which is fast (returns a small text list). Only read the actual image bytes when `processFormatDataRequest` receives a CF_DIB request from the server.

**Alternatives considered**:
- Read and hash image data every tick: Rejected — expensive for large images (10-20MB hashing every 500ms).
- Event-driven clipboard monitoring: Rejected — requires platform-specific event loops (Wayland protocols, X11 selection events) that are much more complex than polling.

## Decision 5: File Organization

**Decision**: Add platform-split files `clipboard_image.go` (shared logic), `clipboard_image_other.go` (Linux), `clipboard_image_windows.go` (Windows) in `internal/gui/`. Keep all CLIPRDR protocol changes in the existing `clipboard.go`.

**Rationale**: Follows the existing convention (`keyhook_other.go` / `keyhook_windows.go`). Keeps platform-specific code isolated behind build tags while shared protocol logic stays in the main clipboard file.

**Alternatives considered**:
- Put everything in `clipboard.go`: Rejected — would make the file too large and mix platform-specific code.
- Create a separate `internal/clipboard/` package: Rejected — over-engineering for a single feature; the GUI package is flat by convention.

## Key Findings

### Existing Code Hooks

| Symbol | File:Line | What Needs to Change |
|--------|-----------|---------------------|
| `clipboardChannel` struct | clipboard.go:87-93 | Add `lastImageAvailable bool`, `pendingFormat uint32` fields |
| `pollClipboard` | clipboard.go:297-324 | Add image MIME type detection alongside text polling |
| `sendFormatList` | clipboard.go:269-275 | Emit CF_DIB (8) when image available, alongside CF_UNICODETEXT (13) |
| `processFormatDataRequest` | clipboard.go:212-236 | Add CF_DIB dispatch: read image, convert to DIB, send |
| `processFormatList` | clipboard.go:162-188 | Scan for CF_DIB (8) in server's format list |
| `processFormatDataResponse` | clipboard.go:varies | Route by `pendingFormat`: CF_DIB → parse DIB, write to local image clipboard |
| `isTextContent` | clipboard.go:45-82 | Reuse for image detection (already detects PNG/JPEG/GIF/BMP/WebP magic bytes) |

### grdp Constants (for reference)

```
CF_DIB          = 8     // Windows DIB format — our target
CF_UNICODETEXT  = 13    // Already supported
CF_DIBV5        = 17    // Extended DIB — out of scope
CF_HDROP        = 15    // File clipboard — out of scope
```

### CF_DIB Wire Format

```
BITMAPINFOHEADER (40 bytes):
  biSize=40, biWidth, biHeight (positive=bottom-up),
  biPlanes=1, biBitCount=32, biCompression=0 (BI_RGB),
  biSizeImage, biXPels=0, biYPels=0, biClrUsed=0, biClrImportant=0

Pixel data: rows bottom-to-top, each pixel B-G-R-A (4 bytes)
```

### Platform Commands

| Platform | Detect Image | Read Image | Write Image |
|----------|-------------|------------|-------------|
| Wayland | `wl-paste --list-types` (check for `image/png`) | `wl-paste --no-newline --type image/png` | `wl-copy --type image/png` (pipe stdin) |
| X11 | `xclip -o -selection clipboard -t TARGETS` | `xclip -o -selection clipboard -t image/png` | `xclip -i -selection clipboard -t image/png` (pipe stdin) |
| Windows | `OpenClipboard` + `IsClipboardFormatAvailable(CF_DIB)` | `GetClipboardData(CF_DIB)` + `GlobalLock` | `SetClipboardData(CF_DIB, hGlobal)` |
