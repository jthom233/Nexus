package vault

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/dr4zz/nexus/internal/crypto"
)

func tempVaultPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "vault.enc")
}

func TestInternalVault_SetAndGet(t *testing.T) {
	path := tempVaultPath(t)
	v, err := OpenInternalAt(path, "test-master-password")
	if err != nil {
		t.Fatalf("OpenInternalAt: %v", err)
	}
	defer v.Close()

	// Set a credential.
	if err := v.Set("conn-1", "s3cret"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Get it back.
	cred, err := v.Get("conn-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cred != "s3cret" {
		t.Fatalf("expected 's3cret', got %q", cred)
	}
}

func TestInternalVault_Update(t *testing.T) {
	path := tempVaultPath(t)
	v, err := OpenInternalAt(path, "pw")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v.Close()

	if err := v.Set("id1", "old"); err != nil {
		t.Fatal(err)
	}
	if err := v.Set("id1", "new"); err != nil {
		t.Fatal(err)
	}
	cred, err := v.Get("id1")
	if err != nil {
		t.Fatal(err)
	}
	if cred != "new" {
		t.Fatalf("expected 'new', got %q", cred)
	}

	// Should still be one entry, not two.
	entries, err := v.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after update, got %d", len(entries))
	}
}

func TestInternalVault_Delete(t *testing.T) {
	path := tempVaultPath(t)
	v, err := OpenInternalAt(path, "pw")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v.Close()

	if err := v.Set("del-me", "pass"); err != nil {
		t.Fatal(err)
	}
	if err := v.Delete("del-me"); err != nil {
		t.Fatal(err)
	}

	_, err = v.Get("del-me")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestInternalVault_DeleteNotFound(t *testing.T) {
	path := tempVaultPath(t)
	v, err := OpenInternalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	if err := v.Delete("nonexistent"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestInternalVault_List(t *testing.T) {
	path := tempVaultPath(t)
	v, err := OpenInternalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	// Empty vault.
	entries, err := v.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}

	// Add entries.
	_ = v.Set("a", "1")
	_ = v.Set("b", "2")
	_ = v.Set("c", "3")

	entries, err = v.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Ensure IDs are present.
	ids := map[string]bool{}
	for _, e := range entries {
		ids[e.ID] = true
	}
	for _, want := range []string{"a", "b", "c"} {
		if !ids[want] {
			t.Errorf("missing entry ID %q", want)
		}
	}
}

func TestInternalVault_WrongPassword(t *testing.T) {
	path := tempVaultPath(t)

	// Create vault with one password.
	v, err := OpenInternalAt(path, "correct-pw")
	if err != nil {
		t.Fatal(err)
	}
	_ = v.Set("id", "secret")
	_ = v.Close()

	// Try to open with the wrong password.
	_, err = OpenInternalAt(path, "wrong-pw")
	if err == nil {
		t.Fatal("expected error with wrong password")
	}
	if !containsError(err, ErrWrongPassword) {
		t.Fatalf("expected ErrWrongPassword, got: %v", err)
	}
}

func TestInternalVault_Persistence(t *testing.T) {
	path := tempVaultPath(t)

	// Create and populate.
	v, err := OpenInternalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	_ = v.Set("persist-1", "val1")
	_ = v.Set("persist-2", "val2")
	_ = v.Close()

	// Reopen.
	v2, err := OpenInternalAt(path, "pw")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer v2.Close()

	cred, err := v2.Get("persist-1")
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if cred != "val1" {
		t.Fatalf("expected 'val1', got %q", cred)
	}

	entries, err := v2.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after reopen, got %d", len(entries))
	}
}

func TestInternalVault_EmptyMasterPassword(t *testing.T) {
	path := tempVaultPath(t)
	_, err := OpenInternalAt(path, "")
	if err == nil {
		t.Fatal("expected error with empty master password")
	}
}

func TestInternalVault_FileCreatedOnDisk(t *testing.T) {
	path := tempVaultPath(t)
	v, err := OpenInternalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	_ = v.Set("x", "y")
	_ = v.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("vault file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("vault file is empty")
	}
	// Check permissions (should be 0600 on Unix; Windows doesn't support Unix perms).
	if runtime.GOOS != "windows" {
		perm := info.Mode().Perm()
		if perm != 0o600 {
			t.Fatalf("expected file permissions 0600, got %04o", perm)
		}
	}
}

func TestPassVault_AvailabilityDetection(t *testing.T) {
	// This test simply verifies that PassAvailable() does not panic
	// and returns a boolean. The actual result depends on the system.
	avail := PassAvailable()
	t.Logf("pass available: %v", avail)
}

func TestKeyringVault_AvailabilityDetection(t *testing.T) {
	avail := KeyringAvailable()
	t.Logf("keyring available: %v", avail)
}

func TestCredentialEntry_Metadata(t *testing.T) {
	path := tempVaultPath(t)
	v, err := OpenInternalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	before := time.Now().UTC().Add(-time.Second)
	_ = v.Set("meta-test", "value")
	after := time.Now().UTC().Add(time.Second)

	entries, err := v.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.ID != "meta-test" {
		t.Fatalf("expected ID 'meta-test', got %q", e.ID)
	}
	if e.Name != "meta-test" {
		t.Fatalf("expected Name 'meta-test', got %q", e.Name)
	}
	if e.CreatedAt.Before(before) || e.CreatedAt.After(after) {
		t.Fatalf("CreatedAt %v not in expected range [%v, %v]", e.CreatedAt, before, after)
	}
	if e.UpdatedAt.Before(before) || e.UpdatedAt.After(after) {
		t.Fatalf("UpdatedAt %v not in expected range [%v, %v]", e.UpdatedAt, before, after)
	}

	// Update and verify UpdatedAt changes.
	time.Sleep(10 * time.Millisecond)
	updateBefore := time.Now().UTC().Add(-time.Second)
	_ = v.Set("meta-test", "value2")
	updateAfter := time.Now().UTC().Add(time.Second)

	entries, _ = v.List()
	e = entries[0]
	if e.UpdatedAt.Before(updateBefore) || e.UpdatedAt.After(updateAfter) {
		t.Fatalf("UpdatedAt after update %v not in expected range", e.UpdatedAt)
	}
	// CreatedAt should not change on update.
	if e.CreatedAt.Before(before) || e.CreatedAt.After(after) {
		t.Fatalf("CreatedAt changed after update: %v", e.CreatedAt)
	}
}

func TestOpen_UnknownBackend(t *testing.T) {
	_, err := Open("foobar", "pw")
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

// TestInternalVault_NewFormatHeader creates a vault, writes an entry, and
// verifies that the file on disk starts with the version byte 0x01.
func TestInternalVault_NewFormatHeader(t *testing.T) {
	path := tempVaultPath(t)

	v, err := OpenInternalAt(path, "testpassword")
	if err != nil {
		t.Fatalf("OpenInternalAt failed: %v", err)
	}
	if err := v.Set("key1", "secret1"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("vault file is empty")
	}
	if data[0] != vaultVersion {
		t.Fatalf("expected first byte 0x%02x, got 0x%02x", vaultVersion, data[0])
	}
	if len(data) < 1+vaultSaltLen {
		t.Fatalf("vault file too short: %d bytes (need at least %d)", len(data), 1+vaultSaltLen)
	}
}

// TestInternalVault_SaltIsRandom creates two separate vaults and verifies their
// on-disk salts differ from each other.
func TestInternalVault_SaltIsRandom(t *testing.T) {
	dir := t.TempDir()

	path1 := filepath.Join(dir, "vault1.enc")
	v1, err := OpenInternalAt(path1, "password")
	if err != nil {
		t.Fatalf("OpenInternalAt vault1 failed: %v", err)
	}
	if err := v1.Set("k", "v"); err != nil {
		t.Fatalf("Set vault1 failed: %v", err)
	}

	path2 := filepath.Join(dir, "vault2.enc")
	v2, err := OpenInternalAt(path2, "password")
	if err != nil {
		t.Fatalf("OpenInternalAt vault2 failed: %v", err)
	}
	if err := v2.Set("k", "v"); err != nil {
		t.Fatalf("Set vault2 failed: %v", err)
	}

	data1, err := os.ReadFile(path1)
	if err != nil {
		t.Fatalf("ReadFile vault1 failed: %v", err)
	}
	data2, err := os.ReadFile(path2)
	if err != nil {
		t.Fatalf("ReadFile vault2 failed: %v", err)
	}

	salt1 := data1[1 : 1+vaultSaltLen]
	salt2 := data2[1 : 1+vaultSaltLen]

	if bytes.Equal(salt1, salt2) {
		t.Fatal("expected different salts for two separate vaults, but they are identical")
	}
}

// TestInternalVault_LegacyMigration writes a legacy-format vault file (raw
// "ENC:..." string encrypted with the static salt), opens it, verifies the
// credential is readable, saves it, then verifies the file now uses the new
// format and all data survives the migration.
func TestInternalVault_LegacyMigration(t *testing.T) {
	path := tempVaultPath(t)
	const password = "migrationpass"

	// Build and write a legacy vault file.
	legacyKey, err := crypto.DeriveKey(password, []byte(legacySalt))
	if err != nil {
		t.Fatalf("DeriveKey for legacy key failed: %v", err)
	}
	store := internalStore{
		Version: 1,
		Entries: []internalEntry{
			{ID: "legacy-id", Name: "legacy-id", Credential: "legacy-secret"},
		},
	}
	storeJSON, err := json.Marshal(&store)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	encData, err := crypto.Encrypt(string(storeJSON), legacyKey)
	if err != nil {
		t.Fatalf("crypto.Encrypt failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(encData), 0o600); err != nil {
		t.Fatalf("WriteFile legacy vault failed: %v", err)
	}

	// Open the vault — should decode successfully despite legacy format.
	v, err := OpenInternalAt(path, password)
	if err != nil {
		t.Fatalf("OpenInternalAt legacy vault failed: %v", err)
	}

	cred, err := v.Get("legacy-id")
	if err != nil {
		t.Fatalf("Get legacy-id failed: %v", err)
	}
	if cred != "legacy-secret" {
		t.Fatalf("expected %q, got %q", "legacy-secret", cred)
	}

	// Saving should migrate to new format.
	if err := v.Set("new-id", "new-secret"); err != nil {
		t.Fatalf("Set new-id failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if data[0] != vaultVersion {
		t.Fatalf("expected version byte 0x%02x after migration, got 0x%02x", vaultVersion, data[0])
	}

	// Reopen and verify both entries survive migration.
	v2, err := OpenInternalAt(path, password)
	if err != nil {
		t.Fatalf("re-open after migration failed: %v", err)
	}
	if cred, err := v2.Get("legacy-id"); err != nil || cred != "legacy-secret" {
		t.Fatalf("legacy-id after migration: cred=%q err=%v", cred, err)
	}
	if cred, err := v2.Get("new-id"); err != nil || cred != "new-secret" {
		t.Fatalf("new-id after migration: cred=%q err=%v", cred, err)
	}
}

// TestInternalVault_WrongPasswordNewFormat creates a vault with one password
// and verifies that opening it with a different password returns an error.
func TestInternalVault_WrongPasswordNewFormat(t *testing.T) {
	path := tempVaultPath(t)

	v, err := OpenInternalAt(path, "correctpassword")
	if err != nil {
		t.Fatalf("OpenInternalAt failed: %v", err)
	}
	if err := v.Set("id1", "secret"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	_ = v.Close()

	_, err = OpenInternalAt(path, "wrongpassword")
	if err == nil {
		t.Fatal("expected error when opening vault with wrong password, got nil")
	}
}

// containsError checks if err wraps target.
func containsError(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		// unwrap
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return false
}
