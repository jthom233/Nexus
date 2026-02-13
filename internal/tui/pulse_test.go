package tui

import (
	"testing"
	"time"

	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/health"
)

func TestPulseSummaryCounts(t *testing.T) {
	connections := []config.Connection{
		{ID: "c1", Name: "server-a", Protocol: config.ProtoSSH, Host: "a.example.com", Group: "prod"},
		{ID: "c2", Name: "server-b", Protocol: config.ProtoSSH, Host: "b.example.com", Group: "prod"},
		{ID: "c3", Name: "server-c", Protocol: config.ProtoRDP, Host: "c.example.com", Group: "dev"},
		{ID: "c4", Name: "server-d", Protocol: config.ProtoVNC, Host: "d.example.com", Group: "dev"},
		{ID: "c5", Name: "server-e", Protocol: config.ProtoSSH, Host: "e.example.com", Group: "staging"},
	}

	statuses := map[string]connStatus{
		"c1": {status: health.Online, latency: 50 * time.Millisecond},
		"c2": {status: health.Offline, latency: 3 * time.Second},
		"c3": {status: health.Degraded, latency: 2500 * time.Millisecond},
		"c4": {status: health.Online, latency: 100 * time.Millisecond},
		// c5 has no status -> Unknown
	}

	p := NewPulseModel()
	p.Refresh(connections, statuses, 3)

	if p.summary.Total != 5 {
		t.Errorf("Total: expected 5, got %d", p.summary.Total)
	}
	if p.summary.Online != 2 {
		t.Errorf("Online: expected 2, got %d", p.summary.Online)
	}
	if p.summary.Offline != 1 {
		t.Errorf("Offline: expected 1, got %d", p.summary.Offline)
	}
	if p.summary.Degraded != 1 {
		t.Errorf("Degraded: expected 1, got %d", p.summary.Degraded)
	}
	if p.summary.Unknown != 1 {
		t.Errorf("Unknown: expected 1, got %d", p.summary.Unknown)
	}
	if p.summary.ActiveSessions != 3 {
		t.Errorf("ActiveSessions: expected 3, got %d", p.summary.ActiveSessions)
	}
}

func TestPulseGroupHealth(t *testing.T) {
	connections := []config.Connection{
		{ID: "c1", Name: "a", Protocol: config.ProtoSSH, Host: "a", Group: "prod"},
		{ID: "c2", Name: "b", Protocol: config.ProtoSSH, Host: "b", Group: "prod"},
		{ID: "c3", Name: "c", Protocol: config.ProtoSSH, Host: "c", Group: "dev"},
		{ID: "c4", Name: "d", Protocol: config.ProtoSSH, Host: "d"}, // ungrouped
	}

	statuses := map[string]connStatus{
		"c1": {status: health.Online},
		"c2": {status: health.Offline},
		"c3": {status: health.Online},
		"c4": {status: health.Offline},
	}

	p := NewPulseModel()
	p.Refresh(connections, statuses, 0)

	if len(p.groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(p.groups))
	}

	// Groups are sorted alphabetically: (ungrouped), dev, prod
	groupMap := make(map[string]GroupHealth)
	for _, g := range p.groups {
		groupMap[g.Name] = g
	}

	prod := groupMap["prod"]
	if prod.Online != 1 || prod.Offline != 1 || prod.Total != 2 {
		t.Errorf("prod: expected Online=1 Offline=1 Total=2, got Online=%d Offline=%d Total=%d", prod.Online, prod.Offline, prod.Total)
	}

	dev := groupMap["dev"]
	if dev.Online != 1 || dev.Offline != 0 || dev.Total != 1 {
		t.Errorf("dev: expected Online=1 Offline=0 Total=1, got Online=%d Offline=%d Total=%d", dev.Online, dev.Offline, dev.Total)
	}

	ungrouped := groupMap["(ungrouped)"]
	if ungrouped.Online != 0 || ungrouped.Offline != 1 || ungrouped.Total != 1 {
		t.Errorf("ungrouped: expected Online=0 Offline=1 Total=1, got Online=%d Offline=%d Total=%d", ungrouped.Online, ungrouped.Offline, ungrouped.Total)
	}
}

func TestPulseProtocols(t *testing.T) {
	connections := []config.Connection{
		{ID: "c1", Protocol: config.ProtoSSH, Host: "a"},
		{ID: "c2", Protocol: config.ProtoSSH, Host: "b"},
		{ID: "c3", Protocol: config.ProtoRDP, Host: "c"},
		{ID: "c4", Protocol: config.ProtoVNC, Host: "d"},
		{ID: "c5", Protocol: config.ProtoSSH, Host: "e"},
	}

	p := NewPulseModel()
	p.Refresh(connections, nil, 0)

	if len(p.protos) != 3 {
		t.Fatalf("expected 3 protocols, got %d", len(p.protos))
	}

	// Sorted by count descending: SSH(3), RDP(1), VNC(1)
	if p.protos[0].Protocol != "SSH" || p.protos[0].Count != 3 {
		t.Errorf("expected SSH:3, got %s:%d", p.protos[0].Protocol, p.protos[0].Count)
	}
}

func TestPulseLatency(t *testing.T) {
	connections := []config.Connection{
		{ID: "c1", Name: "slow", Protocol: config.ProtoSSH, Host: "a"},
		{ID: "c2", Name: "fast", Protocol: config.ProtoSSH, Host: "b"},
		{ID: "c3", Name: "medium", Protocol: config.ProtoSSH, Host: "c"},
		{ID: "c4", Name: "offline", Protocol: config.ProtoSSH, Host: "d"},
	}

	statuses := map[string]connStatus{
		"c1": {status: health.Online, latency: 1500 * time.Millisecond},
		"c2": {status: health.Online, latency: 20 * time.Millisecond},
		"c3": {status: health.Degraded, latency: 2100 * time.Millisecond},
		"c4": {status: health.Offline, latency: 3 * time.Second}, // offline: should be excluded
	}

	p := NewPulseModel()
	p.Refresh(connections, statuses, 0)

	if len(p.latency) != 3 {
		t.Fatalf("expected 3 latency entries (excluding offline), got %d", len(p.latency))
	}

	// Sorted slowest first
	if p.latency[0].Name != "medium" {
		t.Errorf("expected slowest to be 'medium', got '%s'", p.latency[0].Name)
	}
	if p.latency[1].Name != "slow" {
		t.Errorf("expected 2nd slowest to be 'slow', got '%s'", p.latency[1].Name)
	}
}

func TestPulseViewRenders(t *testing.T) {
	p := NewPulseModel()
	p.SetSize(120, 40)
	p.Refresh([]config.Connection{
		{ID: "c1", Name: "test", Protocol: config.ProtoSSH, Host: "test.example.com", Group: "prod"},
	}, map[string]connStatus{
		"c1": {status: health.Online, latency: 50 * time.Millisecond},
	}, 1)

	view := p.View()
	if view == "" {
		t.Fatal("View() returned empty string")
	}
	// Should contain key sections
	if !containsStr(view, "Pulse Dashboard") {
		t.Error("View should contain 'Pulse Dashboard'")
	}
}

func TestFormatLatency(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Microsecond, "500µs"},
		{50 * time.Millisecond, "50ms"},
		{1500 * time.Millisecond, "1.5s"},
	}
	for _, tt := range tests {
		got := formatLatency(tt.d)
		if got != tt.want {
			t.Errorf("formatLatency(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
