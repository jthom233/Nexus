# Implementation Plan: Performance Optimization

**Branch**: `006-performance` | **Date**: 2026-02-18 | **Spec**: [spec.md](spec.md)

## Summary

Four targeted optimizations to eliminate the primary bottlenecks for 480+ connection configs. No new dependencies, no architectural changes — surgical fixes to existing code.

## Technical Context

**Language/Version**: Go 1.25
**Key Files**:
- `internal/crypto/crypto.go` — scrypt-based Encrypt/Decrypt
- `internal/config/config.go` — Save/Load, CRUD methods
- `internal/tui/app.go` — TUI event loop, all handler chains
- `internal/tui/list.go` — list model, rebuildTable, groupFilteredConns
- `internal/tui/filter.go` — fuzzy filter model

## Optimization 1: Cache Derived Encryption Key (CRITICAL)

**Problem**: `Encrypt()` calls `scrypt.Key()` (N=32768, ~100ms) per password per save. With 50 passwords across 480 connections, each save takes ~5s of CPU.

**Solution**: Pre-derive the AES key once when the master password is set. Store the derived key alongside the salt. On subsequent `Encrypt()` calls, reuse the derived key with a fresh nonce/salt for the AES-GCM layer, but skip the scrypt derivation.

**Approach**: Refactor `Encrypt` to accept a pre-derived AES key directly, rather than raw key material that gets passed through scrypt each time. The encryption key stored in `Config.EncryptionKey` is already the master password bytes — we derive once at startup (in `DecryptPasswords`) and cache the derived key. For `Save`, we use the cached derived key directly with AES-GCM, using a fresh random nonce per encryption (which is already how GCM works — the nonce provides uniqueness, not the key derivation).

**Change**: Add `EncryptWithKey(plaintext string, derivedKey []byte) (string, error)` that skips scrypt and goes straight to AES-GCM. Update `Save()` to derive once, then call `EncryptWithKey` for each password.

**Impact**: Save goes from ~5s to ~5ms for 50 passwords.

## Optimization 2: Batch Save for Bulk Operations (CRITICAL)

**Problem**: `DeleteConnection()`, `MoveConnection()` each call `Save()` internally. Bulk operations call these N times = N saves.

**Solution**: Add batch variants that mutate in-memory state without saving, then save once at the end. Specifically:
- `DeleteConnectionNoSave(id string)` — removes from slice, no disk write
- `MoveConnectionNoSave(connID, targetGroup string)` — updates group, no disk write

The bulk handlers in `app.go` already have the pattern of doing work in a loop then rebuilding — they just need to call the no-save variants and do one `Save()` at the end.

**Impact**: Bulk delete of 20 items goes from 20 saves to 1 save.

## Optimization 3: Async Save for Usage Tracking (CRITICAL)

**Problem**: `trackConnectionUsage()` calls `Save()` synchronously, blocking the connect flow.

**Solution**: Fire the save as a background `tea.Cmd` so it doesn't block the connection establishment. The usage data (LastConnectedAt, ConnectCount) is non-critical metadata — if the app crashes before the save completes, losing a single count update is acceptable.

**Impact**: Connect is no longer blocked by config save.

## Optimization 4: Debounce Filter Input (HIGH)

**Problem**: Every keystroke in the filter bar triggers `applyFilter()` which does 2-4 O(n) passes over 480 connections.

**Solution**: Add a 30ms debounce to the filter. On each keystroke, cancel the pending filter timer and start a new one. Only run `applyFilter` when the timer fires. This coalesces rapid typing into a single filter pass.

**Impact**: Typing "ssh" triggers 1 filter pass instead of 3.
