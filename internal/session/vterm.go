package session

import (
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// CellStyle holds the visual attributes of a single terminal cell.
type CellStyle struct {
	FG        string // lipgloss color hex string or empty for default
	BG        string // lipgloss color hex string or empty for default
	Bold      bool
	Underline bool
	Reverse   bool
}

// Cell is a single character cell in the virtual terminal grid.
type Cell struct {
	Char  rune
	Style CellStyle
}

// VTermBuffer is a virtual terminal emulator that interprets ANSI escape
// sequences and produces a renderable cell grid.
type VTermBuffer struct {
	Cells              [][]Cell
	CursorRow          int
	CursorCol          int
	Width              int
	Height             int
	ScrollRegionTop    int
	ScrollRegionBottom int
	DefaultStyle       CellStyle
	mu                 sync.Mutex

	// parser state
	parseState   parserState
	paramBuf     strings.Builder
	currentStyle CellStyle
	pendingWrap  bool // deferred line-wrap: set when cursor reaches end of line

	// UTF-8 multi-byte accumulator: buffers bytes of an in-progress sequence.
	utf8Buf      []byte // bytes collected so far for the current sequence
	utf8Expected int    // total bytes expected for the current sequence (2–4)
}

type parserState int

const (
	stateGround          parserState = iota
	stateEscape                      // received ESC
	stateCSIEntry                    // received ESC [
	stateCSIParam                    // collecting numeric params
	stateCSIIntermediate             // collecting intermediate bytes
	stateOSCString                   // inside OSC (ESC ])
)

// NewVTermBuffer creates a buffer with the given dimensions.
func NewVTermBuffer(width, height int) *VTermBuffer {
	v := &VTermBuffer{
		Width:  width,
		Height: height,
	}
	v.ScrollRegionBottom = height - 1
	v.Cells = makeCellGrid(width, height)
	return v
}

func makeCellGrid(width, height int) [][]Cell {
	cells := make([][]Cell, height)
	for i := range cells {
		cells[i] = make([]Cell, width)
		for j := range cells[i] {
			cells[i][j] = Cell{Char: ' '}
		}
	}
	return cells
}

// Write processes raw terminal output bytes, interpreting ANSI sequences.
// Implements io.Writer.
func (v *VTermBuffer) Write(data []byte) (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	for _, b := range data {
		v.processByte(b)
	}
	return len(data), nil
}

// Resize changes the buffer dimensions, preserving content where possible.
// Shrinking clips; growing fills with spaces.
func (v *VTermBuffer) Resize(width, height int) {
	v.mu.Lock()
	defer v.mu.Unlock()

	newCells := makeCellGrid(width, height)
	copyRows := height
	if v.Height < copyRows {
		copyRows = v.Height
	}
	for r := 0; r < copyRows; r++ {
		copyCols := width
		if v.Width < copyCols {
			copyCols = v.Width
		}
		copy(newCells[r][:copyCols], v.Cells[r][:copyCols])
	}

	v.Cells = newCells
	v.Width = width
	v.Height = height

	// Clamp cursor
	if v.CursorRow >= height {
		v.CursorRow = height - 1
	}
	if v.CursorCol >= width {
		v.CursorCol = width - 1
	}

	// Reset scroll region to full screen
	v.ScrollRegionTop = 0
	v.ScrollRegionBottom = height - 1
}

// Render produces a lipgloss-compatible string representation of the cell grid.
// Pure: does not mutate state.
func (v *VTermBuffer) Render() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.renderRegionLocked(0, v.Height-1)
}

// RenderRegion renders a subregion of the buffer (for scrollback).
func (v *VTermBuffer) RenderRegion(startRow, endRow int) string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.renderRegionLocked(startRow, endRow)
}

// renderRegionLocked renders rows [startRow, endRow] inclusive.
// Caller must hold v.mu.
func (v *VTermBuffer) renderRegionLocked(startRow, endRow int) string {
	if startRow < 0 {
		startRow = 0
	}
	if endRow >= v.Height {
		endRow = v.Height - 1
	}
	if startRow > endRow {
		return ""
	}

	var sb strings.Builder
	for r := startRow; r <= endRow; r++ {
		if r > startRow {
			sb.WriteByte('\n')
		}
		sb.WriteString(renderRow(v.Cells[r]))
	}
	return sb.String()
}

