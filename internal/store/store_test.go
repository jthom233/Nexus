package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/dr4zz/nexus/internal/config"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

func testConnection(id, name, host, group string, tags []string) config.Connection {
	return config.Connection{
		ID:       id,
		Name:     name,
		Protocol: config.ProtoSSH,
		Host:     host,
		Port:     22,
		Username: "admin",
		Group:    group,
		Tags:     tags,
		Favorite: true,
	}
}

// ---------------------------------------------------------------------------
// YAMLStore tests
// ---------------------------------------------------------------------------

func newTempYAMLStore(t *testing.T) *YAMLStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	s, err := NewYAMLStore(path)
	if err != nil {
		t.Fatalf("NewYAMLStore: %v", err)
	}
	return s
}

func TestYAMLStore_CRUD(t *testing.T) {
	s := newTempYAMLStore(t)

	// Add
	c1 := testConnection("1", "server-a", "10.0.0.1", "prod", []string{"web"})
	if err := s.AddConnection(c1); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// List
	all, err := s.ListConnections()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(all))
	}

	// Get
	got, err := s.GetConnection("1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "server-a" {
		t.Fatalf("expected name server-a, got %s", got.Name)
	}

	// Update
	c1.Name = "server-a-updated"
	if err := s.UpdateConnection(c1); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = s.GetConnection("1")
	if got.Name != "server-a-updated" {
		t.Fatalf("expected updated name, got %s", got.Name)
	}

	// Delete
	if err := s.DeleteConnection("1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = s.GetConnection("1")
	if _, ok := err.(*ErrNotFound); !ok {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestYAMLStore_InsertAt(t *testing.T) {
	s := newTempYAMLStore(t)

	c1 := testConnection("1", "first", "10.0.0.1", "", nil)
	c2 := testConnection("2", "second", "10.0.0.2", "", nil)
	c3 := testConnection("3", "middle", "10.0.0.3", "", nil)

	_ = s.AddConnection(c1)
	_ = s.AddConnection(c2)
	_ = s.InsertConnectionAt(c3, 1) // insert between first and second

	all, _ := s.ListConnections()
	if len(all) != 3 {
		t.Fatalf("expected 3 connections, got %d", len(all))
	}
	if all[1].ID != "3" {
		t.Fatalf("expected middle at index 1, got %s", all[1].ID)
	}
}

func TestYAMLStore_Search(t *testing.T) {
	s := newTempYAMLStore(t)

	_ = s.AddConnection(testConnection("1", "web-prod", "10.0.0.1", "production", []string{"web", "nginx"}))
	_ = s.AddConnection(testConnection("2", "db-staging", "10.0.0.2", "staging", []string{"database"}))

	results, err := s.SearchConnections("prod")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].ID != "1" {
		t.Fatalf("unexpected search results: %v", results)
	}
}

func TestYAMLStore_Persistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	s1, _ := NewYAMLStore(path)
	_ = s1.AddConnection(testConnection("1", "persist-me", "10.0.0.1", "", nil))

	// Re-open and verify data survived.
	s2, err := NewYAMLStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := s2.GetConnection("1")
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got.Name != "persist-me" {
		t.Fatalf("expected persist-me, got %s", got.Name)
	}
}

// ---------------------------------------------------------------------------
// SQLiteStore tests
// ---------------------------------------------------------------------------

func newTempSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nexus.db")
	s, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSQLiteStore_CRUD(t *testing.T) {
	s := newTempSQLiteStore(t)

	now := time.Now().Truncate(time.Second)
	c1 := testConnection("1", "server-a", "10.0.0.1", "prod", []string{"web"})
	c1.LastConnectedAt = &now
	c1.ConnectCount = 5

	// Add
	if err := s.AddConnection(c1); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// List
	all, err := s.ListConnections()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(all))
	}

	// Get
	got, err := s.GetConnection("1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "server-a" {
		t.Fatalf("expected name server-a, got %s", got.Name)
	}
	if got.ConnectCount != 5 {
		t.Fatalf("expected connect_count 5, got %d", got.ConnectCount)
	}
	if got.LastConnectedAt == nil || !got.LastConnectedAt.Equal(now) {
		t.Fatalf("last_connected_at mismatch: got %v, want %v", got.LastConnectedAt, now)
	}
	if !reflect.DeepEqual(got.Tags, []string{"web"}) {
		t.Fatalf("tags mismatch: got %v", got.Tags)
	}

	// Update
	c1.Name = "server-a-updated"
	if err := s.UpdateConnection(c1); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = s.GetConnection("1")
	if got.Name != "server-a-updated" {
		t.Fatalf("expected updated name, got %s", got.Name)
	}

	// Delete
	if err := s.DeleteConnection("1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = s.GetConnection("1")
	if _, ok := err.(*ErrNotFound); !ok {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSQLiteStore_InsertAt(t *testing.T) {
	s := newTempSQLiteStore(t)

	c1 := testConnection("1", "first", "10.0.0.1", "", nil)
	c2 := testConnection("2", "second", "10.0.0.2", "", nil)
	c3 := testConnection("3", "middle", "10.0.0.3", "", nil)

	_ = s.AddConnection(c1)
	_ = s.AddConnection(c2)
	_ = s.InsertConnectionAt(c3, 1)

	all, _ := s.ListConnections()
	if len(all) != 3 {
		t.Fatalf("expected 3 connections, got %d", len(all))
	}
	if all[1].ID != "3" {
		t.Fatalf("expected middle at index 1, got %s", all[1].ID)
	}
}

func TestSQLiteStore_FTSSearch(t *testing.T) {
	s := newTempSQLiteStore(t)

	_ = s.AddConnection(testConnection("1", "web-prod", "10.0.0.1", "production", []string{"web", "nginx"}))
	_ = s.AddConnection(testConnection("2", "db-staging", "10.0.0.2", "staging", []string{"database"}))
	_ = s.AddConnection(testConnection("3", "cache-prod", "10.0.0.3", "production", []string{"redis"}))

	tests := []struct {
		query   string
		wantIDs []string
	}{
		{"prod", []string{"1", "3"}},           // matches group "production" and name "web-prod", "cache-prod"
		{"web", []string{"1"}},                  // matches name and tag
		{"staging", []string{"2"}},              // matches group
		{"10.0.0.2", []string{"2"}},             // matches host
		{"nonexistent", nil},                    // no match
	}

	for _, tc := range tests {
		results, err := s.SearchConnections(tc.query)
		if err != nil {
			t.Fatalf("Search(%q): %v", tc.query, err)
		}
		var gotIDs []string
		for _, r := range results {
			gotIDs = append(gotIDs, r.ID)
		}
		if !reflect.DeepEqual(gotIDs, tc.wantIDs) {
			t.Errorf("Search(%q): got IDs %v, want %v", tc.query, gotIDs, tc.wantIDs)
		}
	}
}

func TestSQLiteStore_NotFound(t *testing.T) {
	s := newTempSQLiteStore(t)

	_, err := s.GetConnection("nope")
	if _, ok := err.(*ErrNotFound); !ok {
		t.Fatalf("expected ErrNotFound for Get, got %T: %v", err, err)
	}

	err = s.UpdateConnection(config.Connection{ID: "nope"})
	if _, ok := err.(*ErrNotFound); !ok {
		t.Fatalf("expected ErrNotFound for Update, got %T: %v", err, err)
	}

	err = s.DeleteConnection("nope")
	if _, ok := err.(*ErrNotFound); !ok {
		t.Fatalf("expected ErrNotFound for Delete, got %T: %v", err, err)
	}
}

// ---------------------------------------------------------------------------
// Migration tests
// ---------------------------------------------------------------------------

func TestMigrateYAMLToSQLite(t *testing.T) {
	yamlStore := newTempYAMLStore(t)
	sqliteStore := newTempSQLiteStore(t)

	c1 := testConnection("1", "alpha", "10.0.0.1", "prod", []string{"web"})
	c2 := testConnection("2", "beta", "10.0.0.2", "staging", []string{"db"})
	_ = yamlStore.AddConnection(c1)
	_ = yamlStore.AddConnection(c2)

	if err := MigrateYAMLToSQLite(yamlStore, sqliteStore); err != nil {
		t.Fatalf("MigrateYAMLToSQLite: %v", err)
	}

	all, err := sqliteStore.ListConnections()
	if err != nil {
		t.Fatalf("List after migration: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(all))
	}
	if all[0].Name != "alpha" || all[1].Name != "beta" {
		t.Fatalf("order mismatch: %s, %s", all[0].Name, all[1].Name)
	}
}

func TestMigrateSQLiteToYAML(t *testing.T) {
	sqliteStore := newTempSQLiteStore(t)
	yamlPath := filepath.Join(t.TempDir(), "out.yaml")
	yamlStore, _ := NewYAMLStore(yamlPath)

	c1 := testConnection("1", "alpha", "10.0.0.1", "prod", []string{"web"})
	_ = sqliteStore.AddConnection(c1)

	if err := MigrateSQLiteToYAML(sqliteStore, yamlStore); err != nil {
		t.Fatalf("MigrateSQLiteToYAML: %v", err)
	}

	// Re-open YAML to verify persistence.
	yamlStore2, _ := NewYAMLStore(yamlPath)
	all, _ := yamlStore2.ListConnections()
	if len(all) != 1 || all[0].Name != "alpha" {
		t.Fatalf("unexpected YAML contents: %v", all)
	}
}

func TestMigrationRoundtrip(t *testing.T) {
	// YAML -> SQLite -> YAML and verify data integrity.
	yamlStore1 := newTempYAMLStore(t)
	sqliteStore := newTempSQLiteStore(t)
	yamlPath2 := filepath.Join(t.TempDir(), "roundtrip.yaml")
	yamlStore2, _ := NewYAMLStore(yamlPath2)

	now := time.Now().Truncate(time.Second)
	c := testConnection("rt-1", "roundtrip-server", "192.168.1.1", "dev", []string{"k8s", "monitoring"})
	c.IdentityFile = "~/.ssh/id_ed25519"
	c.LastConnectedAt = &now
	c.ConnectCount = 42
	c.PortForwards = []config.PortForward{
		{Type: config.PortForwardLocal, LocalAddr: "0.0.0.0:8080", RemoteAddr: "localhost:80"},
	}
	_ = yamlStore1.AddConnection(c)

	// YAML -> SQLite
	if err := MigrateYAMLToSQLite(yamlStore1, sqliteStore); err != nil {
		t.Fatalf("YAML->SQLite: %v", err)
	}

	// SQLite -> YAML
	if err := MigrateSQLiteToYAML(sqliteStore, yamlStore2); err != nil {
		t.Fatalf("SQLite->YAML: %v", err)
	}

	orig, _ := yamlStore1.ListConnections()
	final, _ := yamlStore2.ListConnections()

	if len(final) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(final))
	}

	o := orig[0]
	f := final[0]

	if o.ID != f.ID || o.Name != f.Name || o.Host != f.Host || o.Group != f.Group {
		t.Fatalf("basic fields mismatch:\norig:  %+v\nfinal: %+v", o, f)
	}
	if !reflect.DeepEqual(o.Tags, f.Tags) {
		t.Fatalf("tags mismatch: %v vs %v", o.Tags, f.Tags)
	}
	if !reflect.DeepEqual(o.PortForwards, f.PortForwards) {
		t.Fatalf("port_forwards mismatch: %v vs %v", o.PortForwards, f.PortForwards)
	}
	if o.ConnectCount != f.ConnectCount {
		t.Fatalf("connect_count mismatch: %d vs %d", o.ConnectCount, f.ConnectCount)
	}
	if f.LastConnectedAt == nil || !f.LastConnectedAt.Equal(*o.LastConnectedAt) {
		t.Fatalf("last_connected_at mismatch: %v vs %v", o.LastConnectedAt, f.LastConnectedAt)
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestSQLiteStore_EmptySearch(t *testing.T) {
	s := newTempSQLiteStore(t)
	results, err := s.SearchConnections("")
	if err != nil {
		t.Fatalf("empty search: %v", err)
	}
	if results != nil {
		t.Fatalf("expected nil for empty search, got %v", results)
	}
}

func TestSQLiteStore_Reopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nexus.db")

	s1, _ := NewSQLiteStore(path)
	_ = s1.AddConnection(testConnection("1", "persist", "10.0.0.1", "", nil))
	s1.Close()

	s2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	got, err := s2.GetConnection("1")
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got.Name != "persist" {
		t.Fatalf("expected persist, got %s", got.Name)
	}
}

func TestYAMLStore_DeleteNotFound(t *testing.T) {
	s := newTempYAMLStore(t)
	err := s.DeleteConnection("nonexistent")
	if _, ok := err.(*ErrNotFound); !ok {
		t.Fatalf("expected ErrNotFound, got %T: %v", err, err)
	}
}

// Ensure the unused import of os is referenced.
var _ = os.DevNull
