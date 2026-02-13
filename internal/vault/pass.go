package vault

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const passPrefix = "nexus/"

// PassVault implements Vault using the pass (password-store) CLI.
type PassVault struct{}

// PassAvailable returns true if the pass binary is found in PATH.
func PassAvailable() bool {
	_, err := exec.LookPath("pass")
	return err == nil
}

// OpenPass creates a PassVault after verifying that pass is available.
func OpenPass() (*PassVault, error) {
	if !PassAvailable() {
		return nil, fmt.Errorf("%w: 'pass' not found in PATH; install password-store or use vault: internal", ErrBackendUnavailable)
	}
	return &PassVault{}, nil
}

// Get retrieves a credential via `pass show nexus/<id>`.
func (p *PassVault) Get(id string) (string, error) {
	out, err := exec.Command("pass", "show", passPrefix+id).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "not in the password store") ||
			strings.Contains(string(out), "is not a regular file") {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("pass show: %w: %s", err, strings.TrimSpace(string(out)))
	}
	// pass may include trailing newline; return only the first line (the password).
	lines := strings.SplitN(strings.TrimRight(string(out), "\n"), "\n", 2)
	return lines[0], nil
}

// Set stores a credential via `pass insert -f nexus/<id>`.
func (p *PassVault) Set(id string, credential string) error {
	cmd := exec.Command("pass", "insert", "-f", "-m", passPrefix+id)
	cmd.Stdin = strings.NewReader(credential + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pass insert: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Delete removes a credential via `pass rm -f nexus/<id>`.
func (p *PassVault) Delete(id string) error {
	out, err := exec.Command("pass", "rm", "-f", passPrefix+id).CombinedOutput()
	if err != nil {
		return fmt.Errorf("pass rm: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// List returns entries by parsing `pass ls nexus/`.
func (p *PassVault) List() ([]CredentialEntry, error) {
	out, err := exec.Command("pass", "ls", passPrefix).CombinedOutput()
	if err != nil {
		// If the prefix directory doesn't exist, treat as empty.
		if strings.Contains(string(out), "not in the password store") ||
			strings.Contains(string(out), "Error") {
			return nil, nil
		}
		return nil, fmt.Errorf("pass ls: %w: %s", err, strings.TrimSpace(string(out)))
	}

	var entries []CredentialEntry
	now := time.Now().UTC()
	for _, line := range strings.Split(string(out), "\n") {
		// pass ls output uses tree characters; strip them.
		name := strings.TrimSpace(line)
		// Remove tree drawing characters
		for _, ch := range []string{"├── ", "└── ", "│   ", "    "} {
			name = strings.TrimPrefix(name, ch)
		}
		// Also strip any remaining Unicode box-drawing
		name = strings.Map(func(r rune) rune {
			if r == '├' || r == '└' || r == '│' || r == '─' || r == ' ' {
				return -1
			}
			return r
		}, name)
		name = strings.TrimSpace(name)
		if name == "" || strings.HasPrefix(name, "nexus") {
			continue
		}
		entries = append(entries, CredentialEntry{
			ID:        name,
			Name:      name,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	return entries, nil
}

// Close is a no-op for pass.
func (p *PassVault) Close() error {
	return nil
}
