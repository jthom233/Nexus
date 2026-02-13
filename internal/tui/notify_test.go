package tui

import (
	"strings"
	"testing"

	"github.com/dr4zz/nexus/internal/health"
)

func TestDetectStateChanges_NoChanges(t *testing.T) {
	nm := NewNotifyManager()
	old := map[string]health.Status{
		"a": health.Online,
		"b": health.Offline,
	}
	new := map[string]health.Status{
		"a": health.Online,
		"b": health.Offline,
	}

	changes := nm.DetectStateChanges(old, new)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(changes))
	}
}

func TestDetectStateChanges_OnlineToOffline(t *testing.T) {
	nm := NewNotifyManager()
	old := map[string]health.Status{
		"server1": health.Online,
	}
	new := map[string]health.Status{
		"server1": health.Offline,
	}

	changes := nm.DetectStateChanges(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].ConnectionID != "server1" {
		t.Errorf("expected ConnectionID=server1, got %s", changes[0].ConnectionID)
	}
	if changes[0].OldStatus != health.Online {
		t.Errorf("expected OldStatus=Online, got %v", changes[0].OldStatus)
	}
	if changes[0].NewStatus != health.Offline {
		t.Errorf("expected NewStatus=Offline, got %v", changes[0].NewStatus)
	}
}

func TestDetectStateChanges_OfflineToOnline(t *testing.T) {
	nm := NewNotifyManager()
	old := map[string]health.Status{
		"db": health.Offline,
	}
	new := map[string]health.Status{
		"db": health.Online,
	}

	changes := nm.DetectStateChanges(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].NewStatus != health.Online {
		t.Errorf("expected NewStatus=Online, got %v", changes[0].NewStatus)
	}
}

func TestDetectStateChanges_NewConnection(t *testing.T) {
	nm := NewNotifyManager()
	old := map[string]health.Status{}
	new := map[string]health.Status{
		"new-server": health.Online,
	}

	changes := nm.DetectStateChanges(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change for new connection, got %d", len(changes))
	}
	if changes[0].OldStatus != health.Unknown {
		t.Errorf("expected OldStatus=Unknown for new connection, got %v", changes[0].OldStatus)
	}
	if changes[0].NewStatus != health.Online {
		t.Errorf("expected NewStatus=Online, got %v", changes[0].NewStatus)
	}
}

func TestDetectStateChanges_UnknownToUnknownSuppressed(t *testing.T) {
	nm := NewNotifyManager()
	old := map[string]health.Status{}
	new := map[string]health.Status{
		"mystery": health.Unknown,
	}

	changes := nm.DetectStateChanges(old, new)
	if len(changes) != 0 {
		t.Errorf("Unknown->Unknown should be suppressed, got %d changes", len(changes))
	}
}

func TestDetectStateChanges_MultipleChanges(t *testing.T) {
	nm := NewNotifyManager()
	old := map[string]health.Status{
		"a": health.Online,
		"b": health.Offline,
		"c": health.Online,
	}
	new := map[string]health.Status{
		"a": health.Offline,   // changed
		"b": health.Online,    // changed
		"c": health.Online,    // no change
	}

	changes := nm.DetectStateChanges(old, new)
	if len(changes) != 2 {
		t.Errorf("expected 2 changes, got %d", len(changes))
	}
}

func TestDetectStateChanges_ToDegraded(t *testing.T) {
	nm := NewNotifyManager()
	old := map[string]health.Status{
		"slow-server": health.Online,
	}
	new := map[string]health.Status{
		"slow-server": health.Degraded,
	}

	changes := nm.DetectStateChanges(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].NewStatus != health.Degraded {
		t.Errorf("expected NewStatus=Degraded, got %v", changes[0].NewStatus)
	}
}

func TestFormatNotification_Online(t *testing.T) {
	ch := StateChange{
		ConnectionID:   "id1",
		ConnectionName: "web-server",
		OldStatus:      health.Offline,
		NewStatus:      health.Online,
	}
	msg := FormatNotification(ch)
	if msg != "web-server came online" {
		t.Errorf("got %q, want %q", msg, "web-server came online")
	}
}

func TestFormatNotification_Offline(t *testing.T) {
	ch := StateChange{
		ConnectionID:   "id2",
		ConnectionName: "db-server",
		OldStatus:      health.Online,
		NewStatus:      health.Offline,
	}
	msg := FormatNotification(ch)
	if msg != "db-server went offline" {
		t.Errorf("got %q, want %q", msg, "db-server went offline")
	}
}

