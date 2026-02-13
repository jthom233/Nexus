package importexport

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/dr4zz/nexus/internal/config"
)

// Exporter writes connections to a serialised format.
type Exporter interface {
	Export(writer io.Writer, connections []config.Connection) error
}

// ExportOptions controls what is included in the output.
type ExportOptions struct {
	IncludeCredentials bool // when false, passwords are replaced with "***"
}

// mask replaces a non-empty string with "***" when credentials are excluded.
func mask(value string, opts ExportOptions) string {
	if opts.IncludeCredentials || value == "" {
		return value
	}
	return "***"
}

// ---------- CSV Exporter ----------

// CSVExporter writes connections as CSV with a header row.
type CSVExporter struct {
	Options ExportOptions
}

func (e *CSVExporter) Export(writer io.Writer, connections []config.Connection) error {
	w := csv.NewWriter(writer)
	defer w.Flush()

	// Header
	if err := w.Write([]string{"name", "host", "port", "protocol", "username", "password", "group", "tags"}); err != nil {
		return fmt.Errorf("csv write header: %w", err)
	}

	for _, c := range connections {
		port := ""
		if c.Port != 0 {
			port = fmt.Sprintf("%d", c.Port)
		}
		tags := strings.Join(c.Tags, ";")

		row := []string{
			c.Name,
			c.Host,
			port,
			string(c.Protocol),
			c.Username,
			mask(c.Password, e.Options),
			c.Group,
			tags,
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("csv write row %q: %w", c.Name, err)
		}
	}

	return nil
}

// ---------- JSON Exporter ----------

// JSONExporter writes connections as a pretty-printed JSON array.
type JSONExporter struct {
	Options ExportOptions
}

// jsonExportConn is the transfer struct for JSON export.
type jsonExportConn struct {
	Name     string   `json:"name"`
	Host     string   `json:"host"`
	Port     int      `json:"port,omitempty"`
	Protocol string   `json:"protocol"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`
	Group    string   `json:"group,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

func (e *JSONExporter) Export(writer io.Writer, connections []config.Connection) error {
	out := make([]jsonExportConn, 0, len(connections))
	for _, c := range connections {
		out = append(out, jsonExportConn{
			Name:     c.Name,
			Host:     c.Host,
			Port:     c.Port,
			Protocol: string(c.Protocol),
			Username: c.Username,
			Password: mask(c.Password, e.Options),
			Group:    c.Group,
			Tags:     c.Tags,
		})
	}

	enc := json.NewEncoder(writer)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("json encode: %w", err)
	}
	return nil
}

// ---------- YAML Exporter ----------

// YAMLExporter writes connections as a YAML list.
type YAMLExporter struct {
	Options ExportOptions
}

// yamlExportConn is the transfer struct for YAML export.
type yamlExportConn struct {
	Name     string   `yaml:"name"`
	Host     string   `yaml:"host"`
	Port     int      `yaml:"port,omitempty"`
	Protocol string   `yaml:"protocol"`
	Username string   `yaml:"username,omitempty"`
	Password string   `yaml:"password,omitempty"`
	Group    string   `yaml:"group,omitempty"`
	Tags     []string `yaml:"tags,omitempty"`
}

func (e *YAMLExporter) Export(writer io.Writer, connections []config.Connection) error {
	out := make([]yamlExportConn, 0, len(connections))
	for _, c := range connections {
		out = append(out, yamlExportConn{
			Name:     c.Name,
			Host:     c.Host,
			Port:     c.Port,
			Protocol: string(c.Protocol),
			Username: c.Username,
			Password: mask(c.Password, e.Options),
			Group:    c.Group,
			Tags:     c.Tags,
		})
	}

	data, err := yaml.Marshal(out)
	if err != nil {
		return fmt.Errorf("yaml encode: %w", err)
	}
	_, err = writer.Write(data)
	return err
}
