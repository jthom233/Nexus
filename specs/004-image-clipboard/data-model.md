# Data Model: Image Clipboard Support for RDP

**Feature Branch**: `004-image-clipboard`
**Date**: 2026-02-17

## Entities

### ClipboardContent

Represents the detected state of the local system clipboard.

| Field | Type | Description |
|-------|------|-------------|
| hasText | bool | Text content is available on the clipboard |
| hasImage | bool | Image content is available on the clipboard |
| textData | string | The text content (lazy-loaded, only when server requests CF_UNICODETEXT) |
| imageData | []byte | Raw PNG bytes (lazy-loaded, only when server requests CF_DIB) |

**State transitions**:
```
Empty → TextOnly    (user copies text)
Empty → ImageOnly   (user copies screenshot)
Empty → Both        (app puts both text+image on clipboard, e.g. rich content)
TextOnly → ImageOnly (user copies image, replacing text)
ImageOnly → TextOnly (user copies text, replacing image)
Any → Empty         (clipboard cleared)
```

**Detection**: On each poll tick, query platform clipboard for available MIME types (fast, no data read). Compare against previous state. If changed, trigger `sendFormatList` with the updated format set.

### DIBImage

The Windows Device Independent Bitmap wire format used by CLIPRDR.

| Field | Type | Size | Description |
|-------|------|------|-------------|
| biSize | uint32 | 4 | Always 40 (BITMAPINFOHEADER) |
| biWidth | int32 | 4 | Image width in pixels |
| biHeight | int32 | 4 | Image height (positive = bottom-up row order) |
| biPlanes | uint16 | 2 | Always 1 |
| biBitCount | uint16 | 2 | Always 32 (BGRA) |
| biCompression | uint32 | 4 | Always 0 (BI_RGB, uncompressed) |
| biSizeImage | uint32 | 4 | width * height * 4, or 0 |
| biXPelsPerMeter | int32 | 4 | 0 |
| biYPelsPerMeter | int32 | 4 | 0 |
| biClrUsed | uint32 | 4 | 0 |
| biClrImportant | uint32 | 4 | 0 |
| pixelData | []byte | W*H*4 | Bottom-up rows, each pixel: B G R A |

**Total header size**: 40 bytes (fixed)
**Total data size**: 40 + (width * height * 4) bytes

### FormatList

The set of clipboard formats advertised in a CLIPRDR FORMAT_LIST PDU.

| Field | Type | Description |
|-------|------|-------------|
| formats | []FormatEntry | Ordered list of available formats |

**FormatEntry**:

| Field | Type | Description |
|-------|------|-------------|
| formatID | uint32 | Windows clipboard format ID |
| formatName | string | UTF-16LE encoded name (empty for standard formats) |

**Supported format IDs**:

| ID | Constant | Direction | Description |
|----|----------|-----------|-------------|
| 13 | CF_UNICODETEXT | Bidirectional | UTF-16LE text (existing) |
| 8 | CF_DIB | Bidirectional | 32-bit BGRA bitmap (new) |

## Conversion Flows

### Local → Remote (PNG → CF_DIB)

```
1. Read image/png from local clipboard (platform-specific)
2. Decode PNG → Go image.RGBA (top-down, RGBA order)
3. Allocate buffer: 40 bytes header + width*height*4 bytes pixels
4. Write BITMAPINFOHEADER (biHeight = +height for bottom-up)
5. For each row (bottom to top):
     For each pixel:
       Write B, G, R, A (swap R↔B from RGBA source)
6. Send as FORMAT_DATA_RESPONSE payload
```

### Remote → Local (CF_DIB → PNG)

```
1. Receive FORMAT_DATA_RESPONSE payload
2. Parse BITMAPINFOHEADER (first 40 bytes)
3. Validate: biBitCount=32, biCompression=0
4. Read pixel rows (bottom-up if biHeight > 0)
5. For each pixel: swap B↔R to get RGBA order
6. Flip rows to top-down for Go image.RGBA
7. Encode as PNG
8. Write image/png to local clipboard (platform-specific)
```

## Size Constraints

| Metric | Value | Notes |
|--------|-------|-------|
| Max transfer size | 20MB | FR-006 |
| Max image dimensions | ~2290x2290 at 32bpp for 20MB | width*height*4 + 40 ≤ 20MB |
| Typical screenshot | 1920x1080 = ~8MB DIB | Well within limits |
| 4K screenshot | 3840x2160 = ~33MB DIB | Exceeds 20MB — graceful degradation |
