// Package store provides a storage abstraction for Nexus connection management.
// It defines the Store interface and provides YAML and SQLite implementations.
package store

import (
	"github.com/dr4zz/nexus/internal/config"
)

// Store is the persistence interface for connections.
type Store interface {
	// ListConnections returns all connections in order.
	ListConnections() ([]config.Connection, error)

	// GetConnection returns a single connection by ID.
	GetConnection(id string) (config.Connection, error)

	// AddConnection appends a new connection.
	AddConnection(conn config.Connection) error

	// UpdateConnection replaces an existing connection by ID.
	UpdateConnection(conn config.Connection) error

	// DeleteConnection removes a connection by ID.
	DeleteConnection(id string) error

	// InsertConnectionAt inserts a connection at the given position.
	// If index is out of range the connection is appended.
	InsertConnectionAt(conn config.Connection, index int) error

	// SearchConnections returns connections matching the query string.
	// The query is matched against name, host, group, and tags.
	SearchConnections(query string) ([]config.Connection, error)

	// Close releases any resources held by the store.
	Close() error
}

// ErrNotFound is returned when a requested connection does not exist.
type ErrNotFound struct {
	ID string
}

func (e *ErrNotFound) Error() string {
	return "connection not found: " + e.ID
}
