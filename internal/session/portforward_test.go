package session

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/dr4zz/nexus/internal/config"
)

func TestParsePortForwards(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []config.PortForward
		wantErr bool
	}{
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "single local forward with 4 parts",
			input: "L:127.0.0.1:8080:remote:80",
			want: []config.PortForward{
				{Type: config.PortForwardLocal, LocalAddr: "127.0.0.1:8080", RemoteAddr: "remote:80"},
			},
		},
		{
			name:  "single local forward with 3 parts (port:host:port)",
			input: "L:8080:remote:80",
			want: []config.PortForward{
				{Type: config.PortForwardLocal, LocalAddr: "0.0.0.0:8080", RemoteAddr: "remote:80"},
			},
		},
		{
			name:  "single remote forward",
			input: "R:9090:localhost:9090",
			want: []config.PortForward{
				{Type: config.PortForwardRemote, LocalAddr: "0.0.0.0:9090", RemoteAddr: "localhost:9090"},
			},
		},
		{
			name:  "single dynamic forward",
			input: "D:1080",
			want: []config.PortForward{
				{Type: config.PortForwardDynamic, LocalAddr: "1080"},
			},
		},
		{
			name:  "multiple forwards",
			input: "L:8080:remote:80,R:9090:localhost:9090,D:1080",
			want: []config.PortForward{
				{Type: config.PortForwardLocal, LocalAddr: "0.0.0.0:8080", RemoteAddr: "remote:80"},
				{Type: config.PortForwardRemote, LocalAddr: "0.0.0.0:9090", RemoteAddr: "localhost:9090"},
				{Type: config.PortForwardDynamic, LocalAddr: "1080"},
			},
		},
		{
			name:  "spaces around commas",
			input: "L:8080:host:80 , D:1080",
			want: []config.PortForward{
				{Type: config.PortForwardLocal, LocalAddr: "0.0.0.0:8080", RemoteAddr: "host:80"},
				{Type: config.PortForwardDynamic, LocalAddr: "1080"},
			},
		},
		{
			name:    "invalid prefix",
			input:   "X:1234:host:80",
			wantErr: true,
		},
		{
			name:    "missing colon after prefix",
			input:   "L1234",
			wantErr: true,
		},
		{
			name:    "dynamic forward missing address",
			input:   "D:",
			wantErr: true,
		},
		{
			name:    "local forward bad format",
			input:   "L:8080",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.ParsePortForwards(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d forwards, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i].Type != tt.want[i].Type {
					t.Errorf("[%d] type = %q, want %q", i, got[i].Type, tt.want[i].Type)
				}
				if got[i].LocalAddr != tt.want[i].LocalAddr {
					t.Errorf("[%d] localAddr = %q, want %q", i, got[i].LocalAddr, tt.want[i].LocalAddr)
				}
				if got[i].RemoteAddr != tt.want[i].RemoteAddr {
					t.Errorf("[%d] remoteAddr = %q, want %q", i, got[i].RemoteAddr, tt.want[i].RemoteAddr)
				}
			}
		})
	}
}

func TestFormatPortForwards(t *testing.T) {
	forwards := []config.PortForward{
		{Type: config.PortForwardLocal, LocalAddr: "0.0.0.0:8080", RemoteAddr: "remote:80"},
		{Type: config.PortForwardRemote, LocalAddr: "0.0.0.0:9090", RemoteAddr: "localhost:9090"},
		{Type: config.PortForwardDynamic, LocalAddr: "1080"},
	}

	got := config.FormatPortForwards(forwards)
	want := "L:0.0.0.0:8080:remote:80,R:0.0.0.0:9090:localhost:9090,D:1080"
	if got != want {
		t.Fatalf("FormatPortForwards = %q, want %q", got, want)
	}

	// Empty
	got = config.FormatPortForwards(nil)
	if got != "" {
		t.Fatalf("FormatPortForwards(nil) = %q, want empty", got)
	}
}

func TestPortForwardManagerListAndStop(t *testing.T) {
	m := NewPortForwardManager()

	// Start two local listeners (on any available port)
	ln1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	af1 := &ActiveForward{
		Type:       "local",
		LocalAddr:  ln1.Addr().String(),
		RemoteAddr: "remote:80",
		Status:     ForwardStatusActive,
		listener:   ln1,
		stopCh:     make(chan struct{}),
	}
	af2 := &ActiveForward{
		Type:     "dynamic",
		LocalAddr: ln2.Addr().String(),
		Status:   ForwardStatusActive,
		listener: ln2,
		stopCh:   make(chan struct{}),
	}

	m.mu.Lock()
	m.forwards = append(m.forwards, af1, af2)
	m.mu.Unlock()

	// List should return 2 items
	list := m.List()
	if len(list) != 2 {
		t.Fatalf("List() returned %d, want 2", len(list))
	}
	if list[0].Type != "local" || list[0].Status != ForwardStatusActive {
		t.Errorf("list[0] = %+v", list[0])
	}
	if list[1].Type != "dynamic" || list[1].Status != ForwardStatusActive {
		t.Errorf("list[1] = %+v", list[1])
	}

	// Stop index 0
	m.Stop(0)
	list = m.List()
	if list[0].Status != ForwardStatusStopped {
		t.Errorf("after Stop(0), status = %s, want stopped", list[0].Status)
	}
	if list[1].Status != ForwardStatusActive {
		t.Errorf("list[1] should still be active, got %s", list[1].Status)
	}

	// Stop out of range — should not panic
	m.Stop(-1)
	m.Stop(99)

	// StopAll
	m.StopAll()
	list = m.List()
	for i, f := range list {
		if f.Status != ForwardStatusStopped {
			t.Errorf("after StopAll, list[%d].Status = %s", i, f.Status)
		}
	}
}

