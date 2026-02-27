package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/dr4zz/nexus/internal/hooks"
)

// Protocol represents a connection protocol type.
type Protocol string

const (
	ProtoSSH    Protocol = "ssh"
	ProtoRDP    Protocol = "rdp"
	ProtoVNC    Protocol = "vnc"
	ProtoTelnet Protocol = "telnet"
)

func (p Protocol) Valid() bool {
	switch p {
	case ProtoSSH, ProtoRDP, ProtoVNC, ProtoTelnet:
		return true
	}
	return false
}

func (p Protocol) DefaultPort() int {
	switch p {
	case ProtoSSH:
		return 22
	case ProtoRDP:
		return 3389
	case ProtoVNC:
		return 5900
	case ProtoTelnet:
		return 23
	}
	return 0
}

func (p Protocol) Label() string {
	return strings.ToUpper(string(p))
}

// PortForwardType represents the type of SSH port forwarding.
type PortForwardType string

const (
	PortForwardLocal   PortForwardType = "local"
	PortForwardRemote  PortForwardType = "remote"
	PortForwardDynamic PortForwardType = "dynamic"
)

// PortForward represents a single SSH port forwarding rule.
type PortForward struct {
	Type       PortForwardType `yaml:"type"`
	LocalAddr  string          `yaml:"local_addr"`
	RemoteAddr string          `yaml:"remote_addr,omitempty"`
}

// String returns the compact notation (e.g. "L:8080:remote:80", "R:9090:local:9090", "D:1080").
func (pf PortForward) String() string {
	switch pf.Type {
	case PortForwardLocal:
		return fmt.Sprintf("L:%s:%s", pf.LocalAddr, pf.RemoteAddr)
	case PortForwardRemote:
		return fmt.Sprintf("R:%s:%s", pf.LocalAddr, pf.RemoteAddr)
	case PortForwardDynamic:
		return fmt.Sprintf("D:%s", pf.LocalAddr)
	}
	return ""
}

// ParsePortForwards parses a comma-separated port forward spec string.
// Format: "L:localAddr:remoteAddr,R:remoteAddr:localAddr,D:localAddr"
// Examples:
//   - "L:8080:remote:80" — local forward from localhost:8080 to remote:80
//   - "R:9090:localhost:9090" — remote forward from remote:9090 to localhost:9090
//   - "D:1080" — dynamic SOCKS5 on localhost:1080
func ParsePortForwards(spec string) ([]PortForward, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}

	var forwards []PortForward
	parts := strings.Split(spec, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		pf, err := parseOnePortForward(part)
		if err != nil {
			return nil, fmt.Errorf("invalid port forward %q: %w", part, err)
		}
		forwards = append(forwards, pf)
	}
	return forwards, nil
}

func parseOnePortForward(s string) (PortForward, error) {
	if len(s) < 2 || s[1] != ':' {
		return PortForward{}, fmt.Errorf("must start with L:, R:, or D:")
	}

	prefix := strings.ToUpper(s[:1])
	rest := s[2:]

	switch prefix {
	case "L":
		local, remote, err := splitForwardAddrs(rest)
		if err != nil {
			return PortForward{}, fmt.Errorf("local forward: %w", err)
		}
		return PortForward{Type: PortForwardLocal, LocalAddr: local, RemoteAddr: remote}, nil

	case "R":
		// R:remoteAddr:localAddr — the first part is where the remote side listens,
		// the second is the local target.
		remote, local, err := splitForwardAddrs(rest)
		if err != nil {
			return PortForward{}, fmt.Errorf("remote forward: %w", err)
		}
		return PortForward{Type: PortForwardRemote, LocalAddr: remote, RemoteAddr: local}, nil

	case "D":
		if rest == "" {
			return PortForward{}, fmt.Errorf("dynamic forward: missing address")
		}
		return PortForward{Type: PortForwardDynamic, LocalAddr: rest}, nil

	default:
		return PortForward{}, fmt.Errorf("unknown type %q, must be L, R, or D", prefix)
	}
}

