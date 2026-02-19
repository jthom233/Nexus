package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/hooks"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store using a SQLite database (pure-Go, no CGo).
type SQLiteStore struct {
	db   *sql.DB
	path string
}

// NewSQLiteStore opens (or creates) a SQLite-backed store at the given path.
// The schema is auto-created on first open.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	s := &SQLiteStore{db: db, path: path}
	if err := s.createSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	if err := s.migrateSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return s, nil
}

// createSchema creates the connections table and FTS5 virtual table if they
// don't already exist.
func (s *SQLiteStore) createSchema() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS connections (
	id            TEXT PRIMARY KEY,
	position      INTEGER NOT NULL,
	name          TEXT NOT NULL DEFAULT '',
	protocol      TEXT NOT NULL DEFAULT 'ssh',
	host          TEXT NOT NULL DEFAULT '',
	port          INTEGER NOT NULL DEFAULT 0,
	username      TEXT NOT NULL DEFAULT '',
	password      TEXT NOT NULL DEFAULT '',
	identity_file TEXT NOT NULL DEFAULT '',
	proxy_jump    TEXT NOT NULL DEFAULT '',
	proxy_command TEXT NOT NULL DEFAULT '',
	port_forwards TEXT NOT NULL DEFAULT '[]',
	domain        TEXT NOT NULL DEFAULT '',
	grp           TEXT NOT NULL DEFAULT '',
	tags          TEXT NOT NULL DEFAULT '[]',
	rdp_options   TEXT NOT NULL DEFAULT '{}',
	vnc_password       TEXT NOT NULL DEFAULT '',
	credential_profile TEXT NOT NULL DEFAULT '',
	hooks              TEXT NOT NULL DEFAULT '{}',
	favorite      INTEGER NOT NULL DEFAULT 0,
	last_connected_at TEXT NOT NULL DEFAULT '',
	connect_count INTEGER NOT NULL DEFAULT 0
);

CREATE VIRTUAL TABLE IF NOT EXISTS connections_fts USING fts5(
	id UNINDEXED,
	name,
	host,
	grp,
	tags,
	content='connections',
	content_rowid='rowid'
);

-- Triggers to keep FTS in sync with the main table.
CREATE TRIGGER IF NOT EXISTS connections_ai AFTER INSERT ON connections BEGIN
	INSERT INTO connections_fts(rowid, id, name, host, grp, tags)
	VALUES (new.rowid, new.id, new.name, new.host, new.grp, new.tags);
END;

CREATE TRIGGER IF NOT EXISTS connections_ad AFTER DELETE ON connections BEGIN
	INSERT INTO connections_fts(connections_fts, rowid, id, name, host, grp, tags)
	VALUES ('delete', old.rowid, old.id, old.name, old.host, old.grp, old.tags);
END;

CREATE TRIGGER IF NOT EXISTS connections_au AFTER UPDATE ON connections BEGIN
	INSERT INTO connections_fts(connections_fts, rowid, id, name, host, grp, tags)
	VALUES ('delete', old.rowid, old.id, old.name, old.host, old.grp, old.tags);
	INSERT INTO connections_fts(rowid, id, name, host, grp, tags)
	VALUES (new.rowid, new.id, new.name, new.host, new.grp, new.tags);
END;
`
	_, err := s.db.Exec(ddl)
	return err
}

// migrateSchema applies incremental ALTER TABLE statements to bring existing
// databases up to the current schema. Each statement is idempotent: SQLite
// returns "duplicate column name" when the column already exists, which we
// treat as success.
func (s *SQLiteStore) migrateSchema() error {
	migrations := []string{
		`ALTER TABLE connections ADD COLUMN credential_profile TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range migrations {
		if _, err := s.db.Exec(stmt); err != nil {
			// SQLite reports duplicate column additions as an error whose message
			// contains "duplicate column name". Ignore those — the column is
			// already present (fresh DB created by createSchema, or a previous run).
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("migration %q: %w", stmt, err)
		}
	}
	return nil
}

// DB returns the underlying database (useful for migrations / testing).
func (s *SQLiteStore) DB() *sql.DB {
	return s.db
}

