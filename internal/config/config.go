package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/dr4zz/nexus/internal/crypto"
	"gopkg.in/yaml.v3"
)

// Settings holds application-level settings.
type Settings struct {
	HealthCheckInterval string `yaml:"health_check_interval,omitempty"`
	Theme               string `yaml:"theme,omitempty"`
	Vault               string `yaml:"vault,omitempty"` // "internal" (default), "pass", or "keyring"
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
	Version     int          `yaml:"version"`
	Settings    Settings     `yaml:"settings"`
	Groups      []Group      `yaml:"groups,omitempty"`
	Connections []Connection     `yaml:"connections,omitempty"`
	Templates   []ConfigTemplate `yaml:"templates,omitempty"`

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

// Load reads the config from disk. If the file doesn't exist, returns a default config.
func Load() (*Config, error) {
	path := ConfigPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := defaultConfig()
			return cfg, nil
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
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
		for i := range saveCfg.Connections {
			if saveCfg.Connections[i].Password != "" {
				enc, err := crypto.Encrypt(saveCfg.Connections[i].Password, cfg.EncryptionKey)
				if err != nil {
					return err
				}
				saveCfg.Connections[i].Password = enc
			}
			if saveCfg.Connections[i].VNCPassword != "" {
				enc, err := crypto.Encrypt(saveCfg.Connections[i].VNCPassword, cfg.EncryptionKey)
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
	for i, c := range cfg.Connections {
		if c.ID == id {
			cfg.Connections = append(cfg.Connections[:i], cfg.Connections[i+1:]...)
			return Save(cfg)
		}
	}
	return nil
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

func defaultConfig() *Config {
	return &Config{
		Version: 1,
		Settings: Settings{
			HealthCheckInterval: "30s",
			Theme:               "default",
		},
	}
}
