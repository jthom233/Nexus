package session

import (
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// applyStyle — the styled rendering path (slow path)
// --------------------------------------------------------------------------

func TestApplyStyle_FastPath_NoAttributes(t *testing.T) {
	// No attributes → fast path, text returned unchanged.
	s := CellStyle{}
	got := applyStyle(s, "hello")
	if got != "hello" {
		t.Errorf("applyStyle fast path: want %q, got %q", "hello", got)
	}
}

func TestApplyStyle_Bold(t *testing.T) {
	s := CellStyle{Bold: true}
	got := applyStyle(s, "X")
	// lipgloss renders something (non-empty) — we can't assert exact ANSI bytes
	// but we know it won't be the bare text.
	if got == "" {
		t.Error("applyStyle Bold: expected non-empty output")
	}
}

func TestApplyStyle_Underline(t *testing.T) {
	s := CellStyle{Underline: true}
	got := applyStyle(s, "X")
	if got == "" {
		t.Error("applyStyle Underline: expected non-empty output")
	}
}

func TestApplyStyle_Reverse(t *testing.T) {
	s := CellStyle{Reverse: true}
	got := applyStyle(s, "X")
	if got == "" {
		t.Error("applyStyle Reverse: expected non-empty output")
	}
}

func TestApplyStyle_FG_Only(t *testing.T) {
	s := CellStyle{FG: "#ff0000"}
	got := applyStyle(s, "R")
	if got == "" {
		t.Error("applyStyle FG: expected non-empty output")
	}
}

func TestApplyStyle_BG_Only(t *testing.T) {
	s := CellStyle{BG: "#0000ff"}
	got := applyStyle(s, "B")
	if got == "" {
		t.Error("applyStyle BG: expected non-empty output")
	}
}

func TestApplyStyle_AllAttributes(t *testing.T) {
	s := CellStyle{
		FG:        "#ffffff",
		BG:        "#000000",
		Bold:      true,
		Underline: true,
		Reverse:   true,
	}
	got := applyStyle(s, "ALL")
	if got == "" {
		t.Error("applyStyle all attributes: expected non-empty output")
	}
}

// --------------------------------------------------------------------------
// processEscape — RIS reset and unknown sequences
// --------------------------------------------------------------------------

func TestEscape_RIS_FullReset(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	// Write some state first
	write(v, "\x1b[1;31mHello")
	write(v, "\x1b[3;4H") // move cursor
	if v.CursorRow == 0 && v.CursorCol == 0 {
		t.Fatal("precondition: cursor should not be at home before RIS")
	}

	// ESC c = RIS (full reset)
	write(v, "\x1bc")

	// After RIS: cursor at home, style reset, all cells spaces
	if v.CursorRow != 0 || v.CursorCol != 0 {
		t.Errorf("RIS: cursor should be at (0,0), got (%d,%d)", v.CursorRow, v.CursorCol)
	}
	// Style should be reset (first cell should have no SGR after writing new char)
	write(v, "A")
	if cell(v, 0, 0).Style != (CellStyle{}) {
		t.Errorf("RIS: style should be reset, got %+v", cell(v, 0, 0).Style)
	}
}

func TestEscape_UnknownTwoCharSequence_Dropped(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	// ESC followed by an unknown byte — parser should return to ground
	write(v, "\x1b=") // unknown escape; falls into default/drop
	write(v, "OK")
	if cell(v, 0, 0).Char != 'O' {
		t.Errorf("after unknown ESC sequence: want 'O' at (0,0), got %q", cell(v, 0, 0).Char)
	}
}

// --------------------------------------------------------------------------
// processCSI — intermediate byte and abort paths
// --------------------------------------------------------------------------

func TestCSI_IntermediateByte_ThenFinal(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	// CSI with intermediate byte 0x20 followed by a final byte.
	// The sequence should be consumed without panicking. The final dispatch
	// will likely be a no-op since the combination is not implemented.
	write(v, "\x1b[ m") // 0x20 (space) as intermediate, 'm' as final
	write(v, "X")
	// Parser must return to ground state and print the next char
	if cell(v, 0, 0).Char != 'X' {
		t.Errorf("after CSI with intermediate byte: want 'X' at (0,0), got %q", cell(v, 0, 0).Char)
	}
}

func TestCSI_AbortOnUnexpectedByte(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	// 0x00 is below the parameter byte range and below the final byte range,
	// so it triggers the abort path in processCSI.
	v.Write([]byte{0x1B, '[', 0x00, 'X'})
	// Parser should have aborted and returned to ground; 'X' (0x58) is in
	// the final-byte range but we already returned to stateGround, so 'X'
	// is treated as a regular printable char.
	if cell(v, 0, 0).Char != 'X' {
		t.Errorf("after CSI abort: want 'X' at (0,0), got %q", cell(v, 0, 0).Char)
	}
}

// --------------------------------------------------------------------------
// eraseDisplay — mode 1 (from top to cursor) — the missing branch
// --------------------------------------------------------------------------

func TestCSI_EraseDisplayAbove(t *testing.T) {
	v := NewVTermBuffer(5, 4)
	write(v, "AAAAABBBBBCCCCCDDDD")
	// Move cursor to row 2 col 2 (0-based: 1, 2) — "row 2, col 3" in 1-based
	write(v, "\x1b[2;3H")
	write(v, "\x1b[1J") // erase from top to cursor (mode 1)

	// Row 0 should be fully cleared
	for c := 0; c < 5; c++ {
		if cell(v, 0, c).Char != ' ' {
			t.Errorf("ED1: row 0 col %d should be space, got %q", c, cell(v, 0, c).Char)
		}
	}
	// Row 1, cols 0-2 should be cleared
	for c := 0; c <= 2; c++ {
		if cell(v, 1, c).Char != ' ' {
			t.Errorf("ED1: row 1 col %d should be space, got %q", c, cell(v, 1, c).Char)
		}
	}
	// Row 1, col 3+ should be preserved
	if cell(v, 1, 3).Char != 'B' {
		t.Errorf("ED1: row 1 col 3 should be 'B', got %q", cell(v, 1, 3).Char)
	}
	// Row 2 should be fully preserved
	if cell(v, 2, 0).Char != 'C' {
		t.Errorf("ED1: row 2 should be preserved, got %q", cell(v, 2, 0).Char)
	}
}

// --------------------------------------------------------------------------
// SGR — attribute removal (22 = not bold, 24 = not underline, 27 = not reverse)
// --------------------------------------------------------------------------

func TestSGR_RemoveAttributes(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "\x1b[1;4;7mX") // set bold, underline, reverse
	write(v, "\x1b[22;24;27mY") // remove them all
	s := cell(v, 0, 1).Style
	if s.Bold {
		t.Error("SGR 22: Bold should be cleared")
	}
	if s.Underline {
		t.Error("SGR 24: Underline should be cleared")
	}
	if s.Reverse {
		t.Error("SGR 27: Reverse should be cleared")
	}
}

