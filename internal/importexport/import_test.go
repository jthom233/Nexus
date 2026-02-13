package importexport

import (
	"strings"
	"testing"

	"github.com/dr4zz/nexus/internal/config"
)

// ---------- CSV Import ----------

func TestCSVImporter_ValidData(t *testing.T) {
	input := `name,host,port,protocol,username,group,tags
web-prod,10.0.1.1,22,ssh,admin,production,web;linux
db-staging,10.0.2.5,3306,ssh,dba,staging,database
`
	imp := &CSVImporter{}
	conns, err := imp.Import(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conns) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(conns))
	}

	c := conns[0]
	if c.Name != "web-prod" {
		t.Errorf("name = %q, want %q", c.Name, "web-prod")
	}
	if c.Host != "10.0.1.1" {
		t.Errorf("host = %q, want %q", c.Host, "10.0.1.1")
	}
	if c.Port != 22 {
		t.Errorf("port = %d, want 22", c.Port)
	}
	if c.Protocol != config.ProtoSSH {
		t.Errorf("protocol = %q, want %q", c.Protocol, config.ProtoSSH)
	}
	if c.Username != "admin" {
		t.Errorf("username = %q, want %q", c.Username, "admin")
	}
	if c.Group != "production" {
		t.Errorf("group = %q, want %q", c.Group, "production")
	}
	if len(c.Tags) != 2 || c.Tags[0] != "web" || c.Tags[1] != "linux" {
		t.Errorf("tags = %v, want [web linux]", c.Tags)
	}
}

func TestCSVImporter_MissingNameColumn(t *testing.T) {
	input := `host,port,protocol
10.0.1.1,22,ssh
`
	imp := &CSVImporter{}
	_, err := imp.Import(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing name column")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error = %q, expected mention of 'name'", err.Error())
	}
}

func TestCSVImporter_MissingHostColumn(t *testing.T) {
	input := `name,port,protocol
server1,22,ssh
`
	imp := &CSVImporter{}
	_, err := imp.Import(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing host column")
	}
	if !strings.Contains(err.Error(), "host") {
		t.Errorf("error = %q, expected mention of 'host'", err.Error())
	}
}

func TestCSVImporter_DefaultProtocol(t *testing.T) {
	input := `name,host
mybox,192.168.1.1
`
	imp := &CSVImporter{}
	conns, err := imp.Import(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conns[0].Protocol != config.ProtoSSH {
		t.Errorf("protocol = %q, want default %q", conns[0].Protocol, config.ProtoSSH)
	}
}

func TestCSVImporter_EmptyFile(t *testing.T) {
	input := `name,host
`
	imp := &CSVImporter{}
	_, err := imp.Import(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for empty CSV (header only)")
	}
}

// ---------- JSON Import ----------

func TestJSONImporter_ValidData(t *testing.T) {
	input := `[
	{"name":"web-prod","host":"10.0.1.1","port":22,"protocol":"ssh","username":"admin","group":"production","tags":["web","linux"]},
	{"name":"db-staging","host":"10.0.2.5","protocol":"ssh","username":"dba","group":"staging"}
]`
	imp := &JSONImporter{}
	conns, err := imp.Import(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conns) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(conns))
	}

	c := conns[0]
	if c.Name != "web-prod" {
		t.Errorf("name = %q, want %q", c.Name, "web-prod")
	}
	if c.Port != 22 {
		t.Errorf("port = %d, want 22", c.Port)
	}
	if len(c.Tags) != 2 {
		t.Errorf("tags count = %d, want 2", len(c.Tags))
	}
}

func TestJSONImporter_MissingHost(t *testing.T) {
	input := `[{"name":"server1"}]`
	imp := &JSONImporter{}
	_, err := imp.Import(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing host")
	}
}

