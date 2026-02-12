package session

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"

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

	client  *ssh.Client
	session *ssh.Session
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
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
	if err := s.connect(); err != nil {
		return err
	}
	defer s.close()
	return s.shell()
}

func (s *SSHSession) connect() error {
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))

	authMethods := buildSSHAuth(s.Username, s.Password, s.IdentityFile)
	if len(authMethods) == 0 {
		authMethods = defaultSSHAuth()
	}

	config := &ssh.ClientConfig{
		User:            s.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}
	s.client = client
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
	if s.session != nil {
		s.session.Close()
	}
	if s.client != nil {
		s.client.Close()
	}
}

func (s *SSHSession) watchResize(fd int, session *ssh.Session) {
	prevW, prevH, _ := term.GetSize(fd)
	for {
		time.Sleep(250 * time.Millisecond)
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