// socks5ClientHelper uses TCP listeners instead of net.Pipe to avoid
// synchronous pipe deadlocks. It runs the SOCKS5 handshake on the
// server side of a TCP connection.
func socks5ClientHelper(t *testing.T, clientFn func(conn net.Conn)) (string, error) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// Client connects and runs protocol
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		conn, err := net.DialTimeout("tcp", ln.Addr().String(), 2*time.Second)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		clientFn(conn)
	}()

	serverConn, err := ln.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer serverConn.Close()
	serverConn.SetDeadline(time.Now().Add(5 * time.Second))

	dest, handshakeErr := socks5Handshake(serverConn)
	<-doneCh
	return dest, handshakeErr
}

func TestSOCKS5HandshakeIPv4(t *testing.T) {
	dest, err := socks5ClientHelper(t, func(conn net.Conn) {
		// Client greeting: version 5, 1 method (no auth)
		conn.Write([]byte{0x05, 0x01, 0x00})

		// Read server method reply
		reply := make([]byte, 2)
		io.ReadFull(conn, reply)

		// Connect request: version 5, cmd CONNECT, rsv 0, ATYP IPv4
		ip := net.ParseIP("93.184.216.34").To4()
		req := []byte{0x05, 0x01, 0x00, 0x01}
		req = append(req, ip...)
		portBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(portBytes, 80)
		req = append(req, portBytes...)
		conn.Write(req)
	})

	if err != nil {
		t.Fatalf("socks5Handshake error: %v", err)
	}
	if dest != "93.184.216.34:80" {
		t.Fatalf("dest = %q, want 93.184.216.34:80", dest)
	}
}

func TestSOCKS5HandshakeDomain(t *testing.T) {
	dest, err := socks5ClientHelper(t, func(conn net.Conn) {
		// Client greeting
		conn.Write([]byte{0x05, 0x01, 0x00})

		// Read method reply
		reply := make([]byte, 2)
		io.ReadFull(conn, reply)

		// Connect request with domain name
		domain := "example.com"
		req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(domain))}
		req = append(req, []byte(domain)...)
		portBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(portBytes, 443)
		req = append(req, portBytes...)
		conn.Write(req)
	})

	if err != nil {
		t.Fatalf("socks5Handshake error: %v", err)
	}
	if dest != "example.com:443" {
		t.Fatalf("dest = %q, want example.com:443", dest)
	}
}

func TestSOCKS5HandshakeIPv6(t *testing.T) {
	dest, err := socks5ClientHelper(t, func(conn net.Conn) {
		// Client greeting
		conn.Write([]byte{0x05, 0x01, 0x00})

		// Read method reply
		reply := make([]byte, 2)
		io.ReadFull(conn, reply)

		// Connect request with IPv6 address (::1)
		ip := net.ParseIP("::1").To16()
		req := []byte{0x05, 0x01, 0x00, 0x04}
		req = append(req, ip...)
		portBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(portBytes, 8080)
		req = append(req, portBytes...)
		conn.Write(req)
	})

	if err != nil {
		t.Fatalf("socks5Handshake error: %v", err)
	}
	if dest != "::1:8080" {
		t.Fatalf("dest = %q, want ::1:8080", dest)
	}
}

func TestSOCKS5HandshakeNoAcceptableAuth(t *testing.T) {
	_, err := socks5ClientHelper(t, func(conn net.Conn) {
		// Client greeting: version 5, 1 method but only username/password (0x02)
		conn.Write([]byte{0x05, 0x01, 0x02})

		// Read server reject reply
		reply := make([]byte, 2)
		io.ReadFull(conn, reply)
	})

	if err == nil {
		t.Fatal("expected error for no acceptable auth method")
	}
}

func TestSOCKS5HandshakeUnsupportedCommand(t *testing.T) {
	_, err := socks5ClientHelper(t, func(conn net.Conn) {
		// Client greeting
		conn.Write([]byte{0x05, 0x01, 0x00})

		// Read method reply
		reply := make([]byte, 2)
		io.ReadFull(conn, reply)

		// BIND command (0x02) instead of CONNECT (0x01), with IPv4 addr
		req := []byte{0x05, 0x02, 0x00, 0x01, 0, 0, 0, 0, 0, 0}
		conn.Write(req)

		// Read error reply
		errReply := make([]byte, 10)
		io.ReadFull(conn, errReply)
	})

	if err == nil {
		t.Fatal("expected error for unsupported command")
	}
}

func TestSOCKS5HandshakeBadVersion(t *testing.T) {
	_, err := socks5ClientHelper(t, func(conn net.Conn) {
		// SOCKS4 greeting instead of SOCKS5
		conn.Write([]byte{0x04, 0x01, 0x00})
	})

	if err == nil {
		t.Fatal("expected error for bad SOCKS version")
	}
}

func TestPortForwardString(t *testing.T) {
	tests := []struct {
		pf   config.PortForward
		want string
	}{
		{config.PortForward{Type: config.PortForwardLocal, LocalAddr: "0.0.0.0:8080", RemoteAddr: "host:80"}, "L:0.0.0.0:8080:host:80"},
		{config.PortForward{Type: config.PortForwardRemote, LocalAddr: "0.0.0.0:9090", RemoteAddr: "localhost:9090"}, "R:0.0.0.0:9090:localhost:9090"},
		{config.PortForward{Type: config.PortForwardDynamic, LocalAddr: "1080"}, "D:1080"},
	}

	for _, tt := range tests {
		got := tt.pf.String()
		if got != tt.want {
			t.Errorf("PortForward.String() = %q, want %q", got, tt.want)
		}
	}
}
