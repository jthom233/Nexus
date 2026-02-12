package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/hooks"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// ErrDetached is returned from Run() when the user presses Ctrl+\ to detach.
var ErrDetached = errors.New("session detached")

// SessionStatus represents the lifecycle state of a managed session.
type SessionStatus int

const (
	StatusConnecting SessionStatus = iota
	StatusConnected
	StatusDetached
	StatusClosed
)

func (s SessionStatus) String() string {
	switch s {
	case StatusConnecting:
		return "connecting"
	case StatusConnected:
		return "connected"
	case StatusDetached:
		return "detached"
	case StatusClosed:
		return "closed"
	}
	return "unknown"
}

// ringBuffer is a thread-safe circular buffer of byte slices.
type ringBuffer struct {
	mu    sync.Mutex
	buf   [][]byte
	cap   int
	start int
	count int
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{
		buf: make([][]byte, capacity),
		cap: capacity,
	}
}

// Write appends a copy of data to the ring buffer, overwriting oldest if full.
func (r *ringBuffer) Write(data []byte) {
	if len(data) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	cp := make([]byte, len(data))
	copy(cp, data)

	idx := (r.start + r.count) % r.cap
	r.buf[idx] = cp
	if r.count == r.cap {
		// overwrite oldest
		r.start = (r.start + 1) % r.cap
	} else {
		r.count++
	}
}

// Drain returns all buffered data and clears the buffer.
func (r *ringBuffer) Drain() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count == 0 {
		return nil
	}

	var total int
	for i := 0; i < r.count; i++ {
		idx := (r.start + i) % r.cap
		total += len(r.buf[idx])
	}

	result := make([]byte, 0, total)
	for i := 0; i < r.count; i++ {
		idx := (r.start + i) % r.cap
		result = append(result, r.buf[idx]...)
		r.buf[idx] = nil
	}
	r.start = 0
	r.count = 0
	return result
}

// Clear discards all buffered data without returning it.
func (r *ringBuffer) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := 0; i < r.count; i++ {
		idx := (r.start + i) % r.cap
		r.buf[idx] = nil
	}
	r.start = 0
	r.count = 0
}

// stdinResult carries data or an error from the stdin reader goroutine.
type stdinResult struct {
	data []byte
	err  error
}

// ManagedSession wraps an SSH connection with detach/reattach support.
// It implements tea.ExecCommand (Run, SetStdin, SetStdout, SetStderr).
type ManagedSession struct {
	// Connection params
	ID           string
	Name         string
	ConnID       string
	Protocol     string
	Host         string
	Port         int
	Username     string
	Password     string
	IdentityFile string
	ProxyJump    string
	ProxyCommand string
	PortForwards []config.PortForward

	// Lifecycle hooks
	ConnHooks  hooks.Hooks
	hookRunner *hooks.HookRunner
	hookEnv    map[string]string

	// SSH state (persists across detach/reattach cycles)
	client      *ssh.Client
	jumpClients []*ssh.Client // intermediate jump host clients for cleanup
	session     *ssh.Session
	sshStdin    io.WriteCloser
	sshStdout   io.Reader
	sshStderr   io.Reader
	pfManager   *PortForwardManager

	// Buffers for background output capture
	outputBuf *ringBuffer
	stderrBuf *ringBuffer

	// Reader goroutine lifecycle
	stopReaders chan struct{} // closed to signal reader goroutines to stop
	readerWg    sync.WaitGroup // tracks active reader goroutines

	// Lifecycle
	doneCh  chan struct{} // closed when SSH session ends
	doneErr error
	mu      sync.Mutex
	status  SessionStatus

	// Timestamps
	ConnectedAt time.Time
	DetachedAt  time.Time

	// Set by bubbletea before Run()
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	// Track whether we've already connected
	connected bool
}

// NewManagedSession creates a new managed session ready to connect.
func NewManagedSession(id, name, connID, protocol, host string, port int, username, password, identityFile, proxyJump, proxyCommand string) *ManagedSession {
	return &ManagedSession{
		ID:           id,
		Name:         name,
		ConnID:       connID,
		Protocol:     protocol,
		Host:         host,
		Port:         port,
		Username:     username,
		Password:     password,
		IdentityFile: identityFile,
		ProxyJump:    proxyJump,
		ProxyCommand: proxyCommand,
		outputBuf:    newRingBuffer(1000),
		stderrBuf:    newRingBuffer(100),
		doneCh:       make(chan struct{}),
		status:       StatusConnecting,
	}
}

