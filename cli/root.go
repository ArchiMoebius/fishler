package cli

import (
	"os"

	"github.com/ArchiMoebius/fishler/cli/cmd"
	"github.com/ArchiMoebius/fishler/util"
	"github.com/spf13/cobra"
)

const VERSION = "2023.11.10"

var RootCmd = &cobra.Command{
	Version: VERSION,
	Use:     "fishler",
	Short:   "SSH to Docker container",
	Long:    "Leverage SSH and Docker to create containers on-the-fly!",
	Run: func(cmd *cobra.Command, args []string) {
		version, _ := cmd.Flags().GetBool("version")

		if version {
			util.Logger.Infof("Version %s\n", VERSION)
			os.Exit(0)
		}
	},
}

func init() {
	RootCmd.AddCommand(cmd.ServeCmd)
	RootCmd.AddCommand(cmd.ImageCmd)
	RootCmd.AddCommand(cmd.DocCmd)

	RootCmd.Flags().BoolP("version", "v", false, "Show the version and exit")
}
