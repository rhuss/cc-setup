package display

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/term"
	"github.com/rhuss/cc-setup/internal/config"
)

// Styles used throughout the CLI.
var (
	StyleTitle   = lipgloss.NewStyle().Bold(true)
	StyleCyan    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	StyleGreen   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	StyleRed     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	StyleYellow  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	StyleDim     = lipgloss.NewStyle().Faint(true)
	StyleLabel   = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	StyleHeader  = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	StyleCell    = lipgloss.NewStyle().Padding(0, 1)

	// Health indicator styles.
	StyleHealthOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))           // green
	StyleHealthAuth    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))           // yellow
	StyleHealthError   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))           // red
	StyleHealthUnknown = lipgloss.NewStyle().Faint(true)                               // dim

)

// TermWidth returns the current terminal width, defaulting to 80 if detection fails.
func TermWidth() int {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// MaskToken shows the first few characters of a token, masking the rest.
// The mask uses a fixed short suffix to keep output compact.
func MaskToken(token string, visible int) string {
	if visible <= 0 {
		visible = 6
	}
	if len(token) <= visible {
		return token
	}
	return token[:visible] + "..."
}

// DecodeAuth decodes auth from existing headers and returns (typeLabel, displayString).
func DecodeAuth(headers map[string]any) (string, string) {
	if headers == nil {
		return "none", "no auth"
	}

	authRaw, ok := headers["Authorization"]
	if !ok {
		return "none", "no auth"
	}
	auth, ok := authRaw.(string)
	if !ok {
		return "unknown", "auth configured"
	}

	if strings.HasPrefix(auth, "Basic ") {
		decoded, err := base64.StdEncoding.DecodeString(auth[6:])
		if err == nil {
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				return "basic", fmt.Sprintf("basic (user: %s)", parts[0])
			}
		}
		return "basic", "basic"
	}
	if strings.HasPrefix(auth, "Token ") {
		return "apikey", fmt.Sprintf("apikey (%s)", MaskToken(auth[6:], 6))
	}
	if strings.HasPrefix(auth, "Bearer ") {
		return "bearer", fmt.Sprintf("bearer (%s)", MaskToken(auth[7:], 6))
	}

	return "unknown", "auth configured"
}

// ServerEndpoint returns a short display string for the server's endpoint.
func ServerEndpoint(serverDef map[string]any) string {
	stype, _ := serverDef["type"].(string)
	if stype == "stdio" {
		cmd, _ := serverDef["command"].(string)
		args, _ := serverDef["args"].([]any)
		if len(args) > 0 {
			firstArg, _ := args[0].(string)
			return fmt.Sprintf("%s %s", cmd, firstArg)
		}
		return cmd
	}
	url, _ := serverDef["url"].(string)
	return url
}

// serverHeaders extracts the headers map from a server definition.
func serverHeaders(serverDef map[string]any) map[string]any {
	h, _ := serverDef["headers"].(map[string]any)
	return h
}

// RenderServerTable prints a formatted table of servers to stdout.
func RenderServerTable(servers config.ServerMap, showDescription bool) string {
	names := config.ServerNames(servers)
	if len(names) == 0 {
		return StyleYellow.Render("No servers configured")
	}

	headers := []string{"Server", "Type", "Endpoint", "Auth"}
	if showDescription {
		headers = append(headers, "Description")
	}

	rows := make([][]string, 0, len(names))
	for _, name := range names {
		info := servers[name]
		stype, _ := info["type"].(string)
		if stype == "" {
			stype = "?"
		}
		endpoint := ServerEndpoint(info)
		_, authLabel := DecodeAuth(serverHeaders(info))
		desc, _ := info["description"].(string)

		row := []string{name, stype, endpoint, authLabel}
		if showDescription {
			row = append(row, desc)
		}
		rows = append(rows, row)
	}

	t := table.New().
		Width(TermWidth()).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return StyleHeader
			}
			s := StyleCell
			switch col {
			case 0:
				s = s.Foreground(lipgloss.Color("6"))
			}
			if showDescription && col == len(headers)-1 {
				s = s.Faint(true)
			}
			return s
		})

	return t.Render()
}

// RenderActionTable prints a summary table with add/remove actions.
func RenderActionTable(servers config.ServerMap, selected, toRemove []string) string {
	headers := []string{"Server", "Action", "Endpoint", "Auth"}
	rows := make([][]string, 0, len(selected)+len(toRemove))

	for _, name := range selected {
		info := servers[name]
		endpoint := ServerEndpoint(info)
		_, authLabel := DecodeAuth(serverHeaders(info))
		rows = append(rows, []string{name, "add/update", endpoint, authLabel})
	}

	for _, name := range toRemove {
		info := servers[name]
		endpoint := ServerEndpoint(info)
		rows = append(rows, []string{name, "remove", endpoint, "-"})
	}

	t := table.New().
		Width(TermWidth()).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return StyleHeader
			}
			s := StyleCell
			switch col {
			case 0:
				s = s.Foreground(lipgloss.Color("6"))
			case 1:
				// Color the action column: first len(selected) rows are adds, rest are removes
				if row < len(selected) {
					s = s.Foreground(lipgloss.Color("2"))
				} else {
					s = s.Foreground(lipgloss.Color("1"))
				}
			}
			return s
		})

	return t.Render()
}

// RenderImportTable prints a table showing servers to import.
func RenderImportTable(imported config.ServerMap, existing config.ServerMap) string {
	names := config.ServerNames(imported)
	headers := []string{"Server", "Action", "Type", "Endpoint"}

	rows := make([][]string, 0, len(names))
	for _, name := range names {
		entry := imported[name]
		stype, _ := entry["type"].(string)
		if stype == "" {
			stype = "?"
		}
		endpoint := ServerEndpoint(entry)
		action := "import"
		if _, exists := existing[name]; exists {
			action = "skip (exists)"
		}
		rows = append(rows, []string{name, action, stype, endpoint})
	}

	t := table.New().
		Width(TermWidth()).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return StyleHeader
			}
			s := StyleCell
			switch col {
			case 0:
				s = s.Foreground(lipgloss.Color("6"))
			case 1:
				// Color based on action text in the row data
				if row >= 0 && row < len(rows) {
					if rows[row][1] == "import" {
						s = s.Foreground(lipgloss.Color("2"))
					} else {
						s = s.Faint(true)
					}
				}
			}
			return s
		})

	return t.Render()
}
