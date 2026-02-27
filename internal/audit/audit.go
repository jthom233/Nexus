package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// EventType represents the kind of audit event.
type EventType string

const (
	EventConnect       EventType = "connect"
	EventDisconnect    EventType = "disconnect"
	EventError         EventType = "error"
	EventCredentialUse EventType = "credential_use"
	EventConfigChange  EventType = "config_change"
)

// AuditEvent represents a single auditable action.
type AuditEvent struct {
	Timestamp      time.Time `json:"timestamp"`
	ConnectionID   string    `json:"connection_id"`
	ConnectionName string    `json:"connection_name"`
	EventType      EventType `json:"event_type"`
	Duration       string    `json:"duration,omitempty"`
	ExitCode       int       `json:"exit_code,omitempty"`
	Details        string    `json:"details,omitempty"`
}

// AuditFilter defines criteria for querying audit events.
type AuditFilter struct {
	StartDate    *time.Time
	EndDate      *time.Time
	EventType    EventType
	ConnectionID string
	Limit        int
}

// AuditLog manages a JSON lines file-based audit log.
type AuditLog struct {
	mu   sync.Mutex
	file *os.File
	path string
}

// DefaultPath returns the default audit log file path.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".local", "share", "nexus", "audit.log")
}

// Open creates or opens the audit log file at the given path.
func Open(path string) (*AuditLog, error) {
	if path == "" {
		path = DefaultPath()
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("audit: create directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("audit: open file: %w", err)
	}
	return &AuditLog{file: f, path: path}, nil
}

// Log appends a single audit event as a JSON line.
func (a *AuditLog) Log(event AuditEvent) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("audit: marshal event: %w", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := a.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("audit: write event: %w", err)
	}
	return nil
}

// Query reads the audit log and returns events matching the filter.
// Events are returned in chronological order (oldest first).
// If filter.Limit > 0, at most that many events are returned (the most recent).
func (a *AuditLog) Query(filter AuditFilter) ([]AuditEvent, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	f, err := os.Open(a.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("audit: open for read: %w", err)
	}
	defer f.Close()

	var events []AuditEvent
	scanner := bufio.NewScanner(f)
	// Increase buffer size for potentially long lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev AuditEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue // skip malformed lines
		}
		if matchesFilter(ev, filter) {
			events = append(events, ev)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("audit: scan file: %w", err)
	}

	// Apply limit: return the most recent N events.
	if filter.Limit > 0 && len(events) > filter.Limit {
		events = events[len(events)-filter.Limit:]
	}

	return events, nil
}

// Close closes the audit log file.
func (a *AuditLog) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.file != nil {
		return a.file.Close()
	}
	return nil
}

// matchesFilter checks if an event matches the given filter criteria.
func matchesFilter(ev AuditEvent, f AuditFilter) bool {
	if f.StartDate != nil && ev.Timestamp.Before(*f.StartDate) {
		return false
	}
	if f.EndDate != nil && ev.Timestamp.After(*f.EndDate) {
		return false
	}
	if f.EventType != "" && ev.EventType != f.EventType {
		return false
	}
	if f.ConnectionID != "" && ev.ConnectionID != f.ConnectionID {
		return false
	}
	return true
}
