package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/rhuss/cc-mcp-setup/internal/config"
	"github.com/rhuss/cc-mcp-setup/internal/display"
	mcpclient "github.com/rhuss/cc-mcp-setup/internal/mcp"
)

// manageAction represents the action chosen from the server list.
type manageAction int

const (
	actionNone manageAction = iota
	actionAdd
	actionEdit
	actionDelete
	actionSave
	actionImport
	actionTools
	actionQuit
)

// serverItem implements list.Item for the bubbles list.
type serverItem struct {
	name   string
	detail string // "type | endpoint | auth"
	desc   string // raw description text
}

func (i serverItem) Title() string       { return i.name }
func (i serverItem) Description() string { return i.detail }
func (i serverItem) FilterValue() string { return i.name + " " + i.detail + " " + i.desc }

// buildServerItems converts a ServerMap into list items with type, endpoint, and auth info.
func buildServerItems(servers config.ServerMap) []list.Item {
	names := config.ServerNames(servers)
	items := make([]list.Item, 0, len(names))
	for _, name := range names {
		info := servers[name]
		stype, _ := info["type"].(string)
		if stype == "" {
			stype = "?"
		}
		endpoint := display.ServerEndpoint(info)
		_, authLabel := display.DecodeAuth(serverHeaders(info))
		desc, _ := info["description"].(string)

		parts := []string{stype}
		if endpoint != "" {
			parts = append(parts, endpoint)
		}
		parts = append(parts, authLabel)
		detail := strings.Join(parts, " | ")

		items = append(items, serverItem{name: name, detail: detail, desc: desc})
	}
	return items
}

// serverHeaders extracts the headers map from a server definition.
func serverHeaders(serverDef map[string]any) map[string]any {
	h, _ := serverDef["headers"].(map[string]any)
	return h
}

// healthResultMsg delivers the result of an async health check.
type healthResultMsg struct {
	name   string
	result mcpclient.HealthResult
}

// checkboxDelegate renders single-line items with [x]/[ ] checkboxes.
type checkboxDelegate struct {
	checked map[string]bool
	health  map[string]mcpclient.HealthResult
}

func (d checkboxDelegate) Height() int                             { return 1 }
func (d checkboxDelegate) Spacing() int                            { return 0 }
func (d checkboxDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d checkboxDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(serverItem)
	if !ok {
		return
	}

	isFocused := index == m.Index()
	isChecked := d.checked[si.name]

	// Health indicator
	healthDot := display.StyleHealthUnknown.Render("○")
	if hr, ok := d.health[si.name]; ok {
		switch hr.Status {
		case mcpclient.HealthOK:
			healthDot = display.StyleHealthOK.Render("●")
		case mcpclient.HealthAuthRequired:
			healthDot = display.StyleHealthAuth.Render("●")
		case mcpclient.HealthError:
			healthDot = display.StyleHealthError.Render("●")
		}
	}

	cursor := "  "
	if isFocused {
		cursor = "> "
	}

	cb := "[ ]"
	if isChecked {
		cb = "[x]"
	}

	nameWidth := 18
	if len(si.name) >= nameWidth {
		nameWidth = len(si.name) + 1
	}
	paddedName := fmt.Sprintf("%-*s", nameWidth, si.name)

	if isFocused {
		cyan := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
		cyanBold := cyan.Bold(true)
		green := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
		dim := lipgloss.NewStyle().Faint(true)

		cbStyled := cyanBold.Render(cb)
		if isChecked {
			cbStyled = green.Render(cb)
		}

		line := healthDot + " " + cyanBold.Render(cursor) + cbStyled + " " + cyanBold.Render(paddedName) + cyan.Render(si.detail)
		if si.desc != "" {
			line += "  " + dim.Render(si.desc)
		}
		fmt.Fprint(w, line)
	} else {
		dim := lipgloss.NewStyle().Faint(true)
		green := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))

		cbStyled := dim.Render(cb)
		if isChecked {
			cbStyled = green.Render(cb)
		}

		line := healthDot + " " + cursor + cbStyled + " " + paddedName + dim.Render(si.detail)
		if si.desc != "" {
			line += "  " + dim.Render(si.desc)
		}
		fmt.Fprint(w, line)
	}
}

