package cmd

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// formTheme returns a huh theme that uses the app's cyan color scheme
// instead of the default purple/blue. Applied to all huh forms for
// visual consistency with the TUI.
func formTheme() *huh.Theme {
	t := huh.ThemeBase()

	cyan := lipgloss.Color("6")

	// Titles and text cursor in cyan
	t.Focused.Title = t.Focused.Title.Foreground(cyan)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(cyan)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(cyan)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(cyan)
	t.Focused.FocusedButton = t.Focused.FocusedButton.
		Background(cyan).
		Foreground(lipgloss.Color("#ffffff"))
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(cyan)

	return t
}

// confirmTheme returns a theme with green Yes / red No button colors.
// The "focused" button (the currently selected one) gets the bright color,
// the other button gets a dimmed version.
func confirmTheme() *huh.Theme {
	t := formTheme()

	button := lipgloss.NewStyle().Padding(0, 2).MarginRight(1)

	// Focused = currently selected choice (bright green background)
	t.Focused.FocusedButton = button.
		Background(lipgloss.Color("#225522")).
		Foreground(lipgloss.Color("#ffffff")).
		Bold(true)

	// Blurred = the other choice (dark red background, dimmed)
	t.Focused.BlurredButton = button.
		Background(lipgloss.Color("#552222")).
		Foreground(lipgloss.Color("#999999"))

	return t
}

// formKeyMap returns the default huh keymap with ESC added as quit/abort key.
func formKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(key.WithKeys("ctrl+c", "esc"))
	return km
}

// confirmKeyMap returns a keymap with ESC-to-quit and tab added to Toggle.
func confirmKeyMap() *huh.KeyMap {
	km := formKeyMap()
	km.Confirm.Toggle = key.NewBinding(
		key.WithKeys("h", "l", "right", "left", "tab"),
		key.WithHelp("tab/←/→", "toggle"),
	)
	return km
}

// confirm shows a Yes/No prompt with green/red styled buttons.
// Tab, arrow keys, and h/l toggle between options. Enter confirms.
func confirm(title string, defaultYes bool) (bool, error) {
	value := defaultYes

	err := huh.NewConfirm().
		Title(title).
		Affirmative("Yes").
		Negative("No").
		Value(&value).
		WithTheme(confirmTheme()).
		WithKeyMap(confirmKeyMap()).
		Run()

	if err != nil {
		return false, err
	}
	return value, nil
}
