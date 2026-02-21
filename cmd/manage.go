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
	"github.com/rhuss/cc-setup/internal/config"
	"github.com/rhuss/cc-setup/internal/display"
	mcpclient "github.com/rhuss/cc-setup/internal/mcp"
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
	actionSavePlugins
	actionSaveAll
	actionQuit
)

// manageTab identifies which tab is active.
type manageTab int

const (
	tabServers manageTab = iota
	tabPlugins
)

// serverItem implements list.Item for the bubbles list.
type serverItem struct {
	name   string
	detail string // "type | endpoint | auth"
	desc   string // raw description text
}

func (i serverItem) Title() string       { return i.name }
func (i serverItem) Description() string { return i.detail }
func (i serverItem) FilterValue() string { return i.name + filterSep + i.detail + " " + i.desc }

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
	ScopeProject key.Binding
	ScopeUser    key.Binding
	ScopeToggle  key.Binding
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
		ScopeProject: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "project scope")),
		ScopeUser:    key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "user scope")),
		ScopeToggle:  key.NewBinding(key.WithKeys("."), key.WithHelp(".", "switch scope")),
		Quit:         key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "quit")),
	}
}

// bannerHeight is the number of lines the scope banner occupies.
const bannerHeight = 1

// filterSep separates the name from other fields in FilterValue().
const filterSep = "\x00"

// nameFirstFilter matches against item names first. If any name matches the
// search term, only those results are returned. Otherwise it falls back to
// matching against the full filter value (name + detail + description).
func nameFirstFilter(term string, targets []string) []list.Rank {
	names := make([]string, len(targets))
	for i, t := range targets {
		if idx := strings.Index(t, filterSep); idx >= 0 {
			names[i] = t[:idx]
		} else {
			names[i] = t
		}
	}
	if ranks := list.DefaultFilter(term, names); len(ranks) > 0 {
		return ranks
	}
	// Strip separators so DefaultFilter sees clean strings.
	clean := make([]string, len(targets))
	for i, t := range targets {
		clean[i] = strings.ReplaceAll(t, filterSep, " ")
	}
	return list.DefaultFilter(term, clean)
}

// tabKey switches between MCP Servers and Plugins tabs.
var tabKey = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch tab"))

// pendingChange describes a single unsaved modification.
type pendingChange struct {
	name   string
	action string // "add", "remove", "enable", "disable"
}

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

	// Plugin tab state
	tab            manageTab
	pluginList     list.Model
	pluginKeys     pluginKeyMap
	plugins        []config.PluginInfo
	pluginChecked  map[string]bool
	pluginWidthPtr *int

	// Quit confirmation state
	confirmQuit          bool
	pendingServerChanges []pendingChange
	pendingPluginChanges []pendingChange
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

