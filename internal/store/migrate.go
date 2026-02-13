package store

import "fmt"

// MigrateYAMLToSQLite copies all connections from a YAMLStore into a SQLiteStore.
// Existing connections in the SQLiteStore with the same ID are overwritten.
func MigrateYAMLToSQLite(yamlStore *YAMLStore, sqliteStore *SQLiteStore) error {
	conns, err := yamlStore.ListConnections()
	if err != nil {
		return fmt.Errorf("list yaml connections: %w", err)
	}

	for i, conn := range conns {
		// Use insertConn directly to preserve ordering.
		if err := sqliteStore.insertConn(conn, i); err != nil {
			return fmt.Errorf("insert connection %q: %w", conn.ID, err)
		}
	}
	return nil
}

// MigrateSQLiteToYAML copies all connections from a SQLiteStore into a YAMLStore.
// The YAMLStore's connection list is replaced entirely.
func MigrateSQLiteToYAML(sqliteStore *SQLiteStore, yamlStore *YAMLStore) error {
	conns, err := sqliteStore.ListConnections()
	if err != nil {
		return fmt.Errorf("list sqlite connections: %w", err)
	}

	yamlStore.cfg.Connections = conns
	return yamlStore.save()
}
