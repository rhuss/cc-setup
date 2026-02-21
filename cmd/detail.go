package cmd

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/rhuss/cc-setup/internal/config"
	"github.com/rhuss/cc-setup/internal/display"
)

// detailAction represents the action chosen from the detail view.
type detailAction int

const (
	detailNone detailAction = iota
	detailEditField
	detailToolPermissions
	detailBack
)

// detailField represents a single displayable/editable field in the detail view.
type detailField struct {
	label string // display label: "Type", "URL", ...
	value string // current display value
	key   string // field identifier: "type", "url", "auth", "command", "args", "env", "description", "tools"
}

// detailModel is the BubbleTea model for the server detail view.
type detailModel struct {
	name   string
	server map[string]any
	fields []detailField
	cursor int
	width  int
	scope  string
	tab    manageTab
	action detailAction
	selKey string // key of the selected field when action is detailEditField
}

// buildDetailFields builds the field list based on server type.
func buildDetailFields(server map[string]any) []detailField {
	stype, _ := server["type"].(string)
	if stype == "" {
		stype = "?"
	}

	fields := []detailField{
		{label: "Type", value: stype, key: "type"},
	}

	switch stype {
	case "http", "sse":
		url, _ := server["url"].(string)
		fields = append(fields, detailField{label: "URL", value: url, key: "url"})

		headers := serverHeaders(server)
		_, authLabel := display.DecodeAuth(headers)
		fields = append(fields, detailField{label: "Authentication", value: authLabel, key: "auth"})

	case "stdio":
		cmd, _ := server["command"].(string)
		fields = append(fields, detailField{label: "Command", value: cmd, key: "command"})

		var argsDisplay string
		if rawArgs, ok := server["args"].([]any); ok && len(rawArgs) > 0 {
			parts := make([]string, 0, len(rawArgs))
			for _, a := range rawArgs {
				if s, ok := a.(string); ok {
					parts = append(parts, s)
				}
			}
			argsDisplay = strings.Join(parts, " ")
		}
		fields = append(fields, detailField{label: "Arguments", value: argsDisplay, key: "args"})

		var envDisplay string
		if rawEnv, ok := server["env"].(map[string]any); ok && len(rawEnv) > 0 {
			parts := make([]string, 0, len(rawEnv))
			for k, v := range rawEnv {
				parts = append(parts, fmt.Sprintf("%s=%v", k, v))
			}
			envDisplay = strings.Join(parts, ", ")
		}
		fields = append(fields, detailField{label: "Environment", value: envDisplay, key: "env"})
	}

	desc, _ := server["description"].(string)
	fields = append(fields, detailField{label: "Description", value: desc, key: "description"})

	// Tool permissions entry (always last, visually separated)
	fields = append(fields, detailField{label: "Tool permissions", value: "", key: "tools"})

	return fields
}

// newDetailModel constructs a detail model for the given server.
func newDetailModel(name string, server map[string]any, scope string, tab manageTab) detailModel {
	return detailModel{
		name:   name,
		server: server,
		fields: buildDetailFields(server),
		cursor: 0,
		scope:  scope,
		tab:    tab,
	}
}

func (m detailModel) Init() tea.Cmd {
	return nil
}

func (m detailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if m.cursor < len(m.fields)-1 {
				m.cursor++
			}
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			f := m.fields[m.cursor]
			if f.key == "tools" {
				m.action = detailToolPermissions
			} else {
				m.action = detailEditField
				m.selKey = f.key
			}
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))):
			m.action = detailBack
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m detailModel) View() string {
	width := m.width
	if width <= 0 {
		width = 80
	}

	banner := tabbedBanner(m.scope, m.tab, width)

	var b strings.Builder

	// Header with back indicator and server name
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	b.WriteString("\n  " + headerStyle.Render(fmt.Sprintf("\u2190 %s", m.name)) + "\n\n")

	// Compute max label width for alignment
	maxLabel := 0
	for _, f := range m.fields {
		if len(f.label) > maxLabel {
			maxLabel = len(f.label)
		}
	}

	dim := lipgloss.NewStyle().Faint(true)
	cyan := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	cyanBold := cyan.Bold(true)
	arrowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	// Label style: subdued color to distinguish from values
	labelStyle := display.StyleLabel
	labelFocused := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)

	for i, f := range m.fields {
		// Add visual separator before "Tool permissions"
		if f.key == "tools" {
			b.WriteString("\n")
		}

		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		label := fmt.Sprintf("%-*s", maxLabel, f.label)

		if f.key == "tools" {
			// Tool permissions entry: show arrow suffix
			if i == m.cursor {
				b.WriteString("  " + cyanBold.Render(cursor) + labelFocused.Render(label) + "  " + arrowStyle.Render("\u2192") + "\n")
			} else {
				b.WriteString("  " + cursor + labelStyle.Render(label) + "  " + dim.Render("\u2192") + "\n")
			}
		} else {
			// Normal field
			value := f.value
			if value == "" {
				value = dim.Render("(not set)")
			}

			if i == m.cursor {
				b.WriteString("  " + cyanBold.Render(cursor) + labelFocused.Render(label) + "  " + cyan.Render(value) + "\n")
			} else {
				b.WriteString("  " + cursor + labelStyle.Render(label) + "  " + value + "\n")
			}
		}
	}

	// Help line
	b.WriteString("\n  " + dim.Render("enter edit  esc back") + "\n")

	return banner + "\n" + b.String()
}