// tabbedBanner renders a full-width bar with pill-style tab selector on the
// left and scope selector on the right. Active items are shown as colored
// background pills (white text on colored bg), inactive items are dimmed.
// The file path is shown dimmed between them.
//
// Layout:  [MCP Servers]  Plugins      path      [Project]  User
func tabbedBanner(scope string, tab manageTab, width int) string {
	if width <= 0 {
		width = 80
	}

	barBg := lipgloss.Color("#1e1e2e")
	inactive := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")).
		Background(barBg)

	// Tab pill: teal family
	tabPill := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ffffff")).
		Background(lipgloss.Color("#0e7490")).
		Bold(true).
		Padding(0, 1)

	// Scope pills: blue for project, purple for user
	projectPill := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ffffff")).
		Background(lipgloss.Color("#225599")).
		Bold(true).
		Padding(0, 1)
	userPill := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ffffff")).
		Background(lipgloss.Color("#884488")).
		Bold(true).
		Padding(0, 1)

	// Left side: tab selector
	var serversRendered, pluginsRendered string
	if tab == tabServers {
		serversRendered = tabPill.Render("MCP Servers")
		pluginsRendered = inactive.Render("Plugins")
	} else {
		serversRendered = inactive.Render("MCP Servers")
		pluginsRendered = tabPill.Render("Plugins")
	}
	sep := inactive.Render("  ")
	tabPart := inactive.Render(" ") + serversRendered + sep + pluginsRendered

	// Right side: scope selector
	var projectRendered, userRendered string
	if scope == "project" {
		projectRendered = projectPill.Render("Project")
		userRendered = inactive.Render("User")
	} else {
		projectRendered = inactive.Render("Project")
		userRendered = userPill.Render("User")
	}
	scopePart := projectRendered + sep + userRendered + inactive.Render(" ")

	// Center: file path (dimmed)
	var path string
	if tab == tabServers {
		path = config.ConfigPath(scope)
	} else {
		path = config.PluginSettingsPath(scope)
	}
	pathStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#777777")).
		Background(barBg)

	// Calculate spacing
	tabWidth := lipgloss.Width(tabPart)
	scopeWidth := lipgloss.Width(scopePart)
	pathWidth := lipgloss.Width(path)

	// Distribute remaining space: path centered, with padding on both sides
	remaining := width - tabWidth - scopeWidth
	if remaining < pathWidth+4 {
		// Not enough room for path, just pad between tab and scope
		if remaining < 0 {
			remaining = 0
		}
		padStyle := lipgloss.NewStyle().Background(barBg)
		content := tabPart + padStyle.Render(strings.Repeat(" ", remaining)) + scopePart
		return content
	}

	leftPad := (remaining - pathWidth) / 2
	rightPad := remaining - pathWidth - leftPad
	if leftPad < 2 {
		leftPad = 2
	}
	if rightPad < 1 {
		rightPad = 1
	}

	padStyle := lipgloss.NewStyle().Background(barBg)

	content := tabPart +
		padStyle.Render(strings.Repeat(" ", leftPad)) +
		pathStyle.Render(path) +
		padStyle.Render(strings.Repeat(" ", rightPad)) +
		scopePart

	// Pad to full width
	contentWidth := lipgloss.Width(content)
	if contentWidth < width {
		extra := width - contentWidth
		content = tabPart +
			padStyle.Render(strings.Repeat(" ", leftPad)) +
			pathStyle.Render(path) +
			padStyle.Render(strings.Repeat(" ", rightPad+extra)) +
			scopePart
	}

	return content
}

