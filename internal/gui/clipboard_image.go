package gui

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
)

const (
	dibHeaderSize        = 40
	maxImageTransferSize = 20 * 1024 * 1024
)

// encodeDIB encodes an image.Image to a CF_DIB (BITMAPINFOHEADER + BGRA pixels)
// byte slice suitable for transmission over the CLIPRDR virtual channel.
//
// The BITMAPINFOHEADER uses a positive biHeight (bottom-up orientation) and
// 32 bits-per-pixel BI_RGB compression (no colour table). Pixel channels are
// written in BGRA order as required by the DIB format (source image is RGBA).
func encodeDIB(img image.Image) ([]byte, error) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	totalSize := dibHeaderSize + width*height*4
	if totalSize > maxImageTransferSize {
		return nil, fmt.Errorf("clipboard_image: image too large for transfer (%d bytes, max %d)", totalSize, maxImageTransferSize)
	}

	// Ensure we have an *image.RGBA to access pixels directly.
	var rgba *image.RGBA
	if r, ok := img.(*image.RGBA); ok {
		rgba = r
	} else {
		rgba = image.NewRGBA(bounds)
		draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
	}

	buf := &bytes.Buffer{}
	buf.Grow(totalSize)

	// BITMAPINFOHEADER (40 bytes, little-endian).
	binary.Write(buf, binary.LittleEndian, uint32(40))           // biSize
	binary.Write(buf, binary.LittleEndian, int32(width))         // biWidth
	binary.Write(buf, binary.LittleEndian, int32(height))        // biHeight (positive = bottom-up)
	binary.Write(buf, binary.LittleEndian, uint16(1))            // biPlanes
	binary.Write(buf, binary.LittleEndian, uint16(32))           // biBitCount
	binary.Write(buf, binary.LittleEndian, uint32(0))            // biCompression (BI_RGB)
	binary.Write(buf, binary.LittleEndian, uint32(width*height*4)) // biSizeImage
	binary.Write(buf, binary.LittleEndian, int32(0))             // biXPelsPerMeter
	binary.Write(buf, binary.LittleEndian, int32(0))             // biYPelsPerMeter
	binary.Write(buf, binary.LittleEndian, uint32(0))            // biClrUsed
	binary.Write(buf, binary.LittleEndian, uint32(0))            // biClrImportant

	// Pixel rows: bottom-up (row y=height-1 written first).
	row := make([]byte, width*4)
	for y := bounds.Max.Y - 1; y >= bounds.Min.Y; y-- {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			off := (x-bounds.Min.X)*4 + (y-bounds.Min.Y)*rgba.Stride
			r := rgba.Pix[off+0]
			g := rgba.Pix[off+1]
			b := rgba.Pix[off+2]
			a := rgba.Pix[off+3]
			rowOff := (x - bounds.Min.X) * 4
			row[rowOff+0] = b // BGRA: blue first
			row[rowOff+1] = g
			row[rowOff+2] = r
			row[rowOff+3] = a
		}
		buf.Write(row)
	}

	return buf.Bytes(), nil
}

// decodeDIB decodes a CF_DIB byte slice (BITMAPINFOHEADER + BGRA pixels) into
// an *image.RGBA in top-down orientation.
//
// Only 32 bpp BI_RGB DIBs are supported, matching what encodeDIB produces and
// what the RDP server sends in practice for CF_DIB clipboard data.
func decodeDIB(data []byte) (*image.RGBA, error) {
	if len(data) > maxImageTransferSize+dibHeaderSize {
		return nil, fmt.Errorf("clipboard_image: DIB data too large (%d bytes)", len(data))
	}
	if len(data) < dibHeaderSize {
		return nil, fmt.Errorf("clipboard_image: DIB data too short for header (%d bytes)", len(data))
	}

	r := bytes.NewReader(data[:dibHeaderSize])

	var biSize uint32
	binary.Read(r, binary.LittleEndian, &biSize)
	if biSize != 40 {
		return nil, fmt.Errorf("clipboard_image: unsupported BITMAPINFOHEADER size %d (only 40 supported)", biSize)
	}

	var biWidth int32
	var biHeight int32
	binary.Read(r, binary.LittleEndian, &biWidth)
	binary.Read(r, binary.LittleEndian, &biHeight)

	var biPlanes uint16
	var biBitCount uint16
	binary.Read(r, binary.LittleEndian, &biPlanes)
	binary.Read(r, binary.LittleEndian, &biBitCount)

	var biCompression uint32
	binary.Read(r, binary.LittleEndian, &biCompression)

	if biBitCount != 32 {
		return nil, fmt.Errorf("clipboard_image: unsupported biBitCount %d (only 32 supported)", biBitCount)
	}
	if biCompression != 0 {
		return nil, fmt.Errorf("clipboard_image: unsupported biCompression %d (only BI_RGB=0 supported)", biCompression)
	}

	// biHeight > 0: bottom-up; biHeight < 0: top-down.
	bottomUp := biHeight > 0
	width := int(biWidth)
	height := int(biHeight)
	if height < 0 {
		height = -height
	}

	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("clipboard_image: invalid dimensions %dx%d", width, height)
	}

	// Guard against integer overflow: width*height*4 must fit in an int.
	if width > maxImageTransferSize/4/height {
		return nil, fmt.Errorf("clipboard_image: dimensions %dx%d would overflow buffer size", width, height)
	}

	pixelBytes := width * height * 4
	if len(data) < dibHeaderSize+pixelBytes {
		return nil, fmt.Errorf("clipboard_image: DIB data too short for %dx%d image (need %d bytes, have %d)",
			width, height, dibHeaderSize+pixelBytes, len(data))
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	pixels := data[dibHeaderSize:]

	for row := 0; row < height; row++ {
		// Determine which destination row to write.
		var dstY int
		if bottomUp {
			dstY = height - 1 - row
		} else {
			dstY = row
		}

		for col := 0; col < width; col++ {
			srcOff := (row*width + col) * 4
			b := pixels[srcOff+0]
			g := pixels[srcOff+1]
			r := pixels[srcOff+2]
			a := pixels[srcOff+3]

			// Treat fully transparent (a==0) pixels as fully opaque,
			// since CF_DIB without BITMAPV4/V5 header has no reliable alpha.
			if a == 0 {
				a = 0xFF
			}

			dstOff := dstY*img.Stride + col*4
			img.Pix[dstOff+0] = r
			img.Pix[dstOff+1] = g
			img.Pix[dstOff+2] = b
			img.Pix[dstOff+3] = a
		}
	}

	return img, nil
}

// dibToPNG converts CF_DIB bytes to a PNG-encoded byte slice.
func dibToPNG(dibData []byte) ([]byte, error) {
	img, err := decodeDIB(dibData)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("clipboard_image: PNG encode failed: %w", err)
	}
	return buf.Bytes(), nil
}

// pngToDIB converts a PNG-encoded byte slice to CF_DIB bytes.
func pngToDIB(pngData []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("clipboard_image: image decode failed: %w", err)
	}
	return encodeDIB(img)
}