// runServerDetail is the outer loop for the server detail view.
// It alternates between the detail TUI and inline field editors.
func runServerDetail(name string, servers config.ServerMap, scope string, tab manageTab) error {
	for {
		serverDef, ok := servers[name]
		if !ok {
			fmt.Printf("%s Server %s not found.\n",
				display.StyleYellow.Render("Warning:"),
				display.StyleCyan.Render(name))
			return nil
		}

		m := newDetailModel(name, serverDef, scope, tab)
		p := tea.NewProgram(m, tea.WithAltScreen())
		result, err := p.Run()
		if err != nil {
			return fmt.Errorf("detail view error: %w", err)
		}

		final := result.(detailModel)

		switch final.action {
		case detailBack, detailNone:
			return nil

		case detailEditField:
			// Clear screen for inline form
			fmt.Print("\033[2J\033[H")
			if err := editField(name, servers, final.selKey); err != nil && err != errCancelled {
				return err
			}
			// Loop back to show updated detail view

		case detailToolPermissions:
			// Clear screen for tools view
			fmt.Print("\033[2J\033[H")
			if err := runToolPermissions(name, servers, scope); err != nil && err != errCancelled {
				return err
			}
			// Reload servers in case tool permissions changed autoApprove
			reloaded, err := config.LoadServers()
			if err != nil {
				return err
			}
			for k, v := range reloaded {
				servers[k] = v
			}
		}
	}
}

// editField presents a single-field huh form for the given field key,
// updates the server definition in memory, and saves to central config.
func editField(name string, servers config.ServerMap, fieldKey string) error {
	server := servers[name]

	switch fieldKey {
	case "type":
		return editType(name, servers, server)
	case "url":
		return editURL(name, servers, server)
	case "auth":
		return editAuth(name, servers, server)
	case "command":
		return editCommand(name, servers, server)
	case "args":
		return editArgs(name, servers, server)
	case "env":
		return editEnv(name, servers, server)
	case "description":
		return editDescription(name, servers, server)
	}

	return nil
}

func editType(name string, servers config.ServerMap, server map[string]any) error {
	oldType, _ := server["type"].(string)
	newType := oldType

	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title(fmt.Sprintf("Transport type for %s", display.StyleCyan.Render(name))).
			Options(
				huh.NewOption("http (streamable HTTP)", "http"),
				huh.NewOption("sse (server-sent events)", "sse"),
				huh.NewOption("stdio (local process)", "stdio"),
			).
			Value(&newType),
	)).WithTheme(formTheme()).WithKeyMap(formKeyMap()).Run()
	if err != nil {
		return handleAbort(err)
	}

	if newType == oldType {
		return nil // no change
	}

	// Clear old type-specific fields
	switch oldType {
	case "http", "sse":
		delete(server, "url")
		delete(server, "headers")
	case "stdio":
		delete(server, "command")
		delete(server, "args")
		delete(server, "env")
	}

	server["type"] = newType
	servers[name] = server
	return config.SaveServers(servers)
}

func editURL(name string, servers config.ServerMap, server map[string]any) error {
	url, _ := server["url"].(string)

	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Server URL").
			Placeholder("https://example.com/mcp").
			Value(&url).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("URL is required")
				}
				if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
					return fmt.Errorf("URL must start with http:// or https://")
				}
				return nil
			}),
	)).WithTheme(formTheme()).WithKeyMap(formKeyMap()).Run()
	if err != nil {
		return handleAbort(err)
	}

	server["url"] = url
	servers[name] = server
	return config.SaveServers(servers)
}