func newManageModel(servers config.ServerMap, scope string, checked map[string]bool, plugins []config.PluginInfo, pluginChecked map[string]bool, tab manageTab) manageModel {
	items := buildServerItems(servers)
	keys := newManageKeyMap()
	health := make(map[string]mcpclient.HealthResult, len(servers))

	delegate := checkboxDelegate{checked: checked, health: health}

	l := list.New(items, delegate, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.Filter = nameFirstFilter
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()

	// Short help: toggle, add, edit, delete, save, import, tab
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{keys.Toggle, keys.Add, keys.Edit, keys.Delete, keys.Save, keys.Import, tabKey}
	}
	// Full help adds: scope keys
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{keys.Toggle, keys.Add, keys.Edit, keys.Delete, keys.Save, keys.Import, keys.ScopeProject, keys.ScopeUser, tabKey}
	}

	// Build plugin list
	pluginItems := buildPluginItems(plugins)
	pKeys := newPluginKeyMap()
	pluginWidthPtr := new(int)
	*pluginWidthPtr = 80
	pDelegate := pluginCheckboxDelegate{checked: pluginChecked, width: pluginWidthPtr}

	pl := list.New(pluginItems, pDelegate, 0, 0)
	pl.SetShowTitle(false)
	pl.SetShowStatusBar(false)
	pl.Filter = nameFirstFilter
	pl.SetFilteringEnabled(true)
	pl.DisableQuitKeybindings()

	pl.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{pKeys.Toggle, pKeys.ToggleAll, pKeys.Save, keys.ScopeProject, keys.ScopeUser, tabKey}
	}
	pl.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{pKeys.Toggle, pKeys.ToggleAll, pKeys.Save, keys.ScopeProject, keys.ScopeUser, tabKey}
	}

	return manageModel{
		list:           l,
		keys:           keys,
		checked:        checked,
		health:         health,
		scope:          scope,
		servers:        servers,
		tab:            tab,
		pluginList:     pl,
		pluginKeys:     pKeys,
		plugins:        plugins,
		pluginChecked:  pluginChecked,
		pluginWidthPtr: pluginWidthPtr,
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
		m.pluginList.SetSize(msg.Width, msg.Height-bannerHeight)
		if m.pluginWidthPtr != nil {
			*m.pluginWidthPtr = msg.Width
		}
		return m, nil

	case tea.KeyMsg:
		// Handle quit confirmation dialog
		if m.confirmQuit {
			switch msg.String() {
			case "s":
				m.action = actionSaveAll
				return m, tea.Quit
			case "d":
				m.action = actionQuit
				return m, tea.Quit
			case "esc":
				m.confirmQuit = false
				return m, nil
			}
			return m, nil
		}

		// Route to the correct tab's filter state check
		if m.tab == tabServers && m.list.FilterState() == list.Filtering {
			break
		}
		if m.tab == tabPlugins && m.pluginList.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.pluginList, cmd = m.pluginList.Update(msg)
			return m, cmd
		}

		// Tab switching
		if key.Matches(msg, tabKey) {
			if m.tab == tabServers {
				m.tab = tabPlugins
			} else {
				m.tab = tabServers
			}
			return m, nil
		}

		// Scope switching (shared between tabs)
		switch {
		case key.Matches(msg, m.keys.ScopeProject):
			if m.scope != "project" {
				m.scope = "project"
				m.reloadCheckedState()
				m.reloadPluginCheckedState()
			}
			return m, nil

		case key.Matches(msg, m.keys.ScopeUser):
			if m.scope != "user" {
				m.scope = "user"
				m.reloadCheckedState()
				m.reloadPluginCheckedState()
			}
			return m, nil

		case key.Matches(msg, m.keys.ScopeToggle):
			if m.scope == "project" {
				m.scope = "user"
			} else {
				m.scope = "project"
			}
			m.reloadCheckedState()
			m.reloadPluginCheckedState()
			return m, nil
		}

		// Quit (shared between tabs)
		if key.Matches(msg, m.keys.Quit) {
			serverChanges := m.computeServerChanges()
			pluginChanges := m.computePluginChanges()
			if len(serverChanges) > 0 || len(pluginChanges) > 0 {
				m.confirmQuit = true
				m.pendingServerChanges = serverChanges
				m.pendingPluginChanges = pluginChanges
				return m, nil
			}
			m.action = actionQuit
			return m, tea.Quit
		}

		// Tab-specific key handling
		if m.tab == tabPlugins {
			return m.updatePluginTab(msg)
		}

		// Servers tab keys
		switch {
		case key.Matches(msg, m.keys.Toggle):
			if item, ok := m.list.SelectedItem().(serverItem); ok {
				m.checked[item.name] = !m.checked[item.name]
				return m, nil
			}

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
		}
	}

	if m.tab == tabPlugins {
		var cmd tea.Cmd
		m.pluginList, cmd = m.pluginList.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// updatePluginTab handles key events when the plugins tab is active.
func (m manageModel) updatePluginTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.pluginKeys.Toggle):
		if item, ok := m.pluginList.SelectedItem().(pluginItem); ok {
			m.pluginChecked[item.key] = !m.pluginChecked[item.key]
			return m, nil
		}

	case key.Matches(msg, m.pluginKeys.ToggleAll):
		allChecked := true
		for _, p := range m.plugins {
			if !m.pluginChecked[p.Key] {
				allChecked = false
				break
			}
		}
		for _, p := range m.plugins {
			m.pluginChecked[p.Key] = !allChecked
		}
		return m, nil

	case key.Matches(msg, m.pluginKeys.Save):
		m.action = actionSavePlugins
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.pluginList, cmd = m.pluginList.Update(msg)
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

// reloadPluginCheckedState clears the plugin checked map and refills it with
// the effective enabled state for the current scope. User scope is the base;
// project scope overlays on top. The map is mutated in place so the delegate's
// reference stays valid.
func (m *manageModel) reloadPluginCheckedState() {
	for k := range m.pluginChecked {
		delete(m.pluginChecked, k)
	}
	effective := config.EffectiveEnabledPlugins(m.scope)
	for k, v := range effective {
		m.pluginChecked[k] = v
	}
}

