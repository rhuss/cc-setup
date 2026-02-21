package cmd

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/rhuss/cc-setup/internal/config"
	"github.com/rhuss/cc-setup/internal/display"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a server to the central config",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if strings.ContainsAny(name, " \t") {
			return fmt.Errorf("server name must not contain spaces")
		}
		return runServerForm(name, nil)
	},
}

// runAddInteractive prompts for a server name, then runs the add form.
func runAddInteractive() error {
	var name string
	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Server name").
			Placeholder("my-server").
			Value(&name).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("name is required")
				}
				if strings.ContainsAny(s, " \t") {
					return fmt.Errorf("name must not contain spaces")
				}
				return nil
			}),
	)).WithTheme(formTheme()).WithKeyMap(formKeyMap()).Run()
	if err != nil {
		return handleAbort(err)
	}

	return runServerForm(name, nil)
}

// runEdit loads an existing server entry and runs the form with pre-filled values.
func runEdit(name string) error {
	servers, err := config.LoadServers()
	if err != nil {
		return err
	}

	existing, ok := servers[name]
	if !ok {
		fmt.Printf("%s Server %s not found.\n",
			display.StyleYellow.Render("Warning:"),
			display.StyleCyan.Render(name))
		return nil
	}

	return runServerForm(name, existing)
}

