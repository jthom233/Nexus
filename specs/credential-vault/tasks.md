# Credential Vault — Task Breakdown

## Phase 1: Data Layer

### T001: Create CredentialProfile struct and ProfileStore
**File:** `internal/vault/profile.go` (new)
**Description:** Define `CredentialProfile` struct with all fields (ID, Name, Description, Username, Password, IdentityFile, Passphrase, Domain, VNCPassword, Group, Tags, CreatedAt, UpdatedAt). Implement `ProfileStore` wrapper that takes a `vault.Vault` and provides:
- `Create(profile CredentialProfile) error` — validates unique name, generates UUID, serializes to JSON, calls `vault.Set("profile:"+id, json)`
- `Get(name string) (*CredentialProfile, error)` — scans all `profile:*` entries, finds by name
- `GetByID(id string) (*CredentialProfile, error)` — direct lookup by ID
- `Update(profile CredentialProfile) error` — validates name uniqueness (excluding self), updates vault entry
- `Delete(name string) error` — finds by name, calls `vault.Delete(id)`
- `List() ([]CredentialProfile, error)` — returns all profiles, sorted by name
- `FindByGroup(group string) (*CredentialProfile, error)` — returns first profile (alphabetically by name) whose Group matches

Add a `"profile:"` key prefix convention to distinguish profile entries from other vault entries.

**Depends on:** Nothing
**Status:** [ ] Pending

### T002: Add CredentialProfile field to Connection struct
**File:** `internal/config/connection.go`
**Description:** Add `CredentialProfile string` field with `yaml:"credential_profile,omitempty" json:"credential_profile,omitempty"` tags to the `Connection` struct. Ensure it's included in any Clone/Copy methods.
**Depends on:** Nothing
**Status:** [ ] Pending

### T003: SQLite schema migration for credential_profile column
**File:** `internal/store/sqlite.go`
**Description:** Add `credential_profile TEXT DEFAULT ''` column to the `connections` table. Update `createSchema()` to include the column for new databases. Add migration logic for existing databases (ALTER TABLE ADD COLUMN). Update `insertConnection()`, `updateConnection()`, and `scanConnection()` to read/write the new column.
**Depends on:** T002
**Status:** [ ] Pending

### T004: YAML store support for credential_profile
**File:** `internal/store/yaml.go`
**Description:** Ensure the YAML store correctly serializes/deserializes the new `CredentialProfile` field on Connection. This should work automatically via the yaml tag, but verify in the round-trip path.
**Depends on:** T002
**Status:** [ ] Pending

---

## Phase 2: Credential Resolution Engine

### T005: Implement credential resolution function
**File:** `internal/vault/resolve.go` (new)
**Description:** Implement `ResolveCredentials(conn config.Connection, store *ProfileStore) (ResolvedCredentials, map[string]string, error)`.

`ResolvedCredentials` struct mirrors credential fields: Username, Password, IdentityFile, Passphrase, Domain, VNCPassword.

Resolution algorithm:
1. Start with empty result
2. Layer 1 (lowest priority): If `conn.Group != ""`, call `store.FindByGroup(conn.Group)`. Merge non-empty fields, record source as `"group:<name>"`.
3. Layer 2: If `conn.CredentialProfile != ""`, call `store.Get(conn.CredentialProfile)`. Merge non-empty fields, record source as `"profile:<name>"`.
4. Layer 3 (highest priority): Merge non-empty inline fields from `conn` (Username, Password, IdentityFile, Domain, VNCPassword). Record source as `"direct"`.

Return the resolved credentials and the sources map.

**Depends on:** T001
**Status:** [ ] Pending

### T006: Unit tests for credential resolution
**File:** `internal/vault/resolve_test.go` (new)
**Description:** Test cases:
- No profile, no group → inline fields only
- Connection profile only → profile fields merged
- Group profile only → group profile fields merged
- All three layers → correct priority (inline > conn profile > group profile)
- Partial profiles → empty fields don't override lower layers
- Missing/dangling profile reference → graceful fallback
- Multiple profiles for same group → first alphabetically used
- Empty profile → no effect

**Depends on:** T005
**Status:** [ ] Pending

---

## Phase 3: TUI — Vault Views (parallel with Phase 2)