// splitForwardAddrs splits "host:port:host:port" into two "host:port" pairs.
// Also supports just "port:host:port" (local port only, binds 0.0.0.0).
func splitForwardAddrs(s string) (string, string, error) {
	// Strategy: Split on colons and work backwards.
	// Valid forms:
	//   port:host:port           -> 0.0.0.0:port  host:port
	//   host:port:host:port      -> host:port      host:port
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 3:
		// port:host:port
		return "0.0.0.0:" + parts[0], parts[1] + ":" + parts[2], nil
	case 4:
		// host:port:host:port
		return parts[0] + ":" + parts[1], parts[2] + ":" + parts[3], nil
	default:
		return "", "", fmt.Errorf("expected format host:port:host:port or port:host:port, got %q", s)
	}
}

// FormatPortForwards returns the compact comma-separated string representation.
func FormatPortForwards(forwards []PortForward) string {
	if len(forwards) == 0 {
		return ""
	}
	var parts []string
	for _, pf := range forwards {
		parts = append(parts, pf.String())
	}
	return strings.Join(parts, ",")
}

// RDPOptions holds RDP-specific settings.
type RDPOptions struct {
	Resolution        string `yaml:"resolution,omitempty"`
	Fullscreen        bool   `yaml:"fullscreen,omitempty"`
	DynamicResolution bool   `yaml:"dynamic_resolution,omitempty"`
	Security          string `yaml:"security,omitempty"` // "auto", "nla", "tls", "rdp"
}

// Connection represents a single remote connection.
type Connection struct {
	ID           string        `yaml:"id"`
	Name         string        `yaml:"name"`
	Protocol     Protocol      `yaml:"protocol"`
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port,omitempty"`
	Username     string        `yaml:"username,omitempty"`
	Password     string        `yaml:"password,omitempty"`
	IdentityFile string        `yaml:"identity_file,omitempty"`
	ProxyJump    string        `yaml:"proxy_jump,omitempty"`    // comma-separated list of jump hosts (user@host:port)
	ProxyCommand string        `yaml:"proxy_command,omitempty"` // custom proxy command
	PortForwards []PortForward `yaml:"port_forwards,omitempty"` // SSH port forwarding rules
	Domain       string        `yaml:"domain,omitempty"`
	Group        string        `yaml:"group,omitempty"`
	Tags         []string      `yaml:"tags,omitempty"`
	RDPOptions   RDPOptions    `yaml:"rdp_options,omitempty"`
	VNCPassword       string      `yaml:"vnc_password,omitempty"`
	CredentialProfile string      `yaml:"credential_profile,omitempty"`
	Hooks             hooks.Hooks `yaml:"hooks,omitempty"` // lifecycle hooks (pre/post connect/disconnect)

	// Favorites & usage tracking
	Favorite        bool       `yaml:"favorite,omitempty"`
	LastConnectedAt *time.Time `yaml:"last_connected_at,omitempty"`
	ConnectCount    int        `yaml:"connect_count,omitempty"`

	// Ghost session (auto-connect on startup)
	AutoConnect   bool `yaml:"auto_connect,omitempty"`
	AutoReconnect bool `yaml:"auto_reconnect,omitempty"`

	// Notes and custom metadata
	Notes        string            `yaml:"notes,omitempty"`
	CustomFields map[string]string `yaml:"custom_fields,omitempty"`
}

// EffectivePort returns the configured port or the protocol default.
func (c Connection) EffectivePort() int {
	if c.Port != 0 {
		return c.Port
	}
	return c.Protocol.DefaultPort()
}

// HostPort returns host:port string.
func (c Connection) HostPort() string {
	return fmt.Sprintf("%s:%d", c.Host, c.EffectivePort())
}

// SanitizeID produces a kebab-cased ID from a connection name.
// It replaces spaces and dots with hyphens and lowercases the result.
// The optional prefix is prepended (e.g., "ssh-").
func SanitizeID(name, prefix string) string {
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, ".", "-")
	return prefix + id
}

// Group represents a named group with a display color.
type Group struct {
	Name  string `yaml:"name"`
	Color string `yaml:"color,omitempty"`
}
