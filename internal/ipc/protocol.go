package ipc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SocketPath returns the Unix domain socket path for IPC.
func SocketPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), fmt.Sprintf("nexus-%d", os.Getuid()))
	}
	return filepath.Join(dir, "nexus.sock")
}

// Message types for IPC communication.
const (
	// TUI → GUI commands
	MsgOpenTab  = "open-tab"
	MsgCloseTab = "close-tab"
	MsgFocusTab = "focus-tab"
	MsgStatus   = "status"

	// GUI → TUI events
	MsgTabOpened = "tab-opened"
	MsgTabClosed = "tab-closed"
	MsgTabError  = "tab-error"
	MsgGUIReady  = "gui-ready"
)

// Envelope wraps all IPC messages with a type discriminator.
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// OpenTabCmd tells the GUI to open a new tab for a connection.
type OpenTabCmd struct {
	ConnID   string          `json:"conn_id"`
	Protocol string          `json:"protocol"` // "rdp" or "vnc"
	Host     string          `json:"host"`
	Port     int             `json:"port"`
	Username string          `json:"username,omitempty"`
	Password string          `json:"password,omitempty"`
	Domain   string          `json:"domain,omitempty"`   // RDP only
	Options  json.RawMessage `json:"options,omitempty"`   // Protocol-specific
}

// CloseTabCmd tells the GUI to close a specific tab.
type CloseTabCmd struct {
	ConnID string `json:"conn_id"`
}

// FocusTabCmd tells the GUI to bring a tab to the foreground.
type FocusTabCmd struct {
	ConnID string `json:"conn_id"`
}

// TabOpenedEvent is sent by the GUI when a tab is successfully opened.
type TabOpenedEvent struct {
	ConnID string `json:"conn_id"`
}

// TabClosedEvent is sent by the GUI when a tab is closed.
type TabClosedEvent struct {
	ConnID string `json:"conn_id"`
}

// TabErrorEvent is sent by the GUI when a connection error occurs.
type TabErrorEvent struct {
	ConnID string `json:"conn_id"`
	Error  string `json:"error"`
}

// GUIReadyEvent is sent by the GUI when it's ready to accept commands.
type GUIReadyEvent struct{}

// Encode wraps a payload in an Envelope and marshals to JSON.
func Encode(msgType string, payload interface{}) ([]byte, error) {
	p, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("ipc encode payload: %w", err)
	}
	env := Envelope{Type: msgType, Payload: p}
	data, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("ipc encode envelope: %w", err)
	}
	// Newline-delimited for framing
	return append(data, '\n'), nil
}

// Decode unmarshals an Envelope from JSON.
func Decode(data []byte) (*Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("ipc decode: %w", err)
	}
	return &env, nil
}

// DecodePayload unmarshals the payload of an Envelope into the target.
func DecodePayload(env *Envelope, target interface{}) error {
	return json.Unmarshal(env.Payload, target)
}
