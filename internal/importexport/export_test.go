package importexport

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/dr4zz/nexus/internal/config"
)

var testConnections = []config.Connection{
	{
		Name:     "web-prod",
		Host:     "10.0.1.1",
		Port:     22,
		Protocol: config.ProtoSSH,
		Username: "admin",
		Password: "s3cret",
		Group:    "production",
		Tags:     []string{"web", "linux"},
	},
	{
		Name:     "db-staging",
		Host:     "10.0.2.5",
		Port:     3306,
		Protocol: config.ProtoSSH,
		Username: "dba",
		Password: "",
		Group:    "staging",
		Tags:     nil,
	},
}

// ---------- CSV Export ----------

func TestCSVExporter_Format(t *testing.T) {
	var buf bytes.Buffer
	exp := &CSVExporter{Options: ExportOptions{IncludeCredentials: true}}
	if err := exp.Export(&buf, testConnections); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 { // header + 2 data rows
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	// Header check
	if !strings.HasPrefix(lines[0], "name,") {
		t.Errorf("header = %q, expected to start with 'name,'", lines[0])
	}

	// First data row should contain the password since IncludeCredentials=true.
	if !strings.Contains(lines[1], "s3cret") {
		t.Errorf("row 1 = %q, expected to contain password", lines[1])
	}
}

func TestCSVExporter_MaskedCredentials(t *testing.T) {
	var buf bytes.Buffer
	exp := &CSVExporter{Options: ExportOptions{IncludeCredentials: false}}
	if err := exp.Export(&buf, testConnections); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "s3cret") {
		t.Error("output contains plaintext password when IncludeCredentials=false")
	}
	if !strings.Contains(output, "***") {
		t.Error("output missing masked password placeholder '***'")
	}
}

// ---------- JSON Export ----------

func TestJSONExporter_Format(t *testing.T) {
	var buf bytes.Buffer
	exp := &JSONExporter{Options: ExportOptions{IncludeCredentials: true}}
	if err := exp.Export(&buf, testConnections); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be valid JSON
	var parsed []jsonExportConn
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(parsed))
	}
	if parsed[0].Name != "web-prod" {
		t.Errorf("name = %q, want %q", parsed[0].Name, "web-prod")
	}
	if parsed[0].Password != "s3cret" {
		t.Errorf("password = %q, want %q", parsed[0].Password, "s3cret")
	}
}

func TestJSONExporter_MaskedCredentials(t *testing.T) {
	var buf bytes.Buffer
	exp := &JSONExporter{Options: ExportOptions{IncludeCredentials: false}}
	if err := exp.Export(&buf, testConnections); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed []jsonExportConn
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed[0].Password != "***" {
		t.Errorf("password = %q, want '***'", parsed[0].Password)
	}
}

// ---------- YAML Export ----------

func TestYAMLExporter_Format(t *testing.T) {
	var buf bytes.Buffer
	exp := &YAMLExporter{Options: ExportOptions{IncludeCredentials: true}}
	if err := exp.Export(&buf, testConnections); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be valid YAML
	var parsed []yamlExportConn
	if err := yaml.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(parsed))
	}
	if parsed[0].Name != "web-prod" {
		t.Errorf("name = %q, want %q", parsed[0].Name, "web-prod")
	}
	if parsed[0].Password != "s3cret" {
		t.Errorf("password = %q, want %q", parsed[0].Password, "s3cret")
	}
}

func TestYAMLExporter_MaskedCredentials(t *testing.T) {
	var buf bytes.Buffer
	exp := &YAMLExporter{Options: ExportOptions{IncludeCredentials: false}}
	if err := exp.Export(&buf, testConnections); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "s3cret") {
		t.Error("output contains plaintext password when IncludeCredentials=false")
	}
	if !strings.Contains(output, "***") {
		t.Error("output missing masked password placeholder '***'")
	}
}

// ---------- Roundtrip ----------

func TestCSVRoundtrip(t *testing.T) {
	// Export to CSV then re-import and check consistency.
	var buf bytes.Buffer
	exp := &CSVExporter{Options: ExportOptions{IncludeCredentials: false}}
	if err := exp.Export(&buf, testConnections); err != nil {
		t.Fatalf("export error: %v", err)
	}

	imp := &CSVImporter{}
	conns, err := imp.Import(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("import error: %v", err)
	}
	if len(conns) != len(testConnections) {
		t.Fatalf("roundtrip connection count = %d, want %d", len(conns), len(testConnections))
	}
	if conns[0].Name != testConnections[0].Name {
		t.Errorf("roundtrip name = %q, want %q", conns[0].Name, testConnections[0].Name)
	}
}

func TestJSONRoundtrip(t *testing.T) {
	var buf bytes.Buffer
	exp := &JSONExporter{Options: ExportOptions{IncludeCredentials: true}}
	if err := exp.Export(&buf, testConnections); err != nil {
		t.Fatalf("export error: %v", err)
	}

	imp := &JSONImporter{}
	conns, err := imp.Import(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("import error: %v", err)
	}
	if len(conns) != len(testConnections) {
		t.Fatalf("roundtrip connection count = %d, want %d", len(conns), len(testConnections))
	}
	if conns[0].Password != "s3cret" {
		t.Errorf("roundtrip password = %q, want %q", conns[0].Password, "s3cret")
	}
}
