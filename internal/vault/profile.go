package vault

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CredentialProfile stores named credential sets that can be reused across
// connections without embedding secrets directly in host configurations.
type CredentialProfile struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	Username     string    `json:"username,omitempty"`
	Password     string    `json:"password,omitempty"`
	IdentityFile string    `json:"identity_file,omitempty"`
	Passphrase   string    `json:"passphrase,omitempty"`
	Domain       string    `json:"domain,omitempty"`
	VNCPassword  string    `json:"vnc_password,omitempty"`
	Group        string    `json:"group,omitempty"`
	Tags         []string  `json:"tags,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

const profilePrefix = "profile:"

// ProfileStore wraps a Vault and provides typed CRUD operations for
// CredentialProfile values. Profiles are stored with a "profile:" key prefix
// to avoid collisions with other vault entries.
type ProfileStore struct {
	vault Vault
}

// NewProfileStore returns a ProfileStore backed by the given Vault.
func NewProfileStore(v Vault) *ProfileStore {
	return &ProfileStore{vault: v}
}

// Create persists a new profile. The profile's Name must be non-empty and
// unique among all existing profiles. The ID, CreatedAt, and UpdatedAt fields
// are set by Create and returned on the result.
func (s *ProfileStore) Create(p CredentialProfile) (*CredentialProfile, error) {
	if strings.TrimSpace(p.Name) == "" {
		return nil, fmt.Errorf("profile name must not be empty")
	}

	existing, err := s.List()
	if err != nil {
		return nil, fmt.Errorf("checking name uniqueness: %w", err)
	}
	for _, e := range existing {
		if e.Name == p.Name {
			return nil, fmt.Errorf("profile name %q already exists", p.Name)
		}
	}

	now := time.Now().UTC()
	p.ID = uuid.New().String()
	p.CreatedAt = now
	p.UpdatedAt = now

	if err := s.store(p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Get returns the profile with the given name, or nil if no profile has that
// name. Name matching is case-sensitive. A missing profile is not an error.
func (s *ProfileStore) Get(name string) (*CredentialProfile, error) {
	profiles, err := s.List()
	if err != nil {
		return nil, err
	}
	for i := range profiles {
		if profiles[i].Name == name {
			return &profiles[i], nil
		}
	}
	return nil, nil
}

// GetByID returns the profile with the given ID, or nil if no such profile
// exists. A missing profile is not an error.
func (s *ProfileStore) GetByID(id string) (*CredentialProfile, error) {
	raw, err := s.vault.Get(profilePrefix + id)
	if err != nil {
		// Treat a not-found condition as nil rather than an error.
		if err == ErrNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("vault get: %w", err)
	}
	var p CredentialProfile
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, fmt.Errorf("deserializing profile %q: %w", id, err)
	}
	return &p, nil
}

// Update persists changes to an existing profile. The profile's ID must be
// non-empty. The Name must be unique across all profiles except the profile
// being updated. UpdatedAt is set to the current time.
func (s *ProfileStore) Update(p CredentialProfile) error {
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("profile ID must not be empty")
	}

	existing, err := s.List()
	if err != nil {
		return fmt.Errorf("checking name uniqueness: %w", err)
	}
	for _, e := range existing {
		if e.Name == p.Name && e.ID != p.ID {
			return fmt.Errorf("profile name %q already exists", p.Name)
		}
	}

	p.UpdatedAt = time.Now().UTC()
	return s.store(p)
}

// Delete removes the profile with the given name. If no profile with that name
// exists the operation is a no-op.
func (s *ProfileStore) Delete(name string) error {
	p, err := s.Get(name)
	if err != nil {
		return err
	}
	if p == nil {
		return nil
	}
	if err := s.vault.Delete(profilePrefix + p.ID); err != nil {
		return fmt.Errorf("vault delete: %w", err)
	}
	return nil
}

// List returns all profiles sorted alphabetically by Name.
func (s *ProfileStore) List() ([]CredentialProfile, error) {
	entries, err := s.vault.List()
	if err != nil {
		return nil, fmt.Errorf("vault list: %w", err)
	}

	var profiles []CredentialProfile
	for _, entry := range entries {
		if !strings.HasPrefix(entry.ID, profilePrefix) {
			continue
		}
		raw, err := s.vault.Get(entry.ID)
		if err != nil {
			return nil, fmt.Errorf("vault get %q: %w", entry.ID, err)
		}
		var p CredentialProfile
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			return nil, fmt.Errorf("deserializing profile %q: %w", entry.ID, err)
		}
		profiles = append(profiles, p)
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})
	return profiles, nil
}

// FindByGroup returns the first profile (alphabetically by Name) whose Group
// field matches the given group string. Returns nil if no match is found.
func (s *ProfileStore) FindByGroup(group string) (*CredentialProfile, error) {
	profiles, err := s.List()
	if err != nil {
		return nil, err
	}
	for i := range profiles {
		if profiles[i].Group == group {
			return &profiles[i], nil
		}
	}
	return nil, nil
}

// store serializes p and writes it to the vault under its profile key.
func (s *ProfileStore) store(p CredentialProfile) error {
	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("serializing profile: %w", err)
	}
	if err := s.vault.Set(profilePrefix+p.ID, string(data)); err != nil {
		return fmt.Errorf("vault set: %w", err)
	}
	return nil
}
