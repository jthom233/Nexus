package theme

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestAllBuiltInThemesRegistered(t *testing.T) {
	expected := []string{
		"catppuccin-mocha",
		"dracula",
		"gruvbox-dark",
		"nord",
		"tokyonight-storm",
	}
	names := Names()
	if !reflect.DeepEqual(names, expected) {
		t.Errorf("Names() = %v, want %v", names, expected)
	}
}

func TestCurrentDefaultsToTokyonightStorm(t *testing.T) {
	// Reset to default in case another test changed it.
	Set("tokyonight-storm")

	c := Current()
	if c == nil {
		t.Fatal("Current() returned nil")
	}
	if c.Name != "tokyonight-storm" {
		t.Errorf("Current().Name = %q, want %q", c.Name, "tokyonight-storm")
	}
}

func TestSetSwitchesTheme(t *testing.T) {
	defer Set("tokyonight-storm") // restore default after test

	ok := Set("dracula")
	if !ok {
		t.Fatal("Set(\"dracula\") returned false, expected true")
	}
	c := Current()
	if c.Name != "dracula" {
		t.Errorf("after Set(\"dracula\"), Current().Name = %q, want %q", c.Name, "dracula")
	}
}

func TestSetInvalidNameReturnsFalse(t *testing.T) {
	defer Set("tokyonight-storm")

	ok := Set("nonexistent-theme")
	if ok {
		t.Error("Set(\"nonexistent-theme\") returned true, expected false")
	}
	// Current theme should not have changed.
	if Current().Name == "nonexistent-theme" {
		t.Error("Current theme was changed by invalid Set call")
	}
}

func TestNamesReturnsAllThemes(t *testing.T) {
	names := Names()
	if len(names) != 5 {
		t.Errorf("len(Names()) = %d, want 5", len(names))
	}
	// Verify sorted order.
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("Names() not sorted: %v", names)
			break
		}
	}
}

func TestAllThemesHaveNonEmptyFields(t *testing.T) {
	empty := lipgloss.Color("")

	for _, name := range Names() {
		Set(name)
		th := Current()
		t.Run(name, func(t *testing.T) {
			v := reflect.ValueOf(*th)
			typ := v.Type()
			for i := 0; i < v.NumField(); i++ {
				field := typ.Field(i)
				if field.Type == reflect.TypeOf(lipgloss.Color("")) {
					val := v.Field(i).Interface().(lipgloss.Color)
					if val == empty {
						t.Errorf("theme %q field %q is empty", name, field.Name)
					}
				}
			}
		})
	}
	// Restore default.
	Set("tokyonight-storm")
}
