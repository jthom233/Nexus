package session

import (
	"strings"
	"sync"
	"testing"
)

// helper: write a string to the vterm
func write(v *VTermBuffer, s string) {
	v.Write([]byte(s))
}

// helper: cell at (row, col)
func cell(v *VTermBuffer, row, col int) Cell {
	return v.Cells[row][col]
}

// --------------------------------------------------------------------------
// T002 — NewVTermBuffer
// --------------------------------------------------------------------------

func TestNewVTermBuffer_Dimensions(t *testing.T) {
	v := NewVTermBuffer(80, 24)
	if v.Width != 80 {
		t.Errorf("Width: want 80, got %d", v.Width)
	}
	if v.Height != 24 {
		t.Errorf("Height: want 24, got %d", v.Height)
	}
	if len(v.Cells) != 24 {
		t.Errorf("Cells rows: want 24, got %d", len(v.Cells))
	}
	if len(v.Cells[0]) != 80 {
		t.Errorf("Cells cols: want 80, got %d", len(v.Cells[0]))
	}
}

func TestNewVTermBuffer_InitialState(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	if v.CursorRow != 0 || v.CursorCol != 0 {
		t.Errorf("cursor should start at (0,0), got (%d,%d)", v.CursorRow, v.CursorCol)
	}
	if v.ScrollRegionTop != 0 {
		t.Errorf("ScrollRegionTop: want 0, got %d", v.ScrollRegionTop)
	}
	if v.ScrollRegionBottom != 4 {
		t.Errorf("ScrollRegionBottom: want 4, got %d", v.ScrollRegionBottom)
	}
	// All cells should be space with zero style
	for r := range v.Cells {
		for c, cell := range v.Cells[r] {
			if cell.Char != ' ' {
				t.Errorf("cell[%d][%d] char: want ' ', got %q", r, c, cell.Char)
			}
			if cell.Style != (CellStyle{}) {
				t.Errorf("cell[%d][%d] style: want zero, got %+v", r, c, cell.Style)
			}
		}
	}
}

// --------------------------------------------------------------------------
// T004 — Printable characters and basic cursor movement
// --------------------------------------------------------------------------

func TestWrite_PrintableChars(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "Hello")
	for i, ch := range "Hello" {
		if got := cell(v, 0, i).Char; got != ch {
			t.Errorf("col %d: want %q, got %q", i, ch, got)
		}
	}
	if v.CursorCol != 5 {
		t.Errorf("cursor col after 5 chars: want 5, got %d", v.CursorCol)
	}
}

func TestWrite_CarriageReturn(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "Hello\r")
	if v.CursorCol != 0 {
		t.Errorf("CR: cursor col want 0, got %d", v.CursorCol)
	}
}

func TestWrite_LineFeed_NoScroll(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	// \r\n (CRLF) is the standard way to move to start of next line.
	// \n alone (LF) only moves down — cursor column is unchanged.
	write(v, "A\r\nB")
	if cell(v, 0, 0).Char != 'A' {
		t.Errorf("row 0 col 0: want 'A', got %q", cell(v, 0, 0).Char)
	}
	if cell(v, 1, 0).Char != 'B' {
		t.Errorf("row 1 col 0: want 'B', got %q", cell(v, 1, 0).Char)
	}
}

func TestWrite_LineFeed_LFOnly(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	// \n (LF) moves cursor down but does NOT reset column.
	write(v, "AB\nC")
	// After "AB" cursor is at (0,2). \n moves to (1,2). "C" at (1,2).
	if cell(v, 0, 0).Char != 'A' {
		t.Errorf("row 0 col 0: want 'A', got %q", cell(v, 0, 0).Char)
	}
	if cell(v, 1, 2).Char != 'C' {
		t.Errorf("row 1 col 2: want 'C', got %q", cell(v, 1, 2).Char)
	}
}

