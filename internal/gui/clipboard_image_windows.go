//go:build windows

package gui

// detectImageClipboard reports whether the local system clipboard holds image
// data on Windows.
//
// TODO: Implement using Win32 API sequence:
//   - win.OpenClipboard(0)
//   - win.IsClipboardFormatAvailable(8) // CF_DIB = 8
//   - win.CloseClipboard()
func detectImageClipboard() bool {
	return false
}

// readImageClipboard reads the current clipboard image as raw PNG bytes on
// Windows.
//
// TODO: Implement using Win32 API sequence:
//   - win.OpenClipboard(0)
//   - hMem := win.GetClipboardData(8) // CF_DIB = 8
//   - ptr := win.GlobalLock(hMem)
//   - copy DIB bytes from ptr
//   - win.GlobalUnlock(hMem)
//   - win.CloseClipboard()
//   - return dibToPNG(dibBytes)
func readImageClipboard() ([]byte, error) {
	return nil, nil
}

// writeImageClipboard writes raw PNG bytes to the local system clipboard on
// Windows.
//
// TODO: Implement using Win32 API sequence:
//   - pngToDIB(pngData) to get CF_DIB bytes
//   - win.OpenClipboard(0)
//   - win.EmptyClipboard()
//   - hMem := win.GlobalAlloc(win.GMEM_MOVEABLE, uintptr(len(dibBytes)))
//   - ptr := win.GlobalLock(hMem)
//   - copy dibBytes to ptr
//   - win.GlobalUnlock(hMem)
//   - win.SetClipboardData(8, hMem) // CF_DIB = 8
//   - win.CloseClipboard()
func writeImageClipboard(data []byte) error {
	return nil
}
