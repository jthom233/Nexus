package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/vault"
)

// VaultFormSubmitMsg is sent when the vault profile form is submitted.
type VaultFormSubmitMsg struct {
	Profile vault.CredentialProfile
	IsEdit  bool
}

type vaultFormModel struct {
	form   *huh.Form
	isEdit bool
	editID string

	// Field bindings
	name         string
	description  string
	group        string
	username     string
	password     string
	identityFile string
	passphrase   string
	domain       string
	vncPassword  string
	tags         string

	width  int
	height int
}

// newVaultForm constructs a vaultFormModel. If profile is nil, the form is in
// create mode; if non-nil, it is in edit mode pre-populated from profile.
func newVaultForm(groups []config.Group, profile *vault.CredentialProfile) vaultFormModel {
	m := vaultFormModel{}

	if profile != nil {
		m.isEdit = true
		m.editID = profile.ID
		m.name = profile.Name
		m.description = profile.Description
		m.group = profile.Group
		m.username = profile.Username
		m.password = profile.Password
		m.identityFile = profile.IdentityFile
		m.passphrase = profile.Passphrase
		m.domain = profile.Domain
		m.vncPassword = profile.VNCPassword
		m.tags = strings.Join(profile.Tags, ", ")
	}

	groupOptions := []huh.Option[string]{huh.NewOption("(None)", "")}
	for _, g := range groups {
		groupOptions = append(groupOptions, huh.NewOption(g.Name, g.Name))
	}

	m.form = huh.NewForm(
		// Group 1 — Profile Info
		huh.NewGroup(
			huh.NewInput().
				Title("Name").
				Value(&m.name).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("name is required")
					}
					return nil
				}),
			huh.NewInput().
				Title("Description").
				Value(&m.description),
			huh.NewSelect[string]().
				Title("Group").
				Options(groupOptions...).
				Value(&m.group),
		),
		// Group 2 — Credentials
		huh.NewGroup(
			huh.NewInput().
				Title("Username").
				Value(&m.username),
			huh.NewInput().
				Title("Password").
				Value(&m.password).
				EchoMode(huh.EchoModePassword),
			huh.NewInput().
				Title("Identity File").
				Value(&m.identityFile),
			huh.NewInput().
				Title("Passphrase").
				Value(&m.passphrase).
				EchoMode(huh.EchoModePassword),
			huh.NewInput().
				Title("Domain").
				Value(&m.domain),
			huh.NewInput().
				Title("VNC Password").
				Value(&m.vncPassword).
				EchoMode(huh.EchoModePassword),
		),
		// Group 3 — Tags
		huh.NewGroup(
			huh.NewInput().
				Title("Tags (comma-separated)").
				Value(&m.tags),
		),
	).WithTheme(huh.ThemeDracula()).
		WithWidth(m.width).
		WithHeight(m.height)

	return m
}

func (m vaultFormModel) Init() tea.Cmd {
	return m.form.Init()
}

func (m vaultFormModel) Update(msg tea.Msg) (vaultFormModel, tea.Cmd) {
	if m.form == nil {
		return m, nil
	}

	form, cmd := m.form.Update(msg)
	if hf, ok := form.(*huh.Form); ok {
		m.form = hf
	}

	if m.form.State == huh.StateCompleted {
		profile := vault.CredentialProfile{
			Name:         m.name,
			Description:  m.description,
			Group:        m.group,
			Username:     m.username,
			Password:     m.password,
			IdentityFile: m.identityFile,
			Passphrase:   m.passphrase,
			Domain:       m.domain,
			VNCPassword:  m.vncPassword,
			Tags:         parseTags(m.tags),
		}
		if m.isEdit {
			profile.ID = m.editID
		}
		isEdit := m.isEdit
		return m, func() tea.Msg {
			return VaultFormSubmitMsg{Profile: profile, IsEdit: isEdit}
		}
	}

	return m, cmd
}

func (m vaultFormModel) View() string {
	if m.form == nil {
		return ""
	}
	return m.form.View()
}

func (m *vaultFormModel) setSize(w, h int) {
	m.width = w
	m.height = h
	if m.form != nil {
		m.form = m.form.WithWidth(w).WithHeight(h)
	}
}

// parseTags splits a comma-separated string into a trimmed, non-empty slice.
func parseTags(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}