func TestWrite_LineFeed_Scroll(t *testing.T) {
	v := NewVTermBuffer(10, 3)
	// Use CRLF so each line starts at column 0. Filling 3 rows then one more triggers scroll.
	write(v, "AAA\r\nBBB\r\nCCC\r\nDDD")
	// After the 4th CRLF+content, CCC scrolled out and top is now BBB.
	if cell(v, 0, 0).Char != 'B' {
		t.Errorf("row 0 after scroll: want 'B', got %q", cell(v, 0, 0).Char)
	}
	if cell(v, 1, 0).Char != 'C' {
		t.Errorf("row 1 after scroll: want 'C', got %q", cell(v, 1, 0).Char)
	}
	if cell(v, 2, 0).Char != 'D' {
		t.Errorf("row 2 after scroll: want 'D', got %q", cell(v, 2, 0).Char)
	}
}

func TestWrite_Backspace(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "AB\bC")
	if cell(v, 0, 1).Char != 'C' {
		t.Errorf("after backspace: col 1 want 'C', got %q", cell(v, 0, 1).Char)
	}
}

func TestWrite_Tab(t *testing.T) {
	v := NewVTermBuffer(20, 5)
	write(v, "A\tB")
	// Tab advances to col 8
	if v.CursorCol != 9 { // 'B' was at 8, cursor is now at 9
		t.Errorf("cursor after tab: want 9, got %d", v.CursorCol)
	}
	if cell(v, 0, 8).Char != 'B' {
		t.Errorf("char after tab: want 'B' at col 8, got %q", cell(v, 0, 8).Char)
	}
}

func TestWrite_WrapAround(t *testing.T) {
	v := NewVTermBuffer(5, 3)
	// Write exactly 5 chars to fill row 0, then one more to trigger the deferred wrap.
	// With deferred wrap, the wrap fires on the next printable character.
	write(v, "ABCDEF")
	// After 5 chars: pendingWrap=true. 'F' triggers wrap → row 1, col 0, then 'F' written.
	if v.CursorRow != 1 {
		t.Errorf("after wrap: cursor row want 1, got %d", v.CursorRow)
	}
	if v.CursorCol != 1 {
		t.Errorf("after wrap: cursor col want 1, got %d", v.CursorCol)
	}
	if cell(v, 1, 0).Char != 'F' {
		t.Errorf("after wrap: 'F' should be at (1,0), got %q", cell(v, 1, 0).Char)
	}
}

// --------------------------------------------------------------------------
// T005 — Cursor movement sequences
// --------------------------------------------------------------------------

func TestCSI_CursorUp(t *testing.T) {
	v := NewVTermBuffer(10, 10)
	write(v, "\x1b[5;1H") // move to row 5, col 1 (1-based)
	write(v, "\x1b[2A")   // up 2
	if v.CursorRow != 2 {
		t.Errorf("CUU: want row 2, got %d", v.CursorRow)
	}
}

func TestCSI_CursorDown(t *testing.T) {
	v := NewVTermBuffer(10, 10)
	write(v, "\x1b[3B") // down 3 from row 0
	if v.CursorRow != 3 {
		t.Errorf("CUD: want row 3, got %d", v.CursorRow)
	}
}

func TestCSI_CursorForward(t *testing.T) {
	v := NewVTermBuffer(10, 10)
	write(v, "\x1b[4C") // right 4
	if v.CursorCol != 4 {
		t.Errorf("CUF: want col 4, got %d", v.CursorCol)
	}
}

func TestCSI_CursorBack(t *testing.T) {
	v := NewVTermBuffer(10, 10)
	write(v, "\x1b[5C") // right 5
	write(v, "\x1b[2D") // back 2
	if v.CursorCol != 3 {
		t.Errorf("CUB: want col 3, got %d", v.CursorCol)
	}
}

func TestCSI_CursorPosition(t *testing.T) {
	v := NewVTermBuffer(80, 24)
	write(v, "\x1b[5;10H") // row 5, col 10 (1-based → 4, 9)
	if v.CursorRow != 4 {
		t.Errorf("CUP row: want 4, got %d", v.CursorRow)
	}
	if v.CursorCol != 9 {
		t.Errorf("CUP col: want 9, got %d", v.CursorCol)
	}
}

