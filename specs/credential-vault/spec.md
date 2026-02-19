# Credential Vault — Feature Specification

## Overview

Add CRUD credential profiles (credential vault) to Nexus. Credential profiles are named sets of credentials stored in the existing encrypted vault. They can be individually or bulk-applied to sessions, or associated with groups so sessions in that group automatically inherit the profile's credentials.

## User Stories

### US-1: Create Credential Profile
**As a** Nexus user, **I want to** create named credential profiles with username, password, identity file, and other credential fields, **so that** I can reuse the same credentials across multiple sessions without duplicating them.

### US-2: View/List Credential Profiles
**As a** Nexus user, **I want to** browse all my credential profiles in a dedicated vault list view, **so that** I can see what profiles exist, which groups they're associated with, and how many sessions use them.

### US-3: Edit Credential Profile
**As a** Nexus user, **I want to** edit an existing credential profile, **so that** when credentials change (e.g., password rotation), I update one profile and all linked sessions get the new credentials automatically.

### US-4: Delete Credential Profile
**As a** Nexus user, **I want to** delete a credential profile with a warning about orphaned references, **so that** I can clean up unused profiles safely.

### US-5: Assign Profile to Session
**As a** Nexus user, **I want to** assign a credential profile to one or more sessions, **so that** those sessions use the profile's credentials at connect time.

### US-6: Associate Profile with Group
**As a** Nexus user, **I want to** associate a credential profile with a group (via a group dropdown on the profile form), **so that** all sessions in that group automatically inherit the profile's credentials.

### US-7: Override Inherited Credentials
**As a** Nexus user, **I want to** override group-inherited credentials by assigning a different profile directly to a session or by setting inline credential fields on the session, **so that** I have fine-grained control.

### US-8: Save Credentials from Existing Session
**As a** Nexus user, **I want to** save the selected session's current credentials as a new profile, **so that** I can quickly create profiles from existing connections without re-entering data.

### US-9: See Credential Provenance
**As a** Nexus user, **I want to** see where each credential field comes from (direct, profile, or group) when viewing a session's details, **so that** I understand the resolution chain.

## Data Model

### CredentialProfile struct

```go
type CredentialProfile struct {
    ID           string    `json:"id"`           // UUID
    Name         string    `json:"name"`         // any string, must be unique
    Description  string    `json:"description,omitempty"`
    Username     string    `json:"username,omitempty"`
    Password     string    `json:"password,omitempty"`
    IdentityFile string    `json:"identity_file,omitempty"`
    Passphrase   string    `json:"passphrase,omitempty"`   // SSH key passphrase
    Domain       string    `json:"domain,omitempty"`        // RDP/Windows domain
    VNCPassword  string    `json:"vnc_password,omitempty"`
    Group        string    `json:"group,omitempty"`         // associated group name
    Tags         []string  `json:"tags,omitempty"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}
```

**Storage:** Serialized to JSON, stored via existing `Vault.Set(id, jsonString)`. No vault interface changes needed. A `ProfileStore` wrapper layer handles serialization.

### Connection struct change

Add one field:
```go
CredentialProfile string `yaml:"credential_profile,omitempty"` // profile name reference
```

### Group struct — NO changes

Group association is stored on the profile side (`CredentialProfile.Group` field), not on the Group struct.

## Credential Resolution (Inheritance)

Priority chain (highest wins):

```
1. Inline connection fields       → source: "direct"
2. Connection's credential profile → source: "profile:<name>"
3. Group's credential profile      → source: "group:<group-name>"
```

- A group's credential profile = any CredentialProfile whose `.Group` field matches the group name
- Partial overrides work: a profile with only `Username` set is valid; empty fields don't override lower layers
- Multiple profiles can reference the same group — if so, the first match is used (sorted by name, deterministic)

## TUI — Leader Key Group

Repurpose `leader + p` from "Password" to "Profiles":

| Key | Label | Action |
|-----|-------|--------|
| `leader + p + l` | List profiles | Open vault list view |
| `leader + p + c` | Create profile | Open profile creation form |
| `leader + p + e` | Edit profile | Open profile edit form (prompt for which profile) |
| `leader + p + d` | Delete profile | Delete profile (with confirmation + orphan warning) |
| `leader + p + a` | Assign to session | Assign a profile to the selected session(s) |
| `leader + p + s` | Save from session | Save selected session's credentials as a new profile |
| `leader + p + r` | Remove from session | Remove profile assignment from selected session(s) |
| `leader + p + w` | Who uses profile | Show which sessions/groups use a profile |

The existing password actions (`show-password`, `copy-password`) move — show-password is already available via `p` in detail view. Copy-password moves to `leader + d + p` (Data group) or similar.

## TUI — Vault List View

New `viewVault` view, accessible via `leader + p + l` or `:vault` command.

```
  Credential Vault                               3 profiles

  NAME              USERNAME       GROUP          USED BY   UPDATED
  prod-admin        admin          Production     12 conns  2d ago
  staging-deploy    deploy         Staging        5 conns   1w ago
  personal          john           -              3 conns   3w ago

  c:create | e:edit | d:delete | Enter:detail | Esc:back
