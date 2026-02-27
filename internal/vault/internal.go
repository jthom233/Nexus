package vault

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
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
	Version int             `json:"version"`
	Entries []internalEntry `json:"entries"`
}

const (
	vaultVersion  = byte(0x01)
	vaultSaltLen  = 16
	legacySalt    = "nexus-vault-salt"
)

// InternalVault implements Vault using AES-256-GCM encryption on a local JSON file.
type InternalVault struct {
	mu             sync.Mutex
	path           string
	masterPassword string
	salt           []byte
	store          internalStore
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

	v := &InternalVault{
		path:           path,
		masterPassword: masterPassword,
		store: internalStore{
			Version: 1,
		},
	}

	if err := v.load(); err != nil {
		return nil, err
	}
	return v, nil
}

// deriveKey derives the AES key from the vault's master password and salt.
func (v *InternalVault) deriveKey() ([]byte, error) {
	key, err := crypto.DeriveKey(v.masterPassword, v.salt)
	if err != nil {
		return nil, fmt.Errorf("deriving vault key: %w", err)
	}
	return key, nil
}

// load reads and decrypts the vault file. If the file does not exist, the vault
// starts empty (no error). Handles both new format (version byte + salt prefix)
// and legacy format (raw "ENC:..." string with static salt).
func (v *InternalVault) load() error {
	data, err := os.ReadFile(v.path)
	if err != nil {
		if os.IsNotExist(err) {
			// Fresh vault: generate a new random salt now.
			v.salt, err = generateVaultSalt()
			if err != nil {
				return fmt.Errorf("generating vault salt: %w", err)
			}
			return nil
		}
		return fmt.Errorf("reading vault file: %w", err)
	}

	var encryptedData string
	var migrating bool

	if len(data) > 0 && data[0] == vaultVersion {
		// New format: [0x01][16-byte salt][encrypted data...]
		if len(data) < 1+vaultSaltLen {
			return fmt.Errorf("vault file too short for new format")
		}
		v.salt = make([]byte, vaultSaltLen)
		copy(v.salt, data[1:1+vaultSaltLen])
		encryptedData = string(data[1+vaultSaltLen:])
	} else if len(data) > 0 && data[0] == 'E' {
		// Legacy format: raw "ENC:..." string with static salt.
		log.Printf("vault: legacy format detected, will migrate on next save")
		v.salt = []byte(legacySalt)
		encryptedData = string(data)
		migrating = true
	} else {
		return fmt.Errorf("unrecognised vault file format")
	}

	key, err := v.deriveKey()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWrongPassword, err)
	}

	plaintext, err := crypto.Decrypt(encryptedData, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWrongPassword, err)
	}

	var store internalStore
	if err := json.Unmarshal([]byte(plaintext), &store); err != nil {
		return fmt.Errorf("corrupted vault data: %w", err)
	}
	v.store = store

	// If legacy format was detected, assign a new random salt so the next
	// save will write the new format automatically.
	if migrating {
		newSalt, err := generateVaultSalt()
		if err != nil {
			return fmt.Errorf("generating migration salt: %w", err)
		}
		v.salt = newSalt
	}

	return nil
}

// save encrypts and writes the vault to disk using the new format:
// [0x01][16-byte salt][encrypted data].
func (v *InternalVault) save() error {
	key, err := v.deriveKey()
	if err != nil {
		return fmt.Errorf("deriving key for save: %w", err)
	}

	data, err := json.Marshal(&v.store)
	if err != nil {
		return fmt.Errorf("marshaling vault: %w", err)
	}

	encrypted, err := crypto.Encrypt(string(data), key)
	if err != nil {
		return fmt.Errorf("encrypting vault: %w", err)
	}

	// Compose new-format file: [version][salt][encrypted data]
	fileData := make([]byte, 0, 1+vaultSaltLen+len(encrypted))
	fileData = append(fileData, vaultVersion)
	fileData = append(fileData, v.salt...)
	fileData = append(fileData, []byte(encrypted)...)

	dir := filepath.Dir(v.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating vault directory: %w", err)
	}
	return os.WriteFile(v.path, fileData, 0o600)
}

// generateVaultSalt returns a new random 16-byte salt.
func generateVaultSalt() ([]byte, error) {
	salt := make([]byte, vaultSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}
	return salt, nil
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