// ---------------------------------------------------------------------------
// Store interface
// ---------------------------------------------------------------------------

func (s *SQLiteStore) ListConnections() ([]config.Connection, error) {
	rows, err := s.db.Query("SELECT * FROM connections ORDER BY position ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConnections(rows)
}

func (s *SQLiteStore) GetConnection(id string) (config.Connection, error) {
	row := s.db.QueryRow("SELECT * FROM connections WHERE id = ?", id)
	return scanConnection(row)
}

func (s *SQLiteStore) AddConnection(conn config.Connection) error {
	// Append: position = max(position) + 1
	var maxPos sql.NullInt64
	if err := s.db.QueryRow("SELECT MAX(position) FROM connections").Scan(&maxPos); err != nil {
		return err
	}
	pos := int64(0)
	if maxPos.Valid {
		pos = maxPos.Int64 + 1
	}
	return s.insertConn(conn, int(pos))
}

func (s *SQLiteStore) UpdateConnection(conn config.Connection) error {
	// Preserve existing position.
	var pos int
	err := s.db.QueryRow("SELECT position FROM connections WHERE id = ?", conn.ID).Scan(&pos)
	if err != nil {
		if err == sql.ErrNoRows {
			return &ErrNotFound{ID: conn.ID}
		}
		return err
	}

	tagsJSON, _ := json.Marshal(conn.Tags)
	pfJSON, _ := json.Marshal(conn.PortForwards)
	rdpJSON, _ := json.Marshal(conn.RDPOptions)
	hooksJSON, _ := json.Marshal(conn.Hooks)
	lastConn := formatTime(conn.LastConnectedAt)
	fav := boolToInt(conn.Favorite)

	_, err = s.db.Exec(`UPDATE connections SET
		position=?, name=?, protocol=?, host=?, port=?, username=?, password=?,
		identity_file=?, proxy_jump=?, proxy_command=?, port_forwards=?, domain=?,
		grp=?, tags=?, rdp_options=?, vnc_password=?, credential_profile=?, hooks=?, favorite=?,
		last_connected_at=?, connect_count=?
		WHERE id=?`,
		pos, conn.Name, string(conn.Protocol), conn.Host, conn.Port,
		conn.Username, conn.Password, conn.IdentityFile, conn.ProxyJump,
		conn.ProxyCommand, string(pfJSON), conn.Domain, conn.Group,
		string(tagsJSON), string(rdpJSON), conn.VNCPassword, conn.CredentialProfile, string(hooksJSON),
		fav, lastConn, conn.ConnectCount, conn.ID,
	)
	return err
}

func (s *SQLiteStore) DeleteConnection(id string) error {
	res, err := s.db.Exec("DELETE FROM connections WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return &ErrNotFound{ID: id}
	}
	return nil
}

func (s *SQLiteStore) InsertConnectionAt(conn config.Connection, index int) error {
	// Count existing rows.
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM connections").Scan(&count); err != nil {
		return err
	}

	pos := index
	if pos < 0 || pos >= count {
		pos = count // append
	} else {
		// Shift everything at pos and above up by 1.
		if _, err := s.db.Exec("UPDATE connections SET position = position + 1 WHERE position >= ?", pos); err != nil {
			return err
		}
	}
	return s.insertConn(conn, pos)
}

