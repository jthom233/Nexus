package config

import (
	"fmt"
	"strings"
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

// RDPOptions holds RDP-specific settings.
type RDPOptions struct {
	Resolution        string `yaml:"resolution,omitempty"`
	Fullscreen        bool   `yaml:"fullscreen,omitempty"`
	DynamicResolution bool   `yaml:"dynamic_resolution,omitempty"`
}

// Connection represents a single remote connection.
type Connection struct {
	ID           string     `yaml:"id"`
	Name         string     `yaml:"name"`
	Protocol     Protocol   `yaml:"protocol"`
	Host         string     `yaml:"host"`
	Port         int        `yaml:"port,omitempty"`
	Username     string     `yaml:"username,omitempty"`
	Password     string     `yaml:"password,omitempty"`
	IdentityFile string     `yaml:"identity_file,omitempty"`
	ProxyJump    string     `yaml:"proxy_jump,omitempty"`    // comma-separated list of jump hosts (user@host:port)
	ProxyCommand string     `yaml:"proxy_command,omitempty"` // custom proxy command
	Domain       string     `yaml:"domain,omitempty"`
	Group        string     `yaml:"group,omitempty"`
	Tags         []string   `yaml:"tags,omitempty"`
	RDPOptions   RDPOptions `yaml:"rdp_options,omitempty"`
	VNCPassword  string     `yaml:"vnc_password,omitempty"`
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

// Group represents a named group with a display color.
type Group struct {
	Name  string `yaml:"name"`
	Color string `yaml:"color,omitempty"`
}