### T007: Open vault at startup and thread into App
**Files:** `cmd/nexus/main.go`, `internal/tui/app.go`
**Description:** After master password is collected in `handleMasterPassword()`, open the vault via `vault.Open(cfg.Settings.Vault, masterPassword)`. Thread the opened `vault.Vault` into `NewApp()`. Add a `vault vault.Vault` and `profileStore *vault.ProfileStore` field to the `App` struct. Initialize `ProfileStore` wrapping the vault.
**Depends on:** T001
**Status:** [ ] Pending

### T008: Vault list view model
**File:** `internal/tui/vault.go` (new)
**Description:** Implement `vaultModel` following the existing `tableModel` pattern:
- Columns: NAME, USERNAME, GROUP, USED BY, UPDATED
- USED BY: count connections where `CredentialProfile == profile.Name` plus connections in the profile's group
- Hotkeys: `c` (create), `e` (edit), `d` (delete), `Enter` (detail), `Esc` (back)
- Secrets never shown in list
- Add `viewVault` to the `viewKind` enum in app.go

**Depends on:** T001, T007
**Status:** [ ] Pending

### T009: Vault detail view
**File:** `internal/tui/vault.go`
**Description:** Implement profile detail rendering within `vaultModel` (or a sub-model). Shows all profile fields. Password/Passphrase/VNCPassword masked with `****`, `p` key toggles reveal. Shows "Used By" section listing connections and groups. Shows tags, created/updated timestamps.
**Depends on:** T008
**Status:** [ ] Pending

### T010: Profile create/edit form
**File:** `internal/tui/vault_form.go` (new)
**Description:** Implement `vaultFormModel` using `huh.NewForm`:
- Group 1 (Profile Info): Name (required, validated unique on submit), Description, Group (huh.NewSelect with cfg.Groups + "None")
- Group 2 (Credentials): Username, Password (EchoModePassword), Identity File, Passphrase (EchoModePassword), Domain, VNC Password (EchoModePassword)
- Group 3 (Tags): comma-separated text input

On submit, send `VaultFormSubmitMsg{Profile, IsEdit}` to App.
For edit mode, pre-populate fields from existing profile.

**Depends on:** T007
**Status:** [ ] Pending

### T011: Wire vault views into App
**File:** `internal/tui/app.go`
**Description:** Add `vaultModel` and `vaultFormModel` fields to App. Handle:
- `viewVault` in `App.View()` and `App.Update()`
- `VaultFormSubmitMsg` → create/update profile via ProfileStore, flash success message
- `VaultDeleteMsg` → delete profile, show confirmation with orphan count
- Navigation: `pushView(viewVault)` and `pushView(viewVaultForm)`

**Depends on:** T008, T009, T010
**Status:** [ ] Pending

---

## Phase 4: Integration

### T012: Repurpose leader + p for Profiles
**File:** `internal/tui/leader.go`
**Description:** Replace the `Key: "p", Label: "Password"` leader group with:
```go
{Key: "p", Label: "Profiles", Items: []LeaderItem{
    {Key: "l", Label: "List profiles", Action: "profile-list"},
    {Key: "c", Label: "Create profile", Action: "profile-create"},
    {Key: "e", Label: "Edit profile", Action: "profile-edit"},
    {Key: "d", Label: "Delete profile", Action: "profile-delete"},
    {Key: "a", Label: "Assign to session", Action: "profile-assign"},
    {Key: "s", Label: "Save from session", Action: "profile-save-from"},
    {Key: "r", Label: "Remove from session", Action: "profile-remove"},
    {Key: "w", Label: "Who uses profile", Action: "profile-who"},
}}
```
Move `copy-password` to `leader + d + p` (Data group).

**Depends on:** T011
**Status:** [ ] Pending

### T013: Handle leader actions in App
**File:** `internal/tui/app.go`
**Description:** In `handleLeaderAction()`, add cases for all new profile actions:
- `"profile-list"` → `pushView(viewVault)`
- `"profile-create"` → open vault form in create mode
- `"profile-edit"` → prompt for profile selection, open form in edit mode
- `"profile-delete"` → prompt for profile selection, confirm, delete
- `"profile-assign"` → prompt for profile selection, assign to selected connection(s)
- `"profile-save-from"` → capture selected connection's creds, open pre-filled form
- `"profile-remove"` → clear `CredentialProfile` on selected connection(s)
- `"profile-who"` → prompt for profile, show usage flash or filtered list

