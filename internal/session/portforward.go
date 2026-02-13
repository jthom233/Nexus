package session

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"golang.org/x/crypto/ssh"
)

// ForwardStatus represents the state of an active port forward.
type ForwardStatus string

const (
	ForwardStatusActive  ForwardStatus = "active"
	ForwardStatusStopped ForwardStatus = "stopped"
	ForwardStatusError   ForwardStatus = "error"
)

// ActiveForward tracks a single running port forward.
type ActiveForward struct {
	Type             string
	LocalAddr        string
	RemoteAddr       string
	BytesTransferred int64
	Status           ForwardStatus
	listener         net.Listener
	stopCh           chan struct{}
	err              error
}

// PortForwardManager manages SSH port forwards for a single client.
type PortForwardManager struct {
	mu       sync.Mutex
	forwards []*ActiveForward
}

// NewPortForwardManager creates a new manager.
func NewPortForwardManager() *PortForwardManager {
	return &PortForwardManager{}
}

// StartLocal starts a local port forward (-L).
// Listens on localAddr, and for each accepted connection dials remoteAddr
// through the SSH client, then pipes data bidirectionally.
func (m *PortForwardManager) StartLocal(client *ssh.Client, localAddr, remoteAddr string) error {
	ln, err := net.Listen("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("local forward listen %s: %w", localAddr, err)
	}

	af := &ActiveForward{
		Type:       "local",
		LocalAddr:  ln.Addr().String(),
		RemoteAddr: remoteAddr,
		Status:     ForwardStatusActive,
		listener:   ln,
		stopCh:     make(chan struct{}),
	}

	m.mu.Lock()
	m.forwards = append(m.forwards, af)
	m.mu.Unlock()

	go m.acceptLoop(af, func(localConn net.Conn) {
		remoteConn, err := client.Dial("tcp", remoteAddr)
		if err != nil {
			localConn.Close()
			return
		}
		pipeAndCount(localConn, remoteConn, &af.BytesTransferred)
	})

	return nil
}

// StartRemote starts a remote port forward (-R).
// Listens on the remote side at remoteAddr via ssh.Client.Listen(),
// and for each accepted connection dials localAddr on the local side.
func (m *PortForwardManager) StartRemote(client *ssh.Client, remoteAddr, localAddr string) error {
	ln, err := client.Listen("tcp", remoteAddr)
	if err != nil {
		return fmt.Errorf("remote forward listen %s: %w", remoteAddr, err)
	}

	af := &ActiveForward{
		Type:       "remote",
		LocalAddr:  remoteAddr,
		RemoteAddr: localAddr,
		Status:     ForwardStatusActive,
		listener:   ln,
		stopCh:     make(chan struct{}),
	}

	m.mu.Lock()
	m.forwards = append(m.forwards, af)
	m.mu.Unlock()

	go m.acceptLoop(af, func(remoteConn net.Conn) {
		localConn, err := net.Dial("tcp", localAddr)
		if err != nil {
			remoteConn.Close()
			return
		}
		pipeAndCount(remoteConn, localConn, &af.BytesTransferred)
	})

	return nil
}

// StartDynamic starts a dynamic SOCKS5 forward (-D).
// Listens on localAddr and handles SOCKS5 CONNECT requests, dialing
// the requested destination through the SSH client.
func (m *PortForwardManager) StartDynamic(client *ssh.Client, localAddr string) error {
	ln, err := net.Listen("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("dynamic forward listen %s: %w", localAddr, err)
	}

	af := &ActiveForward{
		Type:      "dynamic",
		LocalAddr: ln.Addr().String(),
		Status:    ForwardStatusActive,
		listener:  ln,
		stopCh:    make(chan struct{}),
	}

	m.mu.Lock()
	m.forwards = append(m.forwards, af)
	m.mu.Unlock()

	go m.acceptLoop(af, func(conn net.Conn) {
		handleSOCKS5(conn, client, &af.BytesTransferred)
	})

	return nil
}

// Stop stops the forward at the given index.
func (m *PortForwardManager) Stop(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index < 0 || index >= len(m.forwards) {
		return
	}
	af := m.forwards[index]
	if af.Status == ForwardStatusActive {
		close(af.stopCh)
		af.listener.Close()
		af.Status = ForwardStatusStopped
	}
}

// StopAll stops all active forwards.
func (m *PortForwardManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, af := range m.forwards {
		if af.Status == ForwardStatusActive {
			close(af.stopCh)
			af.listener.Close()
			af.Status = ForwardStatusStopped
		}
	}
}

// List returns a snapshot of all forwards.
func (m *PortForwardManager) List() []ActiveForward {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]ActiveForward, len(m.forwards))
	for i, af := range m.forwards {
		result[i] = ActiveForward{
			Type:             af.Type,
			LocalAddr:        af.LocalAddr,
			RemoteAddr:       af.RemoteAddr,
			BytesTransferred: atomic.LoadInt64(&af.BytesTransferred),
			Status:           af.Status,
		}
	}
	return result
}

