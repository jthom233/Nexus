package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ImportSSHConfig parses ~/.ssh/config and returns connections.
func ImportSSHConfig() ([]Connection, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		return nil, fmt.Errorf("cannot open ssh config: %w", err)
	}
	defer f.Close()

	var connections []Connection
	var current *Connection

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, val, ok := parseSSHLine(line)
		if !ok {
			continue
		}

		switch strings.ToLower(key) {
		case "host":
			// Skip wildcard entries
			if strings.Contains(val, "*") || strings.Contains(val, "?") {
				current = nil
				continue
			}
			if current != nil {
				connections = append(connections, *current)
			}
			current = &Connection{
				ID:       sanitizeID(val),
				Name:     val,
				Protocol: ProtoSSH,
				Port:     22,
			}
		case "hostname":
			if current != nil {
				current.Host = val
			}
		case "user":
			if current != nil {
				current.Username = val
			}
		case "port":
			if current != nil {
				if p, err := strconv.Atoi(val); err == nil {
					current.Port = p
				}
			}
		case "identityfile":
			if current != nil {
				current.IdentityFile = expandTilde(val)
			}
		}
	}

	if current != nil && current.Host != "" {
		connections = append(connections, *current)
	}

	return connections, scanner.Err()
}

func parseSSHLine(line string) (key, value string, ok bool) {
	// Handle both "Key Value" and "Key=Value" formats
	if idx := strings.IndexByte(line, '='); idx > 0 {
		return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
	}
	parts := strings.SplitN(line, " ", 2)
	if len(parts) != 2 {
		parts = strings.SplitN(line, "\t", 2)
	}
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func sanitizeID(name string) string {
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, ".", "-")
	return "ssh-" + id
}

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
