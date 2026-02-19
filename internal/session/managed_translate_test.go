package session

import (
	"bytes"
	"testing"
)

func TestTranslateModifyOtherKeys_ShiftEnter(t *testing.T) {
	// \x1b[27;2;13~ = Shift+Enter → \r (CR)
	input := []byte("\x1b[27;2;13~")
	got := translateModifyOtherKeys(input)
	if !bytes.Equal(got, []byte{'\r'}) {
		t.Errorf("Shift+Enter: got %q, want %q", got, []byte{'\r'})
	}
}

func TestTranslateModifyOtherKeys_CtrlAltEnter(t *testing.T) {
	// \x1b[27;7;13~ = Ctrl+Alt+Enter → \r (CR)
	input := []byte("\x1b[27;7;13~")
	got := translateModifyOtherKeys(input)
	if !bytes.Equal(got, []byte{'\r'}) {
		t.Errorf("Ctrl+Alt+Enter: got %q, want %q", got, []byte{'\r'})
	}
}

func TestTranslateModifyOtherKeys_Mixed(t *testing.T) {
	// Sequence embedded in normal text: "hello" + Shift+Enter + "world"
	input := append([]byte("hello"), []byte("\x1b[27;2;13~world")...)
	got := translateModifyOtherKeys(input)
	want := []byte("hello\rworld")
	if !bytes.Equal(got, want) {
		t.Errorf("mixed: got %q, want %q", got, want)
	}
}

func TestTranslateModifyOtherKeys_MultipleSequences(t *testing.T) {
	// Two modified keys back to back
	input := []byte("\x1b[27;2;13~\x1b[27;7;13~")
	got := translateModifyOtherKeys(input)
	want := []byte{'\r', '\r'}
	if !bytes.Equal(got, want) {
		t.Errorf("multiple: got %q, want %q", got, want)
	}
}

func TestTranslateModifyOtherKeys_NoEscape(t *testing.T) {
	// Plain text — no translation needed.
	input := []byte("hello world\r\n")
	got := translateModifyOtherKeys(input)
	if !bytes.Equal(got, input) {
		t.Errorf("plain text: got %q, want %q", got, input)
	}
}

func TestTranslateModifyOtherKeys_ShiftTab(t *testing.T) {
	// \x1b[27;2;9~ = Shift+Tab → \t (HT, 9)
	input := []byte("\x1b[27;2;9~")
	got := translateModifyOtherKeys(input)
	if !bytes.Equal(got, []byte{'\t'}) {
		t.Errorf("Shift+Tab: got %q, want %q", got, []byte{'\t'})
	}
}

func TestTranslateModifyOtherKeys_PartialSequence(t *testing.T) {
	// Incomplete sequence — should pass through unchanged.
	input := []byte("\x1b[27;2;")
	got := translateModifyOtherKeys(input)
	if !bytes.Equal(got, input) {
		t.Errorf("partial: got %q, want %q", got, input)
	}
}

func TestTranslateModifyOtherKeys_RegularCSI(t *testing.T) {
	// Regular CSI sequence (not modifyOtherKeys) — pass through unchanged.
	input := []byte("\x1b[1;5D") // Ctrl+Left
	got := translateModifyOtherKeys(input)
	if !bytes.Equal(got, input) {
		t.Errorf("regular CSI: got %q, want %q", got, input)
	}
}
