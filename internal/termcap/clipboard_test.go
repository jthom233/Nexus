package termcap

import (
	"bytes"
	"encoding/base64"
	"io"
	"os"
	"strings"
	"testing"
)

func TestOSC52WriteAll(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantB64   string
	}{
		{
			name:    "hello",
			input:   "hello",
			wantB64: base64.StdEncoding.EncodeToString([]byte("hello")),
		},
		{
			name:    "empty string",
			input:   "",
			wantB64: base64.StdEncoding.EncodeToString([]byte("")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("os.Pipe() failed: %v", err)
			}
			old := os.Stdout
			os.Stdout = w

			c := &OSC52Clipboard{}
			writeErr := c.WriteAll(tt.input)

			w.Close()
			os.Stdout = old

			if writeErr != nil {
				t.Fatalf("WriteAll(%q) returned error: %v", tt.input, writeErr)
			}

			var buf bytes.Buffer
			io.Copy(&buf, r)
			r.Close()

			got := buf.String()
			wantSeq := "\x1b]52;c;" + tt.wantB64 + "\x1b\\"

			if got != wantSeq {
				t.Errorf("WriteAll(%q) produced %q, want %q", tt.input, got, wantSeq)
			}
		})
	}
}

func TestOSC52WriteAllLargePayload(t *testing.T) {
	// Create a payload larger than MaxOSC52Payload (74KB)
	largePayload := strings.Repeat("x", MaxOSC52Payload+1)

	c := &OSC52Clipboard{}

	// The large payload should fall back to SystemClipboard.WriteAll.
	// In a CI/test environment without a display server, this will likely
	// return an error from atotto/clipboard. We just verify it does NOT
	// write an OSC 52 sequence to stdout.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() failed: %v", err)
	}
	old := os.Stdout
	os.Stdout = w

	_ = c.WriteAll(largePayload) // error is acceptable (no clipboard provider)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()

	got := buf.String()
	if strings.Contains(got, "\x1b]52;") {
		t.Error("large payload should NOT produce OSC 52 sequence; expected system clipboard fallback")
	}
}

func TestSystemClipboardInterface(t *testing.T) {
	// Verify that SystemClipboard satisfies the Clipboard interface at compile time.
	var _ Clipboard = (*SystemClipboard)(nil)
	var _ Clipboard = (*OSC52Clipboard)(nil)
}

func TestDefaultClipboardNonNil(t *testing.T) {
	// DefaultClipboard should never return nil, even before InitClipboard is called.
	cb := DefaultClipboard()
	if cb == nil {
		t.Error("DefaultClipboard() returned nil")
	}
}
