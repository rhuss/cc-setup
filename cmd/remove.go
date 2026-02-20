package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/rhuss/cc-mcp-setup/internal/config"
	"github.com/rhuss/cc-mcp-setup/internal/display"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove [name...]",
	Short: "Remove server(s) from the central config",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRemove(args)
	},
}

func runRemove(args []string) error {
	servers, err := config.LoadServers()
	if err != nil {
		return err
	}

	if len(servers) == 0 {
		fmt.Println(display.StyleYellow.Render(
			fmt.Sprintf("No servers configured in %s", config.GetConfigFile()),
		))
		return nil
	}

	var toRemove []string

	if len(args) > 0 {
		// Names provided as arguments: validate they exist
		var missing []string
		for _, name := range args {
			if _, exists := servers[name]; exists {
				toRemove = append(toRemove, name)
			} else {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			fmt.Printf("%s Server(s) not found: %s\n",
				display.StyleYellow.Render("Warning:"),
				strings.Join(missing, ", "))
		}
		if len(toRemove) == 0 {
			return nil
		}
	} else {
		// No args: interactive multi-select
		names := config.ServerNames(servers)
		options := make([]huh.Option[string], 0, len(names))
		for _, name := range names {
			info := servers[name]
			desc, _ := info["description"].(string)
			label := fmt.Sprintf("%-20s %s", name, desc)
			options = append(options, huh.NewOption(label, name))
		}

		err = huh.NewMultiSelect[string]().
			Title("Select servers to remove").
			Options(options...).
			Height(len(options)+1).
			Value(&toRemove).
			Run()
		if err != nil {
			return handleAbort(err)
		}
	}

	if len(toRemove) == 0 {
		fmt.Println(display.StyleDim.Render("Nothing selected. Bye."))
		return nil
	}

	// Show what will be removed
	fmt.Println()
	for _, name := range toRemove {
		info := servers[name]
		endpoint := display.ServerEndpoint(info)
		fmt.Printf("  %s %s (%s)\n", display.StyleRed.Render("remove"), display.StyleCyan.Render(name), endpoint)
	}
	fmt.Println()

	confirmed, err := confirm(fmt.Sprintf("Remove %d server(s) from central config?", len(toRemove)), false)
	if err != nil {
		return handleAbort(err)
	}
	if !confirmed {
		fmt.Println(display.StyleDim.Render("Cancelled."))
		return nil
	}

	for _, name := range toRemove {
		delete(servers, name)
	}

	if err := config.SaveServers(servers); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %s Removed %d server(s) from %s\n",
		display.StyleGreen.Render("Done:"),
		len(toRemove),
		config.GetConfigFile())
	fmt.Println()
	return nil
}
