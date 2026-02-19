# Credential Vault — Technical Plan

## Architecture Overview

The credential vault feature adds a `ProfileStore` layer on top of the existing `vault.Vault` interface, a `CredentialProfile` data type, credential resolution engine, and TUI views/commands.

```
┌─────────────────────────────────────────────────────┐
│  TUI Layer (leader keys, commands, views, forms)     │
├─────────────────────────────────────────────────────┤
│  Credential Resolution Engine                        │
│  (resolves profile chain: inline > conn > group)     │
├─────────────────────────────────────────────────────┤
│  ProfileStore (JSON serialization wrapper)            │
├─────────────────────────────────────────────────────┤
│  vault.Vault interface (unchanged)                   │
│  ├── InternalVault (AES-256-GCM encrypted JSON)      │
│  ├── PassVault (pass CLI)                            │
│  └── KeyringVault (OS keyring)                       │
└─────────────────────────────────────────────────────┘
```

## Implementation Phases

### Phase 1: Data Layer — ProfileStore + CredentialProfile

**Files to create:**
- `internal/vault/profile.go` — `CredentialProfile` struct + `ProfileStore` wrapper

**Files to modify:**
- `internal/config/connection.go` — Add `CredentialProfile string` field to `Connection`
- `internal/store/sqlite.go` — Add `credential_profile TEXT` column to connections table

**Approach:**
- `ProfileStore` wraps a `vault.Vault` and provides typed CRUD: `Create(profile)`, `Get(name)`, `Update(profile)`, `Delete(name)`, `List()`, `FindByGroup(group)`.
- Profiles are stored with key prefix `profile:` in the vault to distinguish from other vault entries.
- `CredentialProfile` struct lives in `internal/vault/profile.go` alongside the store.
- Profile names must be unique (enforced by `ProfileStore.Create`).
- SQLite migration: `ALTER TABLE connections ADD COLUMN credential_profile TEXT DEFAULT ''`.

### Phase 2: Credential Resolution Engine

**Files to create:**
- `internal/vault/resolve.go` — Resolution function + `ResolvedCredentials` type
- `internal/vault/resolve_test.go` — Unit tests

**Approach:**
- `ResolveCredentials(conn, vault ProfileStore) (ResolvedCredentials, map[string]string)` function.
- Returns resolved credential values + a `sources` map (`field -> "direct"|"profile:X"|"group:Y"`).
- Resolution order: (1) find group profile via `ProfileStore.FindByGroup(conn.Group)`, (2) find connection profile via `ProfileStore.Get(conn.CredentialProfile)`, (3) inline fields. Higher priority overwrites lower.
- Pure function with no side effects — easy to test.

### Phase 3: TUI — Vault Views

**Files to create:**
- `internal/tui/vault.go` — Vault list view model + vault detail view
- `internal/tui/vault_form.go` — Profile create/edit form (huh)

**Files to modify:**
- `internal/tui/app.go` — Add `viewVault` to viewKind enum, wire Update/View, add vault field to App, handle profile-related messages
- `internal/tui/leader.go` — Repurpose `leader + p` group from "Password" to "Profiles"
- `internal/tui/command.go` — Register `:vault` and `:cred` commands

**Approach:**
- Vault list view follows existing `tableModel` pattern (custom columns, cursor, hotkeys).
- Profile form uses `huh.NewForm` with 3 groups (Info, Credentials, Tags), same pattern as `form.go`.
- Group dropdown populated from `cfg.Groups` + "(None)" option.
- Messages: `VaultFormSubmitMsg`, `VaultDeleteMsg`, `CredAssignMsg`, `CredClearMsg`.

### Phase 4: Integration — Connection Form + Detail + Connect Flow

**Files to modify:**
- `internal/tui/form.go` — Add credential profile dropdown to credentials form group
- `internal/tui/detail.go` — Add provenance display to credential section
- `internal/tui/app.go` — In `connectManaged()`, resolve credentials via engine before passing to session; emit audit event
- `internal/tui/list.go` — Add PROFILE column in wide mode (optional, lower priority)

**Approach:**
- Connection form: add `huh.NewSelect` for credential profile before username/password fields.
- Detail view: call `ResolveCredentials()` and render each field with its source label.
- Connect flow: `app.resolveCredentials(conn)` called in `connectManaged()` before `session.NewManagedSession()`. The resolved username/password/identity override what gets passed to the session builder.
- Audit: `a.logAuditEvent(audit.EventCredentialUse, ...)` with profile name, connection name, source fields.

### Phase 5: Commands + Leader Keys + Bulk Operations

**Files to modify:**
- `internal/tui/leader.go` — Replace Password group items with Profile items
- `internal/tui/command.go` — Register `:vault` and `:cred` commands in `defaultCommands()`
- `internal/tui/app.go` — Handle new leader actions and commands in `handleLeaderAction()` and `handleCommand()`

**Approach:**
- Leader actions map to the same logic as commands (e.g., `"profile-list"` → open vault view, `"profile-create"` → open form).
- `:cred apply <name>` in visual mode applies to all selected connections.
- `:cred who <name>` shows a flash message or opens a filtered list.
- `:vault rename <old> <new>` iterates all connections, updates `CredentialProfile` field, saves config.
- Relocate `copy-password` to `leader + d + p` (Data > Copy password).

## Dependencies

- Phase 2 depends on Phase 1 (needs ProfileStore)
- Phase 3 depends on Phase 1 (needs ProfileStore for CRUD)
- Phase 4 depends on Phase 2 + Phase 3 (needs resolution engine + vault views)
- Phase 5 depends on Phase 3 (needs vault views to open)

**Parallelizable:** Phase 2 and Phase 3 can run in parallel after Phase 1.

## Risk Areas

- **App struct size:** `app.go` is already very large. New message handling adds to it. Mitigate by keeping vault logic in `vault.go` and only routing messages in `app.go`.
- **SQLite migration:** The `ALTER TABLE ADD COLUMN` is safe for SQLite but needs testing with both new and existing databases.
- **Password relocation:** Moving `copy-password` from `leader + p` to `leader + d` is a breaking change for muscle memory. Consider keeping it as an alias temporarily.
- **Vault not opened at startup:** The vault is currently never opened in `main.go`. It must be opened alongside the master password flow and threaded into `NewApp()`.

## Tech Stack (no changes)

All existing: Go, BubbleTea, huh, Lipgloss, SQLite, AES-256-GCM vault.