// SetHooks configures lifecycle hooks for this session.
func (m *ManagedSession) SetHooks(h hooks.Hooks) {
	m.ConnHooks = h
	m.hookRunner = hooks.NewHookRunner()
	m.hookEnv = hooks.ConnectionEnv(m.ConnID, m.Host, m.Port, m.Username, m.Protocol)
}

// runHooksForEvent executes hooks for the given event, logging warnings
// for FailWarn results. Returns error only if a FailAbort hook fails.
func (m *ManagedSession) runHooksForEvent(event hooks.HookEvent) error {
	if m.hookRunner == nil {
		return nil
	}
	hks := m.ConnHooks.ForEvent(event)
	if len(hks) == 0 {
		return nil
	}

	results, err := m.hookRunner.RunHooks(context.Background(), event, hks, m.hookEnv)

	// Log warnings for failed hooks that didn't abort
	for _, r := range results {
		if r.Error != nil && r.Hook.OnFailure == hooks.FailWarn {
			msg := fmt.Sprintf("hook warning [%s] %q: %v\n", event, r.Hook.Command, r.Error)
			m.stderrBuf.Write([]byte(msg))
		}
	}

	return err
}

// SetStdin sets the stdin reader (called by bubbletea before Run).
func (m *ManagedSession) SetStdin(r io.Reader) { m.stdin = r }

// SetStdout sets the stdout writer (called by bubbletea before Run).
func (m *ManagedSession) SetStdout(w io.Writer) { m.stdout = w }

// SetStderr sets the stderr writer (called by bubbletea before Run).
func (m *ManagedSession) SetStderr(w io.Writer) { m.stderr = w }

// Run connects (on first call) and enters the attach loop.
// Returns ErrDetached on Ctrl+\, or the session error when SSH exits.
func (m *ManagedSession) Run() error {
	if !m.connected {
		// Run pre-connect hooks
		if err := m.runHooksForEvent(hooks.PreConnect); err != nil {
			m.setStatus(StatusClosed)
			return fmt.Errorf("pre-connect hook: %w", err)
		}

		if err := m.connect(); err != nil {
			m.setStatus(StatusClosed)
			return err
		}
		if err := m.startShell(); err != nil {
			m.client.Close()
			m.setStatus(StatusClosed)
			return err
		}
		m.connected = true
		m.ConnectedAt = time.Now()
		m.setStatus(StatusConnected)

		// Run post-connect hooks (best-effort)
		m.runHooksForEvent(hooks.PostConnect)

		// Start port forwards
		if len(m.PortForwards) > 0 {
			m.pfManager = NewPortForwardManager()
			for _, pf := range m.PortForwards {
				if err := m.startPortForward(pf); err != nil {
					// Log to stderr buffer but don't fail the session
					msg := fmt.Sprintf("port forward %s: %v\n", pf.String(), err)
					m.stderrBuf.Write([]byte(msg))
				}
			}
		}

		// Clear terminal for new session
		m.stdout.Write([]byte("\033[2J\033[H"))

		// Launch background goroutines for the lifetime of the SSH connection
		m.startReaders()
		go m.waitDone()
	} else {
		// Reattach: stop old reader goroutines, clear stale buffer data,
		// and start fresh readers so this session has independent I/O.
		m.stopAndClearReaders()
		m.startReaders()
	}

	err := m.attachLoop()

	// On detach, stop reader goroutines so they don't orphan and so
	// stale buffer data doesn't bleed into a future reattach.
	if errors.Is(err, ErrDetached) {
		m.signalStopReaders()
	}

	return err
}

// startReaders initializes the stop channel and launches readOutput/readStderr goroutines.
func (m *ManagedSession) startReaders() {
	m.stopReaders = make(chan struct{})
	m.readerWg.Add(2)
	go m.readOutput()
	go m.readStderr()
}

// signalStopReaders closes the stopReaders channel to tell reader goroutines to exit.
// It does not wait for them to finish (they may be blocked in Read()).
func (m *ManagedSession) signalStopReaders() {
	if m.stopReaders != nil {
		select {
		case <-m.stopReaders:
			// already closed
		default:
			close(m.stopReaders)
		}
	}
}

