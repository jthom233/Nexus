// Package vault provides credential management for Nexus connections.
//
// Three backends are supported:
//   - InternalVault: AES-256-GCM encrypted JSON file (default)
//   - PassVault: integration with pass (password-store)
//   - KeyringVault: system keyring via secret-tool / security CLI
package vault

import (
	"errors"
	"fmt"
	"time"
)

// Common errors.
var (
	ErrNotFound      = errors.New("credential not found")
	ErrLocked        = errors.New("vault is locked")
	ErrWrongPassword = errors.New("wrong master password")
	ErrBackendUnavailable = errors.New("vault backend unavailable")
)

// CredentialEntry holds metadata about a stored credential.
type CredentialEntry struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Vault is the interface for credential storage backends.
type Vault interface {
	// Get retrieves a credential by ID.
	Get(id string) (string, error)

	// Set stores a credential, creating or updating as needed.
	Set(id string, credential string) error

	// Delete removes a credential by ID.
	Delete(id string) error

	// List returns metadata for all stored credentials.
	List() ([]CredentialEntry, error)

	// Close persists state and releases resources.
	Close() error
}

// Backend identifies a vault implementation.
type Backend string

const (
	BackendInternal Backend = "internal"
	BackendPass     Backend = "pass"
	BackendKeyring  Backend = "keyring"
)

// Open creates and unlocks a vault for the given backend.
// For the internal backend, masterPassword is required.
// For pass and keyring backends, masterPassword is ignored.
func Open(backend Backend, masterPassword string) (Vault, error) {
	switch backend {
	case BackendInternal, "":
		return OpenInternal(masterPassword)
	case BackendPass:
		return OpenPass()
	case BackendKeyring:
		return OpenKeyring()
	default:
		return nil, fmt.Errorf("unknown vault backend: %q", backend)
	}
}
