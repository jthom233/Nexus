package store

import (
	"os"
	"strings"

	"github.com/dr4zz/nexus/internal/config"
	"gopkg.in/yaml.v3"
)

// YAMLStore implements Store by delegating to an in-memory config.Config
// backed by a YAML file on disk.
type YAMLStore struct {
	path string
	cfg  *config.Config
}

// NewYAMLStore opens (or creates) a YAML-backed store at the given path.
func NewYAMLStore(path string) (*YAMLStore, error) {
	s := &YAMLStore{path: path}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads the YAML file from disk into s.cfg.
func (s *YAMLStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.cfg = &config.Config{Version: 1}
			return nil
		}
		return err
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	s.cfg = &cfg
	return nil
}

// save writes the current config to disk.
func (s *YAMLStore) save() error {
	data, err := yaml.Marshal(s.cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

// Config returns the underlying Config (useful for migrations).
func (s *YAMLStore) Config() *config.Config {
	return s.cfg
}

func (s *YAMLStore) ListConnections() ([]config.Connection, error) {
	out := make([]config.Connection, len(s.cfg.Connections))
	copy(out, s.cfg.Connections)
	return out, nil
}

func (s *YAMLStore) GetConnection(id string) (config.Connection, error) {
	for _, c := range s.cfg.Connections {
		if c.ID == id {
			return c, nil
		}
	}
	return config.Connection{}, &ErrNotFound{ID: id}
}

func (s *YAMLStore) AddConnection(conn config.Connection) error {
	s.cfg.Connections = append(s.cfg.Connections, conn)
	return s.save()
}

func (s *YAMLStore) UpdateConnection(conn config.Connection) error {
	for i, c := range s.cfg.Connections {
		if c.ID == conn.ID {
			s.cfg.Connections[i] = conn
			return s.save()
		}
	}
	return &ErrNotFound{ID: conn.ID}
}

func (s *YAMLStore) DeleteConnection(id string) error {
	for i, c := range s.cfg.Connections {
		if c.ID == id {
			s.cfg.Connections = append(s.cfg.Connections[:i], s.cfg.Connections[i+1:]...)
			return s.save()
		}
	}
	return &ErrNotFound{ID: id}
}

func (s *YAMLStore) InsertConnectionAt(conn config.Connection, index int) error {
	if index < 0 || index >= len(s.cfg.Connections) {
		s.cfg.Connections = append(s.cfg.Connections, conn)
	} else {
		s.cfg.Connections = append(s.cfg.Connections[:index+1], s.cfg.Connections[index:]...)
		s.cfg.Connections[index] = conn
	}
	return s.save()
}

func (s *YAMLStore) SearchConnections(query string) ([]config.Connection, error) {
	q := strings.ToLower(query)
	var results []config.Connection
	for _, c := range s.cfg.Connections {
		if matchConnection(c, q) {
			results = append(results, c)
		}
	}
	return results, nil
}

func (s *YAMLStore) Close() error {
	return nil
}

// matchConnection returns true if any searchable field contains the query.
func matchConnection(c config.Connection, q string) bool {
	if strings.Contains(strings.ToLower(c.Name), q) {
		return true
	}
	if strings.Contains(strings.ToLower(c.Host), q) {
		return true
	}
	if strings.Contains(strings.ToLower(c.Group), q) {
		return true
	}
	for _, tag := range c.Tags {
		if strings.Contains(strings.ToLower(tag), q) {
			return true
		}
	}
	return false
}