// Clear resets the buffer to empty state.
func (v *VTermBuffer) Clear() {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.Cells = makeCellGrid(v.Width, v.Height)
	v.CursorRow = 0
	v.CursorCol = 0
	v.currentStyle = CellStyle{}
	v.parseState = stateGround
	v.paramBuf.Reset()
	v.pendingWrap = false
	v.ScrollRegionTop = 0
	v.ScrollRegionBottom = v.Height - 1
}

// renderRow renders a single row as a string, batching consecutive same-style cells.
func renderRow(row []Cell) string {
	if len(row) == 0 {
		return ""
	}

	var sb strings.Builder
	batchStart := 0
	for i := 1; i <= len(row); i++ {
		if i == len(row) || row[i].Style != row[batchStart].Style {
			s := row[batchStart].Style
			chars := make([]rune, i-batchStart)
			for j, c := range row[batchStart:i] {
				chars[j] = c.Char
			}
			text := string(chars)
			sb.WriteString(applyStyle(s, text))
			batchStart = i
		}
	}
	return sb.String()
}

// applyStyle renders text with the given CellStyle using lipgloss.
func applyStyle(s CellStyle, text string) string {
	// Fast path: no styling
	if s.FG == "" && s.BG == "" && !s.Bold && !s.Underline && !s.Reverse {
		return text
	}

	style := lipgloss.NewStyle()
	if s.FG != "" {
		style = style.Foreground(lipgloss.Color(s.FG))
	}
	if s.BG != "" {
		style = style.Background(lipgloss.Color(s.BG))
	}
	if s.Bold {
		style = style.Bold(true)
	}
	if s.Underline {
		style = style.Underline(true)
	}
	if s.Reverse {
		style = style.Reverse(true)
	}
	return style.Render(text)
}

// --------------------------------------------------------------------------
// ANSI Parser State Machine
// --------------------------------------------------------------------------

func (v *VTermBuffer) processByte(b byte) {
	switch v.parseState {
	case stateGround:
		v.processGround(b)
	case stateEscape:
		v.processEscape(b)
	case stateCSIEntry, stateCSIParam, stateCSIIntermediate:
		v.processCSI(b)
	case stateOSCString:
		v.processOSC(b)
	}
}

func (v *VTermBuffer) processGround(b byte) {
	// If we are mid-way through a multi-byte UTF-8 sequence, accumulate bytes.
	if v.utf8Expected > 0 {
		if b&0xC0 == 0x80 {
			// Valid continuation byte.
			v.utf8Buf = append(v.utf8Buf, b)
			if len(v.utf8Buf) == v.utf8Expected {
				// Sequence complete — decode and emit.
				r, _ := utf8.DecodeRune(v.utf8Buf)
				if r != utf8.RuneError {
					v.putChar(r)
				}
				v.utf8Buf = v.utf8Buf[:0]
				v.utf8Expected = 0
			}
		} else {
			// Invalid continuation — discard accumulated bytes and reprocess.
			v.utf8Buf = v.utf8Buf[:0]
			v.utf8Expected = 0
			v.processGround(b)
		}
		return
	}

	switch b {
	case 0x1B: // ESC
		v.parseState = stateEscape
	case '\n': // LF
		v.lineFeed()
	case '\r': // CR
		v.CursorCol = 0
		v.pendingWrap = false
	case '\t': // HT — advance to next 8-column tab stop
		v.tab()
	case '\b': // BS
		if v.CursorCol > 0 {
			v.CursorCol--
		}
	default:
		switch {
		case b >= 0xF0 && b <= 0xF7: // 4-byte sequence lead
			v.utf8Buf = append(v.utf8Buf[:0], b)
			v.utf8Expected = 4
		case b >= 0xE0 && b <= 0xEF: // 3-byte sequence lead
			v.utf8Buf = append(v.utf8Buf[:0], b)
			v.utf8Expected = 3
		case b >= 0xC0 && b <= 0xDF: // 2-byte sequence lead
			v.utf8Buf = append(v.utf8Buf[:0], b)
			v.utf8Expected = 2
		case b >= 0x20: // printable ASCII
			v.putChar(rune(b))
		}
		// other C0 controls and lone continuation bytes are silently dropped
	}
}

