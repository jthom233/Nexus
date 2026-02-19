package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dr4zz/nexus/internal/crypto"
)

// --- HasMasterPassword ---

func TestHasMasterPassword_False_WhenEmpty(t *testing.T) {
	cfg := &Config{}
	if cfg.HasMasterPassword() {
		t.Fatal("expected HasMasterPassword to return false when PasswordVerify is empty")
	}
}

func TestHasMasterPassword_True_WhenSet(t *testing.T) {
	cfg := &Config{PasswordVerify: "ENC:somethinghere"}
	if !cfg.HasMasterPassword() {
		t.Fatal("expected HasMasterPassword to return true when PasswordVerify is non-empty")
	}
}

// --- SetPasswordVerify + VerifyMasterPassword round-trip ---

func TestSetAndVerifyMasterPassword_RoundTrip(t *testing.T) {
	password := []byte("correct-horse-battery-staple")
	cfg := &Config{}

	if err := cfg.SetPasswordVerify(password); err != nil {
		t.Fatalf("SetPasswordVerify failed: %v", err)
	}

	// PasswordVerify must be set and must have ENC: prefix
	if cfg.PasswordVerify == "" {
		t.Fatal("expected PasswordVerify to be non-empty after SetPasswordVerify")
	}
	if !crypto.IsEncrypted(cfg.PasswordVerify) {
		t.Fatalf("expected ENC: prefix on PasswordVerify, got %q", cfg.PasswordVerify)
	}

	if err := cfg.VerifyMasterPassword(password); err != nil {
		t.Fatalf("VerifyMasterPassword with correct password failed: %v", err)
	}
}

// Each call to SetPasswordVerify should produce a distinct ciphertext (random salt/nonce).
func TestSetPasswordVerify_ProducesUniqueOutput(t *testing.T) {
	password := []byte("same-password")
	cfg := &Config{}

	if err := cfg.SetPasswordVerify(password); err != nil {
		t.Fatalf("first SetPasswordVerify failed: %v", err)
	}
	first := cfg.PasswordVerify

	if err := cfg.SetPasswordVerify(password); err != nil {
		t.Fatalf("second SetPasswordVerify failed: %v", err)
	}
	second := cfg.PasswordVerify

	if first == second {
		t.Fatal("two calls to SetPasswordVerify with the same password must produce different ciphertexts (random salt)")
	}
}

// --- VerifyMasterPassword error cases ---

func TestVerifyMasterPassword_WrongPassword_ReturnsError(t *testing.T) {
	correctPw := []byte("correct-password")
	wrongPw := []byte("wrong-password")
	cfg := &Config{}

	if err := cfg.SetPasswordVerify(correctPw); err != nil {
		t.Fatalf("SetPasswordVerify failed: %v", err)
	}

	err := cfg.VerifyMasterPassword(wrongPw)
	if err == nil {
		t.Fatal("expected VerifyMasterPassword with wrong password to return an error")
	}
	if !strings.Contains(err.Error(), "wrong master password") {
		t.Fatalf("unexpected error message: %q", err.Error())
	}
}

func TestVerifyMasterPassword_EmptyPasswordVerify_ReturnsError(t *testing.T) {
	cfg := &Config{} // PasswordVerify == ""

	err := cfg.VerifyMasterPassword([]byte("any-password"))
	if err == nil {
		t.Fatal("expected error when PasswordVerify is empty")
	}
	if !strings.Contains(err.Error(), "no master password configured") {
		t.Fatalf("unexpected error message: %q", err.Error())
	}
}

// Security: a PasswordVerify without ENC: prefix must be rejected as corrupted,
// even though crypto.Decrypt would pass a non-ENC string through unchanged.
// Without this guard a plain "nexus" stored in the field would bypass verification.
func TestVerifyMasterPassword_NonEncryptedSentinel_ReturnsCorrupted(t *testing.T) {
	cfg := &Config{PasswordVerify: "nexus"} // raw sentinel, no ENC: prefix

	err := cfg.VerifyMasterPassword([]byte("any-password"))
	if err == nil {
		t.Fatal("expected error for non-ENC: prefixed PasswordVerify")
	}
	if !strings.Contains(err.Error(), "corrupted") {
		t.Fatalf("expected 'corrupted' error, got: %q", err.Error())
	}
}

