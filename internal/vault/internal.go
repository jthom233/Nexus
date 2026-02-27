package vault

import (
	"bytes"
	cryptorand "crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dr4zz/nexus/internal/crypto"
)

// File format constants for the versioned vault format.
//
// Layout: [1 byte version][16 bytes salt][encrypted payload]
//
// The version byte distinguishes the new format from legacy files, which
// start with "ENC:" (first byte 0x45 'E'), making 0x01 unambiguous.
const (
	vaultFormatVersion = byte(0x01)
	vaultSaltLen       = 16
	vaultHeaderLen     = 1 + vaultSaltLen // version byte + salt

	// legacySalt is the static salt used in vault files written before the
	// random-salt format was introduced.
	legacySalt = "nexus-vault-salt"
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
	mu             sync.Mutex
	path           string
	masterPassword string
	// salt is the per-vault random KDF salt stored in the file header.
	// nil means no salt has been assigned yet (fresh vault or legacy upgrade);
	// a new random salt will be generated on the first save.
	salt []byte
	// key is the AES key derived from masterPassword + salt.
	// nil when salt is nil; re-derived whenever salt changes.
	key   []byte
	store internalStore
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

// deriveKey derives the AES key from masterPassword and the given salt.
func (v *InternalVault) deriveKey(salt []byte) ([]byte, error) {
	key, err := crypto.DeriveKey(v.masterPassword, salt)
	if err != nil {
		return nil, fmt.Errorf("deriving vault key: %w", err)
	}
	return key, nil
}

// load reads and decrypts the vault file. If the file does not exist the vault
// starts empty (no error). Handles both the current versioned format and the
// legacy static-salt format transparently.
func (v *InternalVault) load() error {
	data, err := os.ReadFile(v.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // fresh vault; salt generated on first save
		}
		return fmt.Errorf("reading vault file: %w", err)
	}

	// Detect format by the first byte.
	// New format:    data[0] == 0x01 (vaultFormatVersion)
	// Legacy format: data[0] == 'E'  (start of "ENC:...")
	if len(data) >= vaultHeaderLen && data[0] == vaultFormatVersion {
		return v.loadNewFormat(data)
	}
	return v.loadLegacyFormat(data)
}

// loadNewFormat reads a vault file in the current versioned format:
//
//	[1 byte version][16 bytes salt][encrypted payload]
func (v *InternalVault) loadNewFormat(data []byte) error {
	if len(data) < vaultHeaderLen {
		return fmt.Errorf("vault file too short for versioned header")
	}
	salt := make([]byte, vaultSaltLen)
	copy(salt, data[1:vaultHeaderLen])
	payload := data[vaultHeaderLen:]

	key, err := v.deriveKey(salt)
	if err != nil {
		return err
	}

	plaintext, err := crypto.Decrypt(string(payload), key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWrongPassword, err)
	}

	var store internalStore
	if err := json.Unmarshal([]byte(plaintext), &store); err != nil {
		return fmt.Errorf("corrupted vault data: %w", err)
	}

	v.salt = salt
	v.key = key
	v.store = store
	return nil
}

// loadLegacyFormat reads a vault file written before the random-salt format.
// It falls back to the static salt, logs a warning, and leaves v.salt nil so
// that the next save will re-encrypt with a freshly generated random salt.
func (v *InternalVault) loadLegacyFormat(data []byte) error {
	log.Printf("vault: opening legacy-format vault at %s; will re-encrypt with random salt on next save", v.path)

	key, err := v.deriveKey([]byte(legacySalt))
	if err != nil {
		return err
	}

	plaintext, err := crypto.Decrypt(string(data), key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWrongPassword, err)
	}

	var store internalStore
	if err := json.Unmarshal([]byte(plaintext), &store); err != nil {
		return fmt.Errorf("corrupted vault data: %w", err)
	}

	// salt/key remain nil; save() will generate a new random salt.
	v.store = store
	return nil
}

// save encrypts and writes the vault to disk in the current versioned format.
// On the first save (fresh vault or legacy upgrade), a new random salt is generated.
func (v *InternalVault) save() error {
	if v.salt == nil {
		// Fresh vault or legacy upgrade: generate a random salt and derive a new key.
		salt := make([]byte, vaultSaltLen)
		if _, err := cryptorand.Read(salt); err != nil {
			return fmt.Errorf("generating vault salt: %w", err)
		}
		key, err := v.deriveKey(salt)
		if err != nil {
			return err
		}
		v.salt = salt
		v.key = key
	}

	data, err := json.Marshal(&v.store)
	if err != nil {
		return fmt.Errorf("marshaling vault: %w", err)
	}

	encrypted, err := crypto.Encrypt(string(data), v.key)
	if err != nil {
		return fmt.Errorf("encrypting vault: %w", err)
	}

	// Build file: [version byte][salt][encrypted payload]
	var buf bytes.Buffer
	buf.WriteByte(vaultFormatVersion)
	buf.Write(v.salt)
	buf.WriteString(encrypted)

	dir := filepath.Dir(v.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating vault directory: %w", err)
	}
	return os.WriteFile(v.path, buf.Bytes(), 0o600)
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
