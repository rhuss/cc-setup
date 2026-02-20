package main

import (
	"os"

	"github.com/rhuss/cc-mcp-setup/cmd"
)

var version = "dev"

func main() {
	cmd.SetVersion(version)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