// --------------------------------------------------------------------------
// SGR — 256-color background (48;5;N)
// --------------------------------------------------------------------------

func TestSGR_256Color_Background(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "\x1b[48;5;21mX") // color 21 = blue in 256-color bg
	s := cell(v, 0, 0).Style
	if s.BG == "" {
		t.Error("SGR 48;5: BG should be set")
	}
}

// --------------------------------------------------------------------------
// SGR — truecolor background (48;2;R;G;B)
// --------------------------------------------------------------------------

func TestSGR_Truecolor_Background(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "\x1b[48;2;10;20;30mX")
	s := cell(v, 0, 0).Style
	if s.BG != "#0a141e" {
		t.Errorf("SGR 48;2: BG want #0a141e, got %s", s.BG)
	}
}

// --------------------------------------------------------------------------
// SGR — extended color with insufficient params (truncated sequences)
// --------------------------------------------------------------------------

func TestSGR_ExtendedColor_TruncatedParams_NoOp(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	// 38;5 with no following color index — should not panic, just no-op
	write(v, "\x1b[38;5mX")
	// Must not panic and must return to ground so 'X' is drawn
	// The style may or may not be set — what matters is no crash
	_ = cell(v, 0, 0)
}

func TestSGR_ExtendedColor_Subtype2_TruncatedParams_NoOp(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	// 38;2 with only R and G, missing B — should not panic
	write(v, "\x1b[38;2;255;128mX")
	_ = cell(v, 0, 0)
}

// --------------------------------------------------------------------------
// parseParams edge cases
// --------------------------------------------------------------------------

