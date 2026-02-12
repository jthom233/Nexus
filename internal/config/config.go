package config

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Settings holds application-level settings.
type Settings struct {
	HealthCheckInterval string `yaml:"health_check_interval,omitempty"`
	Theme               string `yaml:"theme,omitempty"`
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

// Config is the root configuration structure.
type Config struct {
	Version     int          `yaml:"version"`
	Settings    Settings     `yaml:"settings"`
	Groups      []Group      `yaml:"groups,omitempty"`
	Connections []Connection `yaml:"connections,omitempty"`
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
func Save(cfg *Config) error {
	path := ConfigPath()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

// AddConnection adds a connection and saves.
func (cfg *Config) AddConnection(conn Connection) error {
	cfg.Connections = append(cfg.Connections, conn)
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
