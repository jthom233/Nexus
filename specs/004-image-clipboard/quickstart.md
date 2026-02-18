# Developer Quickstart: Image Clipboard Support

**Feature Branch**: `004-image-clipboard`

## Prerequisites

- Go 1.21+
- Linux: `wl-paste`, `wl-copy` (Wayland) or `xclip` (X11) installed
- Windows: No additional tools (uses Win32 API via `github.com/tomatome/win`)

## Key Files

| File | Purpose |
|------|---------|
| `internal/gui/clipboard.go` | CLIPRDR protocol logic — format list, data request/response routing |
| `internal/gui/clipboard_image.go` | Shared image logic — DIB encode/decode, PNG↔BGRA conversion |
| `internal/gui/clipboard_image_other.go` | Linux image clipboard I/O (Wayland/X11 shell-out) |
| `internal/gui/clipboard_image_windows.go` | Windows image clipboard I/O (Win32 API) |

## Architecture

```
pollClipboard (500ms)
  ├── clipboard.ReadAll() → text change detection (existing)
  └── detectImageClipboard() → image availability detection (NEW)
        ↓ (if changed)
  sendFormatList({CF_UNICODETEXT, CF_DIB})
        ↓ (server requests data)
  processFormatDataRequest(fmtID)
  ├── fmtID=13 → readTextClipboard → UTF-16LE encode → send (existing)
  └── fmtID=8  → readImageClipboard → PNG→DIB convert → send (NEW)

Server clipboard change:
  processFormatList(formats)
  ├── has CF_UNICODETEXT → request text → decode → clipboard.WriteAll (existing)
  └── has CF_DIB → request image → DIB→PNG convert → writeImageClipboard (NEW)
```

## Build & Test

```bash
# Build
go build ./...

# Run
go run . --rdp <host>

# Test image clipboard (manual)
# 1. Take a screenshot locally
# 2. Switch to RDP session
# 3. Paste into Paint or similar
# 4. Verify image appears correctly
```

## CF_DIB Format Reference

```
Bytes 0-39:  BITMAPINFOHEADER (40 bytes, LE)
Bytes 40+:   Pixel data, bottom-up rows, BGRA order

Header fields: biSize(4) biWidth(4) biHeight(4) biPlanes(2)
biBitCount(2) biCompression(4) biSizeImage(4) biXPels(4)
biYPels(4) biClrUsed(4) biClrImportant(4)
```