func (v *VTermBuffer) processEscape(b byte) {
	switch b {
	case '[': // CSI
		v.paramBuf.Reset()
		v.parseState = stateCSIEntry
	case ']': // OSC
		v.paramBuf.Reset()
		v.parseState = stateOSCString
	case 'c': // RIS — full reset
		v.Cells = makeCellGrid(v.Width, v.Height)
		v.CursorRow = 0
		v.CursorCol = 0
		v.currentStyle = CellStyle{}
		v.ScrollRegionTop = 0
		v.ScrollRegionBottom = v.Height - 1
		v.parseState = stateGround
	default:
		// Unknown two-character escape: silently drop
		v.parseState = stateGround
	}
}

func (v *VTermBuffer) processCSI(b byte) {
	switch {
	case b >= 0x30 && b <= 0x3F: // parameter bytes: digits, semicolon, <, =, >, ?
		if v.parseState == stateCSIEntry {
			v.parseState = stateCSIParam
		}
		v.paramBuf.WriteByte(b)
	case b >= 0x20 && b <= 0x2F: // intermediate bytes
		v.parseState = stateCSIIntermediate
		v.paramBuf.WriteByte(b)
	case b >= 0x40 && b <= 0x7E: // final byte — dispatch
		v.dispatchCSI(b)
		v.parseState = stateGround
	default:
		// Unexpected byte in CSI; abort sequence
		v.parseState = stateGround
	}
}

func (v *VTermBuffer) processOSC(b byte) {
	// OSC terminated by BEL (0x07) or ST (ESC \, but we simplify to ESC)
	if b == 0x07 || b == 0x1B {
		v.parseState = stateGround
	}
	// OSC content (e.g. window title, color queries) is silently dropped
}

// dispatchCSI handles a completed CSI sequence.
func (v *VTermBuffer) dispatchCSI(final byte) {
	raw := v.paramBuf.String()

	// Private mode sequences begin with '?'
	if strings.HasPrefix(raw, "?") {
		// DECSET/DECRST — cursor visibility etc. No-op for render buffer.
		return
	}

	params := parseParams(raw)

	switch final {
	case 'A': // CUU — cursor up
		n := paramDefault(params, 0, 1)
		v.moveCursor(v.CursorRow-n, v.CursorCol)
	case 'B': // CUD — cursor down
		n := paramDefault(params, 0, 1)
		v.moveCursor(v.CursorRow+n, v.CursorCol)
	case 'C': // CUF — cursor forward
		n := paramDefault(params, 0, 1)
		v.moveCursor(v.CursorRow, v.CursorCol+n)
	case 'D': // CUB — cursor back
		n := paramDefault(params, 0, 1)
		v.moveCursor(v.CursorRow, v.CursorCol-n)
	case 'H', 'f': // CUP / HVP — set cursor position (1-based in protocol)
		row := paramDefault(params, 0, 1) - 1
		col := paramDefault(params, 1, 1) - 1
		v.moveCursor(row, col)
	case 'J': // ED — erase in display
		n := paramDefault(params, 0, 0)
		v.eraseDisplay(n)
	case 'K': // EL — erase in line
		n := paramDefault(params, 0, 0)
		v.eraseLine(n)
	case 'm': // SGR — select graphic rendition
		v.applySGR(params)
	case 'S': // SU — scroll up
		n := paramDefault(params, 0, 1)
		v.scrollUp(n)
	case 'T': // SD — scroll down
		n := paramDefault(params, 0, 1)
		v.scrollDown(n)
	case 'r': // DECSTBM — set top and bottom margins (scroll region), 1-based
		top := paramDefault(params, 0, 1) - 1
		bot := paramDefault(params, 1, v.Height) - 1
		if top < 0 {
			top = 0
		}
		if bot >= v.Height {
			bot = v.Height - 1
		}
		if top < bot {
			v.ScrollRegionTop = top
			v.ScrollRegionBottom = bot
			// Cursor moves to home on DECSTBM
			v.CursorRow = 0
			v.CursorCol = 0
		}
	// All other sequences are silently dropped
	}
}

// --------------------------------------------------------------------------
// Cursor and Cell Operations
// --------------------------------------------------------------------------

func (v *VTermBuffer) moveCursor(row, col int) {
	if row < 0 {
		row = 0
	}
	if row >= v.Height {
		row = v.Height - 1
	}
	if col < 0 {
		col = 0
	}
	if col >= v.Width {
		col = v.Width - 1
	}
	v.CursorRow = row
	v.CursorCol = col
	v.pendingWrap = false
}

