package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/hooks"
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
	portForwards string
	group        string
	tags         string
	resolution   string
	fullscreen   bool
	dynamicRes   bool
	vncPassword  string

	// Hook fields
	hookPreConnect     string
	hookPostConnect    string
	hookPreDisconnect  string
	hookPostDisconnect string
	hookOnFailure      string

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
	f.portForwards = ""
	f.group = ""
	f.tags = ""
	f.resolution = "1920x1080"
	f.fullscreen = false
	f.dynamicRes = true
	f.vncPassword = ""
	f.hookPreConnect = ""
	f.hookPostConnect = ""
	f.hookPreDisconnect = ""
	f.hookPostDisconnect = ""
	f.hookOnFailure = "warn"
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
	f.portForwards = config.FormatPortForwards(conn.PortForwards)
	f.group = conn.Group
	f.tags = strings.Join(conn.Tags, ", ")
	f.resolution = conn.RDPOptions.Resolution
	f.fullscreen = conn.RDPOptions.Fullscreen
	f.vncPassword = conn.VNCPassword
	f.dynamicRes = conn.RDPOptions.DynamicResolution
	f.groups = groups

	// Load hooks from connection
	f.hookPreConnect = hookCommandsToString(conn.Hooks.PreConnect)
	f.hookPostConnect = hookCommandsToString(conn.Hooks.PostConnect)
	f.hookPreDisconnect = hookCommandsToString(conn.Hooks.PreDisconnect)
	f.hookPostDisconnect = hookCommandsToString(conn.Hooks.PostDisconnect)
	f.hookOnFailure = hookOnFailureToString(conn.Hooks)

	f.buildForm()
	f.active = true
}

// hookCommandsToString returns a semicolon-separated string of hook commands.
func hookCommandsToString(hks []hooks.Hook) string {
	if len(hks) == 0 {
		return ""
	}
	cmds := make([]string, 0, len(hks))
	for _, h := range hks {
		cmds = append(cmds, h.Command)
	}
	return strings.Join(cmds, "; ")
}

// hookOnFailureToString returns the on-failure policy from the first non-empty hook list.
func hookOnFailureToString(h hooks.Hooks) string {
	for _, hks := range [][]hooks.Hook{h.PreConnect, h.PostConnect, h.PreDisconnect, h.PostDisconnect} {
		if len(hks) > 0 {
			return string(hks[0].OnFailure)
		}
	}
	return "warn"
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
			huh.NewInput().
				Title("Port Forwards (e.g. L:8080:remote:80,R:9090:local:9090,D:1080)").
				Value(&f.portForwards).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return nil
					}
					_, err := config.ParsePortForwards(s)
					return err
				}),
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
		// Lifecycle hooks
		huh.NewGroup(
			huh.NewInput().
				Title("Pre-Connect Command (shell command to run before connecting)").
				Value(&f.hookPreConnect),
			huh.NewInput().
				Title("Post-Connect Command (shell command to run after connecting)").
				Value(&f.hookPostConnect),
			huh.NewInput().
				Title("Pre-Disconnect Command (shell command to run before disconnecting)").
				Value(&f.hookPreDisconnect),
			huh.NewInput().
				Title("Post-Disconnect Command (shell command to run after disconnecting)").
				Value(&f.hookPostDisconnect),
			huh.NewSelect[string]().
				Title("On Hook Failure").
				Options(
					huh.NewOption("Warn (log and continue)", "warn"),
					huh.NewOption("Abort (stop connection)", "abort"),
					huh.NewOption("Ignore (silently continue)", "ignore"),
				).
				Value(&f.hookOnFailure),
		),
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

	var portForwards []config.PortForward
	if f.portForwards != "" {
		// Validation already passed in the form
		portForwards, _ = config.ParsePortForwards(f.portForwards)
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
		PortForwards: portForwards,
		Group:        f.group,
		Tags:         tags,
		Hooks:        f.buildHooks(),
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

// buildHooks constructs a hooks.Hooks from the form fields.
func (f *formModel) buildHooks() hooks.Hooks {
	onFailure := hooks.OnFailure(f.hookOnFailure)
	if onFailure == "" {
		onFailure = hooks.FailWarn
	}

	var h hooks.Hooks
	h.PreConnect = parseHookCommands(f.hookPreConnect, hooks.PreConnect, onFailure)
	h.PostConnect = parseHookCommands(f.hookPostConnect, hooks.PostConnect, onFailure)
	h.PreDisconnect = parseHookCommands(f.hookPreDisconnect, hooks.PreDisconnect, onFailure)
	h.PostDisconnect = parseHookCommands(f.hookPostDisconnect, hooks.PostDisconnect, onFailure)
	return h
}

// parseHookCommands splits a semicolon-separated command string into Hook slices.
func parseHookCommands(input string, event hooks.HookEvent, onFailure hooks.OnFailure) []hooks.Hook {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	parts := strings.Split(input, ";")
	var result []hooks.Hook
	for _, cmd := range parts {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		result = append(result, hooks.Hook{
			Event:     event,
			Command:   cmd,
			OnFailure: onFailure,
			Timeout:   30 * time.Second,
		})
	}
	return result
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
