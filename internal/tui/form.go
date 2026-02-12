package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dr4zz/nexus/internal/config"
)

// FormSubmitMsg is sent when the form is submitted with a connection.
type FormSubmitMsg struct {
	Conn   config.Connection
	IsEdit bool
}

// FormCancelMsg is sent when the form is cancelled.
type FormCancelMsg struct{}

type formModel struct {
	form   *huh.Form
	active bool
	isEdit bool
	width  int
	height int

	// Field values bound to form
	name         string
	protocol     string
	host         string
	port         string
	username     string
	password     string
	domain       string
	identityFile string
	proxyJump    string
	proxyCommand string
	group        string
	tags         string
	resolution   string
	fullscreen   bool
	dynamicRes   bool
	vncPassword  string

	editID string
	groups []string
}

func newForm(groups []string) formModel {
	return formModel{groups: groups}
}

func newFormPtr(groups []string) *formModel {
	return &formModel{groups: groups}
}

func (f *formModel) startAdd(groups []string) {
	f.isEdit = false
	f.editID = ""
	f.name = ""
	f.protocol = "ssh"
	f.host = ""
	f.port = ""
	f.username = ""
	f.password = ""
	f.domain = ""
	f.identityFile = ""
	f.proxyJump = ""
	f.proxyCommand = ""
	f.group = ""
	f.tags = ""
	f.resolution = "1920x1080"
	f.fullscreen = false
	f.dynamicRes = true
	f.vncPassword = ""
	f.groups = groups
	f.buildForm()
	f.active = true
}

func (f *formModel) startEdit(conn config.Connection, groups []string) {
	f.isEdit = true
	f.editID = conn.ID
	f.name = conn.Name
	f.protocol = string(conn.Protocol)
	f.host = conn.Host
	if conn.Port != 0 {
		f.port = strconv.Itoa(conn.Port)
	} else {
		f.port = ""
	}
	f.username = conn.Username
	f.password = conn.Password
	f.domain = conn.Domain
	f.identityFile = conn.IdentityFile
	f.proxyJump = conn.ProxyJump
	f.proxyCommand = conn.ProxyCommand
	f.group = conn.Group
	f.tags = strings.Join(conn.Tags, ", ")
	f.resolution = conn.RDPOptions.Resolution
	f.fullscreen = conn.RDPOptions.Fullscreen
	f.vncPassword = conn.VNCPassword
	f.dynamicRes = conn.RDPOptions.DynamicResolution
	f.groups = groups
	f.buildForm()
	f.active = true
}

