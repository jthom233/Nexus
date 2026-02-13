package recording

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHeaderFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	rec, err := NewRecorder(path, 120, 40)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	defer rec.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 header line, got %d", len(lines))
	}

	var header asciicastHeader
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatalf("invalid header JSON: %v", err)
	}

	if header.Version != 2 {
		t.Fatalf("expected version 2, got %d", header.Version)
	}
	if header.Width != 120 {
		t.Fatalf("expected width 120, got %d", header.Width)
	}
	if header.Height != 40 {
		t.Fatalf("expected height 40, got %d", header.Height)
	}
	if header.Timestamp <= 0 {
		t.Fatal("expected positive timestamp")
	}
	if header.Env["TERM"] != "xterm-256color" {
		t.Fatalf("expected TERM=xterm-256color, got %q", header.Env["TERM"])
	}
}

func TestOutputEventFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	rec, err := NewRecorder(path, 80, 24)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	rec.WriteOutput([]byte("hello world"))
	rec.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header + event), got %d", len(lines))
	}

	var event [3]any
	if err := json.Unmarshal([]byte(lines[1]), &event); err != nil {
		t.Fatalf("invalid event JSON: %v", err)
	}

	// event[0] = float64 time, event[1] = "o", event[2] = "hello world"
	elapsed, ok := event[0].(float64)
	if !ok {
		t.Fatalf("expected float64 time, got %T", event[0])
	}
	if elapsed < 0 {
		t.Fatalf("expected non-negative elapsed time, got %f", elapsed)
	}

	eventType, ok := event[1].(string)
	if !ok || eventType != "o" {
		t.Fatalf("expected event type 'o', got %v", event[1])
	}

	eventData, ok := event[2].(string)
	if !ok || eventData != "hello world" {
		t.Fatalf("expected data 'hello world', got %v", event[2])
	}
}

func TestInputEventFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	rec, err := NewRecorder(path, 80, 24)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	rec.WriteInput([]byte("ls -la\r"))
	rec.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header + event), got %d", len(lines))
	}

	var event [3]any
	if err := json.Unmarshal([]byte(lines[1]), &event); err != nil {
		t.Fatalf("invalid event JSON: %v", err)
	}

	eventType, ok := event[1].(string)
	if !ok || eventType != "i" {
		t.Fatalf("expected event type 'i', got %v", event[1])
	}

	eventData, ok := event[2].(string)
	if !ok || eventData != "ls -la\r" {
		t.Fatalf("expected data 'ls -la\\r', got %v", event[2])
	}
}

func TestRelativeTimestampsMonotonic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	rec, err := NewRecorder(path, 80, 24)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	// Write multiple events with small delays to ensure monotonic timestamps
	for i := 0; i < 5; i++ {
		rec.WriteOutput([]byte("x"))
		time.Sleep(1 * time.Millisecond)
	}
	rec.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// 1 header + 5 events
	if len(lines) != 6 {
		t.Fatalf("expected 6 lines, got %d", len(lines))
	}

	var prevTime float64
	for i := 1; i < len(lines); i++ {
		var event [3]any
		if err := json.Unmarshal([]byte(lines[i]), &event); err != nil {
			t.Fatalf("invalid event JSON on line %d: %v", i, err)
		}
		elapsed, ok := event[0].(float64)
		if !ok {
			t.Fatalf("line %d: expected float64 time, got %T", i, event[0])
		}
		if elapsed < prevTime {
			t.Fatalf("line %d: timestamp %f < previous %f (not monotonic)", i, elapsed, prevTime)
		}
		prevTime = elapsed
	}
}

