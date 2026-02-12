// Package recording provides asciicast v2 format session recording.
package recording

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// asciicastHeader is the header line of an asciicast v2 file.
type asciicastHeader struct {
	Version   int               `json:"version"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	Timestamp int64             `json:"timestamp"`
	Env       map[string]string `json:"env,omitempty"`
}

// Metadata holds information about a recording, written as a sidecar .meta.json file.
type Metadata struct {
	ConnectionID string  `json:"connection_id"`
	Name         string  `json:"name"`
	Host         string  `json:"host"`
	User         string  `json:"user"`
	StartTime    string  `json:"start_time"`
	DurationSecs float64 `json:"duration_secs"`
}

// Recorder writes SSH session data in asciicast v2 format.
type Recorder struct {
	mu        sync.Mutex
	file      *os.File
	startTime time.Time
	closed    bool
}

// NewRecorder creates a new asciicast v2 recording file and writes the header.
// The path should end with ".cast". The parent directory is created if needed.
func NewRecorder(path string, width, height int) (*Recorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create recording dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create recording file: %w", err)
	}

	now := time.Now()
	header := asciicastHeader{
		Version:   2,
		Width:     width,
		Height:    height,
		Timestamp: now.Unix(),
		Env: map[string]string{
			"TERM":  "xterm-256color",
			"SHELL": "/bin/bash",
		},
	}

	headerBytes, err := json.Marshal(header)
	if err != nil {
		f.Close()
		os.Remove(path)
		return nil, fmt.Errorf("marshal header: %w", err)
	}
	headerBytes = append(headerBytes, '\n')

	if _, err := f.Write(headerBytes); err != nil {
		f.Close()
		os.Remove(path)
		return nil, fmt.Errorf("write header: %w", err)
	}

	return &Recorder{
		file:      f,
		startTime: now,
	}, nil
}

// writeEvent writes a single asciicast v2 event line: [time, type, data].
func (r *Recorder) writeEvent(eventType string, data []byte) {
	if len(data) == 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}

	elapsed := time.Since(r.startTime).Seconds()
	dataStr := string(data)

	// asciicast v2 event: [relative_time, event_type, data_string]
	event := [3]any{elapsed, eventType, dataStr}
	line, err := json.Marshal(event)
	if err != nil {
		return
	}
	line = append(line, '\n')
	r.file.Write(line)
}

// WriteOutput records an output ("o") event.
func (r *Recorder) WriteOutput(data []byte) {
	r.writeEvent("o", data)
}

// WriteInput records an input ("i") event.
func (r *Recorder) WriteInput(data []byte) {
	r.writeEvent("i", data)
}

// Close flushes and closes the recording file.
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true
	return r.file.Close()
}

// Duration returns the elapsed time since the recording started.
func (r *Recorder) Duration() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return time.Since(r.startTime)
}

// Path returns the filesystem path of the recording file.
func (r *Recorder) Path() string {
	return r.file.Name()
}

// WriteMetadata writes a .meta.json sidecar file alongside the .cast file.
func (r *Recorder) WriteMetadata(connID, name, host, user string) error {
	r.mu.Lock()
	dur := time.Since(r.startTime)
	startTime := r.startTime
	r.mu.Unlock()

	meta := Metadata{
		ConnectionID: connID,
		Name:         name,
		Host:         host,
		User:         user,
		StartTime:    startTime.Format(time.RFC3339),
		DurationSecs: dur.Seconds(),
	}

	metaPath := r.file.Name() + ".meta.json"
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(metaPath, data, 0o644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	return nil
}

// DefaultRecordingDir returns the default directory for session recordings.
func DefaultRecordingDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".local", "share", "nexus", "recordings")
}

// RecordingFilename generates a recording filename from connection ID and timestamp.
func RecordingFilename(connID string, t time.Time) string {
	return fmt.Sprintf("%s_%s.cast", connID, t.Format("20060102_150405"))
}