func TestCSI_CursorPosition_Home(t *testing.T) {
	v := NewVTermBuffer(80, 24)
	write(v, "\x1b[5;10H")
	write(v, "\x1b[H") // home (no params = 1;1)
	if v.CursorRow != 0 || v.CursorCol != 0 {
		t.Errorf("CUP home: want (0,0), got (%d,%d)", v.CursorRow, v.CursorCol)
	}
}

func TestCSI_CursorClampAtBounds(t *testing.T) {
	v := NewVTermBuffer(5, 5)
	write(v, "\x1b[100;100H") // way out of bounds
	if v.CursorRow != 4 {
		t.Errorf("clamped row: want 4, got %d", v.CursorRow)
	}
	if v.CursorCol != 4 {
		t.Errorf("clamped col: want 4, got %d", v.CursorCol)
	}
}

// --------------------------------------------------------------------------
// T006 — Erase sequences
// --------------------------------------------------------------------------

func TestCSI_EraseLineRight(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "Hello")
	write(v, "\x1b[1;3H") // move to row 1 col 3 (0-based: 0, 2)
	write(v, "\x1b[K")    // erase to end of line
	for c := 2; c < 10; c++ {
		if cell(v, 0, c).Char != ' ' {
			t.Errorf("EL0: col %d should be space, got %q", c, cell(v, 0, c).Char)
		}
	}
	// Chars before cursor preserved
	if cell(v, 0, 0).Char != 'H' {
		t.Errorf("EL0: col 0 should be 'H', got %q", cell(v, 0, 0).Char)
	}
}

func TestCSI_EraseLineLeft(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "Hello")
	write(v, "\x1b[1;4H") // col 4 (0-based: 0, 3)
	write(v, "\x1b[1K")   // erase to beginning
	for c := 0; c <= 3; c++ {
		if cell(v, 0, c).Char != ' ' {
			t.Errorf("EL1: col %d should be space, got %q", c, cell(v, 0, c).Char)
		}
	}
}

func TestCSI_EraseLineAll(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "Hello")
	write(v, "\x1b[1;1H") // home
	write(v, "\x1b[2K")   // erase entire line
	for c := 0; c < 10; c++ {
		if cell(v, 0, c).Char != ' ' {
			t.Errorf("EL2: col %d should be space, got %q", c, cell(v, 0, c).Char)
		}
	}
}

func TestCSI_EraseDisplayBelow(t *testing.T) {
	v := NewVTermBuffer(5, 4)
	write(v, "AAAAABBBBBCCCCCDDDD")
	write(v, "\x1b[2;3H") // row 2, col 3 (0-based: 1, 2)
	write(v, "\x1b[J")    // erase below
	// Row 0 fully preserved
	if cell(v, 0, 0).Char != 'A' {
		t.Errorf("ED0: row 0 should be preserved")
	}
	// Row 1 cols 0-1 preserved, col 2+ cleared
	if cell(v, 1, 0).Char != 'B' {
		t.Errorf("ED0: row 1 col 0 should be 'B', got %q", cell(v, 1, 0).Char)
	}
	for c := 2; c < 5; c++ {
		if cell(v, 1, c).Char != ' ' {
			t.Errorf("ED0: row 1 col %d should be space", c)
		}
	}
	// Rows 2-3 fully cleared
	for r := 2; r < 4; r++ {
		for c := 0; c < 5; c++ {
			if cell(v, r, c).Char != ' ' {
				t.Errorf("ED0: row %d col %d should be space", r, c)
			}
		}
	}
}

func TestCSI_EraseDisplayAll(t *testing.T) {
	v := NewVTermBuffer(5, 3)
	write(v, "AAAAABBBBBCCCCC")
	write(v, "\x1b[2J") // erase all
	for r := 0; r < 3; r++ {
		for c := 0; c < 5; c++ {
			if cell(v, r, c).Char != ' ' {
				t.Errorf("ED2: row %d col %d should be space", r, c)
			}
		}
	}
}

// --------------------------------------------------------------------------
// T007 — SGR colors and attributes
// --------------------------------------------------------------------------

func TestSGR_Reset(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "\x1b[1;31mHello")
	write(v, "\x1b[mReset")
	// Chars after reset should have no styling
	if cell(v, 0, 5).Style != (CellStyle{}) {
		t.Errorf("SGR reset: expected zero style, got %+v", cell(v, 0, 5).Style)
	}
}

