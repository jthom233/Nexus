package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dr4zz/nexus/internal/crypto"
	"gopkg.in/yaml.v3"
)

// Settings holds application-level settings.
type Settings struct {
	HealthCheckInterval string `yaml:"health_check_interval,omitempty"`
	HealthCheckEnabled  *bool  `yaml:"health_check_enabled,omitempty"` // nil = true (default)
	HealthCheckWorkers  int    `yaml:"health_check_workers,omitempty"` // 0 = 50 (default)
	HealthCheckTimeout  string `yaml:"health_check_timeout,omitempty"` // "" = "3s" (default)
	Theme               string `yaml:"theme,omitempty"`
	Vault               string `yaml:"vault,omitempty"` // "internal" (default), "pass", or "keyring"
	ColorProfile string `yaml:"color_profile,omitempty"` // "auto" (default), "truecolor", "256", "16", "mono"
	Animations   *bool  `yaml:"animations,omitempty"`     // nil = true (default)
}

// HealthInterval returns the parsed health check interval, defaulting to 30s.
func (s Settings) HealthInterval() time.Duration {
	if s.HealthCheckInterval == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(s.HealthCheckInterval)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// HealthEnabled returns whether health checks are enabled, defaulting to true.
func (s Settings) HealthEnabled() bool {
	if s.HealthCheckEnabled == nil {
		return true
	}
	return *s.HealthCheckEnabled
}

// HealthWorkers returns the max concurrent health check workers, defaulting to 50.
func (s Settings) HealthWorkers() int {
	if s.HealthCheckWorkers <= 0 {
		return 50
	}
	return s.HealthCheckWorkers
}

// HealthTimeout returns the per-check TCP dial timeout, defaulting to 3s.
func (s Settings) HealthTimeout() time.Duration {
	if s.HealthCheckTimeout == "" {
		return 3 * time.Second
	}
	d, err := time.ParseDuration(s.HealthCheckTimeout)
	if err != nil {
		return 3 * time.Second
	}
	return d
}

// AnimationsEnabled returns whether animations are enabled, defaulting to true.
func (s Settings) AnimationsEnabled() bool {
	if s.Animations == nil {
		return true
	}
	return *s.Animations
}

// ColorProfileOverride returns the manually configured color profile, or "auto".
func (s Settings) ColorProfileOverride() string {
	if s.ColorProfile == "" {
		return "auto"
	}
	return s.ColorProfile
}

// ConfigTemplate stores a user-defined connection template in the config file.
type ConfigTemplate struct {
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description,omitempty"`
	Protocol      string   `yaml:"protocol,omitempty"`
	Port          int      `yaml:"port,omitempty"`
	Username      string   `yaml:"username,omitempty"`
	Group         string   `yaml:"group,omitempty"`
	Tags          []string `yaml:"tags,omitempty"`
	ProxyJump     string   `yaml:"proxy_jump,omitempty"`
	RecordSession bool     `yaml:"record_session,omitempty"`
}

// Config is the root configuration structure.
type Config struct {
	Version        int              `yaml:"version"`
	Settings       Settings         `yaml:"settings"`
	Groups         []Group          `yaml:"groups,omitempty"`
	Connections    []Connection     `yaml:"connections,omitempty"`
	Templates      []ConfigTemplate `yaml:"templates,omitempty"`
	PasswordVerify string           `yaml:"password_verify,omitempty"` // encrypted sentinel for master password verification

	EncryptionKey []byte `yaml:"-"`
}

// SetKey stores the derived encryption key.
func (cfg *Config) SetKey(key []byte) {
	cfg.EncryptionKey = key
}

// HasEncryptedPasswords returns true if any connection has an ENC:-prefixed password.
func (cfg *Config) HasEncryptedPasswords() bool {
	for _, c := range cfg.Connections {
		if crypto.IsEncrypted(c.Password) || crypto.IsEncrypted(c.VNCPassword) {
			return true
		}
	}
	return false
}

// HasPlaintextPasswords returns true if any connection has a non-empty, non-encrypted password.
func (cfg *Config) HasPlaintextPasswords() bool {
	for _, c := range cfg.Connections {
		if c.Password != "" && !crypto.IsEncrypted(c.Password) {
			return true
		}
		if c.VNCPassword != "" && !crypto.IsEncrypted(c.VNCPassword) {
			return true
		}
	}
	return false
}

// HasAnyPasswords returns true if any connection has a password set.
func (cfg *Config) HasAnyPasswords() bool {
	for _, c := range cfg.Connections {
		if c.Password != "" || c.VNCPassword != "" {
			return true
		}
	}
	return false
}

// DecryptPasswords decrypts all passwords in place using the stored encryption key.
// Non-encrypted strings pass through unchanged (supports mixed state during migration).
func (cfg *Config) DecryptPasswords() error {
	for i := range cfg.Connections {
		if cfg.Connections[i].Password != "" {
			dec, err := crypto.Decrypt(cfg.Connections[i].Password, cfg.EncryptionKey)
			if err != nil {
				return err
			}
			cfg.Connections[i].Password = dec
		}
		if cfg.Connections[i].VNCPassword != "" {
			dec, err := crypto.Decrypt(cfg.Connections[i].VNCPassword, cfg.EncryptionKey)
			if err != nil {
				return err
			}
			cfg.Connections[i].VNCPassword = dec
		}
	}
	return nil
}

// ConfigPath returns the path to the config file, respecting XDG_CONFIG_HOME.
func ConfigPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "nexus", "config.yaml")
}

