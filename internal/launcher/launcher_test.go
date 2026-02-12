package launcher

import (
	"strings"
	"testing"

	"github.com/dr4zz/nexus/internal/config"
)

func TestSSHCommand(t *testing.T) {
	l := &SSHLauncher{}
	conn := config.Connection{
		ID: "test", Host: "10.0.0.1", Port: 22, Username: "admin",
		IdentityFile: "~/.ssh/id_ed25519", Protocol: config.ProtoSSH,
	}
	cmd := l.Command(conn)
	if !strings.HasPrefix(cmd, "ssh (native)") {
		t.Fatalf("expected 'ssh (native)' prefix, got: %s", cmd)
	}
	if !strings.Contains(cmd, "-i ~/.ssh/id_ed25519") {
		t.Fatalf("expected identity file, got: %s", cmd)
	}
	if !strings.Contains(cmd, "admin@10.0.0.1") {
		t.Fatalf("expected user@host, got: %s", cmd)
	}
	// Default port 22 should NOT produce -p flag
	if strings.Contains(cmd, "-p") {
		t.Fatalf("should not include -p for default port, got: %s", cmd)
	}
}

func TestSSHCommandWithPassword(t *testing.T) {
	l := &SSHLauncher{}
	conn := config.Connection{
		ID: "test", Host: "10.0.0.1", Port: 22, Username: "admin",
		Password: "secret123", Protocol: config.ProtoSSH,
	}
	cmd := l.Command(conn)
	if !strings.Contains(cmd, "password auth") {
		t.Fatalf("expected password auth indicator, got: %s", cmd)
	}
	// Password should NOT appear in command display
	if strings.Contains(cmd, "secret123") {
		t.Fatalf("password should not appear in command, got: %s", cmd)
	}
	if !strings.Contains(cmd, "admin@10.0.0.1") {
		t.Fatalf("expected user@host, got: %s", cmd)
	}
}

func TestSSHCommandNonDefaultPort(t *testing.T) {
	l := &SSHLauncher{}
	conn := config.Connection{
		ID: "test", Host: "10.0.0.1", Port: 2222, Username: "root",
		Protocol: config.ProtoSSH,
	}
	cmd := l.Command(conn)
	if !strings.Contains(cmd, "-p 2222") {
		t.Fatalf("expected -p 2222, got: %s", cmd)
	}
}

func TestRDPCommand(t *testing.T) {
	l := &RDPLauncher{}
	conn := config.Connection{
		ID: "test", Host: "192.168.1.50", Port: 3389,
		Username: "user", Password: "secret", Domain: "CORP",
		Protocol: config.ProtoRDP,
		RDPOptions: config.RDPOptions{
			Resolution:        "1920x1080",
			Fullscreen:        true,
			DynamicResolution: true,
		},
	}
	cmd := l.Command(conn)
	if !strings.Contains(cmd, "rdp (native)") {
		t.Fatalf("expected 'rdp (native)' in command, got: %s", cmd)
	}
	if !strings.Contains(cmd, "user@192.168.1.50:3389") {
		t.Fatalf("expected user@host:port, got: %s", cmd)
	}
	// Password should NOT appear in command display
	if strings.Contains(cmd, "secret") {
		t.Fatalf("password should not appear in command, got: %s", cmd)
	}
}

func TestVNCCommand(t *testing.T) {
	l := &VNCLauncher{}
	conn := config.Connection{
		ID: "test", Host: "192.168.1.100", Port: 5900,
		Protocol: config.ProtoVNC,
	}
	cmd := l.Command(conn)
	if !strings.Contains(cmd, "vnc (native)") {
		t.Fatalf("expected 'vnc (native)' in command, got: %s", cmd)
	}
	if !strings.Contains(cmd, "192.168.1.100:5900") {
		t.Fatalf("expected host:port, got: %s", cmd)
	}
}

func TestTelnetCommand(t *testing.T) {
	l := &TelnetLauncher{}
	conn := config.Connection{
		ID: "test", Host: "192.168.1.1", Port: 23,
		Protocol: config.ProtoTelnet,
	}
	cmd := l.Command(conn)
	if !strings.Contains(cmd, "telnet (native)") {
		t.Fatalf("expected 'telnet (native)' in command, got: %s", cmd)
	}
	if !strings.Contains(cmd, "192.168.1.1 23") {
		t.Fatalf("expected host port, got: %s", cmd)
	}
}

func TestForProtocol(t *testing.T) {
	for _, proto := range []config.Protocol{config.ProtoSSH, config.ProtoRDP, config.ProtoVNC, config.ProtoTelnet} {
		l, err := ForProtocol(proto)
		if err != nil {
			t.Fatalf("ForProtocol(%s) returned error: %v", proto, err)
		}
		if l == nil {
			t.Fatalf("ForProtocol(%s) returned nil", proto)
		}
	}

	_, err := ForProtocol("invalid")
	if err == nil {
		t.Fatal("ForProtocol(invalid) should return error")
	}
}