func TestSGR_BoldUnderlineReverse(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "\x1b[1;4;7mX")
	s := cell(v, 0, 0).Style
	if !s.Bold {
		t.Error("SGR 1: Bold not set")
	}
	if !s.Underline {
		t.Error("SGR 4: Underline not set")
	}
	if !s.Reverse {
		t.Error("SGR 7: Reverse not set")
	}
}

func TestSGR_StandardFGBG(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "\x1b[31;42mX") // red fg, green bg
	s := cell(v, 0, 0).Style
	if s.FG != "#cc0000" {
		t.Errorf("SGR 31: FG want #cc0000, got %s", s.FG)
	}
	if s.BG != "#4e9a06" {
		t.Errorf("SGR 42: BG want #4e9a06, got %s", s.BG)
	}
}

func TestSGR_BrightColors(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "\x1b[91;101mX") // bright red fg, bright red bg
	s := cell(v, 0, 0).Style
	if s.FG != "#ef2929" {
		t.Errorf("SGR 91: FG want #ef2929, got %s", s.FG)
	}
	if s.BG != "#ef2929" {
		t.Errorf("SGR 101: BG want #ef2929, got %s", s.BG)
	}
}

func TestSGR_256Color(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "\x1b[38;5;196mX") // color 196 = bright red in xterm-256
	s := cell(v, 0, 0).Style
	if s.FG == "" {
		t.Error("SGR 38;5: FG should be set")
	}
}

func TestSGR_Truecolor(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "\x1b[38;2;255;128;0mX") // orange
	s := cell(v, 0, 0).Style
	if s.FG != "#ff8000" {
		t.Errorf("SGR 38;2: FG want #ff8000, got %s", s.FG)
	}
}

func TestSGR_DefaultColors(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "\x1b[31;42mX") // set colors
	write(v, "\x1b[39;49mY") // reset to default
	s := cell(v, 0, 1).Style
	if s.FG != "" {
		t.Errorf("SGR 39: FG should be empty (default), got %s", s.FG)
	}
	if s.BG != "" {
		t.Errorf("SGR 49: BG should be empty (default), got %s", s.BG)
	}
}

// --------------------------------------------------------------------------
// T008 — Scroll sequences and scroll region
// --------------------------------------------------------------------------

func TestCSI_ScrollUp(t *testing.T) {
	v := NewVTermBuffer(5, 4)
	write(v, "AAAAA\r\nBBBBB\r\nCCCCC\r\nDDDDD")
	write(v, "\x1b[1;1H")
	write(v, "\x1b[2S") // scroll up 2
	// After scrolling up 2: row 0 = CCCCC, row 1 = DDDDD, rows 2-3 cleared
	if cell(v, 0, 0).Char != 'C' {
		t.Errorf("SU: row 0 want 'C', got %q", cell(v, 0, 0).Char)
	}
	if cell(v, 1, 0).Char != 'D' {
		t.Errorf("SU: row 1 want 'D', got %q", cell(v, 1, 0).Char)
	}
	// rows 2-3 should be cleared
	if cell(v, 2, 0).Char != ' ' {
		t.Errorf("SU: row 2 should be space, got %q", cell(v, 2, 0).Char)
	}
}

func TestCSI_ScrollDown(t *testing.T) {
	v := NewVTermBuffer(5, 3)
	// Use CRLF to fill rows predictably
	write(v, "AAAAA\r\nBBBBB\r\nCCCCC")
	write(v, "\x1b[1;1H") // cursor home (doesn't affect content)
	write(v, "\x1b[1T")   // scroll down 1
	// Row 0 should be cleared, row 1 = AAAAA, row 2 = BBBBB
	if cell(v, 0, 0).Char != ' ' {
		t.Errorf("SD: row 0 should be space, got %q", cell(v, 0, 0).Char)
	}
	if cell(v, 1, 0).Char != 'A' {
		t.Errorf("SD: row 1 want 'A', got %q", cell(v, 1, 0).Char)
	}
	if cell(v, 2, 0).Char != 'B' {
		t.Errorf("SD: row 2 want 'B', got %q", cell(v, 2, 0).Char)
	}
}