// Load reads the config from disk. If the file doesn't exist, returns a default
// config and true to indicate a fresh install (first launch).
func Load() (*Config, bool, error) {
	path := ConfigPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := defaultConfig()
			return cfg, true, nil
		}
		return nil, false, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, false, err
	}
	return &cfg, false, nil
}

// HasMasterPassword returns true if a master password has been configured.
func (cfg *Config) HasMasterPassword() bool {
	return cfg.PasswordVerify != ""
}

// VerifyMasterPassword checks a password against the stored verification sentinel.
func (cfg *Config) VerifyMasterPassword(password []byte) error {
	if cfg.PasswordVerify == "" {
		return fmt.Errorf("no master password configured")
	}
	if !crypto.IsEncrypted(cfg.PasswordVerify) {
		return fmt.Errorf("password_verify field is corrupted")
	}
	dec, err := crypto.Decrypt(cfg.PasswordVerify, password)
	if err != nil {
		return fmt.Errorf("wrong master password")
	}
	if dec != "nexus" {
		return fmt.Errorf("wrong master password")
	}
	return nil
}

// SetPasswordVerify encrypts a sentinel value with the given password and stores it.
func (cfg *Config) SetPasswordVerify(password []byte) error {
	enc, err := crypto.Encrypt("nexus", password)
	if err != nil {
		return fmt.Errorf("encrypting password verify: %w", err)
	}
	cfg.PasswordVerify = enc
	return nil
}

// SampleConfig returns a config populated with example connections for first launch.
func SampleConfig() *Config {
	return &Config{
		Version: 1,
		Settings: Settings{
			HealthCheckInterval: "30s",
			Theme:               "default",
		},
		Groups: []Group{
			{Name: "servers", Color: "#5f87af"},
			{Name: "network", Color: "#af875f"},
		},
		Connections: []Connection{
			{
				ID:       "example-ssh",
				Name:     "Example SSH Server",
				Protocol: ProtoSSH,
				Host:     "192.168.1.100",
				Port:     22,
				Username: "admin",
				Group:    "servers",
				Tags:     []string{"example", "linux"},
				Notes:    "Sample SSH connection — edit host/username to match your server",
			},
			{
				ID:       "example-rdp",
				Name:     "Example Windows PC",
				Protocol: ProtoRDP,
				Host:     "192.168.1.200",
				Username: "Administrator",
				Domain:   "WORKGROUP",
				Group:    "servers",
				Tags:     []string{"example", "windows"},
				RDPOptions: RDPOptions{
					Resolution:        "1920x1080",
					DynamicResolution: true,
				},
				Notes: "Sample RDP connection — edit host/credentials to match your machine",
			},
			{
				ID:       "example-vnc",
				Name:     "Example VNC Display",
				Protocol: ProtoVNC,
				Host:     "192.168.1.150",
				Port:     5900,
				Group:    "servers",
				Tags:     []string{"example"},
				Notes:    "Sample VNC connection — edit host and set vnc_password",
			},
			{
				ID:       "example-switch",
				Name:     "Example Network Switch",
				Protocol: ProtoTelnet,
				Host:     "192.168.1.1",
				Port:     23,
				Username: "admin",
				Group:    "network",
				Tags:     []string{"example", "infrastructure"},
				Notes:    "Sample Telnet connection — edit to match your network device",
			},
		},
	}
}

// Save writes the config to disk, creating directories as needed.
// If an encryption key is set, passwords are encrypted on the written copy
// while the in-memory config remains plaintext.
func Save(cfg *Config) error {
	path := ConfigPath()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	// Deep copy connections so we can encrypt without mutating in-memory state.
	saveCfg := *cfg
	saveCfg.Connections = make([]Connection, len(cfg.Connections))
	copy(saveCfg.Connections, cfg.Connections)

	if len(cfg.EncryptionKey) > 0 {
		// Derive the AES key once for this save operation (scrypt is slow).
		salt, err := crypto.GenerateSalt()
		if err != nil {
			return err
		}
		derivedKey, err := crypto.DeriveKey(string(cfg.EncryptionKey), salt)
		if err != nil {
			return err
		}

		for i := range saveCfg.Connections {
			if saveCfg.Connections[i].Password != "" {
				enc, err := crypto.EncryptWithDerivedKey(saveCfg.Connections[i].Password, derivedKey, salt)
				if err != nil {
					return err
				}
				saveCfg.Connections[i].Password = enc
			}
			if saveCfg.Connections[i].VNCPassword != "" {
				enc, err := crypto.EncryptWithDerivedKey(saveCfg.Connections[i].VNCPassword, derivedKey, salt)
				if err != nil {
					return err
				}
				saveCfg.Connections[i].VNCPassword = enc
			}
		}
	}

	data, err := yaml.Marshal(&saveCfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o600)
}

