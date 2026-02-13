package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempLogPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test_audit.log")
}

func TestLogAndQueryAll(t *testing.T) {
	path := tempLogPath(t)
	al, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer al.Close()

	now := time.Now()

	events := []AuditEvent{
		{Timestamp: now.Add(-3 * time.Minute), ConnectionID: "c1", ConnectionName: "server-a", EventType: EventConnect, Details: "SSH connection"},
		{Timestamp: now.Add(-2 * time.Minute), ConnectionID: "c2", ConnectionName: "server-b", EventType: EventConnect, Details: "RDP connection"},
		{Timestamp: now.Add(-1 * time.Minute), ConnectionID: "c1", ConnectionName: "server-a", EventType: EventDisconnect, Duration: "2m30s"},
		{Timestamp: now, ConnectionID: "c2", ConnectionName: "server-b", EventType: EventError, Details: "timeout"},
	}

	for _, ev := range events {
		if err := al.Log(ev); err != nil {
			t.Fatalf("Log: %v", err)
		}
	}

	// Query all
	result, err := al.Query(AuditFilter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result) != 4 {
		t.Fatalf("expected 4 events, got %d", len(result))
	}
}

func TestQueryByEventType(t *testing.T) {
	path := tempLogPath(t)
	al, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer al.Close()

	now := time.Now()
	al.Log(AuditEvent{Timestamp: now.Add(-2 * time.Minute), ConnectionID: "c1", EventType: EventConnect})
	al.Log(AuditEvent{Timestamp: now.Add(-1 * time.Minute), ConnectionID: "c1", EventType: EventDisconnect})
	al.Log(AuditEvent{Timestamp: now, ConnectionID: "c2", EventType: EventConnect})

	result, err := al.Query(AuditFilter{EventType: EventConnect})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 connect events, got %d", len(result))
	}
	for _, ev := range result {
		if ev.EventType != EventConnect {
			t.Errorf("expected EventConnect, got %s", ev.EventType)
		}
	}
}

func TestQueryByConnectionID(t *testing.T) {
	path := tempLogPath(t)
	al, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer al.Close()

	now := time.Now()
	al.Log(AuditEvent{Timestamp: now.Add(-2 * time.Minute), ConnectionID: "c1", EventType: EventConnect})
	al.Log(AuditEvent{Timestamp: now.Add(-1 * time.Minute), ConnectionID: "c2", EventType: EventConnect})
	al.Log(AuditEvent{Timestamp: now, ConnectionID: "c1", EventType: EventDisconnect})

	result, err := al.Query(AuditFilter{ConnectionID: "c1"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 events for c1, got %d", len(result))
	}
}

func TestQueryWithDateRange(t *testing.T) {
	path := tempLogPath(t)
	al, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer al.Close()

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	al.Log(AuditEvent{Timestamp: base.Add(-2 * time.Hour), ConnectionID: "c1", EventType: EventConnect})
	al.Log(AuditEvent{Timestamp: base, ConnectionID: "c2", EventType: EventConnect})
	al.Log(AuditEvent{Timestamp: base.Add(2 * time.Hour), ConnectionID: "c3", EventType: EventConnect})

	start := base.Add(-30 * time.Minute)
	end := base.Add(30 * time.Minute)
	result, err := al.Query(AuditFilter{StartDate: &start, EndDate: &end})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 event in range, got %d", len(result))
	}
	if result[0].ConnectionID != "c2" {
		t.Errorf("expected c2, got %s", result[0].ConnectionID)
	}
}

func TestQueryWithLimit(t *testing.T) {
	path := tempLogPath(t)
	al, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer al.Close()

	now := time.Now()
	for i := 0; i < 20; i++ {
		al.Log(AuditEvent{
			Timestamp:    now.Add(time.Duration(i) * time.Minute),
			ConnectionID: "c1",
			EventType:    EventConnect,
			Details:      "event " + time.Duration(time.Duration(i)*time.Minute).String(),
		})
	}

	result, err := al.Query(AuditFilter{Limit: 5})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result) != 5 {
		t.Fatalf("expected 5 events, got %d", len(result))
	}
	// Should be the 5 most recent (indices 15-19)
	if result[0].Timestamp.Before(now.Add(14 * time.Minute)) {
		t.Errorf("expected recent events, got timestamp %v", result[0].Timestamp)
	}
}

func TestEmptyFile(t *testing.T) {
	path := tempLogPath(t)
	al, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer al.Close()

	result, err := al.Query(AuditFilter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 events, got %d", len(result))
	}
}

func TestDefaultPath(t *testing.T) {
	path := DefaultPath()
	if path == "" {
		t.Fatal("DefaultPath returned empty string")
	}
	if !filepath.IsAbs(path) {
		// On some systems the fallback "." path may not be absolute
		home, _ := os.UserHomeDir()
		if home != "" && home != "." {
			t.Errorf("expected absolute path, got %s", path)
		}
	}
}