// runServerForm handles both add and edit. When prefill is nil, it's an add operation.
// When prefill is non-nil, values are pre-populated from the existing entry.
func runServerForm(name string, prefill map[string]any) error {
	isEdit := prefill != nil

	servers, err := config.LoadServers()
	if err != nil {
		return err
	}

	// For add mode, check if server already exists
	if !isEdit {
		if _, exists := servers[name]; exists {
			fmt.Printf("%s Server %s already exists in the central config.\n",
				display.StyleYellow.Render("Warning:"),
				display.StyleCyan.Render(name))

			overwrite, err := confirm("Overwrite existing entry?", false)
			if err != nil {
				return handleAbort(err)
			}
			if !overwrite {
				return errCancelled
			}
		}
	}

	// Step 1: Select transport type (pre-set from prefill if editing)
	var serverType string
	if isEdit {
		serverType, _ = prefill["type"].(string)
	}
	err = huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title(fmt.Sprintf("Transport type for %s", display.StyleCyan.Render(name))).
			Options(
				huh.NewOption("http (streamable HTTP)", "http"),
				huh.NewOption("sse (server-sent events)", "sse"),
				huh.NewOption("stdio (local process)", "stdio"),
			).
			Value(&serverType),
	)).WithTheme(formTheme()).WithKeyMap(formKeyMap()).Run()
	if err != nil {
		return handleAbort(err)
	}

	entry := map[string]any{
		"type": serverType,
	}

	// Step 2: Type-specific fields
	switch serverType {
	case "http", "sse":
		if err := httpFields(entry, prefill); err != nil {
			return err
		}
	case "stdio":
		if err := stdioFields(entry, prefill); err != nil {
			return err
		}
	}

	// Step 3: Description (pre-set from prefill if editing)
	defaultDesc := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")
	var description string
	if isEdit {
		if d, ok := prefill["description"].(string); ok {
			description = d
		}
	}
	err = huh.NewForm(huh.NewGroup(
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
	entry["description"] = description

	// Step 4: Review and confirm
	label := display.StyleLabel
	value := display.StyleCyan

	fmt.Println()
	fmt.Printf("  %s  %s\n", label.Render("Server:     "), value.Render(name))
	fmt.Printf("  %s  %s\n", label.Render("Type:       "), value.Render(serverType))
	if url, ok := entry["url"].(string); ok {
		fmt.Printf("  %s  %s\n", label.Render("URL:        "), value.Render(url))
	}
	if cmd, ok := entry["command"].(string); ok {
		fmt.Printf("  %s  %s\n", label.Render("Command:    "), value.Render(cmd))
	}
	if args, ok := entry["args"].([]string); ok && len(args) > 0 {
		fmt.Printf("  %s  %s\n", label.Render("Args:       "), value.Render(strings.Join(args, " ")))
	}
	if env, ok := entry["env"].(map[string]any); ok && len(env) > 0 {
		parts := make([]string, 0, len(env))
		for k, v := range env {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
		fmt.Printf("  %s  %s\n", label.Render("Env:        "), value.Render(strings.Join(parts, ", ")))
	}
	if headers, ok := entry["headers"].(map[string]any); ok {
		_, authLabel := display.DecodeAuth(headers)
		fmt.Printf("  %s  %s\n", label.Render("Auth:       "), value.Render(authLabel))
	}
	fmt.Printf("  %s  %s\n", label.Render("Description:"), value.Render(description))
	fmt.Println()

	confirmLabel := "Add this server?"
	doneVerb := "added"
	if isEdit {
		confirmLabel = "Save changes?"
		doneVerb = "updated"
	}

	confirmed, err := confirm(confirmLabel, true)
	if err != nil {
		return handleAbort(err)
	}
	if !confirmed {
		return errCancelled
	}

	servers[name] = entry
	if err := config.SaveServers(servers); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %s %s %s to %s\n",
		display.StyleGreen.Render("Done:"),
		display.StyleCyan.Render(name),
		doneVerb,
		config.GetConfigFile())
	fmt.Println()
	return nil
}

// httpFields collects HTTP/SSE-specific fields, pre-filling from prefill if provided.
func httpFields(entry map[string]any, prefill map[string]any) error {
	var url string
	if prefill != nil {
		url, _ = prefill["url"].(string)
	}
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
	entry["url"] = url

	// Detect existing auth type from prefill
	var authType string
	var prefillUsername string
	if prefill != nil {
		if headers, ok := prefill["headers"].(map[string]any); ok {
			detectedType, _ := display.DecodeAuth(headers)
			authType = detectedType
			// Extract username for basic auth pre-fill
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
	}

	err = huh.NewForm(huh.NewGroup(
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
		entry["headers"] = map[string]any{
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
		entry["headers"] = map[string]any{
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
		entry["headers"] = map[string]any{
			"Authorization": "Token " + token,
		}
	}

	return nil
}

// stdioFields collects stdio-specific fields, pre-filling from prefill if provided.
func stdioFields(entry map[string]any, prefill map[string]any) error {
	var command, argsStr, envStr string

	if prefill != nil {
		command, _ = prefill["command"].(string)

		// Join args back to a space-separated string
		if rawArgs, ok := prefill["args"].([]any); ok {
			parts := make([]string, 0, len(rawArgs))
			for _, a := range rawArgs {
				if s, ok := a.(string); ok {
					// Quote args containing spaces
					if strings.ContainsAny(s, " \t") {
						s = `"` + s + `"`
					}
					parts = append(parts, s)
				}
			}
			argsStr = strings.Join(parts, " ")
		}

		// Join env back to KEY=VALUE, ... string
		if rawEnv, ok := prefill["env"].(map[string]any); ok {
			parts := make([]string, 0, len(rawEnv))
			for k, v := range rawEnv {
				parts = append(parts, fmt.Sprintf("%s=%v", k, v))
			}
			envStr = strings.Join(parts, ", ")
		}
	}

	err := huh.NewForm(
		huh.NewGroup(
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
			huh.NewInput().
				Title("Arguments (space-separated)").
				Placeholder("-y @modelcontextprotocol/server-filesystem /path").
				Value(&argsStr),
			huh.NewInput().
				Title("Environment variables (KEY=VALUE, comma-separated)").
				Placeholder("SOME_VAR=value, OTHER=value").
				Value(&envStr),
		),
	).WithTheme(formTheme()).WithKeyMap(formKeyMap()).Run()
	if err != nil {
		return handleAbort(err)
	}

	entry["command"] = command

	if argsStr != "" {
		entry["args"] = splitArgs(argsStr)
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
			entry["env"] = envMap
		}
	}

	return nil
}

// splitArgs splits a space-separated string into args, respecting quoted strings.
func splitArgs(s string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			if c == quoteChar {
				inQuote = false
			} else {
				current.WriteByte(c)
			}
		} else if c == '"' || c == '\'' {
			inQuote = true
			quoteChar = c
		} else if c == ' ' || c == '\t' {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		} else {
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

// errCancelled is a sentinel indicating the user pressed ESC to cancel.
var errCancelled = fmt.Errorf("cancelled")

func handleAbort(err error) error {
	if err == huh.ErrUserAborted {
		return errCancelled
	}
	return err
}