func (v *VTermBuffer) putChar(r rune) {
	if v.CursorRow < 0 || v.CursorRow >= v.Height {
		return
	}
	// Deferred wrap: flush pending wrap before placing the next character
	if v.pendingWrap {
		v.pendingWrap = false
		v.CursorCol = 0
		v.lineFeed()
	}
	if v.CursorCol < 0 || v.CursorCol >= v.Width {
		return
	}
	v.Cells[v.CursorRow][v.CursorCol] = Cell{Char: r, Style: v.currentStyle}
	v.CursorCol++
	if v.CursorCol >= v.Width {
		// Don't wrap immediately — set pending flag. The wrap fires on the next printable char.
		v.pendingWrap = true
		v.CursorCol = v.Width - 1 // keep cursor visually at last column
	}
}

func (v *VTermBuffer) lineFeed() {
	if v.CursorRow < v.ScrollRegionBottom {
		v.CursorRow++
	} else {
		// At bottom of scroll region: scroll up one line
		v.scrollUp(1)
	}
}

func (v *VTermBuffer) tab() {
	next := ((v.CursorCol / 8) + 1) * 8
	if next >= v.Width {
		next = v.Width - 1
	}
	v.CursorCol = next
}

func (v *VTermBuffer) scrollUp(n int) {
	top := v.ScrollRegionTop
	bot := v.ScrollRegionBottom
	for i := 0; i < n; i++ {
		for r := top; r < bot; r++ {
			// Copy cell data to avoid slice aliasing
			copy(v.Cells[r], v.Cells[r+1])
		}
		// Clear the newly vacated bottom row
		for c := range v.Cells[bot] {
			v.Cells[bot][c] = Cell{Char: ' '}
		}
	}
}

func (v *VTermBuffer) scrollDown(n int) {
	top := v.ScrollRegionTop
	bot := v.ScrollRegionBottom
	for i := 0; i < n; i++ {
		for r := bot; r > top; r-- {
			// Copy cell data to avoid slice aliasing
			copy(v.Cells[r], v.Cells[r-1])
		}
		// Clear the newly vacated top row
		for c := range v.Cells[top] {
			v.Cells[top][c] = Cell{Char: ' '}
		}
	}
}

func (v *VTermBuffer) eraseDisplay(mode int) {
	switch mode {
	case 0: // Erase from cursor to end of screen
		for c := v.CursorCol; c < v.Width; c++ {
			v.Cells[v.CursorRow][c] = Cell{Char: ' '}
		}
		for r := v.CursorRow + 1; r < v.Height; r++ {
			for c := range v.Cells[r] {
				v.Cells[r][c] = Cell{Char: ' '}
			}
		}
	case 1: // Erase from beginning of screen to cursor
		for r := 0; r < v.CursorRow; r++ {
			for c := range v.Cells[r] {
				v.Cells[r][c] = Cell{Char: ' '}
			}
		}
		for c := 0; c <= v.CursorCol && c < v.Width; c++ {
			v.Cells[v.CursorRow][c] = Cell{Char: ' '}
		}
	case 2, 3: // Erase entire screen
		for r := range v.Cells {
			for c := range v.Cells[r] {
				v.Cells[r][c] = Cell{Char: ' '}
			}
		}
	}
}

func (v *VTermBuffer) eraseLine(mode int) {
	if v.CursorRow < 0 || v.CursorRow >= v.Height {
		return
	}
	switch mode {
	case 0: // Erase from cursor to end of line
		for c := v.CursorCol; c < v.Width; c++ {
			v.Cells[v.CursorRow][c] = Cell{Char: ' '}
		}
	case 1: // Erase from beginning of line to cursor
		for c := 0; c <= v.CursorCol && c < v.Width; c++ {
			v.Cells[v.CursorRow][c] = Cell{Char: ' '}
		}
	case 2: // Erase entire line
		for c := range v.Cells[v.CursorRow] {
			v.Cells[v.CursorRow][c] = Cell{Char: ' '}
		}
	}
}

// --------------------------------------------------------------------------
// SGR (Select Graphic Rendition)
// --------------------------------------------------------------------------