func (f *formModel) buildForm() {
	groupOptions := []huh.Option[string]{huh.NewOption("(none)", "")}
	for _, g := range f.groups {
		groupOptions = append(groupOptions, huh.NewOption(g, g))
	}

	f.form = huh.NewForm(
		// Common fields
		huh.NewGroup(
			huh.NewInput().
				Title("Name").
				Value(&f.name).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("name is required")
					}
					return nil
				}),
			huh.NewSelect[string]().
				Title("Protocol").
				Options(
					huh.NewOption("SSH", "ssh"),
					huh.NewOption("RDP", "rdp"),
					huh.NewOption("VNC", "vnc"),
					huh.NewOption("Telnet", "telnet"),
				).
				Value(&f.protocol),
			huh.NewInput().
				Title("Host").
				Value(&f.host).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("host is required")
					}
					return nil
				}),
			huh.NewInput().
				Title("Port (leave empty for default)").
				Value(&f.port).
				Validate(func(s string) error {
					if s == "" {
						return nil
					}
					p, err := strconv.Atoi(s)
					if err != nil || p < 1 || p > 65535 {
						return fmt.Errorf("port must be 1-65535")
					}
					return nil
				}),
		),
		// Credentials (shown for all protocols)
		huh.NewGroup(
			huh.NewInput().
				Title("Username").
				Value(&f.username),
			huh.NewInput().
				Title("Password").
				Value(&f.password).
				EchoMode(huh.EchoModePassword),
		),
		// Grouping & tags
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Group").
				Options(groupOptions...).
				Value(&f.group),
			huh.NewInput().
				Title("Tags (comma-separated)").
				Value(&f.tags),
		),
		// SSH-specific
		huh.NewGroup(
			huh.NewInput().
				Title("Identity File").
				Value(&f.identityFile),
			huh.NewInput().
				Title("ProxyJump (e.g. user@bastion:22,user@bastion2:22)").
				Value(&f.proxyJump),
			huh.NewInput().
				Title("ProxyCommand (e.g. ssh -W %h:%p bastion)").
				Value(&f.proxyCommand),
		).WithHideFunc(func() bool { return f.protocol != "ssh" }),
		// RDP-specific
		huh.NewGroup(
			huh.NewInput().
				Title("Domain").
				Value(&f.domain),
			huh.NewInput().
				Title("Resolution (e.g. 1920x1080)").
				Value(&f.resolution),
			huh.NewConfirm().
				Title("Fullscreen").
				Value(&f.fullscreen),
			huh.NewConfirm().
				Title("Dynamic Resolution").
				Value(&f.dynamicRes),
		).WithHideFunc(func() bool { return f.protocol != "rdp" }),
		// VNC-specific
		huh.NewGroup(
			huh.NewInput().
				Title("VNC Password").
				Value(&f.vncPassword).
				EchoMode(huh.EchoModePassword),
		).WithHideFunc(func() bool { return f.protocol != "vnc" }),
	).WithTheme(huh.ThemeDracula()).
		WithWidth(f.width).
		WithHeight(f.height)
}

func (f *formModel) toConnection() config.Connection {
	id := f.editID
	if id == "" {
		id = sanitizeForID(f.name)
	}

	port := 0
	if f.port != "" {
		port, _ = strconv.Atoi(f.port)
	}

	var tags []string
	if f.tags != "" {
		for _, t := range strings.Split(f.tags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	conn := config.Connection{
		ID:           id,
		Name:         strings.TrimSpace(f.name),
		Protocol:     config.Protocol(f.protocol),
		Host:         strings.TrimSpace(f.host),
		Port:         port,
		Username:     strings.TrimSpace(f.username),
		Password:     f.password,
		IdentityFile: strings.TrimSpace(f.identityFile),
		ProxyJump:    strings.TrimSpace(f.proxyJump),
		ProxyCommand: strings.TrimSpace(f.proxyCommand),
		Group:        f.group,
		Tags:         tags,
	}

	switch f.protocol {
	case "rdp":
		conn.Domain = strings.TrimSpace(f.domain)
		conn.RDPOptions = config.RDPOptions{
			Resolution:        f.resolution,
			Fullscreen:        f.fullscreen,
			DynamicResolution: f.dynamicRes,
		}
	case "vnc":
		conn.VNCPassword = f.vncPassword
	}

	return conn
}

func (f *formModel) Update(msg tea.Msg) tea.Cmd {
	if !f.active || f.form == nil {
		return nil
	}

	// Check for escape to cancel
	if msg, ok := msg.(tea.KeyMsg); ok {
		if msg.String() == "esc" {
			f.active = false
			return func() tea.Msg { return FormCancelMsg{} }
		}
	}

	form, cmd := f.form.Update(msg)
	if hf, ok := form.(*huh.Form); ok {
		f.form = hf
	}

	if f.form.State == huh.StateCompleted {
		f.active = false
		conn := f.toConnection()
		isEdit := f.isEdit
		return func() tea.Msg {
			return FormSubmitMsg{Conn: conn, IsEdit: isEdit}
		}
	}

	return cmd
}

func (f formModel) View() string {
	if !f.active || f.form == nil {
		return ""
	}
	return f.form.View()
}

func sanitizeForID(name string) string {
	id := strings.ToLower(strings.TrimSpace(name))
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, ".", "-")
	return id
}
