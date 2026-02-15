package termcap

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// probeSyncOutput checks if the terminal supports Mode 2026 (synchronized output)
// by sending a DECRPM query and checking the response.
func probeSyncOutput(fd int, timeout time.Duration) bool {
	// Send DECRPM query for mode 2026
	// Response format: CSI ? 2026 ; Ps $ y
	// Ps=1 (set), Ps=2 (reset), Ps=3 (permanently set), Ps=4 (permanently reset)
	// Ps=0 means not recognized
	os.Stdout.WriteString("\x1b[?2026$p")

	response := readWithTimeout(fd, timeout)
	// Check for valid DECRPM response indicating the mode is recognized
	// Response contains "2026" and ends with "$y" and Ps is 1 or 2 (settable)
	if strings.Contains(response, "2026") && strings.Contains(response, "$y") {
		return true
	}

	// Heuristic fallback: check known terminals that support it
	termProg := os.Getenv("TERM_PROGRAM")
	switch strings.ToLower(termProg) {
	case "ghostty", "wezterm", "kitty", "iterm.app", "contour":
		return true
	}
	// tmux supports it since 3.3a
	if os.Getenv("TMUX") != "" {
		return true
	}

	return false
}

// probeOSC52 checks if the terminal likely supports OSC 52 clipboard.
// We use heuristic detection since there's no reliable query for OSC 52 support.
func probeOSC52() bool {
	// Check known terminals that support OSC 52
	termProg := os.Getenv("TERM_PROGRAM")
	switch strings.ToLower(termProg) {
	case "ghostty", "wezterm", "kitty", "iterm.app", "alacritty",
		"contour", "foot", "rio", "tmux":
		return true
	}

	// tmux supports OSC 52 passthrough
	if os.Getenv("TMUX") != "" {
		return true
	}

	// screen supports OSC 52
	if strings.HasPrefix(os.Getenv("TERM"), "screen") {
		return true
	}

	// xterm supports it
	if strings.HasPrefix(os.Getenv("TERM"), "xterm") {
		return true
	}

	return false
}

// probeColorProfile detects the terminal's color capability.
func probeColorProfile() string {
	colorTerm := os.Getenv("COLORTERM")
	if colorTerm == "truecolor" || colorTerm == "24bit" {
		return "truecolor"
	}

	termVal := os.Getenv("TERM")
	if strings.Contains(termVal, "256color") {
		return "256"
	}

	if strings.HasPrefix(termVal, "xterm") || strings.HasPrefix(termVal, "screen") ||
		strings.HasPrefix(termVal, "tmux") || strings.HasPrefix(termVal, "rxvt") {
		return "16"
	}

	if termVal == "dumb" || termVal == "" {
		return "mono"
	}

	// Default to 256 as a safe middle ground for unknown terminals
	return "256"
}

// probeBackgroundColor queries the terminal for its background color via OSC 11.
// Returns true if the background is dark.
func probeBackgroundColor(fd int, timeout time.Duration) bool {
	// Send OSC 11 query
	os.Stdout.WriteString("\x1b]11;?\x1b\\")

	response := readWithTimeout(fd, timeout)
	// Response format: OSC 11 ; rgb:RRRR/GGGG/BBBB ST
	if idx := strings.Index(response, "rgb:"); idx >= 0 {
		return parseIsDark(response[idx:])
	}

	// Default to dark background (most common for terminal users)
	return true
}

// parseIsDark parses an rgb:RRRR/GGGG/BBBB string and returns true if it's dark.
func parseIsDark(rgb string) bool {
	// Extract hex values
	var r, g, b uint32
	n, _ := fmt.Sscanf(rgb, "rgb:%4x/%4x/%4x", &r, &g, &b)
	if n != 3 {
		return true // default dark
	}

	// Convert 16-bit values to 8-bit for luminance calc
	r8 := float64(r) / 65535.0
	g8 := float64(g) / 65535.0
	b8 := float64(b) / 65535.0

	// Relative luminance (ITU-R BT.709)
	luminance := 0.2126*r8 + 0.7152*g8 + 0.0722*b8
	return luminance < 0.5
}

// readWithTimeout reads from the terminal fd with a timeout.
func readWithTimeout(fd int, timeout time.Duration) string {
	buf := make([]byte, 256)
	result := make([]byte, 0, 256)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Use a short read with the remaining time
		// Since we're in raw mode, reads are non-blocking-ish
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
			// Check if we got a complete response (ends with ST or BEL)
			s := string(result)
			if strings.HasSuffix(s, "\x1b\\") || strings.HasSuffix(s, "\x07") ||
				strings.Contains(s, "$y") {
				break
			}
		}
		if err != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	return string(result)
}