// computeServerChanges returns the list of server changes vs the current config.
func (m *manageModel) computeServerChanges() []pendingChange {
	existing := config.ReadMcpServers(m.scope)
	var changes []pendingChange
	for _, name := range config.ServerNames(m.servers) {
		_, inConfig := existing[name]
		isChecked := m.checked[name]
		if isChecked && !inConfig {
			changes = append(changes, pendingChange{name, "add"})
		} else if !isChecked && inConfig {
			changes = append(changes, pendingChange{name, "remove"})
		}
	}
	return changes
}

// computePluginChanges returns the list of plugin changes vs the current config.
func (m *manageModel) computePluginChanges() []pendingChange {
	effective := config.EffectiveEnabledPlugins(m.scope)
	var changes []pendingChange
	for _, p := range m.plugins {
		isChecked := m.pluginChecked[p.Key]
		wasChecked := effective[p.Key]
		if isChecked && !wasChecked {
			changes = append(changes, pendingChange{p.Key, "enable"})
		} else if !isChecked && wasChecked {
			changes = append(changes, pendingChange{p.Key, "disable"})
		}
	}
	return changes
}

func (m manageModel) View() string {
	banner := tabbedBanner(m.scope, m.tab, m.width)
	if m.confirmQuit {
		return banner + "\n" + m.confirmQuitView()
	}
	if m.tab == tabPlugins {
		return banner + "\n" + m.pluginList.View()
	}
	return banner + "\n" + m.list.View()
}

