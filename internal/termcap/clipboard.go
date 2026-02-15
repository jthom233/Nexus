package termcap

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/atotto/clipboard"
)

// MaxOSC52Payload is the maximum size in bytes before base64 encoding.
// Most terminals support up to 100KB base64-encoded (about 74KB raw).
const MaxOSC52Payload = 74 * 1024

// Clipboard provides read/write access to the system clipboard.
type Clipboard interface {
	WriteAll(text string) error
	ReadAll() (string, error)
}

// defaultClipboard is set during initialization.
var defaultClipboard Clipboard

// DefaultClipboard returns the clipboard implementation selected at startup.
func DefaultClipboard() Clipboard {
	if defaultClipboard == nil {
		return &SystemClipboard{}
	}
	return defaultClipboard
}

// InitClipboard sets up the clipboard based on detected capabilities.
func InitClipboard(caps *Capabilities) {
	if caps.OSC52 {
		defaultClipboard = &OSC52Clipboard{}
	} else {
		defaultClipboard = &SystemClipboard{}
	}
}

// OSC52Clipboard uses OSC 52 escape sequences for clipboard access.
// Works over SSH, inside tmux, and in screen without X11/Wayland.
type OSC52Clipboard struct{}

func (c *OSC52Clipboard) WriteAll(text string) error {
	if len(text) > MaxOSC52Payload {
		// Fall back to system clipboard for large payloads
		return (&SystemClipboard{}).WriteAll(text)
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	// OSC 52 ; c (clipboard) ; base64data ST
	seq := fmt.Sprintf("\x1b]52;c;%s\x1b\\", encoded)
	_, err := os.Stdout.WriteString(seq)
	return err
}

func (c *OSC52Clipboard) ReadAll() (string, error) {
	// OSC 52 read is not widely supported and can be a security concern.
	// Fall back to system clipboard for reads.
	return (&SystemClipboard{}).ReadAll()
}

// SystemClipboard wraps atotto/clipboard for native system clipboard access.
type SystemClipboard struct{}

func (c *SystemClipboard) WriteAll(text string) error {
	return clipboard.WriteAll(text)
}

func (c *SystemClipboard) ReadAll() (string, error) {
	return clipboard.ReadAll()
}
