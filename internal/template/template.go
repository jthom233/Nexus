// Package template provides reusable connection templates for Nexus.
//
// Templates are partial connection definitions that pre-fill form fields
// when creating new connections. Built-in templates cover common protocols
// and use cases; users can also define custom templates in their config.
package template

import (
	"sort"

	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/hooks"
)

// Template is a partial connection definition used to pre-fill new connections.
type Template struct {
	Name          string      `yaml:"name"`
	Description   string      `yaml:"description,omitempty"`
	Protocol      string      `yaml:"protocol,omitempty"`
	Port          int         `yaml:"port,omitempty"`
	Username      string      `yaml:"username,omitempty"`
	Group         string      `yaml:"group,omitempty"`
	Tags          []string    `yaml:"tags,omitempty"`
	ProxyJump     string      `yaml:"proxy_jump,omitempty"`
	RecordSession bool        `yaml:"record_session,omitempty"`
	Hooks         hooks.Hooks `yaml:"hooks,omitempty"`
}

// TemplateStore manages built-in and user-defined templates.
type TemplateStore struct {
	templates map[string]Template
}

// NewTemplateStore returns a store pre-populated with built-in templates.
func NewTemplateStore() *TemplateStore {
	s := &TemplateStore{
		templates: make(map[string]Template),
	}
	for _, t := range builtinTemplates() {
		s.templates[t.Name] = t
	}
	return s
}

// Get returns a template by name and whether it was found.
func (s *TemplateStore) Get(name string) (Template, bool) {
	t, ok := s.templates[name]
	return t, ok
}

// List returns all templates sorted by name.
func (s *TemplateStore) List() []Template {
	out := make([]Template, 0, len(s.templates))
	for _, t := range s.templates {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// Add inserts or replaces a template.
func (s *TemplateStore) Add(t Template) {
	s.templates[t.Name] = t
}

// Remove deletes a template by name.
func (s *TemplateStore) Remove(name string) {
	delete(s.templates, name)
}

// Apply fills in empty/zero fields on conn from the template.
// Fields already set on the connection are never overwritten.
func Apply(t Template, conn *config.Connection) {
	if conn.Protocol == "" && t.Protocol != "" {
		conn.Protocol = config.Protocol(t.Protocol)
	}
	if conn.Port == 0 && t.Port != 0 {
		conn.Port = t.Port
	}
	if conn.Username == "" && t.Username != "" {
		conn.Username = t.Username
	}
	if conn.Group == "" && t.Group != "" {
		conn.Group = t.Group
	}
	if len(conn.Tags) == 0 && len(t.Tags) > 0 {
		conn.Tags = make([]string, len(t.Tags))
		copy(conn.Tags, t.Tags)
	}
	if conn.ProxyJump == "" && t.ProxyJump != "" {
		conn.ProxyJump = t.ProxyJump
	}
	if conn.Hooks.IsEmpty() && !t.Hooks.IsEmpty() {
		conn.Hooks = t.Hooks
	}
}

// builtinTemplates returns the set of default templates shipped with Nexus.
func builtinTemplates() []Template {
	return []Template{
		{
			Name:        "ssh-standard",
			Description: "Standard SSH connection on port 22",
			Protocol:    "ssh",
			Port:        22,
		},
		{
			Name:        "ssh-jumphost",
			Description: "SSH via jump host (fill in ProxyJump)",
			Protocol:    "ssh",
			Port:        22,
			ProxyJump:   "user@bastion:22",
		},
		{
			Name:        "rdp-windows",
			Description: "RDP connection for Windows hosts",
			Protocol:    "rdp",
			Port:        3389,
		},
		{
			Name:        "vnc-linux",
			Description: "VNC connection for Linux desktops",
			Protocol:    "vnc",
			Port:        5900,
		},
		{
			Name:        "telnet-network",
			Description: "Telnet for network equipment",
			Protocol:    "telnet",
			Port:        23,
		},
	}
}

// LoadUserTemplates adds user-defined templates from the config to the store.
// User templates with the same name as a built-in will override the built-in.
func (s *TemplateStore) LoadUserTemplates(cfgTemplates []config.ConfigTemplate) {
	for _, ct := range cfgTemplates {
		s.templates[ct.Name] = Template{
			Name:          ct.Name,
			Description:   ct.Description,
			Protocol:      ct.Protocol,
			Port:          ct.Port,
			Username:      ct.Username,
			Group:         ct.Group,
			Tags:          ct.Tags,
			ProxyJump:     ct.ProxyJump,
			RecordSession: ct.RecordSession,
		}
	}
}

// Names returns a sorted list of template names.
func (s *TemplateStore) Names() []string {
	names := make([]string, 0, len(s.templates))
	for name := range s.templates {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