// acceptLoop runs the accept loop for a forward, calling handler for each connection.
func (m *PortForwardManager) acceptLoop(af *ActiveForward, handler func(net.Conn)) {
	for {
		conn, err := af.listener.Accept()
		if err != nil {
			select {
			case <-af.stopCh:
				// Normal shutdown
				return
			default:
				m.mu.Lock()
				af.Status = ForwardStatusError
				af.err = err
				m.mu.Unlock()
				return
			}
		}
		go handler(conn)
	}
}

// pipeAndCount bidirectionally copies data between two connections,
// counting bytes transferred into the given counter.
func pipeAndCount(a, b net.Conn, counter *int64) {
	var wg sync.WaitGroup
	wg.Add(2)

	copyFn := func(dst, src net.Conn) {
		defer wg.Done()
		n, _ := io.Copy(dst, src)
		atomic.AddInt64(counter, n)
		// Signal the other side that we're done sending
		if tc, ok := dst.(*net.TCPConn); ok {
			tc.CloseWrite()
		} else {
			dst.Close()
		}
	}

	go copyFn(a, b)
	go copyFn(b, a)
	wg.Wait()
	a.Close()
	b.Close()
}

// SOCKS5 protocol constants.
const (
	socks5Version      = 0x05
	socks5AuthNone     = 0x00
	socks5AuthReject   = 0xFF
	socks5CmdConnect   = 0x01
	socks5AddrIPv4     = 0x01
	socks5AddrDomain   = 0x03
	socks5AddrIPv6     = 0x04
	socks5ReplySuccess = 0x00
	socks5ReplyFail    = 0x01
)

// handleSOCKS5 performs the SOCKS5 handshake on conn and pipes data through the SSH client.
func handleSOCKS5(conn net.Conn, client *ssh.Client, counter *int64) {
	defer conn.Close()

	destAddr, err := socks5Handshake(conn)
	if err != nil {
		return
	}

	// Dial through SSH
	remote, err := client.Dial("tcp", destAddr)
	if err != nil {
		// Send failure reply
		conn.Write([]byte{socks5Version, socks5ReplyFail, 0x00, socks5AddrIPv4, 0, 0, 0, 0, 0, 0})
		return
	}

	// Send success reply
	conn.Write([]byte{socks5Version, socks5ReplySuccess, 0x00, socks5AddrIPv4, 0, 0, 0, 0, 0, 0})

	// Pipe data
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		n, _ := io.Copy(remote, conn)
		atomic.AddInt64(counter, n)
	}()
	go func() {
		defer wg.Done()
		n, _ := io.Copy(conn, remote)
		atomic.AddInt64(counter, n)
	}()
	wg.Wait()
	remote.Close()
}

// socks5Handshake performs the SOCKS5 greeting and connect request,
// returning the requested destination address as "host:port".
func socks5Handshake(conn net.Conn) (string, error) {
	// --- Greeting ---
	// Client sends: VER, NMETHODS, METHODS...
	buf := make([]byte, 258)
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return "", fmt.Errorf("read greeting: %w", err)
	}
	ver := buf[0]
	nMethods := int(buf[1])
	if ver != socks5Version {
		return "", fmt.Errorf("unsupported SOCKS version: %d", ver)
	}
	if _, err := io.ReadFull(conn, buf[:nMethods]); err != nil {
		return "", fmt.Errorf("read methods: %w", err)
	}

	// Check for no-auth method
	hasNoAuth := false
	for i := 0; i < nMethods; i++ {
		if buf[i] == socks5AuthNone {
			hasNoAuth = true
			break
		}
	}
	if !hasNoAuth {
		conn.Write([]byte{socks5Version, socks5AuthReject})
		return "", fmt.Errorf("no acceptable auth method")
	}

	// Reply: use no auth
	if _, err := conn.Write([]byte{socks5Version, socks5AuthNone}); err != nil {
		return "", fmt.Errorf("write auth reply: %w", err)
	}

	// --- Connect request ---
	// VER, CMD, RSV, ATYP, DST.ADDR, DST.PORT
	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return "", fmt.Errorf("read request: %w", err)
	}
	if buf[0] != socks5Version {
		return "", fmt.Errorf("bad version in request: %d", buf[0])
	}
	if buf[1] != socks5CmdConnect {
		conn.Write([]byte{socks5Version, 0x07, 0x00, socks5AddrIPv4, 0, 0, 0, 0, 0, 0})
		return "", fmt.Errorf("unsupported command: %d", buf[1])
	}

	addrType := buf[3]
	var host string
	switch addrType {
	case socks5AddrIPv4:
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return "", fmt.Errorf("read ipv4 addr: %w", err)
		}
		host = net.IP(buf[:4]).String()

	case socks5AddrDomain:
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return "", fmt.Errorf("read domain len: %w", err)
		}
		domainLen := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:domainLen]); err != nil {
			return "", fmt.Errorf("read domain: %w", err)
		}
		host = string(buf[:domainLen])

	case socks5AddrIPv6:
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return "", fmt.Errorf("read ipv6 addr: %w", err)
		}
		host = net.IP(buf[:16]).String()

	default:
		return "", fmt.Errorf("unsupported address type: %d", addrType)
	}

	// Read port (2 bytes, big-endian)
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return "", fmt.Errorf("read port: %w", err)
	}
	port := binary.BigEndian.Uint16(buf[:2])

	return fmt.Sprintf("%s:%d", host, port), nil
}