// stopAndClearReaders signals reader goroutines to stop, waits briefly for them
// to exit, then clears the ring buffers so stale data doesn't bleed through.
func (m *ManagedSession) stopAndClearReaders() {
	m.signalStopReaders()
	// Give readers a moment to notice the stop signal and exit.
	// They will exit after their next Read() returns or immediately if
	// they check the stop channel between reads.
	done := make(chan struct{})
	go func() {
		m.readerWg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		// Readers may still be blocked in Read(); proceed anyway.
		// They will see stopReaders is closed on their next iteration
		// and will not write to the buffers.
	}
	m.outputBuf.Clear()
	m.stderrBuf.Clear()
}

func (m *ManagedSession) startPortForward(pf config.PortForward) error {
	switch pf.Type {
	case config.PortForwardLocal:
		return m.pfManager.StartLocal(m.client, pf.LocalAddr, pf.RemoteAddr)
	case config.PortForwardRemote:
		return m.pfManager.StartRemote(m.client, pf.LocalAddr, pf.RemoteAddr)
	case config.PortForwardDynamic:
		return m.pfManager.StartDynamic(m.client, pf.LocalAddr)
	default:
		return fmt.Errorf("unknown forward type: %s", pf.Type)
	}
}

func (m *ManagedSession) connect() error {
	if m.ProxyJump != "" {
		return m.connectViaJumpHosts()
	}

	addr := net.JoinHostPort(m.Host, strconv.Itoa(m.Port))

	authMethods := buildSSHAuth(m.Username, m.Password, m.IdentityFile)
	if len(authMethods) == 0 {
		authMethods = defaultSSHAuth()
	}

	config := &ssh.ClientConfig{
		User:            m.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}
	m.client = client
	return nil
}

func (m *ManagedSession) connectViaJumpHosts() error {
	hops := strings.Split(m.ProxyJump, ",")

	var currentClient *ssh.Client
	var jumpClients []*ssh.Client

	for _, hop := range hops {
		hop = strings.TrimSpace(hop)
		hopUser, hopHost, hopPort := parseJumpHost(hop)

		var conn net.Conn
		var err error
		if currentClient == nil {
			// First hop: direct TCP connection
			conn, err = net.DialTimeout("tcp", net.JoinHostPort(hopHost, hopPort), 10*time.Second)
		} else {
			// Subsequent hops: dial through existing SSH client
			conn, err = currentClient.Dial("tcp", net.JoinHostPort(hopHost, hopPort))
		}
		if err != nil {
			// Clean up any already-established jump clients
			for i := len(jumpClients) - 1; i >= 0; i-- {
				jumpClients[i].Close()
			}
			return fmt.Errorf("jump host %s: %w", hop, err)
		}

		// SSH handshake through the connection
		authMethods := buildSSHAuth(hopUser, "", "")
		if len(authMethods) == 0 {
			authMethods = defaultSSHAuth()
		}

		ncc, chans, reqs, err := ssh.NewClientConn(conn, net.JoinHostPort(hopHost, hopPort), &ssh.ClientConfig{
			User:            hopUser,
			Auth:            authMethods,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         10 * time.Second,
		})
		if err != nil {
			conn.Close()
			for i := len(jumpClients) - 1; i >= 0; i-- {
				jumpClients[i].Close()
			}
			return fmt.Errorf("jump host SSH %s: %w", hop, err)
		}
		currentClient = ssh.NewClient(ncc, chans, reqs)
		jumpClients = append(jumpClients, currentClient)
	}

	// Final hop: connect to target through the last jump host
	targetAddr := net.JoinHostPort(m.Host, strconv.Itoa(m.Port))
	conn, err := currentClient.Dial("tcp", targetAddr)
	if err != nil {
		for i := len(jumpClients) - 1; i >= 0; i-- {
			jumpClients[i].Close()
		}
		return fmt.Errorf("target dial via jump: %w", err)
	}

	authMethods := buildSSHAuth(m.Username, m.Password, m.IdentityFile)
	if len(authMethods) == 0 {
		authMethods = defaultSSHAuth()
	}

	ncc, chans, reqs, err := ssh.NewClientConn(conn, targetAddr, &ssh.ClientConfig{
		User:            m.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	})
	if err != nil {
		conn.Close()
		for i := len(jumpClients) - 1; i >= 0; i-- {
			jumpClients[i].Close()
		}
		return fmt.Errorf("target SSH via jump: %w", err)
	}

	m.client = ssh.NewClient(ncc, chans, reqs)
	m.jumpClients = jumpClients
	return nil
}