func TestJSONImporter_DefaultProtocol(t *testing.T) {
	input := `[{"name":"box","host":"1.2.3.4"}]`
	imp := &JSONImporter{}
	conns, err := imp.Import(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conns[0].Protocol != config.ProtoSSH {
		t.Errorf("protocol = %q, want default %q", conns[0].Protocol, config.ProtoSSH)
	}
}

func TestJSONImporter_InvalidProtocol(t *testing.T) {
	input := `[{"name":"box","host":"1.2.3.4","protocol":"ftp"}]`
	imp := &JSONImporter{}
	_, err := imp.Import(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for invalid protocol")
	}
}

// ---------- Merge Strategies ----------

func TestApplyMerge_Skip(t *testing.T) {
	existing := []config.Connection{
		{ID: "a", Name: "Server A", Host: "10.0.0.1"},
	}
	imported := []config.Connection{
		{ID: "a2", Name: "Server A", Host: "10.0.0.99"}, // duplicate name
		{ID: "b", Name: "Server B", Host: "10.0.0.2"},    // new
	}

	merged, result := ApplyMerge(existing, imported, MergeSkip)

	if result.Created != 1 {
		t.Errorf("created = %d, want 1", result.Created)
	}
	if result.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", result.Skipped)
	}
	if result.Updated != 0 {
		t.Errorf("updated = %d, want 0", result.Updated)
	}
	if len(merged) != 2 {
		t.Fatalf("merged length = %d, want 2", len(merged))
	}
	// Original should be unchanged.
	if merged[0].Host != "10.0.0.1" {
		t.Errorf("existing host changed to %q, want 10.0.0.1", merged[0].Host)
	}
}

func TestApplyMerge_Overwrite(t *testing.T) {
	existing := []config.Connection{
		{ID: "a", Name: "Server A", Host: "10.0.0.1"},
	}
	imported := []config.Connection{
		{ID: "a2", Name: "Server A", Host: "10.0.0.99"},
	}

	merged, result := ApplyMerge(existing, imported, MergeOverwrite)

	if result.Updated != 1 {
		t.Errorf("updated = %d, want 1", result.Updated)
	}
	if result.Created != 0 {
		t.Errorf("created = %d, want 0", result.Created)
	}
	if len(merged) != 1 {
		t.Fatalf("merged length = %d, want 1", len(merged))
	}
	// Should be overwritten with new host but keep original ID.
	if merged[0].Host != "10.0.0.99" {
		t.Errorf("host = %q, want 10.0.0.99", merged[0].Host)
	}
	if merged[0].ID != "a" {
		t.Errorf("ID = %q, want 'a' (preserved)", merged[0].ID)
	}
}

func TestApplyMerge_Rename(t *testing.T) {
	existing := []config.Connection{
		{ID: "a", Name: "Server A", Host: "10.0.0.1"},
	}
	imported := []config.Connection{
		{ID: "a2", Name: "Server A", Host: "10.0.0.99"},
	}

	merged, result := ApplyMerge(existing, imported, MergeRename)

	if result.Created != 1 {
		t.Errorf("created = %d, want 1", result.Created)
	}
	if result.Skipped != 0 {
		t.Errorf("skipped = %d, want 0", result.Skipped)
	}
	if len(merged) != 2 {
		t.Fatalf("merged length = %d, want 2", len(merged))
	}
	// Both should exist, renamed one should have a suffix.
	if merged[1].Name != "Server A (2)" {
		t.Errorf("renamed name = %q, want %q", merged[1].Name, "Server A (2)")
	}
}

func TestApplyMerge_RenameMultiple(t *testing.T) {
	existing := []config.Connection{
		{ID: "a", Name: "Box", Host: "10.0.0.1"},
		{ID: "a2", Name: "Box (2)", Host: "10.0.0.2"},
	}
	imported := []config.Connection{
		{ID: "b", Name: "Box", Host: "10.0.0.99"},
	}

	merged, result := ApplyMerge(existing, imported, MergeRename)

	if result.Created != 1 {
		t.Errorf("created = %d, want 1", result.Created)
	}
	if len(merged) != 3 {
		t.Fatalf("merged length = %d, want 3", len(merged))
	}
	// Should skip "Box (2)" since it already exists and pick "Box (3)".
	if merged[2].Name != "Box (3)" {
		t.Errorf("renamed name = %q, want %q", merged[2].Name, "Box (3)")
	}
}
