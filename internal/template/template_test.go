package template

import (
	"testing"

	"github.com/dr4zz/nexus/internal/config"
)

func TestBuiltinTemplatesExist(t *testing.T) {
	store := NewTemplateStore()
	expected := []string{
		"ssh-standard",
		"ssh-jumphost",
		"rdp-windows",
		"vnc-linux",
		"telnet-network",
	}
	for _, name := range expected {
		tpl, ok := store.Get(name)
		if !ok {
			t.Errorf("built-in template %q not found", name)
			continue
		}
		if tpl.Name != name {
			t.Errorf("template Name = %q, want %q", tpl.Name, name)
		}
		if tpl.Description == "" {
			t.Errorf("template %q has empty description", name)
		}
		if tpl.Protocol == "" {
			t.Errorf("template %q has empty protocol", name)
		}
		if tpl.Port == 0 {
			t.Errorf("template %q has zero port", name)
		}
	}
}

func TestListReturnsAllBuiltins(t *testing.T) {
	store := NewTemplateStore()
	list := store.List()
	if len(list) != 5 {
		t.Errorf("List() returned %d templates, want 5", len(list))
	}
	// Verify sorted order
	for i := 1; i < len(list); i++ {
		if list[i-1].Name >= list[i].Name {
			t.Errorf("List() not sorted: %q >= %q", list[i-1].Name, list[i].Name)
		}
	}
}

func TestApplyFillsEmptyFields(t *testing.T) {
	tpl := Template{
		Name:      "test",
		Protocol:  "ssh",
		Port:      2222,
		Username:  "admin",
		Group:     "servers",
		Tags:      []string{"linux", "prod"},
		ProxyJump: "bastion:22",
	}

	conn := &config.Connection{
		Name: "my-server",
		Host: "10.0.0.1",
	}

	Apply(tpl, conn)

	if conn.Protocol != "ssh" {
		t.Errorf("Protocol = %q, want %q", conn.Protocol, "ssh")
	}
	if conn.Port != 2222 {
		t.Errorf("Port = %d, want %d", conn.Port, 2222)
	}
	if conn.Username != "admin" {
		t.Errorf("Username = %q, want %q", conn.Username, "admin")
	}
	if conn.Group != "servers" {
		t.Errorf("Group = %q, want %q", conn.Group, "servers")
	}
	if len(conn.Tags) != 2 || conn.Tags[0] != "linux" || conn.Tags[1] != "prod" {
		t.Errorf("Tags = %v, want [linux prod]", conn.Tags)
	}
	if conn.ProxyJump != "bastion:22" {
		t.Errorf("ProxyJump = %q, want %q", conn.ProxyJump, "bastion:22")
	}
	// Existing fields must not be touched
	if conn.Name != "my-server" {
		t.Errorf("Name = %q, want %q", conn.Name, "my-server")
	}
	if conn.Host != "10.0.0.1" {
		t.Errorf("Host = %q, want %q", conn.Host, "10.0.0.1")
	}
}

func TestApplyDoesNotOverwriteExistingFields(t *testing.T) {
	tpl := Template{
		Name:      "test",
		Protocol:  "rdp",
		Port:      3389,
		Username:  "admin",
		Group:     "windows",
		Tags:      []string{"desktop"},
		ProxyJump: "jump:22",
	}

	conn := &config.Connection{
		Name:      "my-pc",
		Protocol:  "ssh",
		Port:      22,
		Username:  "root",
		Group:     "linux",
		Tags:      []string{"server"},
		ProxyJump: "my-bastion:22",
	}

	Apply(tpl, conn)

	if conn.Protocol != "ssh" {
		t.Errorf("Protocol overwritten: got %q, want %q", conn.Protocol, "ssh")
	}
	if conn.Port != 22 {
		t.Errorf("Port overwritten: got %d, want %d", conn.Port, 22)
	}
	if conn.Username != "root" {
		t.Errorf("Username overwritten: got %q, want %q", conn.Username, "root")
	}
	if conn.Group != "linux" {
		t.Errorf("Group overwritten: got %q, want %q", conn.Group, "linux")
	}
	if len(conn.Tags) != 1 || conn.Tags[0] != "server" {
		t.Errorf("Tags overwritten: got %v, want [server]", conn.Tags)
	}
	if conn.ProxyJump != "my-bastion:22" {
		t.Errorf("ProxyJump overwritten: got %q, want %q", conn.ProxyJump, "my-bastion:22")
	}
}

func TestAddAndRemove(t *testing.T) {
	store := NewTemplateStore()
	custom := Template{
		Name:        "custom-ssh",
		Description: "My custom SSH template",
		Protocol:    "ssh",
		Port:        8022,
		Username:    "deploy",
	}

	store.Add(custom)

	got, ok := store.Get("custom-ssh")
	if !ok {
		t.Fatal("Add: template not found after adding")
	}
	if got.Port != 8022 {
		t.Errorf("Add: Port = %d, want %d", got.Port, 8022)
	}

	// Verify it appears in List
	found := false
	for _, tpl := range store.List() {
		if tpl.Name == "custom-ssh" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Add: template not present in List()")
	}

	store.Remove("custom-ssh")
	_, ok = store.Get("custom-ssh")
	if ok {
		t.Error("Remove: template still found after removal")
	}
}

func TestGetNonExistent(t *testing.T) {
	store := NewTemplateStore()
	_, ok := store.Get("does-not-exist")
	if ok {
		t.Error("Get returned true for non-existent template")
	}
}

func TestRemoveNonExistent(t *testing.T) {
	store := NewTemplateStore()
	before := len(store.List())
	store.Remove("does-not-exist") // should not panic
	after := len(store.List())
	if before != after {
		t.Errorf("Remove non-existent changed count: %d -> %d", before, after)
	}
}

func TestApplyTagsCopied(t *testing.T) {
	tpl := Template{
		Name: "tag-test",
		Tags: []string{"a", "b"},
	}

	conn := &config.Connection{}
	Apply(tpl, conn)

	// Mutate the template tags — should not affect connection
	tpl.Tags[0] = "MUTATED"
	if conn.Tags[0] == "MUTATED" {
		t.Error("Apply did not copy tags — mutation propagated")
	}
}
