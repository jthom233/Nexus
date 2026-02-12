package importexport

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dr4zz/nexus/internal/config"
)

// Importer reads connections from a serialised format.
type Importer interface {
	Import(reader io.Reader) ([]config.Connection, error)
}

// MergeStrategy controls how imported connections interact with existing ones.
type MergeStrategy int

const (
	// MergeSkip leaves existing connections untouched and only adds new ones.
	MergeSkip MergeStrategy = iota
	// MergeOverwrite replaces existing connections that share the same name.
	MergeOverwrite
	// MergeRename appends a numeric suffix to imported connections whose name
	// already exists, so both versions are kept.
	MergeRename
)

// ImportResult tallies the outcome of an import operation.
type ImportResult struct {
	Created int
	Updated int
	Skipped int
}

// ---------- CSV Importer ----------

// CSVImporter parses CSV with a header row mapping to connection fields.
// Required columns: name, host.  Optional: port, protocol, username, group, tags.
type CSVImporter struct{}

func (c *CSVImporter) Import(reader io.Reader) ([]config.Connection, error) {
	r := csv.NewReader(reader)
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1 // allow ragged rows

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv parse error: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("csv must contain a header row and at least one data row")
	}

	// Build column index from header.
	header := records[0]
	idx := make(map[string]int, len(header))
	for i, col := range header {
		idx[strings.TrimSpace(strings.ToLower(col))] = i
	}

	// name and host are required.
	if _, ok := idx["name"]; !ok {
		return nil, fmt.Errorf("csv header missing required column: name")
	}
	if _, ok := idx["host"]; !ok {
		return nil, fmt.Errorf("csv header missing required column: host")
	}

	var conns []config.Connection
	for lineNo, row := range records[1:] {
		conn, err := csvRowToConnection(idx, row, lineNo+2)
		if err != nil {
			return nil, err
		}
		conns = append(conns, conn)
	}

	return conns, nil
}

func csvCol(idx map[string]int, row []string, col string) string {
	i, ok := idx[col]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func csvRowToConnection(idx map[string]int, row []string, lineNo int) (config.Connection, error) {
	name := csvCol(idx, row, "name")
	host := csvCol(idx, row, "host")
	if name == "" || host == "" {
		return config.Connection{}, fmt.Errorf("csv line %d: name and host are required", lineNo)
	}

	proto := config.Protocol(strings.ToLower(csvCol(idx, row, "protocol")))
	if proto == "" {
		proto = config.ProtoSSH
	}
	if !proto.Valid() {
		return config.Connection{}, fmt.Errorf("csv line %d: invalid protocol %q", lineNo, proto)
	}

	port := 0
	if ps := csvCol(idx, row, "port"); ps != "" {
		p, err := strconv.Atoi(ps)
		if err != nil {
			return config.Connection{}, fmt.Errorf("csv line %d: invalid port %q", lineNo, ps)
		}
		port = p
	}

	var tags []string
	if t := csvCol(idx, row, "tags"); t != "" {
		for _, tag := range strings.Split(t, ";") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}

	conn := config.Connection{
		ID:       sanitizeID(name),
		Name:     name,
		Protocol: proto,
		Host:     host,
		Port:     port,
		Username: csvCol(idx, row, "username"),
		Group:    csvCol(idx, row, "group"),
		Tags:     tags,
	}
	return conn, nil
}

// ---------- JSON Importer ----------

// JSONImporter parses a JSON array of connection objects.
type JSONImporter struct{}

// jsonConnection is a transfer struct for JSON import with looser typing.
type jsonConnection struct {
	Name     string   `json:"name"`
	Host     string   `json:"host"`
	Port     int      `json:"port,omitempty"`
	Protocol string   `json:"protocol,omitempty"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`
	Group    string   `json:"group,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

func (j *JSONImporter) Import(reader io.Reader) ([]config.Connection, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("json read error: %w", err)
	}

	var raw []jsonConnection
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("json parse error: %w", err)
	}

	var conns []config.Connection
	for i, r := range raw {
		if r.Name == "" || r.Host == "" {
			return nil, fmt.Errorf("json entry %d: name and host are required", i)
		}
		proto := config.Protocol(strings.ToLower(r.Protocol))
		if proto == "" {
			proto = config.ProtoSSH
		}
		if !proto.Valid() {
			return nil, fmt.Errorf("json entry %d: invalid protocol %q", i, r.Protocol)
		}
		conns = append(conns, config.Connection{
			ID:       sanitizeID(r.Name),
			Name:     r.Name,
			Protocol: proto,
			Host:     r.Host,
			Port:     r.Port,
			Username: r.Username,
			Password: r.Password,
			Group:    r.Group,
			Tags:     r.Tags,
		})
	}
	return conns, nil
}

// ---------- Merge ----------

// ApplyMerge combines imported connections into the existing list using the
// given strategy and returns the result counts together with the merged slice.
func ApplyMerge(existing, imported []config.Connection, strategy MergeStrategy) ([]config.Connection, ImportResult) {
	// Build a lookup of existing connections by name.
	byName := make(map[string]int, len(existing))
	for i, c := range existing {
		byName[strings.ToLower(c.Name)] = i
	}

	result := ImportResult{}
	merged := make([]config.Connection, len(existing))
	copy(merged, existing)

	for _, imp := range imported {
		key := strings.ToLower(imp.Name)
		idx, exists := byName[key]

		switch {
		case !exists:
			merged = append(merged, imp)
			byName[key] = len(merged) - 1
			result.Created++

		case strategy == MergeSkip:
			result.Skipped++

		case strategy == MergeOverwrite:
			// Preserve the original ID so references stay valid.
			imp.ID = merged[idx].ID
			merged[idx] = imp
			result.Updated++

		case strategy == MergeRename:
			// Find a unique name.
			base := imp.Name
			n := 2
			for {
				candidate := fmt.Sprintf("%s (%d)", base, n)
				if _, taken := byName[strings.ToLower(candidate)]; !taken {
					imp.Name = candidate
					imp.ID = sanitizeID(candidate)
					break
				}
				n++
			}
			merged = append(merged, imp)
			byName[strings.ToLower(imp.Name)] = len(merged) - 1
			result.Created++
		}
	}

	return merged, result
}

// sanitizeID produces a kebab-cased ID from a connection name.
func sanitizeID(name string) string {
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, ".", "-")
	return id
}