// parseJumpHost parses a jump host string in the format user@host:port.
// Port defaults to "22" if not specified. User defaults to $USER if not specified.
func parseJumpHost(hop string) (user, host, port string) {
	port = "22"
	if at := strings.LastIndex(hop, "@"); at >= 0 {
		user = hop[:at]
		hop = hop[at+1:]
	}
	if h, p, err := net.SplitHostPort(hop); err == nil {
		host, port = h, p
	} else {
		host = hop
	}
	if user == "" {
		user = os.Getenv("USER")
	}
	return
}

func (m *ManagedSession) startShell() error {
	sess, err := m.client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh session: %w", err)
	}
	m.session = sess

	// Request PTY with default size; will be updated on attach
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		sess.Close()
		return fmt.Errorf("ssh pty: %w", err)
	}

	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		return fmt.Errorf("ssh stdin pipe: %w", err)
	}
	m.sshStdin = stdin

	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		return fmt.Errorf("ssh stdout pipe: %w", err)
	}
	m.sshStdout = stdout

	stderr, err := sess.StderrPipe()
	if err != nil {
		sess.Close()
		return fmt.Errorf("ssh stderr pipe: %w", err)
	}
	m.sshStderr = stderr

	if err := sess.Shell(); err != nil {
		sess.Close()
		return fmt.Errorf("ssh shell: %w", err)
	}

	return nil
}

// readOutput continuously reads SSH stdout into the ring buffer.
// It exits when stopReaders is closed or the SSH pipe returns an error.
func (m *ManagedSession) readOutput() {
	defer m.readerWg.Done()
	buf := make([]byte, 32*1024)
	for {
		n, err := m.sshStdout.Read(buf)
		if n > 0 {
			select {
			case <-m.stopReaders:
				return
			default:
				m.outputBuf.Write(buf[:n])
			}
		}
		if err != nil {
			return
		}
		select {
		case <-m.stopReaders:
			return
		default:
		}
	}
}

// readStderr continuously reads SSH stderr into the ring buffer.
// It exits when stopReaders is closed or the SSH pipe returns an error.
func (m *ManagedSession) readStderr() {
	defer m.readerWg.Done()
	buf := make([]byte, 4*1024)
	for {
		n, err := m.sshStderr.Read(buf)
		if n > 0 {
			select {
			case <-m.stopReaders:
				return
			default:
				m.stderrBuf.Write(buf[:n])
			}
		}
		if err != nil {
			return
		}
		select {
		case <-m.stopReaders:
			return
		default:
		}
	}
}

// waitDone waits for the SSH session to end and signals via doneCh.
func (m *ManagedSession) waitDone() {
	err := m.session.Wait()
	m.mu.Lock()
	m.doneErr = err
	m.status = StatusClosed
	m.mu.Unlock()

	// Run post-disconnect hooks (best-effort, before cleanup)
	m.runHooksForEvent(hooks.PostDisconnect)

	close(m.doneCh)

	// Stop port forwards
	if m.pfManager != nil {
		m.pfManager.StopAll()
	}

	// Clean up
	m.session.Close()
	m.client.Close()
	// Close jump host clients in reverse order
	for i := len(m.jumpClients) - 1; i >= 0; i-- {
		m.jumpClients[i].Close()
	}
	m.jumpClients = nil
}