// Security: any arbitrary non-ENC: string in PasswordVerify must be rejected.
func TestVerifyMasterPassword_ArbitraryPlaintext_ReturnsCorrupted(t *testing.T) {
	cfg := &Config{PasswordVerify: "not-an-encrypted-value"}

	err := cfg.VerifyMasterPassword([]byte("any-password"))
	if err == nil {
		t.Fatal("expected error for non-ENC: prefixed PasswordVerify")
	}
	if !strings.Contains(err.Error(), "corrupted") {
		t.Fatalf("expected 'corrupted' error, got: %q", err.Error())
	}
}

// Adversarial: if someone manually sets password_verify: nexus (raw sentinel)
// in the YAML and then loads the config, VerifyMasterPassword must not grant access.
func TestVerifyMasterPassword_ManuallySetRawSentinel_YAMLRoundTrip(t *testing.T) {
	// Simulate a config loaded from YAML where password_verify is the raw sentinel.
	yamlInput := []byte("version: 1\npassword_verify: nexus\n")

	var cfg Config
	if err := unmarshalConfig(yamlInput, &cfg); err != nil {
		t.Fatalf("yaml unmarshal failed: %v", err)
	}

	err := cfg.VerifyMasterPassword([]byte("nexus"))
	if err == nil {
		t.Fatal("expected VerifyMasterPassword to reject raw sentinel loaded from YAML")
	}
	if !strings.Contains(err.Error(), "corrupted") {
		t.Fatalf("expected 'corrupted' error, got: %q", err.Error())
	}
}

// --- Load() ---

// Load() must return isNew=true when the config file does not exist.
func TestLoad_ReturnsIsNew_WhenFileAbsent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, isNew, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if !isNew {
		t.Fatal("expected isNew=true when config file does not exist")
	}
	if cfg == nil {
		t.Fatal("expected non-nil default config on fresh install")
	}
}

