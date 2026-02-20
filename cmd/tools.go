package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rhuss/cc-mcp-setup/internal/config"
	"github.com/rhuss/cc-mcp-setup/internal/display"
	mcpclient "github.com/rhuss/cc-mcp-setup/internal/mcp"
)

// toolCache caches discovered tools per server name across invocations within
// the same process.
var toolCache sync.Map // map[string][]mcpclient.ToolInfo

// runToolPermissions discovers tools for the given server and launches an
// interactive permissions screen.
func runToolPermissions(name string, servers config.ServerMap, scope string) error {
	serverDef, ok := servers[name]
	if !ok {
		fmt.Printf("%s Server %s not found.\n",
			display.StyleYellow.Render("Warning:"),
			display.StyleCyan.Render(name))
		return nil
	}

	// Try cache first.
	var tools []mcpclient.ToolInfo
	if cached, ok := toolCache.Load(name); ok {
		tools = cached.([]mcpclient.ToolInfo)
	} else {
		fmt.Printf("  Discovering tools for %s...\n", display.StyleCyan.Render(name))
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		var err error
		tools, err = mcpclient.ListTools(ctx, name, serverDef)
		if err != nil {
			fmt.Printf("  %s %v\n\n", display.StyleRed.Render("Error:"), err)
			return nil // non-fatal, return to list
		}
		if len(tools) == 0 {
			fmt.Printf("  %s\n\n", display.StyleDim.Render("No tools found."))
			return nil
		}
		toolCache.Store(name, tools)
	}

	// Load current permissions from settings.local.json for the active scope.
	approved := config.ReadToolPermissions(scope, name)
	approvedSet := make(map[string]bool, len(tools))
	if len(approved) == 1 && approved[0] == "*" {
		// Wildcard: pre-check all tools.
		for _, t := range tools {
			approvedSet[t.Name] = true
		}
	} else {
		for _, t := range approved {
			approvedSet[t] = true
		}
	}

	m := newToolsModel(name, tools, approvedSet, scope)
	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return fmt.Errorf("tools screen error: %w", err)
	}

	final := result.(toolsModel)
	if !final.saved {
		return nil
	}

	// Build the new permission list from checked tools.
	var selected []string
	allToolNames := make([]string, 0, len(tools))
	for _, t := range tools {
		allToolNames = append(allToolNames, t.Name)
		if final.checked[t.Name] {
			selected = append(selected, t.Name)
		}
	}

	// Write to settings.local.json for the active scope.
	writtenPath, err := config.WriteToolPermissions(final.scope, name, selected, allToolNames)
	if err != nil {
		return fmt.Errorf("failed to write permissions: %w", err)
	}

	// Keep central config autoApprove in sync.
	mcpclient.UpdateAutoApprove(serverDef, selected, len(tools))
	if err := config.SaveServers(servers); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println()
	if len(selected) == len(tools) {
		fmt.Printf("  %s permissions: %s for %s (all %d tools)\n",
			display.StyleGreen.Render("Saved:"),
			display.StyleCyan.Render("*"),
			display.StyleCyan.Render(name),
			len(tools))
	} else if len(selected) > 0 {
		fmt.Printf("  %s permissions updated for %s (%d/%d tools)\n",
			display.StyleGreen.Render("Saved:"),
			display.StyleCyan.Render(name),
			len(selected), len(tools))
	} else {
		fmt.Printf("  %s permissions removed for %s\n",
			display.StyleGreen.Render("Saved:"),
			display.StyleCyan.Render(name))
	}
	fmt.Printf("  %s %s\n\n",
		display.StyleDim.Render("Written to"),
		writtenPath)

	return nil
}

// toolItem implements list.Item for the tools list.
type toolItem struct {
	name string
	desc string // cleaned single-line description
	hint string // emoji marker, empty when no annotations
}

func (i toolItem) Title() string       { return i.name }
func (i toolItem) Description() string { return i.desc }
func (i toolItem) FilterValue() string { return i.name + " " + i.desc }

// cleanDescription extracts a single-line summary from a possibly multi-line
// tool description. It takes the first non-empty line and strips leading/trailing
// whitespace. Sections like "Args:", "Returns:", "Raises:" are dropped.
func cleanDescription(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Stop at docstring sections.
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "args:") ||
			strings.HasPrefix(lower, "returns:") ||
			strings.HasPrefix(lower, "raises:") ||
			strings.HasPrefix(lower, "parameters:") ||
			strings.HasPrefix(lower, "example:") {
			break
		}
		return line
	}
	return ""
}

// toolsKeyMap holds key bindings for the tools permissions screen.
type toolsKeyMap struct {
	Toggle       key.Binding
	ToggleAll    key.Binding
	Save         key.Binding
	ScopeProject key.Binding
	ScopeUser    key.Binding
	Quit         key.Binding
}

func newToolsKeyMap() toolsKeyMap {
	return toolsKeyMap{
		Toggle:       key.NewBinding(key.WithKeys(" ", "x"), key.WithHelp("space", "toggle")),
		ToggleAll:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "toggle all")),
		Save:         key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "save")),
		ScopeProject: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "project scope")),
		ScopeUser:    key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "user scope")),
		Quit:         key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "cancel")),
	}
}

// toolCheckboxDelegate renders tool items as two lines:
// line 1: cursor + checkbox + tool name
// line 2: indented description (truncated to terminal width)
type toolCheckboxDelegate struct {
	checked map[string]bool
	width   *int // pointer to terminal width, updated by model
}

