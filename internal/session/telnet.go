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

// iacState represents the current position in the IAC state machine.
type iacState int

const (
	iacNone   iacState = iota // normal data
	iacGotIAC                 // saw IAC byte, waiting for command
	iacGotCmd                 // saw 3-byte command (DO/DONT/WILL/WONT), waiting for option
	iacInSub                  // inside subnegotiation (after IAC SB)
	iacSubIAC                 // inside subneg, saw IAC (could be IAC SE or IAC IAC)
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

	// Persistent IAC parser state — survives across conn.Read() calls so that
	// IAC sequences split across chunk boundaries are handled correctly.
	parseState iacState
	pendingCmd byte   // the command byte (DO/DONT/WILL/WONT) when in iacGotCmd
	subBuf     []byte // subnegotiation accumulator
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

// processData feeds bytes from a single Read() call through the persistent IAC
// state machine. Because parseState, pendingCmd, and subBuf survive across
// calls, an IAC sequence that is split across two Read() calls is handled
// correctly — the first call leaves the machine in a mid-sequence state and the
// second call continues from there.
func (t *TelnetSession) processData(data []byte, fd int, hasTerminal bool) {
	// plainStart marks the beginning of a run of non-IAC bytes that can be
	// forwarded to stdout in a single Write call rather than byte-by-byte.
	plainStart := -1

	flushPlain := func(end int) {
		if plainStart >= 0 && end > plainStart {
			t.stdout.Write(data[plainStart:end])
			plainStart = -1
		}
	}

	for i, b := range data {
		switch t.parseState {

		case iacNone:
			if b == iacByte {
				flushPlain(i)
				t.parseState = iacGotIAC
			} else {
				// Accumulate plain bytes for a bulk write.
				if plainStart < 0 {
					plainStart = i
				}
			}

		case iacGotIAC:
			switch b {
			case iacByte:
				// IAC IAC → literal 0xFF in the data stream.
				t.stdout.Write([]byte{0xFF})
				t.parseState = iacNone
			case iacSB:
				t.subBuf = t.subBuf[:0]
				t.parseState = iacInSub
			case iacSE:
				// Unexpected IAC SE outside subneg — ignore and return to normal.
				t.parseState = iacNone
			case iacDo, iacDont, iacWill, iacWont:
				t.pendingCmd = b
				t.parseState = iacGotCmd
			default:
				// NOP, GA, or other single-byte commands — consume and continue.
				t.parseState = iacNone
			}

		case iacGotCmd:
			// b is the option byte for the pending 3-byte command.
			switch t.pendingCmd {
			case iacDo:
				t.handleDo(b, fd, hasTerminal)
			case iacDont:
				t.handleDont(b)
			case iacWill:
				t.handleWill(b)
			case iacWont:
				// We do not currently advertise any options, so a WONT from the
				// server needs no action.
			}
			t.parseState = iacNone

		case iacInSub:
			if b == iacByte {
				t.parseState = iacSubIAC
			} else {
				t.subBuf = append(t.subBuf, b)
			}

		case iacSubIAC:
			switch b {
			case iacSE:
				// End of subnegotiation — process the accumulated buffer.
				t.processSubneg(t.subBuf)
				t.subBuf = t.subBuf[:0]
				t.parseState = iacNone
			case iacByte:
				// IAC IAC inside subneg → literal 0xFF in subneg data.
				t.subBuf = append(t.subBuf, 0xFF)
				t.parseState = iacInSub
			default:
				// Malformed subneg; treat the IAC as a fresh command start.
				t.parseState = iacInSub
			}
		}
	}

	// Flush any trailing plain bytes.
	flushPlain(len(data))
}

// processSubneg handles a completed subnegotiation payload (option byte first).
func (t *TelnetSession) processSubneg(payload []byte) {
	// Currently we have no subnegotiation handling that requires parsing the
	// payload — NAWS is initiated by us, not the server. This hook is provided
	// for future extension (e.g., terminal-type subneg).
	_ = payload
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
