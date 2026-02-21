package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var version = "dev"

// SetVersion sets the version string from main.
func SetVersion(v string) {
	version = v
}

var rootCmd = &cobra.Command{
	Use:   "cc-setup",
	Short: "Manage MCP servers and plugins for Claude Code",
	Long: `Interactive CLI to manage MCP servers and plugins per project in Claude Code.
Define all your servers once in a central config, then cherry-pick which ones
to enable for each project. Enable or disable plugins per scope. This keeps
Claude's context clean by loading only the tools you actually need.`,
	// Default: interactive management hub
	RunE: func(cmd *cobra.Command, args []string) error {
		return runManage()
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("cc-setup", version)
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