func editAuth(name string, servers config.ServerMap, server map[string]any) error {
	// Detect existing auth type
	var authType string
	var prefillUsername string
	headers := serverHeaders(server)
	if headers != nil {
		detectedType, _ := display.DecodeAuth(headers)
		authType = detectedType
		if authType == "basic" {
			if authRaw, ok := headers["Authorization"].(string); ok && strings.HasPrefix(authRaw, "Basic ") {
				decoded, err := base64.StdEncoding.DecodeString(authRaw[6:])
				if err == nil {
					parts := strings.SplitN(string(decoded), ":", 2)
					if len(parts) == 2 {
						prefillUsername = parts[0]
					}
				}
			}
		}
	}

	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Authentication").
			Options(
				huh.NewOption("None", "none"),
				huh.NewOption("Basic (username + password)", "basic"),
				huh.NewOption("Bearer token", "bearer"),
				huh.NewOption("API key (Token header)", "apikey"),
			).
			Value(&authType),
	)).WithTheme(formTheme()).WithKeyMap(formKeyMap()).Run()
	if err != nil {
		return handleAbort(err)
	}

	switch authType {
	case "none":
		delete(server, "headers")

	case "basic":
		username := prefillUsername
		var password string
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Username").
					Value(&username).
					Validate(func(s string) error {
						if s == "" {
							return fmt.Errorf("username is required")
						}
						return nil
					}),
				huh.NewInput().
					Title("Password").
					EchoMode(huh.EchoModePassword).
					Value(&password).
					Validate(func(s string) error {
						if s == "" {
							return fmt.Errorf("password is required")
						}
						return nil
					}),
			),
		).WithTheme(formTheme()).WithKeyMap(formKeyMap()).Run()
		if err != nil {
			return handleAbort(err)
		}
		encoded := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		server["headers"] = map[string]any{
			"Authorization": "Basic " + encoded,
		}

	case "bearer":
		var token string
		err = huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title("Bearer token").
				EchoMode(huh.EchoModePassword).
				Value(&token).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("token is required")
					}
					return nil
				}),
		)).WithTheme(formTheme()).WithKeyMap(formKeyMap()).Run()
		if err != nil {
			return handleAbort(err)
		}
		server["headers"] = map[string]any{
			"Authorization": "Bearer " + token,
		}

	case "apikey":
		var token string
		err = huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title("API key").
				EchoMode(huh.EchoModePassword).
				Value(&token).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("API key is required")
					}
					return nil
				}),
		)).WithTheme(formTheme()).WithKeyMap(formKeyMap()).Run()
		if err != nil {
			return handleAbort(err)
		}
		server["headers"] = map[string]any{
			"Authorization": "Token " + token,
		}
	}

	servers[name] = server
	return config.SaveServers(servers)
}

func editCommand(name string, servers config.ServerMap, server map[string]any) error {
	command, _ := server["command"].(string)

	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Command").
			Placeholder("npx, uvx, node, python...").
			Value(&command).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("command is required")
				}
				return nil
			}),
	)).WithTheme(formTheme()).WithKeyMap(formKeyMap()).Run()
	if err != nil {
		return handleAbort(err)
	}

	server["command"] = command
	servers[name] = server
	return config.SaveServers(servers)
}

func editArgs(name string, servers config.ServerMap, server map[string]any) error {
	var argsStr string
	if rawArgs, ok := server["args"].([]any); ok {
		parts := make([]string, 0, len(rawArgs))
		for _, a := range rawArgs {
			if s, ok := a.(string); ok {
				if strings.ContainsAny(s, " \t") {
					s = `"` + s + `"`
				}
				parts = append(parts, s)
			}
		}
		argsStr = strings.Join(parts, " ")
	}

	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Arguments (space-separated)").
			Placeholder("-y @modelcontextprotocol/server-filesystem /path").
			Value(&argsStr),
	)).WithTheme(formTheme()).WithKeyMap(formKeyMap()).Run()
	if err != nil {
		return handleAbort(err)
	}

	if argsStr != "" {
		server["args"] = splitArgs(argsStr)
	} else {
		delete(server, "args")
	}

	servers[name] = server
	return config.SaveServers(servers)
}

func editEnv(name string, servers config.ServerMap, server map[string]any) error {
	var envStr string
	if rawEnv, ok := server["env"].(map[string]any); ok {
		parts := make([]string, 0, len(rawEnv))
		for k, v := range rawEnv {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
		envStr = strings.Join(parts, ", ")
	}

	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Environment variables (KEY=VALUE, comma-separated)").
			Placeholder("SOME_VAR=value, OTHER=value").
			Value(&envStr),
	)).WithTheme(formTheme()).WithKeyMap(formKeyMap()).Run()
	if err != nil {
		return handleAbort(err)
	}

	if envStr != "" {
		envMap := make(map[string]any)
		for _, pair := range strings.Split(envStr, ",") {
			pair = strings.TrimSpace(pair)
			if parts := strings.SplitN(pair, "=", 2); len(parts) == 2 {
				envMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
		if len(envMap) > 0 {
			server["env"] = envMap
		} else {
			delete(server, "env")
		}
	} else {
		delete(server, "env")
	}

	servers[name] = server
	return config.SaveServers(servers)
}

func editDescription(name string, servers config.ServerMap, server map[string]any) error {
	description, _ := server["description"].(string)
	defaultDesc := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")

	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Description").
			Value(&description).
			Placeholder(defaultDesc),
	)).WithTheme(formTheme()).WithKeyMap(formKeyMap()).Run()
	if err != nil {
		return handleAbort(err)
	}

	if description == "" {
		description = defaultDesc
	}
	server["description"] = description

	servers[name] = server
	return config.SaveServers(servers)
}
