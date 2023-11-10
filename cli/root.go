package cli

import (
	"github.com/ArchiMoebius/fishler/cli/cmd"
	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "fishler",
	Short: "SSH to Docker container",
	Long:  "Leverage SSH and Docker to create containers on-the-fly!",
}

func init() {
	RootCmd.AddCommand(cmd.ServeCmd)
	RootCmd.AddCommand(cmd.ImageCmd)
	RootCmd.AddCommand(cmd.DocCmd)
}
