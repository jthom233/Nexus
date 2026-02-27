package session

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dr4zz/nexus/internal/config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// SSHSession is a native SSH client session using x/crypto/ssh.
// It implements tea.ExecCommand (Run, SetStdin, SetStdout, SetStderr).
type SSHSession struct {
	Host         string
	Port         int
	Username     string
	Password     string
	IdentityFile string
	ProxyJump    string
	ProxyCommand string
	PortForwards []config.PortForward

	client      *ssh.Client
	jumpClients []*ssh.Client // intermediate jump host clients for cleanup
	session     *ssh.Session
	pfManager   *PortForwardManager
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer

	doneCh    chan struct{}
	closeOnce sync.Once
}

// SetStdin sets the stdin reader (called by bubbletea before Run).
func (s *SSHSession) SetStdin(r io.Reader) { s.stdin = r }

// SetStdout sets the stdout writer (called by bubbletea before Run).
func (s *SSHSession) SetStdout(w io.Writer) { s.stdout = w }

// SetStderr sets the stderr writer (called by bubbletea before Run).
func (s *SSHSession) SetStderr(w io.Writer) { s.stderr = w }

// Run connects to the SSH server and runs an interactive shell.
// This satisfies tea.ExecCommand and blocks until the shell exits.
func (s *SSHSession) Run() error {
	s.doneCh = make(chan struct{})
	if err := s.connect(); err != nil {
		return err
	}
	defer s.close()

	// Start port forwards
	if len(s.PortForwards) > 0 {
		s.pfManager = NewPortForwardManager()
		for _, pf := range s.PortForwards {
			if err := s.startPortForward(pf); err != nil {
				// Log but don't fail the session for a port forward error
				if s.stderr != nil {
					fmt.Fprintf(s.stderr, "port forward %s: %v\n", pf.String(), err)
				}
			}
		}
	}

	return s.shell()
}

func (s *SSHSession) startPortForward(pf config.PortForward) error {
	switch pf.Type {
	case config.PortForwardLocal:
		return s.pfManager.StartLocal(s.client, pf.LocalAddr, pf.RemoteAddr)
	case config.PortForwardRemote:
		return s.pfManager.StartRemote(s.client, pf.LocalAddr, pf.RemoteAddr)
	case config.PortForwardDynamic:
		return s.pfManager.StartDynamic(s.client, pf.LocalAddr)
	default:
		return fmt.Errorf("unknown forward type: %s", pf.Type)
	}
}

func (s *SSHSession) connect() error {
	if s.ProxyJump != "" {
		return s.connectViaJumpHosts()
	}

	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))

	authMethods := buildSSHAuth(s.Username, s.Password, s.IdentityFile)
	if len(authMethods) == 0 {
		authMethods = defaultSSHAuth()
	}

	config := &ssh.ClientConfig{
		User:            s.Username,
		Auth:            authMethods,
		HostKeyCallback: HostKeyCallback(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}
	s.client = client
	return nil
}

