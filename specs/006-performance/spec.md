# Feature Specification: Performance Optimization for Large Connection Sets

**Feature Branch**: `006-performance`
**Created**: 2026-02-18
**Status**: Draft
**Input**: User report: "With 480+ connections, it takes a while to delete an item or connect to an SSH session"

## Problem Statement

At 480+ connections, common operations (delete, connect, edit, favorite toggle) exhibit noticeable latency. Root cause analysis identified three critical bottlenecks and two high-impact issues.

## Root Cause Analysis

### Critical: scrypt key derivation on every Save (~100ms per password)

`crypto.Encrypt()` calls `scrypt.Key()` with N=32768 on every password, every save. With 480 connections, even 10 passwords = ~1s per save. Save is called on delete, connect, edit, move, favorite toggle — every common operation.

### Critical: Bulk delete calls Save N times

Visual-mode bulk delete calls `cfg.DeleteConnection(id)` per item, each triggering a full Save with encryption. Deleting 10 items = 10 full saves.

### Critical: Config save on every connect

`trackConnectionUsage()` saves the entire config just to update `LastConnectedAt`/`ConnectCount` on one connection.

### High: Filter/search O(n) per keystroke

Every keystroke during fuzzy filter runs 2-4 full passes through all connections with string allocation and fuzzy matching — no debounce.

### High: rebuildTable called excessively

21+ call sites, often chained with `groupFilteredConns()` + `countsByStatus()` — three separate O(n) passes that could be one.

## Acceptance Criteria

1. **Given** 480 connections with 50 encrypted passwords, **When** a single connection is deleted, **Then** the operation completes in under 200ms (vs multi-second today).
2. **Given** 20 connections selected in visual mode, **When** bulk delete is confirmed, **Then** config is saved exactly once (not 20 times).
3. **Given** a user connects to an SSH session, **When** tracking usage, **Then** the save does not block the connection flow.
4. **Given** 480 connections, **When** typing in the filter bar, **Then** results update within 50ms with no perceptible lag.

## Out of Scope

- Migrating from YAML to SQLite (separate feature)
- Connection indexing / lookup maps (low-impact at 480 items)
- Health check optimization (moderate impact, separate concern)
