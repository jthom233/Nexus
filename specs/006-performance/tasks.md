# Tasks: Performance Optimization

**Input**: Design documents from `/specs/006-performance/`
**Prerequisites**: plan.md (required), spec.md (required)

## Format: `[ID] Description`

---

## Phase 1: Encryption Key Caching (CRITICAL)

- [ ] T001 Add `EncryptWithKey(plaintext string, derivedKey []byte) (string, error)` to `internal/crypto/crypto.go` — AES-256-GCM encryption using a pre-derived 32-byte key directly (no scrypt). Fresh random nonce per call. Same output format as existing `Encrypt` (`ENC:base64(nonce+ciphertext)`), but without the salt prefix (salt was only needed for scrypt derivation).
- [ ] T002 Add `DeriveAndCacheKey(password []byte) []byte` helper — calls `scrypt.Key` once, returns the 32-byte derived key. Used at startup and by Save.
- [ ] T003 Add `DerivedKey []byte` field to `Config` struct (yaml:"-") to cache the derived AES key.
- [ ] T004 Update `Save()` in `internal/config/config.go` — if `cfg.DerivedKey` is set, call `crypto.EncryptWithKey` instead of `crypto.Encrypt`. Derive once at start of Save if `DerivedKey` is nil but `EncryptionKey` is set.
- [ ] T005 Update startup flow to populate `cfg.DerivedKey` when master password is provided — wherever `cfg.SetKey()` is called, also derive and cache.
- [ ] T006 Add unit test for `EncryptWithKey` / `Decrypt` round-trip in `internal/crypto/crypto_test.go`.

---

## Phase 2: Batch Save for Bulk Operations (CRITICAL)

- [ ] T007 Add `DeleteConnectionNoSave(id string)` to `internal/config/config.go` — removes connection from slice without calling Save.
- [ ] T008 Add `MoveConnectionNoSave(connID, targetGroup string) error` to `internal/config/config.go` — updates group field without calling Save. Auto-creates group if needed.
- [ ] T009 Update bulk delete handler in `internal/tui/app.go` — use `DeleteConnectionNoSave` in loop, call `Save` once after loop.
- [ ] T010 Update bulk move handler in `internal/tui/app.go` — use `MoveConnectionNoSave` in loop, call `Save` once after loop.

---

## Phase 3: Async Save for Usage Tracking (CRITICAL)

- [ ] T011 Update `trackConnectionUsage` in `internal/tui/app.go` — return a `tea.Cmd` that performs `config.Save` in the background instead of calling it synchronously.

---

## Phase 4: Filter Debounce (HIGH)

- [ ] T012 Add debounce mechanism to filter input in `internal/tui/app.go` — on filter keystroke, schedule a `tea.Tick` of 30ms. If another keystroke arrives before the tick fires, cancel and reschedule. Only call `applyFilter` when the tick fires.

---

## Phase 5: Verification

- [ ] T013 `go build ./...` passes
- [ ] T014 `go test ./...` passes
- [ ] T015 Manual test: delete single connection — verify sub-200ms
- [ ] T016 Manual test: bulk delete 10+ connections — verify single save
- [ ] T017 Manual test: connect to SSH — verify no perceptible delay
- [ ] T018 Manual test: type rapidly in filter bar — verify smooth response
