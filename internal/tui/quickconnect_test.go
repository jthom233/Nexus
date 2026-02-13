package tui

import (
	"testing"

	"github.com/dr4zz/nexus/internal/config"
)

func TestParseTargetUserHostPort(t *testing.T) {
	conn, err := ParseTarget("admin@server.example.com:2222")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.Username != "admin" {
		t.Errorf("expected username 'admin', got %q", conn.Username)
	}
	if conn.Host != "server.example.com" {
		t.Errorf("expected host 'server.example.com', got %q", conn.Host)
	}
	if conn.Port != 2222 {
		t.Errorf("expected port 2222, got %d", conn.Port)
	}
	if conn.Protocol != config.ProtoSSH {
		t.Errorf("expected protocol SSH, got %s", conn.Protocol)
	}
	if conn.ID[:3] != "qc-" {
		t.Errorf("expected ID prefix 'qc-', got %q", conn.ID)
	}
}

func TestParseTargetHostOnly(t *testing.T) {
	conn, err := ParseTarget("myserver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.Host != "myserver" {
		t.Errorf("expected host 'myserver', got %q", conn.Host)
	}
	if conn.Port != 22 {
		t.Errorf("expected port 22 (default SSH), got %d", conn.Port)
	}
	if conn.Protocol != config.ProtoSSH {
		t.Errorf("expected protocol SSH, got %s", conn.Protocol)
	}
	if conn.Username != "" {
		t.Errorf("expected empty username, got %q", conn.Username)
	}
}

func TestParseTargetSSHScheme(t *testing.T) {
	conn, err := ParseTarget("ssh://root@prod-server")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.Protocol != config.ProtoSSH {
		t.Errorf("expected protocol SSH, got %s", conn.Protocol)
	}
	if conn.Username != "root" {
		t.Errorf("expected username 'root', got %q", conn.Username)
	}
	if conn.Host != "prod-server" {
		t.Errorf("expected host 'prod-server', got %q", conn.Host)
	}
	if conn.Port != 22 {
		t.Errorf("expected port 22, got %d", conn.Port)
	}
}

func TestParseTargetTelnetScheme(t *testing.T) {
	conn, err := ParseTarget("telnet://switch.local:23")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.Protocol != config.ProtoTelnet {
		t.Errorf("expected protocol Telnet, got %s", conn.Protocol)
	}
	if conn.Host != "switch.local" {
		t.Errorf("expected host 'switch.local', got %q", conn.Host)
	}
	if conn.Port != 23 {
		t.Errorf("expected port 23, got %d", conn.Port)
	}
}

func TestParseTargetRDPByPort(t *testing.T) {
	conn, err := ParseTarget("windows-box:3389")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.Protocol != config.ProtoRDP {
		t.Errorf("expected protocol RDP, got %s", conn.Protocol)
	}
	if conn.Host != "windows-box" {
		t.Errorf("expected host 'windows-box', got %q", conn.Host)
	}
	if conn.Port != 3389 {
		t.Errorf("expected port 3389, got %d", conn.Port)
	}
}

func TestParseTargetVNCByPort(t *testing.T) {
	conn, err := ParseTarget("kvm-host:5900")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.Protocol != config.ProtoVNC {
		t.Errorf("expected protocol VNC, got %s", conn.Protocol)
	}
	if conn.Host != "kvm-host" {
		t.Errorf("expected host 'kvm-host', got %q", conn.Host)
	}
	if conn.Port != 5900 {
		t.Errorf("expected port 5900, got %d", conn.Port)
	}
}

func TestParseTargetHostPort(t *testing.T) {
	conn, err := ParseTarget("server:8022")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.Protocol != config.ProtoSSH {
		t.Errorf("expected protocol SSH (default for unknown port), got %s", conn.Protocol)
	}
	if conn.Port != 8022 {
		t.Errorf("expected port 8022, got %d", conn.Port)
	}
}

func TestParseTargetInvalid(t *testing.T) {
	tests := []string{
		"",
		"   ",
		"ftp://server",
	}
	for _, target := range tests {
		_, err := ParseTarget(target)
		if err == nil {
			t.Errorf("expected error for target %q, got nil", target)
		}
	}
}

func TestRecentListDedupAndCap(t *testing.T) {
	qc := NewQuickConnect()

	// Add items
	for i := 0; i < 25; i++ {
		qc.AddRecent("host" + string(rune('a'+i)))
	}
	if len(qc.RecentList()) != maxRecentTargets {
		t.Errorf("expected %d recent entries, got %d", maxRecentTargets, len(qc.RecentList()))
	}

	// Test dedup: add an existing item
	qc.AddRecent("hosty") // should move to front
	if qc.RecentList()[0] != "hosty" {
		t.Errorf("expected 'hosty' at front, got %q", qc.RecentList()[0])
	}
	if len(qc.RecentList()) != maxRecentTargets {
		t.Errorf("expected %d recent entries after dedup, got %d", maxRecentTargets, len(qc.RecentList()))
	}

	// Verify no duplicates
	seen := make(map[string]bool)
	for _, r := range qc.RecentList() {
		if seen[r] {
			t.Errorf("duplicate recent entry: %q", r)
		}
		seen[r] = true
	}
}

func TestRecentListDedup(t *testing.T) {
	qc := NewQuickConnect()

	qc.AddRecent("server1")
	qc.AddRecent("server2")
	qc.AddRecent("server1") // duplicate — should move to front

	list := qc.RecentList()
	if len(list) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list))
	}
	if list[0] != "server1" {
		t.Errorf("expected 'server1' at front, got %q", list[0])
	}
	if list[1] != "server2" {
		t.Errorf("expected 'server2' second, got %q", list[1])
	}
}

func TestParseTargetSSHSchemeWithPort(t *testing.T) {
	conn, err := ParseTarget("ssh://deploy@ci.internal:2222")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.Protocol != config.ProtoSSH {
		t.Errorf("expected SSH, got %s", conn.Protocol)
	}
	if conn.Username != "deploy" {
		t.Errorf("expected username 'deploy', got %q", conn.Username)
	}
	if conn.Host != "ci.internal" {
		t.Errorf("expected host 'ci.internal', got %q", conn.Host)
	}
	if conn.Port != 2222 {
		t.Errorf("expected port 2222, got %d", conn.Port)
	}
}

func TestParseTargetRDPScheme(t *testing.T) {
	conn, err := ParseTarget("rdp://admin@desktop:3390")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.Protocol != config.ProtoRDP {
		t.Errorf("expected RDP, got %s", conn.Protocol)
	}
	if conn.Port != 3390 {
		t.Errorf("expected port 3390, got %d", conn.Port)
	}
}

func TestParseTargetVNCScheme(t *testing.T) {
	conn, err := ParseTarget("vnc://viewer@kvm:5901")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.Protocol != config.ProtoVNC {
		t.Errorf("expected VNC, got %s", conn.Protocol)
	}
	if conn.Port != 5901 {
		t.Errorf("expected port 5901, got %d", conn.Port)
	}
}

func TestActivateDeactivate(t *testing.T) {
	qc := NewQuickConnect()

	if qc.active {
		t.Error("expected inactive initially")
	}

	qc.Activate()
	if !qc.active {
		t.Error("expected active after Activate()")
	}

	qc.Deactivate()
	if qc.active {
		t.Error("expected inactive after Deactivate()")
	}
}