func (s *SSHSession) connectViaJumpHosts() error {
	hops := strings.Split(s.ProxyJump, ",")

	var currentClient *ssh.Client
	var jumpClients []*ssh.Client

	for _, hop := range hops {
		hop = strings.TrimSpace(hop)
		hopUser, hopHost, hopPort := parseJumpHost(hop)

		var conn net.Conn
		var err error
		if currentClient == nil {
			conn, err = net.DialTimeout("tcp", net.JoinHostPort(hopHost, hopPort), 10*time.Second)
		} else {
			conn, err = currentClient.Dial("tcp", net.JoinHostPort(hopHost, hopPort))
		}
		if err != nil {
			for i := len(jumpClients) - 1; i >= 0; i-- {
				jumpClients[i].Close()
			}
			return fmt.Errorf("jump host %s: %w", hop, err)
		}

		authMethods := buildSSHAuth(hopUser, "", "")
		if len(authMethods) == 0 {
			authMethods = defaultSSHAuth()
		}

		ncc, chans, reqs, err := ssh.NewClientConn(conn, net.JoinHostPort(hopHost, hopPort), &ssh.ClientConfig{
			User:            hopUser,
			Auth:            authMethods,
			HostKeyCallback: HostKeyCallback(),
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
	targetAddr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	conn, err := currentClient.Dial("tcp", targetAddr)
	if err != nil {
		for i := len(jumpClients) - 1; i >= 0; i-- {
			jumpClients[i].Close()
		}
		return fmt.Errorf("target dial via jump: %w", err)
	}

	authMethods := buildSSHAuth(s.Username, s.Password, s.IdentityFile)
	if len(authMethods) == 0 {
		authMethods = defaultSSHAuth()
	}

	ncc, chans, reqs, err := ssh.NewClientConn(conn, targetAddr, &ssh.ClientConfig{
		User:            s.Username,
		Auth:            authMethods,
		HostKeyCallback: HostKeyCallback(),
		Timeout:         10 * time.Second,
	})
	if err != nil {
		conn.Close()
		for i := len(jumpClients) - 1; i >= 0; i-- {
			jumpClients[i].Close()
		}
		return fmt.Errorf("target SSH via jump: %w", err)
	}

	s.client = ssh.NewClient(ncc, chans, reqs)
	s.jumpClients = jumpClients
	return nil
}

func (s *SSHSession) shell() error {
	session, err := s.client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh session: %w", err)
	}
	s.session = session
	defer session.Close()

	// Determine the terminal fd for raw mode and resize
	stdinFile, isFile := s.stdin.(*os.File)
	var fd int
	var hasTerminal bool
	if isFile {
		fd = int(stdinFile.Fd())
		hasTerminal = term.IsTerminal(fd)
	}

	if hasTerminal {
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return fmt.Errorf("terminal raw mode: %w", err)
		}
		defer term.Restore(fd, oldState)
	}

	// Get terminal size
	w, h := 80, 24
	if hasTerminal {
		if tw, th, err := term.GetSize(fd); err == nil {
			w, h = tw, th
		}
	}

	// Request PTY
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", h, w, modes); err != nil {
		return fmt.Errorf("ssh pty: %w", err)
	}

	// Pipe session I/O
	sshStdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("ssh stdin pipe: %w", err)
	}
	sshStdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ssh stdout pipe: %w", err)
	}
	sshStderr, err := session.StderrPipe()
	if err != nil {
		return fmt.Errorf("ssh stderr pipe: %w", err)
	}

	// Start shell
	if err := session.Shell(); err != nil {
		return fmt.Errorf("ssh shell: %w", err)
	}

	// Handle terminal resize in background
	if hasTerminal {
		go s.watchResize(fd, session)
	}

	// Copy I/O using the streams provided by bubbletea
	go io.Copy(sshStdin, s.stdin)
	go io.Copy(s.stdout, sshStdout)
	go io.Copy(s.stderr, sshStderr)

	return session.Wait()
}

func (s *SSHSession) close() {
	// Signal watchResize goroutine to stop
	if s.doneCh != nil {
		s.closeOnce.Do(func() { close(s.doneCh) })
	}

	// Stop all port forwards first
	if s.pfManager != nil {
		s.pfManager.StopAll()
	}

	if s.session != nil {
		s.session.Close()
	}
	if s.client != nil {
		s.client.Close()
	}
	// Close jump host clients in reverse order
	for i := len(s.jumpClients) - 1; i >= 0; i-- {
		s.jumpClients[i].Close()
	}
	s.jumpClients = nil
}

func (s *SSHSession) watchResize(fd int, session *ssh.Session) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	prevW, prevH, _ := term.GetSize(fd)
	for {
		select {
		case <-s.doneCh:
			return
		case <-ticker.C:
			w, h, err := term.GetSize(fd)
			if err != nil {
				return
			}
			if w != prevW || h != prevH {
				_ = session.WindowChange(h, w)
				prevW, prevH = w, h
			}
		}
	}
}
