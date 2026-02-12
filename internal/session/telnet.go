package session

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"

	"golang.org/x/term"
)

// Telnet IAC (Interpret As Command) constants.
const (
	iacByte byte = 255 // IAC
	iacDont byte = 254 // DONT
	iacDo   byte = 253 // DO
	iacWont byte = 252 // WONT
	iacWill byte = 251 // WILL
	iacSB   byte = 250 // Sub-negotiation Begin
	iacSE   byte = 240 // Sub-negotiation End

	optEcho byte = 1  // Echo
	optSGA  byte = 3  // Suppress Go Ahead
	optNAWS byte = 31 // Negotiate About Window Size
	optTT   byte = 24 // Terminal Type
)

// TelnetSession is a native telnet client with IAC negotiation.
// It implements tea.ExecCommand (Run, SetStdin, SetStdout, SetStderr).
type TelnetSession struct {
	Host string
	Port int

	conn   net.Conn
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// SetStdin sets the stdin reader (called by bubbletea before Run).
func (t *TelnetSession) SetStdin(r io.Reader) { t.stdin = r }

// SetStdout sets the stdout writer (called by bubbletea before Run).
func (t *TelnetSession) SetStdout(w io.Writer) { t.stdout = w }

// SetStderr sets the stderr writer (called by bubbletea before Run).
func (t *TelnetSession) SetStderr(w io.Writer) { t.stderr = w }

// Run connects to the telnet server and runs an interactive session.
// This satisfies tea.ExecCommand and blocks until disconnect.
func (t *TelnetSession) Run() error {
	if err := t.connect(); err != nil {
		return err
	}
	defer t.conn.Close()
	return t.shell()
}

func (t *TelnetSession) connect() error {
	addr := net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("telnet dial: %w", err)
	}
	t.conn = conn
	return nil
}

func (t *TelnetSession) shell() error {
	// Determine the terminal fd for raw mode
	stdinFile, isFile := t.stdin.(*os.File)
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

	// Copy from server to stdout, handling IAC sequences
	done := make(chan error, 2)
	go func() {
		done <- t.readLoop(fd, hasTerminal)
	}()

	// Copy from stdin to server
	go func() {
		_, err := io.Copy(t.conn, t.stdin)
		done <- err
	}()

	return <-done
}

func (t *TelnetSession) readLoop(fd int, hasTerminal bool) error {
	buf := make([]byte, 4096)
	for {
		n, err := t.conn.Read(buf)
		if n > 0 {
			t.processData(buf[:n], fd, hasTerminal)
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func (t *TelnetSession) processData(data []byte, fd int, hasTerminal bool) {
	i := 0
	for i < len(data) {
		if data[i] == iacByte {
			if i+1 >= len(data) {
				break
			}
			switch data[i+1] {
			case iacByte:
				t.stdout.Write([]byte{0xFF})
				i += 2
			case iacDo:
				if i+2 < len(data) {
					t.handleDo(data[i+2], fd, hasTerminal)
					i += 3
				} else {
					i = len(data)
				}
			case iacDont:
				if i+2 < len(data) {
					t.handleDont(data[i+2])
					i += 3
				} else {
					i = len(data)
				}
			case iacWill:
				if i+2 < len(data) {
					t.handleWill(data[i+2])
					i += 3
				} else {
					i = len(data)
				}
			case iacWont:
				if i+2 < len(data) {
					i += 3
				} else {
					i = len(data)
				}
			case iacSB:
				j := i + 2
				for j+1 < len(data) {
					if data[j] == iacByte && data[j+1] == iacSE {
						j += 2
						break
					}
					j++
				}
				i = j
			default:
				i += 2
			}
		} else {
			start := i
			for i < len(data) && data[i] != iacByte {
				i++
			}
			t.stdout.Write(data[start:i])
		}
	}
}

func (t *TelnetSession) handleDo(option byte, fd int, hasTerminal bool) {
	switch option {
	case optNAWS:
		t.send(iacByte, iacWill, optNAWS)
		t.sendWindowSize(fd, hasTerminal)
	case optTT:
		t.send(iacByte, iacWill, optTT)
	default:
		t.send(iacByte, iacWont, option)
	}
}

func (t *TelnetSession) handleDont(option byte) {
	t.send(iacByte, iacWont, option)
}

func (t *TelnetSession) handleWill(option byte) {
	switch option {
	case optEcho, optSGA:
		t.send(iacByte, iacDo, option)
	default:
		t.send(iacByte, iacDont, option)
	}
}

func (t *TelnetSession) sendWindowSize(fd int, hasTerminal bool) {
	w, h := 80, 24
	if hasTerminal {
		if tw, th, err := term.GetSize(fd); err == nil {
			w, h = tw, th
		}
	}
	t.send(
		iacByte, iacSB, optNAWS,
		byte(w>>8), byte(w&0xFF),
		byte(h>>8), byte(h&0xFF),
		iacByte, iacSE,
	)
}

func (t *TelnetSession) send(data ...byte) {
	t.conn.Write(data)
}