// attachLoop enters raw mode and runs the interactive I/O loop.
// Returns ErrDetached on Ctrl+\ or the session error when SSH exits.
func (m *ManagedSession) attachLoop() error {
	// Use os.Stdin directly for terminal operations — the reader passed by
	// bubbletea may be wrapped and not type-assertable to *os.File.
	fd := int(os.Stdin.Fd())
	hasTerminal := term.IsTerminal(fd)

	if hasTerminal {
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return fmt.Errorf("terminal raw mode: %w", err)
		}
		defer term.Restore(fd, oldState)
	}

	// Catch SIGQUIT (Ctrl+\ when terminal is not in raw mode) as a fallback
	// detach mechanism. In raw mode, Ctrl+\ sends byte 0x1C which is caught
	// in the stdin read loop below.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGQUIT)
	defer signal.Stop(sigCh)

	// Send current terminal size
	if hasTerminal {
		if w, h, err := term.GetSize(fd); err == nil {
			_ = m.session.WindowChange(h, w)
		}
	}

	m.setStatus(StatusConnected)

	// Flush any buffered output
	if data := m.outputBuf.Drain(); len(data) > 0 {
		m.stdout.Write(data)
	}
	if data := m.stderrBuf.Drain(); len(data) > 0 {
		m.stderr.Write(data)
	}

	// detachCh is closed when this attachLoop exits (detach, session end,
	// or error). The stdin reader goroutine monitors it so it stops
	// competing for os.Stdin once this session is no longer attached.
	detachCh := make(chan struct{})
	defer close(detachCh)

	stdinCh := make(chan stdinResult, 1)

	// Start a stdin reader goroutine scoped to this attach cycle.
	// It exits when detachCh is closed (checked after each Read).
	go func() {
		buf := make([]byte, 4*1024)
		for {
			n, err := m.stdin.Read(buf)

			// Check if we've detached while blocked on Read.
			select {
			case <-detachCh:
				return
			default:
			}

			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				select {
				case stdinCh <- stdinResult{data: data}:
				case <-detachCh:
					return
				}
			}
			if err != nil {
				select {
				case stdinCh <- stdinResult{err: err}:
				case <-detachCh:
				}
				return
			}
		}
	}()

	// Channel to stop the resize watcher
	stopResize := make(chan struct{})

	// Start resize watcher
	if hasTerminal {
		go func() {
			prevW, prevH, _ := term.GetSize(fd)
			for {
				select {
				case <-stopResize:
					return
				case <-m.doneCh:
					return
				case <-time.After(250 * time.Millisecond):
				}
				w, h, err := term.GetSize(fd)
				if err != nil {
					return
				}
				if w != prevW || h != prevH {
					_ = m.session.WindowChange(h, w)
					prevW, prevH = w, h
				}
			}
		}()
	}

	// Poll interval for draining output buffer
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-m.doneCh:
			// Session ended
			close(stopResize)
			// Flush remaining
			if data := m.outputBuf.Drain(); len(data) > 0 {
				m.stdout.Write(data)
			}
			m.mu.Lock()
			err := m.doneErr
			m.mu.Unlock()
			return err

		case <-sigCh:
			// SIGQUIT (Ctrl+\ in non-raw mode) — treat as detach
			close(stopResize)
			m.DetachedAt = time.Now()
			m.setStatus(StatusDetached)
			return ErrDetached

		case res := <-stdinCh:
			if res.err != nil {
				close(stopResize)
				return res.err
			}
			// Check for detach key (Ctrl+\, byte 0x1C)
			for _, b := range res.data {
				if b == 0x1C {
					close(stopResize)
					m.DetachedAt = time.Now()
					m.setStatus(StatusDetached)
					return ErrDetached
				}
			}
			// Forward to SSH stdin
			m.sshStdin.Write(res.data)

		case <-ticker.C:
			// Drain any buffered output to terminal
			if data := m.outputBuf.Drain(); len(data) > 0 {
				m.stdout.Write(data)
			}
			if data := m.stderrBuf.Drain(); len(data) > 0 {
				m.stderr.Write(data)
			}
		}
	}
}

// Kill forcefully closes the SSH session and client.
func (m *ManagedSession) Kill() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.status == StatusClosed {
		return
	}
	m.status = StatusClosed

	// Run pre-disconnect hooks (best-effort)
	// Note: we run these without holding the lock since they may take time.
	// We already set status to Closed above to prevent re-entry.
	m.mu.Unlock()
	m.runHooksForEvent(hooks.PreDisconnect)
	m.mu.Lock()

	// Stop port forwards
	if m.pfManager != nil {
		m.pfManager.StopAll()
	}

	if m.session != nil {
		m.session.Close()
	}
	if m.client != nil {
		m.client.Close()
	}
	// Close jump host clients in reverse order
	for i := len(m.jumpClients) - 1; i >= 0; i-- {
		m.jumpClients[i].Close()
	}
	m.jumpClients = nil
}

// Status returns the current session status.
func (m *ManagedSession) Status() SessionStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *ManagedSession) setStatus(s SessionStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = s
}

// IsAlive returns true if the session has not closed.
func (m *ManagedSession) IsAlive() bool {
	select {
	case <-m.doneCh:
		return false
	default:
		return true
	}
}

// Uptime returns the duration since the session was connected.
func (m *ManagedSession) Uptime() time.Duration {
	if m.ConnectedAt.IsZero() {
		return 0
	}
	return time.Since(m.ConnectedAt)
}

// PortForwardList returns the list of active port forwards for this session.
func (m *ManagedSession) PortForwardList() []ActiveForward {
	if m.pfManager == nil {
		return nil
	}
	return m.pfManager.List()
}