// AddConnection adds a connection and saves.
func (cfg *Config) AddConnection(conn Connection) error {
	cfg.Connections = append(cfg.Connections, conn)
	return Save(cfg)
}

// InsertConnectionAt inserts a connection at the given index and saves.
// If the index is out of range, the connection is appended.
func (cfg *Config) InsertConnectionAt(conn Connection, index int) error {
	if index < 0 || index >= len(cfg.Connections) {
		cfg.Connections = append(cfg.Connections, conn)
	} else {
		cfg.Connections = append(cfg.Connections[:index+1], cfg.Connections[index:]...)
		cfg.Connections[index] = conn
	}
	return Save(cfg)
}

// UpdateConnection replaces a connection by ID and saves.
func (cfg *Config) UpdateConnection(conn Connection) error {
	for i, c := range cfg.Connections {
		if c.ID == conn.ID {
			cfg.Connections[i] = conn
			return Save(cfg)
		}
	}
	return nil
}

// DeleteConnection removes a connection by ID and saves.
func (cfg *Config) DeleteConnection(id string) error {
	cfg.DeleteConnectionNoSave(id)
	return Save(cfg)
}

// DeleteConnectionNoSave removes a connection by ID without saving.
// Use for batch operations where a single Save is called after all mutations.
func (cfg *Config) DeleteConnectionNoSave(id string) {
	for i, c := range cfg.Connections {
		if c.ID == id {
			cfg.Connections = append(cfg.Connections[:i], cfg.Connections[i+1:]...)
			return
		}
	}
}

// FindConnection returns a connection by ID.
func (cfg *Config) FindConnection(id string) *Connection {
	for i, c := range cfg.Connections {
		if c.ID == id {
			return &cfg.Connections[i]
		}
	}
	return nil
}

// GroupNames returns the list of group names.
func (cfg *Config) GroupNames() []string {
	names := make([]string, len(cfg.Groups))
	for i, g := range cfg.Groups {
		names[i] = g.Name
	}
	return names
}

// GroupColor returns the color for a group, or empty string.
func (cfg *Config) GroupColor(name string) string {
	for _, g := range cfg.Groups {
		if g.Name == name {
			return g.Color
		}
	}
	return ""
}

// FindGroup returns a pointer to the group with the given name, or nil if not found.
func (cfg *Config) FindGroup(name string) *Group {
	for i, g := range cfg.Groups {
		if g.Name == name {
			return &cfg.Groups[i]
		}
	}
	return nil
}

// HasConnectionsInGroup returns true if any connection belongs to the given group.
func (cfg *Config) HasConnectionsInGroup(name string) bool {
	for _, c := range cfg.Connections {
		if c.Group == name {
			return true
		}
	}
	return false
}

// AddGroup creates a new named group. Returns an error if the name is empty or already exists.
func (cfg *Config) AddGroup(name string) error {
	if name == "" {
		return errors.New("group name cannot be empty")
	}
	if cfg.FindGroup(name) != nil {
		return fmt.Errorf("group %q already exists", name)
	}
	cfg.Groups = append(cfg.Groups, Group{Name: name})
	return Save(cfg)
}

// DeleteGroup removes a group by name. Returns an error if not found or has connections.
func (cfg *Config) DeleteGroup(name string) error {
	if cfg.FindGroup(name) == nil {
		return fmt.Errorf("group %q not found", name)
	}
	if cfg.HasConnectionsInGroup(name) {
		return fmt.Errorf("group %q has connections assigned to it", name)
	}
	for i, g := range cfg.Groups {
		if g.Name == name {
			cfg.Groups = append(cfg.Groups[:i], cfg.Groups[i+1:]...)
			return Save(cfg)
		}
	}
	return nil
}

// MoveConnection moves a connection to a different group. Auto-creates the target group if needed.
func (cfg *Config) MoveConnection(connID string, targetGroup string) error {
	if err := cfg.MoveConnectionNoSave(connID, targetGroup); err != nil {
		return err
	}
	return Save(cfg)
}

// MoveConnectionNoSave moves a connection to a different group without saving.
// Use for batch operations where a single Save is called after all mutations.
func (cfg *Config) MoveConnectionNoSave(connID string, targetGroup string) error {
	conn := cfg.FindConnection(connID)
	if conn == nil {
		return fmt.Errorf("connection %q not found", connID)
	}
	if targetGroup != "" && cfg.FindGroup(targetGroup) == nil {
		cfg.Groups = append(cfg.Groups, Group{Name: targetGroup})
	}
	conn.Group = targetGroup
	return nil
}

func defaultConfig() *Config {
	return &Config{
		Version: 1,
		Settings: Settings{
			HealthCheckInterval: "30s",
			Theme:               "default",
		},
	}
}