func (v *VTermBuffer) applySGR(params []int) {
	if len(params) == 0 {
		params = []int{0}
	}
	for i := 0; i < len(params); i++ {
		p := params[i]
		switch {
		case p == 0:
			v.currentStyle = CellStyle{}
		case p == 1:
			v.currentStyle.Bold = true
		case p == 4:
			v.currentStyle.Underline = true
		case p == 7:
			v.currentStyle.Reverse = true
		case p == 22:
			v.currentStyle.Bold = false
		case p == 24:
			v.currentStyle.Underline = false
		case p == 27:
			v.currentStyle.Reverse = false
		case p == 39:
			v.currentStyle.FG = "" // default foreground
		case p == 49:
			v.currentStyle.BG = "" // default background

		case p >= 30 && p <= 37: // standard foreground
			v.currentStyle.FG = ansiColor(p - 30)
		case p >= 40 && p <= 47: // standard background
			v.currentStyle.BG = ansiColor(p - 40)
		case p >= 90 && p <= 97: // bright foreground
			v.currentStyle.FG = ansiBrightColor(p - 90)
		case p >= 100 && p <= 107: // bright background
			v.currentStyle.BG = ansiBrightColor(p - 100)

		case p == 38 || p == 48: // extended color
			if i+1 >= len(params) {
				return
			}
			subtype := params[i+1]
			isFG := p == 38
			switch subtype {
			case 5: // 256-color: 38;5;N
				if i+2 < len(params) {
					color := xterm256Color(params[i+2])
					if isFG {
						v.currentStyle.FG = color
					} else {
						v.currentStyle.BG = color
					}
					i += 2
				}
			case 2: // truecolor: 38;2;R;G;B
				if i+4 < len(params) {
					color := fmt.Sprintf("#%02x%02x%02x", params[i+2], params[i+3], params[i+4])
					if isFG {
						v.currentStyle.FG = color
					} else {
						v.currentStyle.BG = color
					}
					i += 4
				}
			}
		}
	}
}

// --------------------------------------------------------------------------
// Color Tables
// --------------------------------------------------------------------------

// ansiColor returns the hex color for a standard ANSI color index (0-7).
func ansiColor(n int) string {
	colors := [8]string{
		"#000000", // 0 black
		"#cc0000", // 1 red
		"#4e9a06", // 2 green
		"#c4a000", // 3 yellow
		"#3465a4", // 4 blue
		"#75507b", // 5 magenta
		"#06989a", // 6 cyan
		"#d3d7cf", // 7 white
	}
	if n < 0 || n > 7 {
		return ""
	}
	return colors[n]
}

// ansiBrightColor returns the hex color for a bright ANSI color index (0-7).
func ansiBrightColor(n int) string {
	colors := [8]string{
		"#555753", // 0 bright black (dark gray)
		"#ef2929", // 1 bright red
		"#8ae234", // 2 bright green
		"#fce94f", // 3 bright yellow
		"#729fcf", // 4 bright blue
		"#ad7fa8", // 5 bright magenta
		"#34e2e2", // 6 bright cyan
		"#eeeeec", // 7 bright white
	}
	if n < 0 || n > 7 {
		return ""
	}
	return colors[n]
}

// xterm256Color returns the hex color for an xterm-256 color index.
// Index 0-7: standard, 8-15: bright, 16-231: 6x6x6 cube, 232-255: grayscale.
func xterm256Color(n int) string {
	if n < 0 || n > 255 {
		return ""
	}
	if n < 8 {
		return ansiColor(n)
	}
	if n < 16 {
		return ansiBrightColor(n - 8)
	}
	if n < 232 {
		// 6x6x6 RGB cube
		idx := n - 16
		b := idx % 6
		g := (idx / 6) % 6
		r := idx / 36
		cubeVal := func(v int) int {
			if v == 0 {
				return 0
			}
			return 55 + v*40
		}
		return fmt.Sprintf("#%02x%02x%02x", cubeVal(r), cubeVal(g), cubeVal(b))
	}
	// Grayscale ramp: 232 = #080808, step of 10
	gray := 8 + (n-232)*10
	return fmt.Sprintf("#%02x%02x%02x", gray, gray, gray)
}

// --------------------------------------------------------------------------
// Parameter Parsing Helpers
// --------------------------------------------------------------------------

// parseParams parses a semicolon-separated CSI parameter string into integers.
// Empty segments are treated as 0.
func parseParams(s string) []int {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	result := make([]int, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			result = append(result, 0)
			continue
		}
		n := 0
		for _, c := range p {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			}
		}
		result = append(result, n)
	}
	return result
}

// paramDefault returns params[idx] if it exists and is non-zero, otherwise def.
func paramDefault(params []int, idx, def int) int {
	if idx < len(params) && params[idx] != 0 {
		return params[idx]
	}
	return def
}