```

Secrets (password, passphrase, VNC password) never shown in list view.

## TUI — Profile Form

Multi-group huh form (follows existing form.go pattern):

**Group 1 — Profile Info:** Name (required, unique), Description, Group (dropdown of existing groups + "None")
**Group 2 — Credentials:** Username, Password (EchoModePassword), Identity File, Passphrase (EchoModePassword), Domain, VNC Password (EchoModePassword)
**Group 3 — Tags:** Comma-separated tag input

## TUI — Commands

| Command | Description |
|---------|-------------|
| `:vault` | Open vault list view |
| `:vault add` | Open profile creation form |
| `:vault edit <name>` | Open profile edit form |
| `:vault del <name>` | Delete profile (with confirmation) |
| `:vault show <name>` | Show profile detail |
| `:vault rename <old> <new>` | Rename profile (updates all connection references) |
| `:cred save <name>` | Save selected session's credentials as new profile |
| `:cred apply <name>` | Assign profile to selected session(s) |
| `:cred clear` | Remove profile from selected session(s) |
| `:cred who <name>` | List sessions/groups using this profile |
| `:cred orphans` | List sessions with dangling profile references |

## Integration Points

### Connection Form
Add "Credential Profile" dropdown to the credentials tab. When selected, username/password fields show profile values as placeholders but remain editable for inline overrides.

### Connection Detail View
Show provenance per credential field:
```
  Credentials [profile: prod-admin]
  Username:     admin           from profile
  Password:     ****            from profile
  Identity:     ~/.ssh/my_key   direct
  Domain:       CORP            from group: Production
```

### Connect Flow
In `connectManaged()` — resolve credentials from profile before passing to session. Emit `EventCredentialUse` audit event.

### Connection List (wide mode)
Add "PROFILE" column showing resolved profile name (`*` suffix if inherited from group).

## Security

- All secrets stored via existing encrypted vault — no new encryption needed
- Secrets masked everywhere by default; `p` key reveals in detail views
- `EventCredentialUse` audit event emitted when profile credentials are used at connect time
- Profile deletion shows confirmation with list of affected sessions

## Edge Cases

- **Deleted profile:** Connection with dangling reference resolves as if no profile set. Detail view shows `[NOT FOUND]` warning.
- **Renamed profile:** `:vault rename` updates all `Connection.CredentialProfile` references atomically.
- **Empty profile:** Valid but has no effect on resolution.
- **Multiple profiles for same group:** First match alphabetically is used (deterministic).
- **Profile with no group:** Can only be assigned directly to sessions, not inherited.

## Acceptance Criteria

- [ ] CRUD operations for credential profiles via both leader keys and commands
- [ ] Vault list view with profile table
- [ ] Profile form with group dropdown
- [ ] Profile-to-session assignment (individual and bulk via visual mode)
- [ ] Group inheritance via profile's group field
- [ ] Credential resolution with correct priority chain
- [ ] Provenance display in connection detail view
- [ ] Credential profile dropdown in connection form
- [ ] Audit trail for credential use
- [ ] SQLite schema migration for credential_profile column on connections
- [ ] Orphan detection and warning on profile deletion