**Depends on:** T011, T012
**Status:** [ ] Pending

### T014: Register `:vault` and `:cred` commands
**File:** `internal/tui/command.go`
**Description:** Add to `defaultCommands()`:
- `{Name: "vault", Aliases: nil, Description: "Manage credential profiles", ArgSpec: "[add|edit|del|show|rename] [args]"}`
- `{Name: "cred", Aliases: nil, Description: "Credential shortcuts", ArgSpec: "[save|apply|clear|who|orphans] [args]"}`

Handle in `app.handleCommand()` — parse subcommands and route to appropriate logic.

**Depends on:** T011
**Status:** [ ] Pending

### T015: Credential resolution in connect flow
**File:** `internal/tui/app.go`
**Description:** In `connectManaged()` (and `connectByID()` if separate), before passing credentials to `session.NewManagedSession()`:
1. Call `vault.ResolveCredentials(conn, app.profileStore)`
2. Use resolved Username, Password, IdentityFile instead of inline `conn` fields
3. Emit `audit.EventCredentialUse` if any credential came from a profile

**Depends on:** T005, T007
**Status:** [ ] Pending

### T016: Add credential profile dropdown to connection form
**File:** `internal/tui/form.go`
**Description:** In the credentials form group, add a `huh.NewSelect` for "Credential Profile" before the username/password fields. Options populated from `profileStore.List()` + "(none)". Selected value stored in `Connection.CredentialProfile`.

**Depends on:** T007
**Status:** [ ] Pending

### T017: Provenance display in connection detail view
**File:** `internal/tui/detail.go`
**Description:** In the credential section of the detail view, call `ResolveCredentials()` and display each field with its source:
```
Username:     admin           from profile
Password:     ****            from profile
Identity:     ~/.ssh/key      direct
```
Handle dangling references gracefully: show `[NOT FOUND]` warning.

**Depends on:** T005, T007
**Status:** [ ] Pending

### T018: Profile column in connection list (wide mode)
**File:** `internal/tui/list.go`
**Description:** In wide mode, add a "PROFILE" column showing the resolved profile name. Append `*` suffix if the profile is inherited from the group rather than directly assigned.

**Depends on:** T005
**Status:** [ ] Pending

---

## Phase 5: Polish

### T019: Profile deletion with orphan warning
**File:** `internal/tui/app.go`
**Description:** When deleting a profile, count connections referencing it (direct + group). Show confirmation dialog: "Profile 'X' is used by N connections. Delete anyway?" On confirm, delete profile. Dangling references remain (resolved gracefully by T005/T017).

**Depends on:** T011
**Status:** [ ] Pending

### T020: `:vault rename` with reference update
**File:** `internal/tui/app.go`
**Description:** `:vault rename <old> <new>` — rename profile in vault, then iterate all connections in cfg, update any `CredentialProfile == old` to `new`, save config. Atomic operation.

**Depends on:** T014
**Status:** [ ] Pending

### T021: `:cred orphans` command
**File:** `internal/tui/app.go`
**Description:** List all connections whose `CredentialProfile` field references a profile name that doesn't exist in the vault. Display as flash message or filtered list.

**Depends on:** T014
**Status:** [ ] Pending

---

## Dependency Graph

```
T001 ──┬──> T005 ──> T006
       │          ╲
       ├──> T007 ──┬──> T008 ──> T009 ──┐
       │           ├──> T010 ────────────┤
T002 ──┤           │                     ╲
       ├──> T003   └──────────────────> T011 ──> T012 ──> T013
       ├──> T004                          │ ╲
       │                                  ├──> T014 ──> T020, T021
       │    T005 + T007 ──> T015          ├──> T019
       │    T005 + T007 ──> T017          │
       │    T007 ──> T016                 │
       │    T005 ──> T018                 │
       └──────────────────────────────────┘
```

## Parallel Opportunities

- **T001 + T002 + T004**: All independent, run in parallel
- **T003**: Depends on T002 only
- **T005 + T007 + T008 + T010**: After T001 completes, Phase 2 (T005-T006) and Phase 3 (T007-T010) run in parallel
- **T015 + T016 + T017 + T018**: All can run in parallel after their respective dependencies
