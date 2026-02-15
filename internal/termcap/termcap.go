package termcap

import (
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// Capabilities holds detected terminal capabilities.
type Capabilities struct {
	SyncOutput     bool   // Terminal supports Mode 2026 (synchronized output)
	OSC52          bool   // Terminal supports OSC 52 clipboard
	ColorProfile   string // "truecolor", "256", "16", "mono"
	DarkBackground bool   // Terminal has a dark background
}

var (
	current *Capabilities
	once    sync.Once
)

// Current returns the detected capabilities. Must call Detect() first.
func Current() *Capabilities {
	if current == nil {
		return &Capabilities{
			ColorProfile:   "truecolor",
			DarkBackground: true,
		}
	}
	return current
}

// Detect probes the terminal for capabilities. Should be called once at startup
// BEFORE the bubbletea program starts (needs raw terminal access).
// Total budget: <500ms.
func Detect() *Capabilities {
	once.Do(func() {
		caps := &Capabilities{
			ColorProfile:   "truecolor",
			DarkBackground: true,
		}

		// Only probe if stdout is a terminal
		if !term.IsTerminal(int(os.Stdout.Fd())) {
			current = caps
			return
		}

		// Save and restore terminal state
		fd := int(os.Stdin.Fd())
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			current = caps
			return
		}
		defer term.Restore(fd, oldState)

		// Run probes with individual timeouts
		caps.SyncOutput = probeSyncOutput(fd, 100*time.Millisecond)
		caps.OSC52 = probeOSC52()
		caps.ColorProfile = probeColorProfile()
		caps.DarkBackground = probeBackgroundColor(fd, 100*time.Millisecond)

		current = caps
	})
	return current
}
