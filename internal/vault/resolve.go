package vault

import (
	"github.com/dr4zz/nexus/internal/config"
)

// ResolvedCredentials holds the final credential values after applying all
// resolution layers (group profile, connection profile, and inline fields).
type ResolvedCredentials struct {
	Username     string
	Password     string
	IdentityFile string
	Passphrase   string
	Domain       string
	VNCPassword  string
}

// ResolveCredentials resolves credentials for a connection using a three-layer
// priority system. Lower priority layers are applied first and higher priority
// layers override them, but only for non-empty values.
//
// Layer priority (lowest to highest):
//  1. Group profile — looked up via conn.Group
//  2. Connection profile — looked up via conn.CredentialProfile
//  3. Inline fields — values set directly on conn
//
// The returned sources map records which layer provided each field, using keys:
// "username", "password", "identity_file", "passphrase", "domain", "vnc_password".
//
// Missing or dangling profile references are silently skipped and are not errors.
// Only vault I/O errors are returned.
func ResolveCredentials(conn config.Connection, store *ProfileStore) (*ResolvedCredentials, map[string]string, error) {
	result := &ResolvedCredentials{}
	sources := make(map[string]string)

	// Layer 1: group profile (lowest priority).
	if conn.Group != "" {
		profile, err := store.FindByGroup(conn.Group)
		if err != nil {
			return nil, nil, err
		}
		if profile != nil {
			mergeProfile(result, sources, profile, "group:"+profile.Name)
		}
	}

	// Layer 2: named credential profile.
	if conn.CredentialProfile != "" {
		profile, err := store.Get(conn.CredentialProfile)
		if err != nil {
			return nil, nil, err
		}
		if profile != nil {
			mergeProfile(result, sources, profile, "profile:"+profile.Name)
		}
		// If profile == nil the reference is dangling — skip silently.
	}

	// Layer 3: inline connection fields (highest priority).
	// Note: Connection has no Passphrase field, so passphrase can only come
	// from a profile.
	if conn.Username != "" {
		result.Username = conn.Username
		sources["username"] = "direct"
	}
	if conn.Password != "" {
		result.Password = conn.Password
		sources["password"] = "direct"
	}
	if conn.IdentityFile != "" {
		result.IdentityFile = conn.IdentityFile
		sources["identity_file"] = "direct"
	}
	if conn.Domain != "" {
		result.Domain = conn.Domain
		sources["domain"] = "direct"
	}
	if conn.VNCPassword != "" {
		result.VNCPassword = conn.VNCPassword
		sources["vnc_password"] = "direct"
	}

	return result, sources, nil
}

// mergeProfile copies non-empty fields from profile into result, recording
// sourcePrefix as the source for each field that is set. Only fields that are
// non-empty in the profile are merged; empty profile fields never clear an
// already-set value in result.
func mergeProfile(result *ResolvedCredentials, sources map[string]string, profile *CredentialProfile, sourcePrefix string) {
	if profile.Username != "" {
		result.Username = profile.Username
		sources["username"] = sourcePrefix
	}
	if profile.Password != "" {
		result.Password = profile.Password
		sources["password"] = sourcePrefix
	}
	if profile.IdentityFile != "" {
		result.IdentityFile = profile.IdentityFile
		sources["identity_file"] = sourcePrefix
	}
	if profile.Passphrase != "" {
		result.Passphrase = profile.Passphrase
		sources["passphrase"] = sourcePrefix
	}
	if profile.Domain != "" {
		result.Domain = profile.Domain
		sources["domain"] = sourcePrefix
	}
	if profile.VNCPassword != "" {
		result.VNCPassword = profile.VNCPassword
		sources["vnc_password"] = sourcePrefix
	}
}
