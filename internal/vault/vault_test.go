package vault

import (
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

// TestInternalVault_NewFormatHeader verifies that a vault saved with the new
// format starts with the version byte (0x01) followed by a 16-byte salt.
func TestInternalVault_NewFormatHeader(t *testing.T) {
	path := tempVaultPath(t)
	v, err := OpenInternalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	_ = v.Set("k", "v")
	_ = v.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading vault file: %v", err)
	}
	if len(data) < vaultHeaderLen {
		t.Fatalf("vault file too short: %d bytes", len(data))
	}
	if data[0] != vaultFormatVersion {
		t.Fatalf("expected version byte 0x%02x, got 0x%02x", vaultFormatVersion, data[0])
	}
}

// TestInternalVault_SaltIsRandom verifies that two independently created vaults
// receive different salts (the probability of collision on 16 random bytes is negligible).
func TestInternalVault_SaltIsRandom(t *testing.T) {
	path1 := tempVaultPath(t)
	v1, err := OpenInternalAt(path1, "pw")
	if err != nil {
		t.Fatal(err)
	}
	_ = v1.Set("x", "1")
	_ = v1.Close()

	path2 := tempVaultPath(t)
	v2, err := OpenInternalAt(path2, "pw")
	if err != nil {
		t.Fatal(err)
	}
	_ = v2.Set("x", "1")
	_ = v2.Close()

	data1, _ := os.ReadFile(path1)
	data2, _ := os.ReadFile(path2)

	if len(data1) < vaultHeaderLen || len(data2) < vaultHeaderLen {
		t.Fatal("vault files too short")
	}
	salt1 := data1[1:vaultHeaderLen]
	salt2 := data2[1:vaultHeaderLen]

	allEqual := true
	for i := range salt1 {
		if salt1[i] != salt2[i] {
			allEqual = false
			break
		}
	}
	if allEqual {
		t.Fatal("two vaults have identical salts — expected random salts")
	}
}

// TestInternalVault_LegacyMigration writes a vault file in the old static-salt
// format (plain ENC:... string, no header), opens it with the new code, verifies
// the data is readable, then saves and confirms the file is now in the new format.
func TestInternalVault_LegacyMigration(t *testing.T) {
	path := tempVaultPath(t)

	// Build a legacy-format vault file manually.
	// The legacy format is: encrypted JSON written directly to disk as a plain
	// "ENC:..." string, derived from the static "nexus-vault-salt" salt.
	legacyKey, err := crypto.DeriveKey("test-pw", []byte("nexus-vault-salt"))
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	legacyJSON := `{"version":1,"entries":[{"id":"legacy-id","name":"legacy-id","credential":"legacy-secret","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}]}`
	encrypted, err := crypto.Encrypt(legacyJSON, legacyKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(encrypted), 0o600); err != nil {
		t.Fatalf("writing legacy vault: %v", err)
	}

	// Open with new code — should succeed via legacy path.
	v, err := OpenInternalAt(path, "test-pw")
	if err != nil {
		t.Fatalf("OpenInternalAt legacy vault: %v", err)
	}

	cred, err := v.Get("legacy-id")
	if err != nil {
		t.Fatalf("Get from legacy vault: %v", err)
	}
	if cred != "legacy-secret" {
		t.Fatalf("expected 'legacy-secret', got %q", cred)
	}

	// Save should upgrade to new format.
	if err := v.Close(); err != nil {
		t.Fatalf("Close (upgrade save): %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < vaultHeaderLen {
		t.Fatalf("upgraded file too short: %d bytes", len(data))
	}
	if data[0] != vaultFormatVersion {
		t.Fatalf("after upgrade, expected version byte 0x%02x, got 0x%02x", vaultFormatVersion, data[0])
	}

	// Re-open the upgraded vault and confirm data survived.
	v2, err := OpenInternalAt(path, "test-pw")
	if err != nil {
		t.Fatalf("re-open after upgrade: %v", err)
	}
	defer v2.Close()

	cred2, err := v2.Get("legacy-id")
	if err != nil {
		t.Fatalf("Get after upgrade: %v", err)
	}
	if cred2 != "legacy-secret" {
		t.Fatalf("data corrupted after upgrade: got %q", cred2)
	}
}

// TestInternalVault_WrongPasswordNewFormat confirms that a wrong password fails
// cleanly on a new-format vault.
func TestInternalVault_WrongPasswordNewFormat(t *testing.T) {
	path := tempVaultPath(t)

	v, err := OpenInternalAt(path, "correct")
	if err != nil {
		t.Fatal(err)
	}
	_ = v.Set("id", "secret")
	_ = v.Close()

	// Verify the file is in the new format.
	data, _ := os.ReadFile(path)
	if len(data) < 1 || data[0] != vaultFormatVersion {
		t.Fatal("vault not in new format after save")
	}

	_, err = OpenInternalAt(path, "wrong")
	if err == nil {
		t.Fatal("expected error with wrong password on new-format vault")
	}
	if !containsError(err, ErrWrongPassword) {
		t.Fatalf("expected ErrWrongPassword, got: %v", err)
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