func TestFormatNotification_Degraded(t *testing.T) {
	ch := StateChange{
		ConnectionID:   "id3",
		ConnectionName: "api-server",
		OldStatus:      health.Online,
		NewStatus:      health.Degraded,
	}
	msg := FormatNotification(ch)
	if msg != "api-server became degraded" {
		t.Errorf("got %q, want %q", msg, "api-server became degraded")
	}
}

func TestFormatNotification_FallbackToID(t *testing.T) {
	ch := StateChange{
		ConnectionID: "some-id",
		NewStatus:    health.Online,
	}
	msg := FormatNotification(ch)
	if !strings.Contains(msg, "some-id") {
		t.Errorf("expected message to contain connection ID, got %q", msg)
	}
}

func TestFormatNotificationWithTransition(t *testing.T) {
	ch := StateChange{
		ConnectionName: "web",
		OldStatus:      health.Offline,
		NewStatus:      health.Online,
	}
	msg := FormatNotificationWithTransition(ch)
	if !strings.Contains(msg, "offline") || !strings.Contains(msg, "online") {
		t.Errorf("expected transition details, got %q", msg)
	}
	if !strings.Contains(msg, "web") {
		t.Errorf("expected connection name, got %q", msg)
	}
}

func TestRecordAndRetrieveStatuses(t *testing.T) {
	nm := NewNotifyManager()
	statuses := map[string]health.Status{
		"a": health.Online,
		"b": health.Offline,
	}
	nm.RecordStatuses(statuses)

	prev := nm.PreviousStatuses()
	if len(prev) != 2 {
		t.Fatalf("expected 2 previous statuses, got %d", len(prev))
	}
	if prev["a"] != health.Online {
		t.Errorf("expected a=Online, got %v", prev["a"])
	}
	if prev["b"] != health.Offline {
		t.Errorf("expected b=Offline, got %v", prev["b"])
	}
}

func TestRecordStatuses_IsolatedCopy(t *testing.T) {
	nm := NewNotifyManager()
	statuses := map[string]health.Status{
		"a": health.Online,
	}
	nm.RecordStatuses(statuses)

	// Modify original map — should not affect recorded state
	statuses["a"] = health.Offline
	statuses["b"] = health.Online

	prev := nm.PreviousStatuses()
	if prev["a"] != health.Online {
		t.Errorf("recorded status should not be affected by original map mutation")
	}
	if _, exists := prev["b"]; exists {
		t.Error("recorded statuses should not contain keys added after recording")
	}
}

func TestPopulateNames(t *testing.T) {
	changes := []StateChange{
		{ConnectionID: "id1"},
		{ConnectionID: "id2", ConnectionName: "already-named"},
		{ConnectionID: "id3"},
	}
	names := map[string]string{
		"id1": "server-one",
		"id3": "server-three",
	}

	PopulateNames(changes, names)

	if changes[0].ConnectionName != "server-one" {
		t.Errorf("expected server-one, got %q", changes[0].ConnectionName)
	}
	if changes[1].ConnectionName != "already-named" {
		t.Errorf("expected already-named (unchanged), got %q", changes[1].ConnectionName)
	}
	if changes[2].ConnectionName != "server-three" {
		t.Errorf("expected server-three, got %q", changes[2].ConnectionName)
	}
}

func TestSummarizeChanges_Empty(t *testing.T) {
	result := SummarizeChanges(nil)
	if result != "" {
		t.Errorf("expected empty string for no changes, got %q", result)
	}
}

func TestSummarizeChanges_Single(t *testing.T) {
	changes := []StateChange{
		{ConnectionName: "web", NewStatus: health.Online},
	}
	result := SummarizeChanges(changes)
	if result != "web came online" {
		t.Errorf("got %q, want %q", result, "web came online")
	}
}

func TestSummarizeChanges_Multiple(t *testing.T) {
	changes := []StateChange{
		{ConnectionName: "web", NewStatus: health.Online},
		{ConnectionName: "db", NewStatus: health.Offline},
	}
	result := SummarizeChanges(changes)
	if !strings.Contains(result, "2 connections changed") {
		t.Errorf("expected summary with count, got %q", result)
	}
	if !strings.Contains(result, "web came online") {
		t.Errorf("expected web notification in summary, got %q", result)
	}
	if !strings.Contains(result, "db went offline") {
		t.Errorf("expected db notification in summary, got %q", result)
	}
}

func TestNewNotifyManager_DefaultDisabled(t *testing.T) {
	nm := NewNotifyManager()
	if nm.NotifyEnabled {
		t.Error("NotifyEnabled should default to false")
	}
}