func TestCSI_ScrollRegion(t *testing.T) {
	v := NewVTermBuffer(5, 5)
	// Use CRLF to fill rows predictably (no accidental scrolling)
	write(v, "AAAAA\r\nBBBBB\r\nCCCCC\r\nDDDDD\r\nEEEEE")
	// Set scroll region to rows 2-4 (1-based → 0-based: 1-3)
	write(v, "\x1b[2;4r")
	if v.ScrollRegionTop != 1 {
		t.Errorf("DECSTBM top: want 1, got %d", v.ScrollRegionTop)
	}
	if v.ScrollRegionBottom != 3 {
		t.Errorf("DECSTBM bottom: want 3, got %d", v.ScrollRegionBottom)
	}
	// Cursor should move to home
	if v.CursorRow != 0 || v.CursorCol != 0 {
		t.Errorf("DECSTBM: cursor should be at home, got (%d,%d)", v.CursorRow, v.CursorCol)
	}
	// Row 0 (outside scroll region) should still be 'A'
	if cell(v, 0, 0).Char != 'A' {
		t.Errorf("DECSTBM: row 0 should be 'A', got %q", cell(v, 0, 0).Char)
	}
	// Row 4 (outside scroll region) should still be 'E'
	if cell(v, 4, 0).Char != 'E' {
		t.Errorf("DECSTBM: row 4 should be 'E', got %q", cell(v, 4, 0).Char)
	}
}

// --------------------------------------------------------------------------
// T009 — Resize, Render, RenderRegion, Clear
// --------------------------------------------------------------------------

func TestResize_Grow(t *testing.T) {
	v := NewVTermBuffer(5, 3)
	write(v, "Hello")
	v.Resize(10, 5)
	if v.Width != 10 || v.Height != 5 {
		t.Errorf("Resize grow: want 10x5, got %dx%d", v.Width, v.Height)
	}
	// Original content preserved
	if cell(v, 0, 0).Char != 'H' {
		t.Errorf("Resize grow: original char lost, got %q", cell(v, 0, 0).Char)
	}
	// New cells are spaces
	if cell(v, 4, 9).Char != ' ' {
		t.Errorf("Resize grow: new cell should be space, got %q", cell(v, 4, 9).Char)
	}
}

func TestResize_Shrink(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "Hello World")
	v.Resize(5, 3)
	if v.Width != 5 || v.Height != 3 {
		t.Errorf("Resize shrink: want 5x3, got %dx%d", v.Width, v.Height)
	}
	// Content in visible region preserved
	if cell(v, 0, 0).Char != 'H' {
		t.Errorf("Resize shrink: char 'H' lost")
	}
}

func TestResize_CursorClamped(t *testing.T) {
	v := NewVTermBuffer(10, 10)
	write(v, "\x1b[8;8H") // row 8, col 8 (0-based: 7, 7)
	v.Resize(5, 5)
	if v.CursorRow >= 5 {
		t.Errorf("cursor row not clamped: got %d", v.CursorRow)
	}
	if v.CursorCol >= 5 {
		t.Errorf("cursor col not clamped: got %d", v.CursorCol)
	}
}

func TestRender_Basic(t *testing.T) {
	// Use width > len("Hello") to avoid wrap-triggered scroll
	v := NewVTermBuffer(20, 2)
	write(v, "Hello\r\nWorld")
	out := v.Render()
	if !strings.Contains(out, "Hello") {
		t.Errorf("Render: missing 'Hello' in %q", out)
	}
	if !strings.Contains(out, "World") {
		t.Errorf("Render: missing 'World' in %q", out)
	}
	// Two rows = one newline separator
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Errorf("Render: want 2 lines, got %d", len(lines))
	}
}

