package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dr4zz/nexus/internal/crypto"
)

// internalEntry is a single credential stored in the encrypted vault file.
type internalEntry struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Credential string    `json:"credential"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// internalStore is the top-level structure serialized to JSON before encryption.
type internalStore struct {
	Version int              `json:"version"`
	Entries []internalEntry  `json:"entries"`
}

// InternalVault implements Vault using AES-256-GCM encryption on a local JSON file.
type InternalVault struct {
	mu       sync.Mutex
	path     string
	key      []byte
	store    internalStore
}

// VaultPath returns the default encrypted vault file path.
func VaultPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "nexus", "vault.enc")
}

// OpenInternal opens or creates an internal vault protected by masterPassword.
func OpenInternal(masterPassword string) (*InternalVault, error) {
	return OpenInternalAt(VaultPath(), masterPassword)
}

// OpenInternalAt opens or creates an internal vault at a specific path.
func OpenInternalAt(path string, masterPassword string) (*InternalVault, error) {
	if masterPassword == "" {
		return nil, fmt.Errorf("%w: master password is required for internal vault", ErrLocked)
	}

	key := crypto.DeriveKey(masterPassword, []byte("nexus-vault-salt"))

	v := &InternalVault{
		path: path,
		key:  key,
		store: internalStore{
			Version: 1,
		},
	}

	if err := v.load(); err != nil {
		return nil, err
	}
	return v, nil
}

// load reads and decrypts the vault file. If the file does not exist, the vault
// starts empty (no error).
func (v *InternalVault) load() error {
	data, err := os.ReadFile(v.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // fresh vault
		}
		return fmt.Errorf("reading vault file: %w", err)
	}

	plaintext, err := crypto.Decrypt(string(data), v.key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWrongPassword, err)
	}

	var store internalStore
	if err := json.Unmarshal([]byte(plaintext), &store); err != nil {
		return fmt.Errorf("corrupted vault data: %w", err)
	}
	v.store = store
	return nil
}

// save encrypts and writes the vault to disk.
func (v *InternalVault) save() error {
	data, err := json.Marshal(&v.store)
	if err != nil {
		return fmt.Errorf("marshaling vault: %w", err)
	}

	encrypted, err := crypto.Encrypt(string(data), v.key)
	if err != nil {
		return fmt.Errorf("encrypting vault: %w", err)
	}

	dir := filepath.Dir(v.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating vault directory: %w", err)
	}
	return os.WriteFile(v.path, []byte(encrypted), 0o600)
}

// findIndex returns the index of an entry by ID, or -1.
func (v *InternalVault) findIndex(id string) int {
	for i, e := range v.store.Entries {
		if e.ID == id {
			return i
		}
	}
	return -1
}

// Get retrieves a credential by ID.
func (v *InternalVault) Get(id string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	idx := v.findIndex(id)
	if idx < 0 {
		return "", ErrNotFound
	}
	return v.store.Entries[idx].Credential, nil
}

// Set stores or updates a credential.
func (v *InternalVault) Set(id string, credential string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := time.Now().UTC()
	idx := v.findIndex(id)
	if idx >= 0 {
		v.store.Entries[idx].Credential = credential
		v.store.Entries[idx].UpdatedAt = now
	} else {
		v.store.Entries = append(v.store.Entries, internalEntry{
			ID:         id,
			Name:       id,
			Credential: credential,
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	return v.save()
}

// Delete removes a credential by ID.
func (v *InternalVault) Delete(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	idx := v.findIndex(id)
	if idx < 0 {
		return ErrNotFound
	}
	v.store.Entries = append(v.store.Entries[:idx], v.store.Entries[idx+1:]...)
	return v.save()
}

// List returns metadata for all stored credentials.
func (v *InternalVault) List() ([]CredentialEntry, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	entries := make([]CredentialEntry, len(v.store.Entries))
	for i, e := range v.store.Entries {
		entries[i] = CredentialEntry{
			ID:        e.ID,
			Name:      e.Name,
			CreatedAt: e.CreatedAt,
			UpdatedAt: e.UpdatedAt,
		}
	}
	return entries, nil
}

// Close persists the vault to disk.
func (v *InternalVault) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.save()
}