// manageKeyMap holds the key bindings for the unified manage screen.
type manageKeyMap struct {
	Toggle       key.Binding
	Add          key.Binding
	Edit         key.Binding
	Delete       key.Binding
	Save         key.Binding
	Import       key.Binding
	Tools        key.Binding
	ScopeProject key.Binding
	ScopeUser    key.Binding
	Quit         key.Binding
}

func newManageKeyMap() manageKeyMap {
	return manageKeyMap{
		Toggle:       key.NewBinding(key.WithKeys(" ", "x"), key.WithHelp("space", "toggle")),
		Add:          key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
		Edit:         key.NewBinding(key.WithKeys("e", "enter"), key.WithHelp("e/enter", "edit")),
		Delete:       key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		Save:         key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "save")),
		Import:       key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "import")),
		Tools:        key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tools")),
		ScopeProject: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "project scope")),
		ScopeUser:    key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "user scope")),
		Quit:         key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "quit")),
	}
}

// bannerHeight is the number of lines the scope banner occupies.
const bannerHeight = 1

// manageModel is the BubbleTea model for the unified server management screen.
type manageModel struct {
	list     list.Model
	keys     manageKeyMap
	action   manageAction
	selected string
	checked  map[string]bool
	health   map[string]mcpclient.HealthResult
	scope    string
	servers  config.ServerMap
	width    int
}

// loadCheckedState reads the Claude config for the given scope and returns
// a map of server names to whether they are enabled.
func loadCheckedState(servers config.ServerMap, scope string) map[string]bool {
	existing := config.ReadMcpServers(scope)
	checked := make(map[string]bool, len(servers))
	for name := range servers {
		if _, ok := existing[name]; ok {
			checked[name] = true
		}
	}
	return checked
}

// scopeBanner renders a full-width colored bar showing the active scope.
func scopeBanner(scope string, width int) string {
	if width <= 0 {
		width = 80
	}

	path := config.ConfigPath(scope)
	label := fmt.Sprintf(" %s  %s ", strings.ToUpper(scope), path)
	pad := width - lipgloss.Width(label)
	if pad < 0 {
		pad = 0
	}
	label += strings.Repeat(" ", pad)

	var style lipgloss.Style
	if scope == "project" {
		style = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#ffffff")).
			Background(lipgloss.Color("#225599"))
	} else {
		style = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#ffffff")).
			Background(lipgloss.Color("#884488"))
	}

	return style.Render(label)
}

func newManageModel(servers config.ServerMap, scope string, checked map[string]bool) manageModel {
	items := buildServerItems(servers)
	keys := newManageKeyMap()
	health := make(map[string]mcpclient.HealthResult, len(servers))

	delegate := checkboxDelegate{checked: checked, health: health}

	l := list.New(items, delegate, 0, 0)
	l.Title = "MCP Servers"
	l.Styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("6")).
		Padding(0, 1)
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()

	// Short help: toggle, add, edit, delete, save, import, tools
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{keys.Toggle, keys.Add, keys.Edit, keys.Delete, keys.Save, keys.Import, keys.Tools}
	}
	// Full help adds: scope keys
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{keys.Toggle, keys.Add, keys.Edit, keys.Delete, keys.Save, keys.Import, keys.Tools, keys.ScopeProject, keys.ScopeUser}
	}

	return manageModel{
		list:    l,
		keys:    keys,
		checked: checked,
		health:  health,
		scope:   scope,
		servers: servers,
	}
}

func (m manageModel) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.servers))
	for name, def := range m.servers {
		name, def := name, def // capture loop vars
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result := mcpclient.CheckHealth(ctx, name, def)
			return healthResultMsg{name: name, result: result}
		})
	}
	return tea.Batch(cmds...)
}