func TestRenderRegion_Subset(t *testing.T) {
	v := NewVTermBuffer(5, 4)
	write(v, "AAAAABBBBBCCCCCDDDD")
	out := v.RenderRegion(1, 2) // rows 1 and 2 (0-based)
	if !strings.Contains(out, "BBBBB") {
		t.Errorf("RenderRegion: missing 'BBBBB' in %q", out)
	}
	if !strings.Contains(out, "CCCCC") {
		t.Errorf("RenderRegion: missing 'CCCCC' in %q", out)
	}
	if strings.Contains(out, "AAAAA") {
		t.Errorf("RenderRegion: should not contain 'AAAAA'")
	}
}

func TestRenderRegion_OutOfBounds(t *testing.T) {
	v := NewVTermBuffer(5, 3)
	// Should not panic
	out := v.RenderRegion(-1, 100)
	if out == "" {
		t.Error("RenderRegion with out-of-bounds: should return non-empty string")
	}
}

func TestClear(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "\x1b[1;31mHello")
	write(v, "\x1b[3;4H")
	v.Clear()
	if v.CursorRow != 0 || v.CursorCol != 0 {
		t.Errorf("Clear: cursor should be at home, got (%d,%d)", v.CursorRow, v.CursorCol)
	}
	for r := range v.Cells {
		for c, cell := range v.Cells[r] {
			if cell.Char != ' ' || cell.Style != (CellStyle{}) {
				t.Errorf("Clear: cell[%d][%d] not empty: %+v", r, c, cell)
			}
		}
	}
}

// --------------------------------------------------------------------------
// Thread safety
// --------------------------------------------------------------------------

func TestThreadSafety_WritePlusRender(t *testing.T) {
	v := NewVTermBuffer(80, 24)
	var wg sync.WaitGroup
	// Concurrent writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				v.Write([]byte("Hello\r\n"))
			}
		}()
	}
	// Concurrent readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = v.Render()
			}
		}()
	}
	wg.Wait()
}

// --------------------------------------------------------------------------
// ANSI parser edge cases
// --------------------------------------------------------------------------

func TestParser_UnknownSequencesSilentlyDropped(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	// Various unknown sequences followed by normal text
	write(v, "\x1b[?1049h") // alternate screen — unknown, drop
	write(v, "\x1b]0;title\x07") // OSC title — drop
	write(v, "OK")
	if cell(v, 0, 0).Char != 'O' {
		t.Errorf("after unknown sequences: want 'O' at (0,0), got %q", cell(v, 0, 0).Char)
	}
}

func TestParser_PartialSequenceAtEndOfWrite(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	// Write ESC [ split across two Write calls
	v.Write([]byte{0x1B, '['})
	v.Write([]byte{'1', 'm', 'X'})
	if !cell(v, 0, 0).Style.Bold {
		t.Error("split CSI SGR bold: expected bold after partial write")
	}
}

func TestParser_MultipleParams(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "\x1b[1;4;31mX") // bold + underline + red fg in one SGR
	s := cell(v, 0, 0).Style
	if !s.Bold {
		t.Error("multi-param SGR: bold not set")
	}
	if !s.Underline {
		t.Error("multi-param SGR: underline not set")
	}
	if s.FG != "#cc0000" {
		t.Errorf("multi-param SGR: FG want #cc0000, got %s", s.FG)
	}
}

// --------------------------------------------------------------------------
// Color table spot checks
// --------------------------------------------------------------------------

func TestXterm256Color_CubeAndGrayscale(t *testing.T) {
	// Color 16 = first cube entry = black (0,0,0)
	if got := xterm256Color(16); got != "#000000" {
		t.Errorf("xterm256[16]: want #000000, got %s", got)
	}
	// Color 232 = first grayscale = #080808
	if got := xterm256Color(232); got != "#080808" {
		t.Errorf("xterm256[232]: want #080808, got %s", got)
	}
	// Color 255 = last grayscale = 8 + 23*10 = 238 → #eeeeee
	if got := xterm256Color(255); got != "#eeeeee" {
		t.Errorf("xterm256[255]: want #eeeeee, got %s", got)
	}
	// Out of range
	if got := xterm256Color(-1); got != "" {
		t.Errorf("xterm256[-1]: want empty, got %s", got)
	}
	if got := xterm256Color(256); got != "" {
		t.Errorf("xterm256[256]: want empty, got %s", got)
	}
}