func (d toolCheckboxDelegate) Height() int                             { return 2 }
func (d toolCheckboxDelegate) Spacing() int                            { return 0 }
func (d toolCheckboxDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

// descIndent is the number of characters to indent the description line,
// aligning it under the tool name: "  [ ] " = 6 chars.
const descIndent = 6

func (d toolCheckboxDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ti, ok := item.(toolItem)
	if !ok {
		return
	}

	isFocused := index == m.Index()
	isChecked := d.checked[ti.name]

	cursor := "  "
	if isFocused {
		cursor = "> "
	}

	cb := "[ ]"
	if isChecked {
		cb = "[x]"
	}

	// Determine available width for the description.
	termWidth := 80
	if d.width != nil && *d.width > 0 {
		termWidth = *d.width
	}
	descWidth := termWidth - descIndent - 1 // -1 for safety margin
	if descWidth < 20 {
		descWidth = 20
	}

	// Truncate description to fit.
	desc := ti.desc
	if len(desc) > descWidth {
		desc = desc[:descWidth-3] + "..."
	}

	dim := lipgloss.NewStyle().Faint(true)

	hintSuffix := ""
	if ti.hint != "" {
		hintSuffix = " " + ti.hint
	}

	if isFocused {
		cyan := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
		cyanBold := cyan.Bold(true)
		green := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)

		cbStyled := cyanBold.Render(cb)
		if isChecked {
			cbStyled = green.Render(cb)
		}

		line1 := cyanBold.Render(cursor) + cbStyled + " " + cyanBold.Render(ti.name) + hintSuffix
		line2 := strings.Repeat(" ", descIndent) + dim.Render(desc)
		fmt.Fprint(w, line1+"\n"+line2)
	} else {
		green := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))

		cbStyled := dim.Render(cb)
		if isChecked {
			cbStyled = green.Render(cb)
		}

		line1 := cursor + cbStyled + " " + ti.name + hintSuffix
		line2 := strings.Repeat(" ", descIndent) + dim.Render(desc)
		fmt.Fprint(w, line1+"\n"+line2)
	}
}

// toolsModel is the BubbleTea model for the tool permissions screen.
type toolsModel struct {
	list       list.Model
	keys       toolsKeyMap
	checked    map[string]bool
	serverName string
	tools      []mcpclient.ToolInfo
	scope      string
	saved      bool
	widthPtr   *int // shared with delegate for description truncation
}

// toolsBannerHeight is the number of lines used by the banner.
const toolsBannerHeight = 1

func newToolsModel(serverName string, tools []mcpclient.ToolInfo, checked map[string]bool, scope string) toolsModel {
	items := make([]list.Item, 0, len(tools))
	for _, t := range tools {
		items = append(items, toolItem{
			name: t.Name,
			desc: cleanDescription(t.Description),
			hint: mcpclient.FormatToolHint(t),
		})
	}

	keys := newToolsKeyMap()
	// Shared width pointer so the delegate can read the current terminal width
	// even though it's passed by value into the list.
	widthPtr := new(int)
	*widthPtr = 80
	delegate := toolCheckboxDelegate{checked: checked, width: widthPtr}

	l := list.New(items, delegate, 0, 0)
	l.Title = "Tool Permissions"
	l.Styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("6")).
		Padding(0, 1)
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()

	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{keys.Toggle, keys.ToggleAll, keys.Save, keys.ScopeProject, keys.ScopeUser, keys.Quit}
	}
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{keys.Toggle, keys.ToggleAll, keys.Save, keys.ScopeProject, keys.ScopeUser, keys.Quit}
	}

	return toolsModel{
		list:       l,
		keys:       keys,
		checked:    checked,
		serverName: serverName,
		tools:      tools,
		scope:      scope,
		widthPtr:   widthPtr,
	}
}

func (m toolsModel) Init() tea.Cmd {
	return nil
}

func (m toolsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		*m.widthPtr = msg.Width
		m.list.SetSize(msg.Width, msg.Height-toolsBannerHeight)
		return m, nil

	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Toggle):
			if item, ok := m.list.SelectedItem().(toolItem); ok {
				m.checked[item.name] = !m.checked[item.name]
				return m, nil
			}

		case key.Matches(msg, m.keys.ToggleAll):
			// If all are checked, deselect all; otherwise select all.
			allChecked := true
			for _, t := range m.tools {
				if !m.checked[t.Name] {
					allChecked = false
					break
				}
			}
			for _, t := range m.tools {
				m.checked[t.Name] = !allChecked
			}
			return m, nil

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

		case key.Matches(msg, m.keys.Save):
			m.saved = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// reloadCheckedState clears the checked map and refills it from the current
// scope's settings.local.json. The map is mutated in place so the delegate's
// reference stays valid.
func (m *toolsModel) reloadCheckedState() {
	for k := range m.checked {
		delete(m.checked, k)
	}
	approved := config.ReadToolPermissions(m.scope, m.serverName)
	if len(approved) == 1 && approved[0] == "*" {
		for _, t := range m.tools {
			m.checked[t.Name] = true
		}
	} else {
		for _, t := range approved {
			m.checked[t] = true
		}
	}
}

func (m toolsModel) View() string {
	return toolsBanner(m.scope, m.serverName, *m.widthPtr) + "\n" + m.list.View()
}

// toolsBanner renders a full-width scope-aware bar with the scope, target file, and server name.
func toolsBanner(scope, serverName string, width int) string {
	if width <= 0 {
		width = 80
	}

	path := config.SettingsPath(scope)
	label := fmt.Sprintf(" %s  %s  %s ", strings.ToUpper(scope), path, serverName)
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