func (m manageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case healthResultMsg:
		m.health[msg.name] = msg.result
		if msg.result.Status == mcpclient.HealthAuthRequired {
			m.list.NewStatusMessage(
				display.StyleYellow.Render(msg.name+": ") +
					display.StyleDim.Render("reachable (requires OAuth)"),
			)
		} else if msg.result.Err != nil {
			m.list.NewStatusMessage(
				display.StyleRed.Render(msg.name+": ") +
					display.StyleDim.Render(msg.result.Err.Error()),
			)
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.list.SetSize(msg.Width, msg.Height-bannerHeight)
		return m, nil

	case tea.KeyMsg:
		// Don't intercept keys while filtering
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			m.action = actionQuit
			return m, tea.Quit

		case key.Matches(msg, m.keys.Toggle):
			if item, ok := m.list.SelectedItem().(serverItem); ok {
				m.checked[item.name] = !m.checked[item.name]
				return m, nil
			}

		case key.Matches(msg, m.keys.ScopeProject):
			if m.scope != "project" {
				m.scope = "project"
				m.reloadCheckedState()
			}
			return m, nil

		case key.Matches(msg, m.keys.ScopeUser):
			if m.scope != "user" {
				m.scope = "user"
				m.reloadCheckedState()
			}
			return m, nil

		case key.Matches(msg, m.keys.Add):
			m.action = actionAdd
			return m, tea.Quit

		case key.Matches(msg, m.keys.Edit):
			if item, ok := m.list.SelectedItem().(serverItem); ok {
				m.action = actionEdit
				m.selected = item.name
				return m, tea.Quit
			}

		case key.Matches(msg, m.keys.Delete):
			if item, ok := m.list.SelectedItem().(serverItem); ok {
				m.action = actionDelete
				m.selected = item.name
				return m, tea.Quit
			}

		case key.Matches(msg, m.keys.Save):
			m.action = actionSave
			return m, tea.Quit

		case key.Matches(msg, m.keys.Import):
			m.action = actionImport
			return m, tea.Quit

		case key.Matches(msg, m.keys.Tools):
			if item, ok := m.list.SelectedItem().(serverItem); ok {
				m.action = actionTools
				m.selected = item.name
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// reloadCheckedState clears the checked map and refills it from the current scope's
// Claude config. The map is mutated in place so the delegate's reference stays valid.
func (m *manageModel) reloadCheckedState() {
	for k := range m.checked {
		delete(m.checked, k)
	}
	existing := config.ReadMcpServers(m.scope)
	for name := range m.servers {
		if _, ok := existing[name]; ok {
			m.checked[name] = true
		}
	}
}

func (m manageModel) View() string {
	return scopeBanner(m.scope, m.width) + "\n" + m.list.View()
}

// runSave computes adds/removes vs the current Claude config, shows a summary,
// confirms, and writes to the scope's config file.
func runSave(servers config.ServerMap, checked map[string]bool, scope string) error {
	existing := config.ReadMcpServers(scope)

	var selected []string
	for _, name := range config.ServerNames(servers) {
		if checked[name] {
			selected = append(selected, name)
		}
	}

	// Servers that were configured but are now unchecked (only those in central config)
	var toRemove []string
	for name := range existing {
		if _, inCentral := servers[name]; inCentral && !checked[name] {
			toRemove = append(toRemove, name)
		}
	}

	if len(selected) == 0 && len(toRemove) == 0 {
		fmt.Println(display.StyleDim.Render("No changes to apply."))
		return nil
	}

	target := config.ConfigPath(scope)
	scopeLabel := "user (global)"
	if scope == "project" {
		scopeLabel = "project (cwd)"
	}

	fmt.Println()
	fmt.Printf("  Scope: %s\n", display.StyleTitle.Render(scopeLabel))
	fmt.Printf("  File:  %s\n", target)
	fmt.Println()
	fmt.Println(display.RenderActionTable(servers, selected, toRemove))
	fmt.Println()

	confirmed, err := confirm("Apply this configuration?", true)
	if err != nil {
		return handleAbort(err)
	}
	if !confirmed {
		return errCancelled
	}

	entries := config.ServerMap{}
	for _, name := range selected {
		entries[name] = config.BuildEntryForClaude(servers[name])
	}

	path, err := config.WriteMcpServers(scope, entries, toRemove)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	if len(entries) > 0 {
		names := make([]string, 0, len(entries))
		for n := range entries {
			names = append(names, n)
		}
		fmt.Printf("  %s %s\n", display.StyleGreen.Render("Added"), strings.Join(names, ", "))
	}
	if len(toRemove) > 0 {
		fmt.Printf("  %s %s\n", display.StyleRed.Render("Removed"), strings.Join(toRemove, ", "))
	}
	fmt.Println()
	fmt.Printf("  Written to %s\n", path)
	fmt.Println()
	fmt.Println(display.StyleDim.Render("Run '/mcp' in Claude Code to verify server connectivity."))
	fmt.Println()

	return nil
}

// runManage is the outer loop that alternates between the BubbleTea screen and action handlers.
// Scope persists across iterations so the user stays in their chosen scope.
func runManage() error {
	scope := "project"

	for {
		servers, err := config.LoadServers()
		if err != nil {
			return err
		}

		if len(servers) == 0 {
			return runManageEmpty()
		}

		checked := loadCheckedState(servers, scope)
		m := newManageModel(servers, scope, checked)
		p := tea.NewProgram(m, tea.WithAltScreen())
		result, err := p.Run()
		if err != nil {
			return fmt.Errorf("list error: %w", err)
		}

		final := result.(manageModel)
		scope = final.scope // preserve scope for next iteration

		// Clear screen after leaving alt screen so inline forms start clean
		if final.action != actionQuit && final.action != actionNone {
			fmt.Print("\033[2J\033[H")
		}

		switch final.action {
		case actionQuit, actionNone:
			return nil

		case actionAdd:
			if err := runAddInteractive(); err != nil && err != errCancelled {
				return err
			}

		case actionEdit:
			if err := runEdit(final.selected); err != nil && err != errCancelled {
				return err
			}

		case actionDelete:
			if err := runDeleteSingle(final.selected); err != nil && err != errCancelled {
				return err
			}

		case actionSave:
			if err := runSave(final.servers, final.checked, final.scope); err != nil && err != errCancelled {
				return err
			}

		case actionImport:
			if err := runImport(); err != nil && err != errCancelled {
				return err
			}

		case actionTools:
			if err := runToolPermissions(final.selected, final.servers, final.scope); err != nil && err != errCancelled {
				return err
			}
		}

		// Loop back to re-read config and show fresh list
	}
}

// runManageEmpty handles the case when no servers are configured.
func runManageEmpty() error {
	fmt.Println()
	fmt.Println(display.StyleYellow.Render(
		fmt.Sprintf("No servers configured in %s", config.GetConfigFile()),
	))
	fmt.Println()

	var choice string
	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Get started").
			Options(
				huh.NewOption("Add a new server", "add"),
				huh.NewOption("Import from existing Claude config", "import"),
				huh.NewOption("Quit", "quit"),
			).
			Value(&choice),
	)).WithKeyMap(formKeyMap()).Run()
	if err != nil {
		return handleAbort(err)
	}

	switch choice {
	case "add":
		return runAddInteractive()
	case "import":
		return runImport()
	}
	return nil
}

// runDeleteSingle confirms and deletes a single server.
func runDeleteSingle(name string) error {
	servers, err := config.LoadServers()
	if err != nil {
		return err
	}

	info, exists := servers[name]
	if !exists {
		fmt.Printf("%s Server %s not found.\n",
			display.StyleYellow.Render("Warning:"),
			display.StyleCyan.Render(name))
		return nil
	}

	endpoint := display.ServerEndpoint(info)
	fmt.Println()
	fmt.Printf("  %s %s (%s)\n",
		display.StyleRed.Render("delete"),
		display.StyleCyan.Render(name),
		endpoint)
	fmt.Println()

	confirmed, err := confirm("Remove this server from the central config?", false)
	if err != nil {
		return handleAbort(err)
	}
	if !confirmed {
		return nil
	}

	delete(servers, name)
	if err := config.SaveServers(servers); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %s %s removed from %s\n",
		display.StyleGreen.Render("Done:"),
		display.StyleCyan.Render(name),
		config.GetConfigFile())
	fmt.Println()
	return nil
}