// confirmQuitView renders the unsaved changes summary and action options.
func (m manageModel) confirmQuitView() string {
	var b strings.Builder

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	dim := lipgloss.NewStyle().Faint(true)
	bold := lipgloss.NewStyle().Bold(true)

	b.WriteString("\n  " + title.Render("Unsaved changes") + "\n")

	if len(m.pendingServerChanges) > 0 {
		b.WriteString("\n  " + bold.Render("MCP Servers") + "\n")
		for _, c := range m.pendingServerChanges {
			if c.action == "add" {
				b.WriteString(fmt.Sprintf("    %s %s\n", green.Render("+ add   "), c.name))
			} else {
				b.WriteString(fmt.Sprintf("    %s %s\n", red.Render("- remove"), c.name))
			}
		}
	}

	if len(m.pendingPluginChanges) > 0 {
		b.WriteString("\n  " + bold.Render("Plugins") + "\n")
		for _, c := range m.pendingPluginChanges {
			if c.action == "enable" {
				b.WriteString(fmt.Sprintf("    %s %s\n", green.Render("+ enable "), c.name))
			} else {
				b.WriteString(fmt.Sprintf("    %s %s\n", red.Render("- disable"), c.name))
			}
		}
	}

	b.WriteString("\n  " + dim.Render("[s] Save & quit  [d] Discard & quit  [esc] Back to list") + "\n")

	return b.String()
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
// Scope and tab persist across iterations so the user stays in their chosen state.
func runManage() error {
	scope := "project"
	tab := tabServers

	for {
		servers, err := config.LoadServers()
		if err != nil {
			return err
		}

		if len(servers) == 0 {
			return runManageEmpty()
		}

		checked := loadCheckedState(servers, scope)

		// Discover plugins and compute effective enabled state
		plugins, _ := config.DiscoverPlugins()
		pluginEffective := config.EffectiveEnabledPlugins(scope)
		// Merge plugin list with both user and project enabled maps
		// so plugins referenced in either scope appear in the list
		plugins = config.MergePluginSources(plugins,
			config.ReadEnabledPlugins("user"),
			config.ReadEnabledPlugins("project"),
		)

		m := newManageModel(servers, scope, checked, plugins, pluginEffective, tab)
		p := tea.NewProgram(m, tea.WithAltScreen())
		result, err := p.Run()
		if err != nil {
			return fmt.Errorf("list error: %w", err)
		}

		final := result.(manageModel)
		scope = final.scope // preserve scope for next iteration
		tab = final.tab     // preserve tab for next iteration

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
			if err := runServerDetail(final.selected, servers, final.scope, final.tab); err != nil && err != errCancelled {
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

		case actionSavePlugins:
			if err := runSavePlugins(final.pluginChecked, final.scope); err != nil && err != errCancelled {
				return err
			}

		case actionSaveAll:
			if err := applySaveAll(final.servers, final.checked, final.pluginChecked, final.scope); err != nil {
				return err
			}
			return nil
		}

		// Loop back to re-read config and show fresh list
	}
}

// runSavePlugins saves the plugin enabled/disabled state to the scope's settings.json.
// The checked map contains the effective (desired) state. For project scope,
// WriteEnabledPlugins computes the delta from user scope automatically.
func runSavePlugins(checked map[string]bool, scope string) error {
	// Compare against current effective state to detect changes
	existing := config.EffectiveEnabledPlugins(scope)

	var enabled, disabled []string
	for key, isChecked := range checked {
		wasChecked := existing[key]
		if isChecked && !wasChecked {
			enabled = append(enabled, key)
		} else if !isChecked && wasChecked {
			disabled = append(disabled, key)
		}
	}

	if len(enabled) == 0 && len(disabled) == 0 {
		fmt.Println(display.StyleDim.Render("No plugin changes to apply."))
		return nil
	}

	target := config.PluginSettingsPath(scope)
	scopeLabel := "user (global)"
	if scope == "project" {
		scopeLabel = "project (overrides)"
	}

	fmt.Println()
	fmt.Printf("  Scope: %s\n", display.StyleTitle.Render(scopeLabel))
	fmt.Printf("  File:  %s\n", target)
	fmt.Println()
	if len(enabled) > 0 {
		for _, name := range enabled {
			fmt.Printf("  %s %s\n", display.StyleGreen.Render("enable "), name)
		}
	}
	if len(disabled) > 0 {
		for _, name := range disabled {
			fmt.Printf("  %s %s\n", display.StyleRed.Render("disable"), name)
		}
	}
	fmt.Println()

	confirmed, err := confirm("Apply plugin changes?", true)
	if err != nil {
		return handleAbort(err)
	}
	if !confirmed {
		return errCancelled
	}

	path, err := config.WriteEnabledPlugins(scope, checked)
	if err != nil {
		return fmt.Errorf("failed to write plugin settings: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %s plugin settings updated\n", display.StyleGreen.Render("Saved:"))
	fmt.Printf("  Written to %s\n", path)
	fmt.Println()

	return nil
}

// applySaveAll saves both server and plugin changes without additional confirmation.
// Used when the user confirms from the quit dialog.
func applySaveAll(servers config.ServerMap, checked map[string]bool, pluginChecked map[string]bool, scope string) error {
	saved := false

	// Save server changes
	existing := config.ReadMcpServers(scope)
	var selected []string
	for _, name := range config.ServerNames(servers) {
		if checked[name] {
			selected = append(selected, name)
		}
	}
	var toRemove []string
	for name := range existing {
		if _, inCentral := servers[name]; inCentral && !checked[name] {
			toRemove = append(toRemove, name)
		}
	}
	if len(selected) > 0 || len(toRemove) > 0 {
		entries := config.ServerMap{}
		for _, name := range selected {
			entries[name] = config.BuildEntryForClaude(servers[name])
		}
		path, err := config.WriteMcpServers(scope, entries, toRemove)
		if err != nil {
			return fmt.Errorf("failed to write server config: %w", err)
		}
		fmt.Println()
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
		fmt.Printf("  Written to %s\n", path)
		saved = true
	}

	// Save plugin changes
	pluginEffective := config.EffectiveEnabledPlugins(scope)
	hasPluginChanges := false
	for k, isChecked := range pluginChecked {
		if isChecked != pluginEffective[k] {
			hasPluginChanges = true
			break
		}
	}
	if hasPluginChanges {
		path, err := config.WriteEnabledPlugins(scope, pluginChecked)
		if err != nil {
			return fmt.Errorf("failed to write plugin settings: %w", err)
		}
		if !saved {
			fmt.Println()
		}
		fmt.Printf("  %s plugin settings updated\n", display.StyleGreen.Render("Saved:"))
		fmt.Printf("  Written to %s\n", path)
		saved = true
	}

	if saved {
		fmt.Println()
	}

	return nil
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
	)).WithTheme(formTheme()).WithKeyMap(formKeyMap()).Run()
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
