package vault

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const keyringService = "nexus"

// KeyringVault implements Vault using the system keyring.
// On Linux it uses secret-tool (libsecret), on macOS it uses the security CLI.
type KeyringVault struct {
	platform string // "linux" or "darwin"
}

// KeyringAvailable returns true if the system keyring CLI tool is available.
func KeyringAvailable() bool {
	switch runtime.GOOS {
	case "linux":
		_, err := exec.LookPath("secret-tool")
		return err == nil
	case "darwin":
		_, err := exec.LookPath("security")
		return err == nil
	default:
		return false
	}
}

// OpenKeyring creates a KeyringVault after verifying availability.
func OpenKeyring() (*KeyringVault, error) {
	if !KeyringAvailable() {
		tool := "secret-tool"
		if runtime.GOOS == "darwin" {
			tool = "security"
		}
		return nil, fmt.Errorf(
			"%w: '%s' not found; install it or use vault: internal",
			ErrBackendUnavailable, tool,
		)
	}
	return &KeyringVault{platform: runtime.GOOS}, nil
}

// Get retrieves a credential from the system keyring.
func (k *KeyringVault) Get(id string) (string, error) {
	switch k.platform {
	case "linux":
		return k.linuxGet(id)
	case "darwin":
		return k.darwinGet(id)
	default:
		return "", ErrBackendUnavailable
	}
}

// Set stores a credential in the system keyring.
func (k *KeyringVault) Set(id string, credential string) error {
	switch k.platform {
	case "linux":
		return k.linuxSet(id, credential)
	case "darwin":
		return k.darwinSet(id, credential)
	default:
		return ErrBackendUnavailable
	}
}

// Delete removes a credential from the system keyring.
func (k *KeyringVault) Delete(id string) error {
	switch k.platform {
	case "linux":
		return k.linuxDelete(id)
	case "darwin":
		return k.darwinDelete(id)
	default:
		return ErrBackendUnavailable
	}
}

// List returns entries. Note: system keyrings do not provide easy enumeration,
// so this is a best-effort implementation.
func (k *KeyringVault) List() ([]CredentialEntry, error) {
	switch k.platform {
	case "linux":
		return k.linuxList()
	default:
		// macOS security CLI does not support easy enumeration of generic passwords.
		return nil, nil
	}
}

// Close is a no-op for keyring.
func (k *KeyringVault) Close() error {
	return nil
}

// --- Linux (secret-tool / libsecret) ---

func (k *KeyringVault) linuxGet(id string) (string, error) {
	out, err := exec.Command("secret-tool", "lookup", "service", keyringService, "id", id).CombinedOutput()
	if err != nil {
		if len(out) == 0 {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("secret-tool lookup: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func (k *KeyringVault) linuxSet(id string, credential string) error {
	cmd := exec.Command("secret-tool", "store", "--label", keyringService+"/"+id,
		"service", keyringService, "id", id)
	cmd.Stdin = strings.NewReader(credential)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("secret-tool store: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (k *KeyringVault) linuxDelete(id string) error {
	out, err := exec.Command("secret-tool", "clear", "service", keyringService, "id", id).CombinedOutput()
	if err != nil {
		return fmt.Errorf("secret-tool clear: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (k *KeyringVault) linuxList() ([]CredentialEntry, error) {
	// secret-tool search returns all matching entries.
	out, err := exec.Command("secret-tool", "search", "service", keyringService).CombinedOutput()
	if err != nil {
		// No results is not an error for our purposes.
		if strings.TrimSpace(string(out)) == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("secret-tool search: %w: %s", err, strings.TrimSpace(string(out)))
	}

	var entries []CredentialEntry
	now := time.Now().UTC()
	// Parse secret-tool search output: lines contain "attribute.id = <value>"
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "attribute.id = ") {
			id := strings.TrimPrefix(line, "attribute.id = ")
			id = strings.TrimSpace(id)
			if id != "" {
				entries = append(entries, CredentialEntry{
					ID:        id,
					Name:      id,
					CreatedAt: now,
					UpdatedAt: now,
				})
			}
		}
	}
	return entries, nil
}

// --- macOS (security CLI) ---

func (k *KeyringVault) darwinGet(id string) (string, error) {
	out, err := exec.Command("security", "find-generic-password",
		"-s", keyringService, "-a", id, "-w").CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "could not be found") ||
			strings.Contains(string(out), "SecKeychainSearchCopyNext") {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("security find-generic-password: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func (k *KeyringVault) darwinSet(id string, credential string) error {
	// Delete first to avoid "duplicate item" errors on update.
	_ = k.darwinDelete(id)

	out, err := exec.Command("security", "add-generic-password",
		"-s", keyringService, "-a", id, "-w", credential).CombinedOutput()
	if err != nil {
		return fmt.Errorf("security add-generic-password: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (k *KeyringVault) darwinDelete(id string) error {
	out, err := exec.Command("security", "delete-generic-password",
		"-s", keyringService, "-a", id).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "could not be found") ||
			strings.Contains(string(out), "SecKeychainSearchCopyNext") {
			return ErrNotFound
		}
		return fmt.Errorf("security delete-generic-password: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
