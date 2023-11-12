package cli

import (
	cmd "github.com/archimoebius/fishler/cli/cmd"
)

// Execute is the entry point for the cli
// called from main
func Execute() error {
	return cmd.RootCmd.Execute()
}