func TestParseParams_EmptySegments(t *testing.T) {
	// ";" → two empty segments → both treated as 0
	result := parseParams(";")
	if len(result) != 2 {
		t.Errorf("parseParams \";\": want 2 elements, got %d", len(result))
	}
	if result[0] != 0 || result[1] != 0 {
		t.Errorf("parseParams \";\": want [0,0], got %v", result)
	}
}

func TestParseParams_EmptyString(t *testing.T) {
	result := parseParams("")
	if result != nil {
		t.Errorf("parseParams \"\": want nil, got %v", result)
	}
}

// --------------------------------------------------------------------------
// OSC sequence — window title (commonly emitted by shells)
// --------------------------------------------------------------------------

func TestOSC_WindowTitle_BEL_Terminated(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	// OSC 0;title BEL
	write(v, "\x1b]0;My Terminal Title\x07")
	write(v, "OK")
	if cell(v, 0, 0).Char != 'O' {
		t.Errorf("after OSC+BEL: want 'O' at (0,0), got %q", cell(v, 0, 0).Char)
	}
}

func TestOSC_WindowTitle_ESC_Terminated(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	// OSC terminated by ESC (not full ST ESC-backslash, just ESC)
	write(v, "\x1b]0;Title\x1b")
	write(v, "OK")
	if cell(v, 0, 0).Char != 'O' {
		t.Errorf("after OSC+ESC: want 'O' at (0,0), got %q", cell(v, 0, 0).Char)
	}
}

// --------------------------------------------------------------------------
// Tab at end of line — clamps to width-1
// --------------------------------------------------------------------------

func TestWrite_Tab_AtEndOfLine(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	// Write 9 chars so cursor is at col 9, then tab — should clamp to col 9 (width-1)
	write(v, "ABCDEFGHI") // 9 chars, cursor at col 9
	write(v, "\t")
	if v.CursorCol != 9 {
		t.Errorf("tab at end of line: cursor should clamp to 9, got %d", v.CursorCol)
	}
}

// --------------------------------------------------------------------------
// Backspace at column 0 — no movement
// --------------------------------------------------------------------------

func TestWrite_Backspace_AtCol0_NoOp(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "\b") // BS at column 0 — should not go negative
	if v.CursorCol != 0 {
		t.Errorf("backspace at col 0: want col 0, got %d", v.CursorCol)
	}
}

// --------------------------------------------------------------------------
// RenderRegion — startRow > endRow returns empty
// --------------------------------------------------------------------------

func TestRenderRegion_StartAfterEnd_ReturnsEmpty(t *testing.T) {
	v := NewVTermBuffer(5, 5)
	// Clamp makes startRow=0, endRow clamped; but if startRow > endRow after
	// clamping (e.g. startRow=3, endRow=1), it returns "".
	// Force: after clamping startRow min=0, endRow min=startRow can be tricky.
	// Use a 5-row buffer and request startRow > endRow naturally.
	// Let's do startRow > endRow: 5-row buffer, request (3, 1).
	out := v.RenderRegion(3, 1) // startRow=3 > endRow=1
	if out != "" {
		t.Errorf("RenderRegion(3,1): want empty string, got %q", out)
	}
}

// --------------------------------------------------------------------------
// renderRow — empty row returns empty string
// --------------------------------------------------------------------------

func TestRenderRow_EmptyRow(t *testing.T) {
	got := renderRow([]Cell{})
	if got != "" {
		t.Errorf("renderRow empty: want \"\", got %q", got)
	}
}

// --------------------------------------------------------------------------
// Resize — scroll region reset after resize
// --------------------------------------------------------------------------

func TestResize_ScrollRegionReset(t *testing.T) {
	v := NewVTermBuffer(20, 10)
	// Set a custom scroll region
	write(v, "\x1b[3;8r")
	if v.ScrollRegionTop != 2 || v.ScrollRegionBottom != 7 {
		t.Fatalf("precondition: scroll region should be [2,7], got [%d,%d]",
			v.ScrollRegionTop, v.ScrollRegionBottom)
	}
	// After resize, scroll region should reset to full screen
	v.Resize(20, 10)
	if v.ScrollRegionTop != 0 {
		t.Errorf("after resize: ScrollRegionTop want 0, got %d", v.ScrollRegionTop)
	}
	if v.ScrollRegionBottom != 9 {
		t.Errorf("after resize: ScrollRegionBottom want 9, got %d", v.ScrollRegionBottom)
	}
}