// Load() must return isNew=false when the config file already exists.
func TestLoad_ReturnsIsNew_False_WhenFileExists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Write a minimal valid config to the expected path.
	cfgPath := filepath.Join(dir, "nexus", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cfg, isNew, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if isNew {
		t.Fatal("expected isNew=false when config file exists")
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Version != 1 {
		t.Fatalf("expected version=1, got %d", cfg.Version)
	}
}

// Load() must return an error on malformed YAML, not a nil config.
func TestLoad_ReturnsError_OnMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgPath := filepath.Join(dir, "nexus", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	// Valid YAML but a type mismatch: version is a string, not an int.
	if err := os.WriteFile(cfgPath, []byte("version: [1, 2, 3]\n"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cfg, _, err := Load()
	if err == nil {
		t.Fatalf("expected error for malformed YAML, got cfg=%+v", cfg)
	}
}

// --- SampleConfig ---

func TestSampleConfig_UniqueIDs(t *testing.T) {
	cfg := SampleConfig()
	seen := make(map[string]bool)
	for _, c := range cfg.Connections {
		if seen[c.ID] {
			t.Errorf("duplicate connection ID in SampleConfig: %q", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestSampleConfig_RequiredFieldsNonEmpty(t *testing.T) {
	cfg := SampleConfig()
	for _, c := range cfg.Connections {
		if c.ID == "" {
			t.Errorf("connection %q has empty ID", c.Name)
		}
		if c.Name == "" {
			t.Errorf("connection with ID %q has empty Name", c.ID)
		}
		if c.Host == "" {
			t.Errorf("connection %q has empty Host", c.ID)
		}
		if !c.Protocol.Valid() {
			t.Errorf("connection %q has invalid protocol %q", c.ID, c.Protocol)
		}
	}
}

func TestSampleConfig_VersionSet(t *testing.T) {
	cfg := SampleConfig()
	if cfg.Version == 0 {
		t.Fatal("expected SampleConfig to return a config with Version set")
	}
}

// --- SSH import: alias-as-hostname ---

// parseSSHConfigFromString is a test helper that writes content to a temp file,
// temporarily overwrites HOME so ImportSSHConfig picks it up, and returns the result.
func parseSSHConfigFromString(t *testing.T, content string) ([]Connection, error) {
	t.Helper()
	fakeHome := t.TempDir()
	sshDir := filepath.Join(fakeHome, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("MkdirAll .ssh failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile ssh/config failed: %v", err)
	}
	// Override HOME so os.UserHomeDir() returns our temp dir.
	orig := os.Getenv("HOME")
	os.Setenv("HOME", fakeHome)
	t.Cleanup(func() { os.Setenv("HOME", orig) })

	return ImportSSHConfig()
}

// When a Host entry has no HostName directive, the alias must be used as hostname.
func TestImportSSHConfig_NoHostName_UsesAlias(t *testing.T) {
	sshCfg := `Host myserver
    User admin
    Port 2222
`
	conns, err := parseSSHConfigFromString(t, sshCfg)
	if err != nil {
		t.Fatalf("ImportSSHConfig failed: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(conns))
	}
	if conns[0].Host != "myserver" {
		t.Errorf("expected Host=%q (alias), got %q", "myserver", conns[0].Host)
	}
	if conns[0].Username != "admin" {
		t.Errorf("expected Username=admin, got %q", conns[0].Username)
	}
	if conns[0].Port != 2222 {
		t.Errorf("expected Port=2222, got %d", conns[0].Port)
	}
}

// When a Host entry has an explicit HostName, that must take precedence over the alias.
func TestImportSSHConfig_ExplicitHostName_TakesPrecedence(t *testing.T) {
	sshCfg := `Host myalias
    HostName 10.0.0.5
    User deploy
`
	conns, err := parseSSHConfigFromString(t, sshCfg)
	if err != nil {
		t.Fatalf("ImportSSHConfig failed: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(conns))
	}
	if conns[0].Host != "10.0.0.5" {
		t.Errorf("expected Host=10.0.0.5, got %q", conns[0].Host)
	}
	if conns[0].Name != "myalias" {
		t.Errorf("expected Name=myalias, got %q", conns[0].Name)
	}
}

// Wildcard host entries must be skipped entirely.
func TestImportSSHConfig_WildcardEntry_IsSkipped(t *testing.T) {
	sshCfg := `Host *
    ServerAliveInterval 60

Host realserver
    HostName 192.168.1.1
`
	conns, err := parseSSHConfigFromString(t, sshCfg)
	if err != nil {
		t.Fatalf("ImportSSHConfig failed: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection (wildcard skipped), got %d", len(conns))
	}
	if conns[0].Name != "realserver" {
		t.Errorf("expected Name=realserver, got %q", conns[0].Name)
	}
}

// Multiple hosts without HostName: each alias must become its own hostname.
func TestImportSSHConfig_MultipleHosts_NoHostName(t *testing.T) {
	sshCfg := `Host alpha
    User root

Host beta
    User deploy

Host gamma
    HostName 10.0.0.99
    User ops
`
	conns, err := parseSSHConfigFromString(t, sshCfg)
	if err != nil {
		t.Fatalf("ImportSSHConfig failed: %v", err)
	}
	if len(conns) != 3 {
		t.Fatalf("expected 3 connections, got %d", len(conns))
	}

	byName := make(map[string]Connection)
	for _, c := range conns {
		byName[c.Name] = c
	}

	if byName["alpha"].Host != "alpha" {
		t.Errorf("alpha: expected Host=alpha, got %q", byName["alpha"].Host)
	}
	if byName["beta"].Host != "beta" {
		t.Errorf("beta: expected Host=beta, got %q", byName["beta"].Host)
	}
	if byName["gamma"].Host != "10.0.0.99" {
		t.Errorf("gamma: expected Host=10.0.0.99, got %q", byName["gamma"].Host)
	}
}

// Empty SSH config file should return zero connections without error.
func TestImportSSHConfig_EmptyFile_NoConnections(t *testing.T) {
	conns, err := parseSSHConfigFromString(t, "")
	if err != nil {
		t.Fatalf("ImportSSHConfig on empty file failed: %v", err)
	}
	if len(conns) != 0 {
		t.Fatalf("expected 0 connections for empty config, got %d", len(conns))
	}
}

// unmarshalConfig is a package-internal test helper that parses YAML bytes into
// a Config by writing to a temp dir and going through the production Load() path.
// This ensures the test exercises the same deserialization code the app uses.
func unmarshalConfig(data []byte, cfg *Config) error {
	dir, err := os.MkdirTemp("", "nexus-cfg-test-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	orig := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", dir)
	defer os.Setenv("XDG_CONFIG_HOME", orig)

	cfgPath := filepath.Join(dir, "nexus", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		return err
	}

	loaded, _, err := Load()
	if err != nil {
		return err
	}
	*cfg = *loaded
	return nil
}