func (s *SQLiteStore) SearchConnections(query string) ([]config.Connection, error) {
	// Use FTS5 MATCH with a prefix query for a nice search experience.
	// Escape special FTS5 characters by wrapping each term in double quotes.
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return nil, nil
	}

	var ftsTerms []string
	for _, t := range terms {
		// Quote the term and add a wildcard for prefix matching.
		escaped := strings.ReplaceAll(t, `"`, `""`)
		ftsTerms = append(ftsTerms, fmt.Sprintf(`"%s"*`, escaped))
	}
	ftsQuery := strings.Join(ftsTerms, " ")

	rows, err := s.db.Query(`
		SELECT c.* FROM connections c
		JOIN connections_fts f ON c.id = f.id
		WHERE connections_fts MATCH ?
		ORDER BY c.position ASC`, ftsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConnections(rows)
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (s *SQLiteStore) insertConn(conn config.Connection, pos int) error {
	tagsJSON, _ := json.Marshal(conn.Tags)
	pfJSON, _ := json.Marshal(conn.PortForwards)
	rdpJSON, _ := json.Marshal(conn.RDPOptions)
	hooksJSON, _ := json.Marshal(conn.Hooks)
	lastConn := formatTime(conn.LastConnectedAt)
	fav := boolToInt(conn.Favorite)

	_, err := s.db.Exec(`INSERT INTO connections (
		id, position, name, protocol, host, port, username, password,
		identity_file, proxy_jump, proxy_command, port_forwards, domain,
		grp, tags, rdp_options, vnc_password, credential_profile, hooks, favorite,
		last_connected_at, connect_count
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		conn.ID, pos, conn.Name, string(conn.Protocol), conn.Host, conn.Port,
		conn.Username, conn.Password, conn.IdentityFile, conn.ProxyJump,
		conn.ProxyCommand, string(pfJSON), conn.Domain, conn.Group,
		string(tagsJSON), string(rdpJSON), conn.VNCPassword, conn.CredentialProfile, string(hooksJSON),
		fav, lastConn, conn.ConnectCount,
	)
	return err
}

// scanConnections reads all rows into a Connection slice.
func scanConnections(rows *sql.Rows) ([]config.Connection, error) {
	var out []config.Connection
	for rows.Next() {
		c, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// scanConnection reads a single row from QueryRow.
func scanConnection(row *sql.Row) (config.Connection, error) {
	var (
		c          config.Connection
		pos        int
		proto      string
		tagsJSON   string
		pfJSON     string
		rdpJSON    string
		hooksJSON  string
		fav        int
		lastConn   string
	)

	err := row.Scan(
		&c.ID, &pos, &c.Name, &proto, &c.Host, &c.Port,
		&c.Username, &c.Password, &c.IdentityFile, &c.ProxyJump,
		&c.ProxyCommand, &pfJSON, &c.Domain, &c.Group,
		&tagsJSON, &rdpJSON, &c.VNCPassword, &c.CredentialProfile, &hooksJSON,
		&fav, &lastConn, &c.ConnectCount,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return config.Connection{}, &ErrNotFound{}
		}
		return config.Connection{}, err
	}

	c.Protocol = config.Protocol(proto)
	c.Favorite = fav != 0
	c.LastConnectedAt = parseTime(lastConn)

	_ = json.Unmarshal([]byte(tagsJSON), &c.Tags)
	_ = json.Unmarshal([]byte(pfJSON), &c.PortForwards)
	_ = json.Unmarshal([]byte(rdpJSON), &c.RDPOptions)
	_ = json.Unmarshal([]byte(hooksJSON), &c.Hooks)

	return c, nil
}

// scanRow scans a single row from a Rows cursor.
func scanRow(rows *sql.Rows) (config.Connection, error) {
	var (
		c          config.Connection
		pos        int
		proto      string
		tagsJSON   string
		pfJSON     string
		rdpJSON    string
		hooksJSON  string
		fav        int
		lastConn   string
	)

	err := rows.Scan(
		&c.ID, &pos, &c.Name, &proto, &c.Host, &c.Port,
		&c.Username, &c.Password, &c.IdentityFile, &c.ProxyJump,
		&c.ProxyCommand, &pfJSON, &c.Domain, &c.Group,
		&tagsJSON, &rdpJSON, &c.VNCPassword, &c.CredentialProfile, &hooksJSON,
		&fav, &lastConn, &c.ConnectCount,
	)
	if err != nil {
		return config.Connection{}, err
	}

	c.Protocol = config.Protocol(proto)
	c.Favorite = fav != 0
	c.LastConnectedAt = parseTime(lastConn)

	_ = json.Unmarshal([]byte(tagsJSON), &c.Tags)
	_ = json.Unmarshal([]byte(pfJSON), &c.PortForwards)
	_ = json.Unmarshal([]byte(rdpJSON), &c.RDPOptions)
	_ = json.Unmarshal([]byte(hooksJSON), &c.Hooks)

	return c, nil
}

func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func parseTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Ensure unused import is referenced.
var _ hooks.Hooks
