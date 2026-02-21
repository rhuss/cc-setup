package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rhuss/cc-setup/internal/config"
)

// pluginItem implements list.Item for the plugins list.
type pluginItem struct {
	key         string // "name@marketplace"
	name        string
	marketplace string
	version     string
	desc        string // single-line description
}

func (i pluginItem) Title() string       { return i.key }
func (i pluginItem) Description() string { return i.desc }
func (i pluginItem) FilterValue() string { return i.key + filterSep + i.desc }

// buildPluginItems converts a slice of PluginInfo into list items.
func buildPluginItems(plugins []config.PluginInfo) []list.Item {
	items := make([]list.Item, 0, len(plugins))
	for _, p := range plugins {
		items = append(items, pluginItem{
			key:         p.Key,
			name:        p.Name,
			marketplace: p.Marketplace,
			version:     p.Version,
			desc:        p.Description,
		})
	}
	return items
}

// pluginKeyMap holds key bindings for the plugin tab.
type pluginKeyMap struct {
	Toggle    key.Binding
	ToggleAll key.Binding
	Save      key.Binding
	Quit      key.Binding
}

func newPluginKeyMap() pluginKeyMap {
	return pluginKeyMap{
		Toggle:    key.NewBinding(key.WithKeys(" ", "x"), key.WithHelp("space", "toggle")),
		ToggleAll: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "toggle all")),
		Save:      key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "save")),
		Quit:      key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "quit")),
	}
}

// pluginDescIndent is the number of characters to indent the description line,
// aligning it under the plugin name: "  [x] " = 6 chars.
const pluginDescIndent = 6

// pluginCheckboxDelegate renders plugin items as two lines:
// line 1: cursor + checkbox + name (padded) + marketplace (dim) + version (dim)
// line 2: indented description (truncated to terminal width)
type pluginCheckboxDelegate struct {
	checked map[string]bool
	width   *int
}

func (d pluginCheckboxDelegate) Height() int                             { return 2 }
func (d pluginCheckboxDelegate) Spacing() int                            { return 0 }
func (d pluginCheckboxDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d pluginCheckboxDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	pi, ok := item.(pluginItem)
	if !ok {
		return
	}

	isFocused := index == m.Index()
	isChecked := d.checked[pi.key]

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
	descWidth := termWidth - pluginDescIndent - 1
	if descWidth < 20 {
		descWidth = 20
	}

	// Truncate description to fit.
	desc := pi.desc
	if len(desc) > descWidth {
		desc = desc[:descWidth-3] + "..."
	}

	// Build the metadata suffix: marketplace + version
	var meta string
	if pi.marketplace != "" {
		meta = pi.marketplace
	}
	if pi.version != "" {
		if meta != "" {
			meta += "  "
		}
		meta += "v" + pi.version
	}

	dim := lipgloss.NewStyle().Faint(true)

	// Pad the plugin name
	nameWidth := 20
	if len(pi.name) >= nameWidth {
		nameWidth = len(pi.name) + 2
	}
	paddedName := fmt.Sprintf("%-*s", nameWidth, pi.name)

	if isFocused {
		cyan := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
		cyanBold := cyan.Bold(true)
		green := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)

		cbStyled := cyanBold.Render(cb)
		if isChecked {
			cbStyled = green.Render(cb)
		}

		line1 := cyanBold.Render(cursor) + cbStyled + " " + cyanBold.Render(paddedName) + dim.Render(meta)
		line2 := strings.Repeat(" ", pluginDescIndent) + dim.Render(desc)
		fmt.Fprint(w, line1+"\n"+line2)
	} else {
		green := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))

		cbStyled := dim.Render(cb)
		if isChecked {
			cbStyled = green.Render(cb)
		}

		line1 := cursor + cbStyled + " " + paddedName + dim.Render(meta)
		line2 := strings.Repeat(" ", pluginDescIndent) + dim.Render(desc)
		fmt.Fprint(w, line1+"\n"+line2)
	}
}