func TestMetadataGeneration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	rec, err := NewRecorder(path, 80, 24)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	rec.WriteOutput([]byte("test data"))
	time.Sleep(10 * time.Millisecond)

	if err := rec.WriteMetadata("conn-123", "My Server", "10.0.0.1", "admin"); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}
	rec.Close()

	metaPath := path + ".meta.json"
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}

	var meta Metadata
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatalf("invalid metadata JSON: %v", err)
	}

	if meta.ConnectionID != "conn-123" {
		t.Fatalf("expected connection_id 'conn-123', got %q", meta.ConnectionID)
	}
	if meta.Name != "My Server" {
		t.Fatalf("expected name 'My Server', got %q", meta.Name)
	}
	if meta.Host != "10.0.0.1" {
		t.Fatalf("expected host '10.0.0.1', got %q", meta.Host)
	}
	if meta.User != "admin" {
		t.Fatalf("expected user 'admin', got %q", meta.User)
	}
	if meta.StartTime == "" {
		t.Fatal("expected non-empty start_time")
	}
	// Parse start_time to verify format
	if _, err := time.Parse(time.RFC3339, meta.StartTime); err != nil {
		t.Fatalf("invalid start_time format: %v", err)
	}
	if meta.DurationSecs <= 0 {
		t.Fatalf("expected positive duration, got %f", meta.DurationSecs)
	}
}

func TestCloseProducesValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	rec, err := NewRecorder(path, 80, 24)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	rec.WriteOutput([]byte("line 1\r\n"))
	rec.WriteInput([]byte("cmd\r"))
	rec.WriteOutput([]byte("line 2\r\n"))
	rec.Close()

	// Verify the entire file is valid asciicast v2
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (1 header + 3 events), got %d", len(lines))
	}

	// Validate header
	var header map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatalf("invalid header: %v", err)
	}
	if header["version"].(float64) != 2 {
		t.Fatal("header version != 2")
	}

	// Validate each event line
	expectedTypes := []string{"o", "i", "o"}
	expectedData := []string{"line 1\r\n", "cmd\r", "line 2\r\n"}
	for i := 1; i < len(lines); i++ {
		var event [3]any
		if err := json.Unmarshal([]byte(lines[i]), &event); err != nil {
			t.Fatalf("invalid event on line %d: %v", i, err)
		}
		if event[1].(string) != expectedTypes[i-1] {
			t.Fatalf("line %d: expected type %q, got %q", i, expectedTypes[i-1], event[1])
		}
		if event[2].(string) != expectedData[i-1] {
			t.Fatalf("line %d: expected data %q, got %q", i, expectedData[i-1], event[2])
		}
	}
}

func TestEmptyDataNotRecorded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	rec, err := NewRecorder(path, 80, 24)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	rec.WriteOutput(nil)
	rec.WriteOutput([]byte{})
	rec.WriteInput(nil)
	rec.WriteInput([]byte{})
	rec.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line (header only, no events for empty data), got %d", len(lines))
	}
}

func TestDurationIncreases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	rec, err := NewRecorder(path, 80, 24)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	defer rec.Close()

	d1 := rec.Duration()
	time.Sleep(10 * time.Millisecond)
	d2 := rec.Duration()

	if d2 <= d1 {
		t.Fatalf("expected duration to increase: d1=%v d2=%v", d1, d2)
	}
}

func TestRecordingFilename(t *testing.T) {
	ts := time.Date(2025, 6, 15, 14, 30, 45, 0, time.UTC)
	name := RecordingFilename("my-server", ts)
	expected := "my-server_20250615_143045.cast"
	if name != expected {
		t.Fatalf("expected %q, got %q", expected, name)
	}
}

func TestDoubleCloseIsNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	rec, err := NewRecorder(path, 80, 24)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	if err := rec.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("second Close should be nil: %v", err)
	}
}

func TestWriteAfterCloseIgnored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	rec, err := NewRecorder(path, 80, 24)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	rec.Close()

	// These should not panic or error
	rec.WriteOutput([]byte("after close"))
	rec.WriteInput([]byte("after close"))
}

func TestParentDirectoryCreation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "test.cast")

	rec, err := NewRecorder(path, 80, 24)
	if err != nil {
		t.Fatalf("NewRecorder with nested dirs: %v", err)
	}
	rec.Close()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("recording file not created: %v", err)
	}
}
