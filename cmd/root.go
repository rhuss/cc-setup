package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// SetVersionInfo sets the version, commit, and date strings from main.
func SetVersionInfo(v, c, d string) {
	version = v
	commit = c
	date = d
	rootCmd.Version = fmt.Sprintf("%s (%s %s)", version, commit, date)
}

var rootCmd = &cobra.Command{
	Use:   "cc-setup",
	Short: "Manage MCP servers and plugins for Claude Code",
	Long: `Interactive CLI to manage MCP servers and plugins per project in Claude Code.
Define all your servers once in a central config, then cherry-pick which ones
to enable for each project. Enable or disable plugins per scope. This keeps
Claude's context clean by loading only the tools you actually need.

Central server config: ~/.config/cc-setup/mcp.json (or $XDG_CONFIG_HOME/cc-setup/mcp.json)`,
	// Default: interactive management hub
	RunE: func(cmd *cobra.Command, args []string) error {
		return runManage()
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.SetVersionTemplate("cc-setup {{ .Version }}\n")
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("cc-setup %s (%s %s)\n", version, commit, date)
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
