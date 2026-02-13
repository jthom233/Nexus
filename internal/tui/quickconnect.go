package tui

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dr4zz/nexus/internal/config"
)

const maxRecentTargets = 20

// QuickConnectMsg is sent when the user submits a quick-connect target.
type QuickConnectMsg struct {
	Conn config.Connection
}

// QuickConnect provides a prompt for connecting to servers without saving them.
type QuickConnect struct {
	input      textinput.Model
	recentList []string // most-recent-first, max 20
	active     bool
	width      int

	// Tab completion through recent targets
	recentIdx int  // -1 = not cycling, 0+ = index in recentList
	cycling   bool // true when Tab was last pressed
}

// NewQuickConnect creates a new QuickConnect prompt.
func NewQuickConnect() *QuickConnect {
	ti := textinput.New()
	ti.Prompt = "Connect> "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff9e64")).Bold(true)
	ti.Placeholder = "user@host:port"
	ti.CharLimit = 256
	return &QuickConnect{
		input:     ti,
		recentIdx: -1,
	}
}

// Activate shows the quick connect input.
func (qc *QuickConnect) Activate() {
	qc.active = true
	qc.input.Focus()
	qc.input.SetValue("")
	qc.recentIdx = -1
	qc.cycling = false
}

// Deactivate hides the quick connect input.
func (qc *QuickConnect) Deactivate() {
	qc.active = false
	qc.input.Blur()
	qc.input.SetValue("")
	qc.recentIdx = -1
	qc.cycling = false
}

// AddRecent adds a target to the recent list (dedup, cap 20).
func (qc *QuickConnect) AddRecent(target string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}
	// Remove duplicate
	for i, r := range qc.recentList {
		if r == target {
			qc.recentList = append(qc.recentList[:i], qc.recentList[i+1:]...)
			break
		}
	}
	// Prepend
	qc.recentList = append([]string{target}, qc.recentList...)
	if len(qc.recentList) > maxRecentTargets {
		qc.recentList = qc.recentList[:maxRecentTargets]
	}
}

// RecentList returns the recent targets.
func (qc *QuickConnect) RecentList() []string {
	return qc.recentList
}

// Update handles input for the quick connect prompt.
func (qc *QuickConnect) Update(msg tea.Msg) tea.Cmd {
	if !qc.active {
		return nil
	}

	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "enter":
			target := strings.TrimSpace(qc.input.Value())
			if target == "" {
				return nil
			}
			conn, err := ParseTarget(target)
			if err != nil {
				// Return nothing — caller checks flash
				return nil
			}
			qc.AddRecent(target)
			qc.Deactivate()
			return func() tea.Msg { return QuickConnectMsg{Conn: conn} }
		case "esc":
			qc.Deactivate()
			return nil
		case "tab":
			if len(qc.recentList) == 0 {
				return nil
			}
			if !qc.cycling {
				qc.cycling = true
				qc.recentIdx = 0
			} else {
				qc.recentIdx = (qc.recentIdx + 1) % len(qc.recentList)
			}
			qc.input.SetValue(qc.recentList[qc.recentIdx])
			qc.input.SetCursor(len(qc.recentList[qc.recentIdx]))
			return nil
		default:
			qc.cycling = false
			qc.recentIdx = -1
		}
	}

	var cmd tea.Cmd
	qc.input, cmd = qc.input.Update(msg)
	return cmd
}

// View renders the quick connect input.
func (qc *QuickConnect) View() string {
	if !qc.active {
		return ""
	}
	return lipgloss.NewStyle().Width(qc.width).Padding(0, 1).Render(qc.input.View())
}

// ParseTarget parses a connection target string into a temporary Connection.
// Supported formats:
//   - "user@host:port"
//   - "host:port"
//   - "host"
//   - "ssh://user@host:port"
//   - "telnet://host:port"
//   - "rdp://host:port"
//   - "vnc://host:port"
func ParseTarget(target string) (config.Connection, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return config.Connection{}, fmt.Errorf("empty target")
	}

	var (
		proto    config.Protocol
		user     string
		host     string
		port     int
		explicit bool // protocol was explicitly given via scheme
	)

	// Check for URL scheme (ssh://, telnet://, rdp://, vnc://)
	if idx := strings.Index(target, "://"); idx > 0 {
		scheme := strings.ToLower(target[:idx])
		rest := target[idx+3:]
		explicit = true

		switch scheme {
		case "ssh":
			proto = config.ProtoSSH
		case "telnet":
			proto = config.ProtoTelnet
		case "rdp":
			proto = config.ProtoRDP
		case "vnc":
			proto = config.ProtoVNC
		default:
			return config.Connection{}, fmt.Errorf("unsupported protocol: %s", scheme)
		}

		// Parse the rest as a URL authority (user@host:port)
		u, err := url.Parse(target)
		if err != nil {
			return config.Connection{}, fmt.Errorf("invalid URL: %w", err)
		}
		host = u.Hostname()
		if host == "" {
			// Fallback: parse manually
			host, user, port = parseAuthority(rest)
			if host == "" {
				return config.Connection{}, fmt.Errorf("missing host in %q", target)
			}
		} else {
			user = u.User.Username()
			if u.Port() != "" {
				p, err := strconv.Atoi(u.Port())
				if err != nil {
					return config.Connection{}, fmt.Errorf("invalid port: %s", u.Port())
				}
				port = p
			}
		}
	} else {
		// No scheme — parse as user@host:port
		host, user, port = parseAuthority(target)
		if host == "" {
			return config.Connection{}, fmt.Errorf("missing host in %q", target)
		}
	}

	// Auto-detect protocol from port if not explicit
	if !explicit {
		proto = detectProtocol(port)
	}

	// Apply default port if not specified
	if port == 0 {
		port = proto.DefaultPort()
	}

	id := fmt.Sprintf("qc-%s-%s-%d", user, host, port)
	name := host
	if user != "" {
		name = user + "@" + host
	}
	if port != proto.DefaultPort() {
		name += ":" + strconv.Itoa(port)
	}

	return config.Connection{
		ID:       id,
		Name:     name,
		Protocol: proto,
		Host:     host,
		Port:     port,
		Username: user,
	}, nil
}

// parseAuthority parses "user@host:port", "host:port", or "host".
// Returns host, user, port (0 if not specified).
func parseAuthority(s string) (host, user string, port int) {
	// Split user@rest
	if at := strings.LastIndex(s, "@"); at >= 0 {
		user = s[:at]
		s = s[at+1:]
	}

	// Split host:port — handle IPv6 [host]:port
	if strings.HasPrefix(s, "[") {
		// IPv6: [host]:port
		end := strings.Index(s, "]")
		if end < 0 {
			host = s[1:] // malformed, best effort
			return
		}
		host = s[1:end]
		rest := s[end+1:]
		if strings.HasPrefix(rest, ":") {
			p, err := strconv.Atoi(rest[1:])
			if err == nil {
				port = p
			}
		}
		return
	}

	// Regular host:port — only split on last colon if what follows is a number
	if colon := strings.LastIndex(s, ":"); colon >= 0 {
		portStr := s[colon+1:]
		p, err := strconv.Atoi(portStr)
		if err == nil && p > 0 && p <= 65535 {
			host = s[:colon]
			port = p
			return
		}
	}

	host = s
	return
}

// detectProtocol auto-detects protocol from port number.
func detectProtocol(port int) config.Protocol {
	switch port {
	case 23:
		return config.ProtoTelnet
	case 3389:
		return config.ProtoRDP
	case 5900:
		return config.ProtoVNC
	default:
		return config.ProtoSSH
	}
}
