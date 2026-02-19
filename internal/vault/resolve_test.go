package vault

import (
	"path/filepath"
	"testing"

	"github.com/dr4zz/nexus/internal/config"
)

// setupTestStore creates a fresh, empty ProfileStore backed by a temporary
// InternalVault. The vault file is placed inside the test's temp directory.
func setupTestStore(t *testing.T) *ProfileStore {
	t.Helper()
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.enc")
	v, err := OpenInternalAt(vaultPath, "testpass")
	if err != nil {
		t.Fatalf("OpenInternalAt: %v", err)
	}
	t.Cleanup(func() { _ = v.Close() })
	return NewProfileStore(v)
}

// mustCreate creates a profile or fatally fails the test.
func mustCreate(t *testing.T, store *ProfileStore, p CredentialProfile) *CredentialProfile {
	t.Helper()
	created, err := store.Create(p)
	if err != nil {
		t.Fatalf("ProfileStore.Create(%q): %v", p.Name, err)
	}
	return created
}

// TestResolve_NoProfileNoGroup verifies that a connection with only inline
// credentials resolves to source "direct" for every supplied field.
func TestResolve_NoProfileNoGroup(t *testing.T) {
	store := setupTestStore(t)

	conn := config.Connection{
		Username: "alice",
		Password: "secret",
	}

	creds, sources, err := ResolveCredentials(conn, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if creds.Username != "alice" {
		t.Errorf("Username: want %q, got %q", "alice", creds.Username)
	}
	if creds.Password != "secret" {
		t.Errorf("Password: want %q, got %q", "secret", creds.Password)
	}
	if sources["username"] != "direct" {
		t.Errorf("sources[username]: want %q, got %q", "direct", sources["username"])
	}
	if sources["password"] != "direct" {
		t.Errorf("sources[password]: want %q, got %q", "direct", sources["password"])
	}
	// No profile fields — passphrase must be absent from sources.
	if _, ok := sources["passphrase"]; ok {
		t.Errorf("sources[passphrase] should not be set, got %q", sources["passphrase"])
	}
}

// TestResolve_ConnectionProfileOnly verifies that credentials come from the
// named connection profile when CredentialProfile is set.
func TestResolve_ConnectionProfileOnly(t *testing.T) {
	store := setupTestStore(t)
	mustCreate(t, store, CredentialProfile{
		Name:     "myprofile",
		Username: "bob",
		Password: "bobpass",
	})

	conn := config.Connection{
		CredentialProfile: "myprofile",
	}

	creds, sources, err := ResolveCredentials(conn, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if creds.Username != "bob" {
		t.Errorf("Username: want %q, got %q", "bob", creds.Username)
	}
	if creds.Password != "bobpass" {
		t.Errorf("Password: want %q, got %q", "bobpass", creds.Password)
	}
	if sources["username"] != "profile:myprofile" {
		t.Errorf("sources[username]: want %q, got %q", "profile:myprofile", sources["username"])
	}
	if sources["password"] != "profile:myprofile" {
		t.Errorf("sources[password]: want %q, got %q", "profile:myprofile", sources["password"])
	}
}

// TestResolve_GroupProfileOnly verifies that a profile whose Group field
// matches conn.Group is applied at the lowest priority layer.
func TestResolve_GroupProfileOnly(t *testing.T) {
	store := setupTestStore(t)
	mustCreate(t, store, CredentialProfile{
		Name:     "groupprofile",
		Group:    "servers",
		Username: "carol",
		Password: "carolpass",
	})

	conn := config.Connection{
		Group: "servers",
	}

	creds, sources, err := ResolveCredentials(conn, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if creds.Username != "carol" {
		t.Errorf("Username: want %q, got %q", "carol", creds.Username)
	}
	if creds.Password != "carolpass" {
		t.Errorf("Password: want %q, got %q", "carolpass", creds.Password)
	}
	if sources["username"] != "group:groupprofile" {
		t.Errorf("sources[username]: want %q, got %q", "group:groupprofile", sources["username"])
	}
	if sources["password"] != "group:groupprofile" {
		t.Errorf("sources[password]: want %q, got %q", "group:groupprofile", sources["password"])
	}
}

// TestResolve_AllThreeLayers verifies the full three-layer priority system.
// Group profile: Username, Password, Domain.
// Connection profile: Username (overrides group), IdentityFile.
// Inline: Password (overrides both layers).
// Expected: Password="direct", Username="profile:X", IdentityFile="profile:X", Domain="group:Y".
func TestResolve_AllThreeLayers(t *testing.T) {
	store := setupTestStore(t)
	mustCreate(t, store, CredentialProfile{
		Name:     "groupY",
		Group:    "prod",
		Username: "group-user",
		Password: "group-pass",
		Domain:   "corp.local",
	})
	mustCreate(t, store, CredentialProfile{
		Name:         "profileX",
		Username:     "profile-user",
		IdentityFile: "/home/user/.ssh/id_rsa",
	})

	conn := config.Connection{
		Group:             "prod",
		CredentialProfile: "profileX",
		Password:          "inline-pass",
	}

	creds, sources, err := ResolveCredentials(conn, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Password: inline overrides group profile.
	if creds.Password != "inline-pass" {
		t.Errorf("Password: want %q, got %q", "inline-pass", creds.Password)
	}
	if sources["password"] != "direct" {
		t.Errorf("sources[password]: want %q, got %q", "direct", sources["password"])
	}

	// Username: connection profile overrides group profile.
	if creds.Username != "profile-user" {
		t.Errorf("Username: want %q, got %q", "profile-user", creds.Username)
	}
	if sources["username"] != "profile:profileX" {
		t.Errorf("sources[username]: want %q, got %q", "profile:profileX", sources["username"])
	}

	// IdentityFile: set only by connection profile.
	if creds.IdentityFile != "/home/user/.ssh/id_rsa" {
		t.Errorf("IdentityFile: want %q, got %q", "/home/user/.ssh/id_rsa", creds.IdentityFile)
	}
	if sources["identity_file"] != "profile:profileX" {
		t.Errorf("sources[identity_file]: want %q, got %q", "profile:profileX", sources["identity_file"])
	}

	// Domain: only the group profile sets it; connection profile and inline do not.
	if creds.Domain != "corp.local" {
		t.Errorf("Domain: want %q, got %q", "corp.local", creds.Domain)
	}
	if sources["domain"] != "group:groupY" {
		t.Errorf("sources[domain]: want %q, got %q", "group:groupY", sources["domain"])
	}
}

// TestResolve_PartialProfile verifies that a profile supplying only Username
// does not override an inline Password, and that empty profile fields leave
// the result empty (not set to empty string from a previous layer).
func TestResolve_PartialProfile(t *testing.T) {
	store := setupTestStore(t)
	mustCreate(t, store, CredentialProfile{
		Name:     "partial",
		Username: "profile-only-user",
		// Password intentionally empty.
	})

	conn := config.Connection{
		CredentialProfile: "partial",
		Password:          "inline-only-pass",
	}

	creds, sources, err := ResolveCredentials(conn, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if creds.Username != "profile-only-user" {
		t.Errorf("Username: want %q, got %q", "profile-only-user", creds.Username)
	}
	if sources["username"] != "profile:partial" {
		t.Errorf("sources[username]: want %q, got %q", "profile:partial", sources["username"])
	}

	// Inline password should not be overridden by the empty profile password.
	if creds.Password != "inline-only-pass" {
		t.Errorf("Password: want %q, got %q", "inline-only-pass", creds.Password)
	}
	if sources["password"] != "direct" {
		t.Errorf("sources[password]: want %q, got %q", "direct", sources["password"])
	}
}

// TestResolve_DanglingProfileReference verifies that a CredentialProfile
// pointing to a name that does not exist causes no error and is silently
// skipped, leaving the result empty.
func TestResolve_DanglingProfileReference(t *testing.T) {
	store := setupTestStore(t)

	conn := config.Connection{
		CredentialProfile: "nonexistent",
	}

	creds, sources, err := ResolveCredentials(conn, store)
	if err != nil {
		t.Fatalf("unexpected error for dangling profile: %v", err)
	}

	if creds.Username != "" || creds.Password != "" {
		t.Errorf("expected empty credentials for dangling profile, got Username=%q Password=%q",
			creds.Username, creds.Password)
	}
	if len(sources) != 0 {
		t.Errorf("expected empty sources map for dangling profile, got %v", sources)
	}
}

// TestResolve_MissingGroupProfile verifies that when conn.Group is set but no
// profile has a matching Group field, no error occurs and the result is empty.
func TestResolve_MissingGroupProfile(t *testing.T) {
	store := setupTestStore(t)
	// Create a profile for a different group so the store is not empty.
	mustCreate(t, store, CredentialProfile{
		Name:     "othergroup",
		Group:    "staging",
		Username: "staging-user",
	})

	conn := config.Connection{
		Group: "production", // no profile matches this group
	}

	creds, sources, err := ResolveCredentials(conn, store)
	if err != nil {
		t.Fatalf("unexpected error for missing group: %v", err)
	}

	if creds.Username != "" {
		t.Errorf("expected empty Username, got %q", creds.Username)
	}
	if len(sources) != 0 {
		t.Errorf("expected empty sources map, got %v", sources)
	}
}

// TestResolve_EmptyProfile verifies that a profile with all credential fields
// empty has no effect on the resolved result.
func TestResolve_EmptyProfile(t *testing.T) {
	store := setupTestStore(t)
	mustCreate(t, store, CredentialProfile{
		Name: "empty-profile",
		// All credential fields intentionally left empty.
	})

	conn := config.Connection{
		CredentialProfile: "empty-profile",
	}

	creds, sources, err := ResolveCredentials(conn, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if creds.Username != "" || creds.Password != "" || creds.IdentityFile != "" ||
		creds.Passphrase != "" || creds.Domain != "" || creds.VNCPassword != "" {
		t.Errorf("expected all-empty credentials from empty profile, got %+v", creds)
	}
	if len(sources) != 0 {
		t.Errorf("expected empty sources map from empty profile, got %v", sources)
	}
}

// TestResolve_PassphraseFromProfile verifies that Passphrase can only be
// provided by a profile (Connection has no Passphrase field), and that the
// source is correctly recorded.
func TestResolve_PassphraseFromProfile(t *testing.T) {
	store := setupTestStore(t)
	mustCreate(t, store, CredentialProfile{
		Name:       "keypair",
		Passphrase: "my-key-passphrase",
	})

	conn := config.Connection{
		CredentialProfile: "keypair",
	}

	creds, sources, err := ResolveCredentials(conn, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if creds.Passphrase != "my-key-passphrase" {
		t.Errorf("Passphrase: want %q, got %q", "my-key-passphrase", creds.Passphrase)
	}
	if sources["passphrase"] != "profile:keypair" {
		t.Errorf("sources[passphrase]: want %q, got %q", "profile:keypair", sources["passphrase"])
	}
}

// TestResolve_PassphraseNotFromInline verifies that even when a connection
// has no CredentialProfile, the resolved Passphrase is always empty because
// Connection struct has no Passphrase field.
func TestResolve_PassphraseNotFromInline(t *testing.T) {
	store := setupTestStore(t)

	conn := config.Connection{
		Username: "dave",
		Password: "davepass",
		// No CredentialProfile — no way to set passphrase.
	}

	creds, sources, err := ResolveCredentials(conn, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if creds.Passphrase != "" {
		t.Errorf("expected empty Passphrase with no profile, got %q", creds.Passphrase)
	}
	if _, ok := sources["passphrase"]; ok {
		t.Errorf("sources should not contain passphrase key, got %q", sources["passphrase"])
	}
}

// TestResolve_VNCPasswordAllLayers verifies VNCPassword participates in
// the same three-layer resolution as other credential fields.
func TestResolve_VNCPasswordAllLayers(t *testing.T) {
	store := setupTestStore(t)
	mustCreate(t, store, CredentialProfile{
		Name:        "vncgroup",
		Group:       "vnc-hosts",
		VNCPassword: "group-vnc",
	})
	mustCreate(t, store, CredentialProfile{
		Name:        "vncprofile",
		VNCPassword: "profile-vnc",
	})

	// Connection profile overrides group profile.
	connProfile := config.Connection{
		Group:             "vnc-hosts",
		CredentialProfile: "vncprofile",
	}
	creds, sources, err := ResolveCredentials(connProfile, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.VNCPassword != "profile-vnc" {
		t.Errorf("VNCPassword: want %q, got %q", "profile-vnc", creds.VNCPassword)
	}
	if sources["vnc_password"] != "profile:vncprofile" {
		t.Errorf("sources[vnc_password]: want %q, got %q", "profile:vncprofile", sources["vnc_password"])
	}

	// Inline overrides connection profile.
	connInline := config.Connection{
		Group:             "vnc-hosts",
		CredentialProfile: "vncprofile",
		VNCPassword:       "inline-vnc",
	}
	creds2, sources2, err := ResolveCredentials(connInline, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds2.VNCPassword != "inline-vnc" {
		t.Errorf("VNCPassword (inline): want %q, got %q", "inline-vnc", creds2.VNCPassword)
	}
	if sources2["vnc_password"] != "direct" {
		t.Errorf("sources[vnc_password] (inline): want %q, got %q", "direct", sources2["vnc_password"])
	}
}

// TestResolve_EmptyConnection verifies that resolving a fully empty connection
// (no inline fields, no profile, no group) yields an all-zero result with an
// empty sources map and no error.
func TestResolve_EmptyConnection(t *testing.T) {
	store := setupTestStore(t)

	creds, sources, err := ResolveCredentials(config.Connection{}, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if creds == nil {
		t.Fatal("expected non-nil ResolvedCredentials")
	}
	empty := ResolvedCredentials{}
	if *creds != empty {
		t.Errorf("expected all-empty credentials, got %+v", creds)
	}
	if len(sources) != 0 {
		t.Errorf("expected empty sources map, got %v", sources)
	}
}

// TestResolve_GroupProfileDoesNotOverrideConnectionProfile verifies that the
// group profile (lower priority) cannot override a field already set by the
// connection profile (higher priority).
func TestResolve_GroupProfileDoesNotOverrideConnectionProfile(t *testing.T) {
	store := setupTestStore(t)
	mustCreate(t, store, CredentialProfile{
		Name:     "g",
		Group:    "team",
		Username: "group-user",
		Password: "group-pass",
	})
	mustCreate(t, store, CredentialProfile{
		Name:     "p",
		Username: "profile-user",
		// Password intentionally empty — group password should survive.
	})

	conn := config.Connection{
		Group:             "team",
		CredentialProfile: "p",
	}

	creds, sources, err := ResolveCredentials(conn, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Username: connection profile wins over group.
	if creds.Username != "profile-user" {
		t.Errorf("Username: want %q, got %q", "profile-user", creds.Username)
	}
	if sources["username"] != "profile:p" {
		t.Errorf("sources[username]: want %q, got %q", "profile:p", sources["username"])
	}

	// Password: connection profile has no password, so group password stands.
	if creds.Password != "group-pass" {
		t.Errorf("Password: want %q, got %q", "group-pass", creds.Password)
	}
	if sources["password"] != "group:g" {
		t.Errorf("sources[password]: want %q, got %q", "group:g", sources["password"])
	}
}