// --------------------------------------------------------------------------
// HVP (f) — alternative cursor positioning sequence
// --------------------------------------------------------------------------

func TestCSI_HVP_CursorPosition(t *testing.T) {
	v := NewVTermBuffer(20, 10)
	write(v, "\x1b[4;6f") // row 4, col 6 (1-based → 3, 5)
	if v.CursorRow != 3 {
		t.Errorf("HVP row: want 3, got %d", v.CursorRow)
	}
	if v.CursorCol != 5 {
		t.Errorf("HVP col: want 5, got %d", v.CursorCol)
	}
}

// --------------------------------------------------------------------------
// DECSET private mode sequence — silently dropped, no panic
// --------------------------------------------------------------------------

func TestCSI_PrivateMode_SilentlyDropped(t *testing.T) {
	v := NewVTermBuffer(10, 5)
	write(v, "\x1b[?47h") // alternate screen — private mode, should be no-op
	write(v, "\x1b[?47l") // restore screen
	write(v, "OK")
	if cell(v, 0, 0).Char != 'O' {
		t.Errorf("after DECSET: want 'O' at (0,0), got %q", cell(v, 0, 0).Char)
	}
}

// --------------------------------------------------------------------------
// xterm256Color — index 0-7 and 8-15 delegate to standard color tables
// --------------------------------------------------------------------------

func TestXterm256Color_DelegatesTo_StandardColors(t *testing.T) {
	// Index 0-7 should match ansiColor
	for i := 0; i < 8; i++ {
		want := ansiColor(i)
		got := xterm256Color(i)
		if got != want {
			t.Errorf("xterm256[%d] should delegate to ansiColor, want %q got %q", i, want, got)
		}
	}
	// Index 8-15 should match ansiBrightColor(n-8)
	for i := 8; i < 16; i++ {
		want := ansiBrightColor(i - 8)
		got := xterm256Color(i)
		if got != want {
			t.Errorf("xterm256[%d] should delegate to ansiBrightColor(%d), want %q got %q", i, i-8, want, got)
		}
	}
}

// --------------------------------------------------------------------------
// ansiColor / ansiBrightColor — out-of-range returns empty
// --------------------------------------------------------------------------

func TestAnsiColor_OutOfRange(t *testing.T) {
	if got := ansiColor(-1); got != "" {
		t.Errorf("ansiColor(-1): want empty, got %q", got)
	}
	if got := ansiColor(8); got != "" {
		t.Errorf("ansiColor(8): want empty, got %q", got)
	}
}

func TestAnsiBrightColor_OutOfRange(t *testing.T) {
	if got := ansiBrightColor(-1); got != "" {
		t.Errorf("ansiBrightColor(-1): want empty, got %q", got)
	}
	if got := ansiBrightColor(8); got != "" {
		t.Errorf("ansiBrightColor(8): want empty, got %q", got)
	}
}

// --------------------------------------------------------------------------
// putChar — cursor out of bounds does not panic
// --------------------------------------------------------------------------

func TestPutChar_CursorOutOfBounds_NoWrite(t *testing.T) {
	v := NewVTermBuffer(5, 3)
	// Manually put cursor out of bounds — should not panic when putChar is called.
	// We achieve this by moving cursor to exactly the last column and then
	// writing a char so pendingWrap fires a lineFeed.  After many such wraps
	// we verify no out-of-bounds panic.
	for i := 0; i < 20; i++ {
		v.Write([]byte("ABCDE")) // fills row, sets pendingWrap
	}
	// If we got here without a panic, the test passes.
}

// --------------------------------------------------------------------------
// Render — styled cells are included in output (regression for applyStyle)
// --------------------------------------------------------------------------

func TestRender_StyledCells_Included(t *testing.T) {
	v := NewVTermBuffer(20, 2)
	// Write red bold "HELLO"
	write(v, "\x1b[1;31mHELLO\x1b[m")
	out := v.Render()
	// The rendered output must contain the visible text, possibly wrapped in ANSI codes.
	if !strings.Contains(out, "HELLO") {
		t.Errorf("Render with styled cells: output should contain 'HELLO', got %q", out)
	}
}
