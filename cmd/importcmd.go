package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/rhuss/mcp-setup/internal/config"
	"github.com/rhuss/mcp-setup/internal/display"
	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import servers from existing Claude config into central config",
	Long: `Import server definitions from an existing Claude Code config
into the central config for reuse across projects.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runImport()
	},
}

func runImport() error {
	// Step 1: Choose source
	projectPath := config.ConfigPath("project")
	userPath := config.ConfigPath("user")

	var source string
	err := huh.NewSelect[string]().
		Title("Import servers from").
		Options(
			huh.NewOption(fmt.Sprintf("Both configs  %s", display.StyleDim.Render("(recommended)")), "both"),
			huh.NewOption(fmt.Sprintf("Project       %s", display.StyleDim.Render(projectPath)), "project"),
			huh.NewOption(fmt.Sprintf("User          %s", display.StyleDim.Render(userPath)), "user"),
		).
		Value(&source).
		Run()
	if err != nil {
		return handleAbort(err)
	}

	// Gather servers from selected sources
	imported := config.ServerMap{}
	if source == "project" || source == "both" {
		for name, entry := range config.ReadMcpServers("project") {
			imported[name] = entry
		}
	}
	if source == "user" || source == "both" {
		for name, entry := range config.ReadMcpServers("user") {
			// Don't overwrite project entries with user entries
			if _, exists := imported[name]; !exists {
				imported[name] = entry
			}
		}
	}

	if len(imported) == 0 {
		fmt.Println(display.StyleYellow.Render("No servers found in the selected config(s)."))
		return nil
	}

	// Add description field (auto-generate from server name)
	for name, entry := range imported {
		if _, has := entry["description"]; !has {
			entry["description"] = strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")
		}
	}

	// Load existing central config
	existing, err := config.LoadServers()
	if err != nil {
		return err
	}

	// Show preview table
	fmt.Println()
	fmt.Println(display.RenderImportTable(imported, existing))
	fmt.Println()

	// Count new vs skipped
	var newCount int
	for name := range imported {
		if _, exists := existing[name]; !exists {
			newCount++
		}
	}

	if newCount == 0 {
		fmt.Println(display.StyleDim.Render("All servers already exist in central config. Nothing to import."))
		return nil
	}

	skipCount := len(imported) - newCount

	confirmed, err := confirm(fmt.Sprintf("Import %d server(s) into %s?", newCount, config.GetConfigFile()), true)
	if err != nil {
		return handleAbort(err)
	}
	if !confirmed {
		fmt.Println(display.StyleDim.Render("Cancelled."))
		return nil
	}

	// Merge: don't overwrite existing entries
	for name, entry := range imported {
		if _, exists := existing[name]; !exists {
			existing[name] = entry
		}
	}

	if err := config.SaveServers(existing); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %s Imported %d server(s)\n", display.StyleGreen.Render("Done:"), newCount)
	if skipCount > 0 {
		fmt.Printf("  %s\n", display.StyleDim.Render(fmt.Sprintf("Skipped %d existing server(s)", skipCount)))
	}
	fmt.Printf("  Written to %s\n", config.GetConfigFile())
	fmt.Println()
	fmt.Println(display.StyleDim.Render("You can now run 'mcp-setup' to select servers per project."))
	fmt.Println()
	return nil
}
