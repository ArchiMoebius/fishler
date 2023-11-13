package cli

import (
	"fmt"
	"log"
	"os"

	config "github.com/archimoebius/fishler/cli/config/root"
	"github.com/archimoebius/fishler/util"
	"github.com/spf13/cobra"
)

const VERSION = "2023.11.12"

func CallPersistentPreRun(cmd *cobra.Command, args []string) {
	if parent := cmd.Parent(); parent != nil {
		if parent.PersistentPreRun != nil {
			parent.PersistentPreRun(parent, args)
		}
	}
}

var RootCmd = &cobra.Command{
	Version: VERSION,
	Use:     "fishler",
	Short:   "SSH to Docker container",
	Long:    "Leverage SSH and Docker to create containers on-the-fly!",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		CallPersistentPreRun(cmd, args)
		config.Load()

		logger := util.SetLogger(fmt.Sprintf("%s/system.log", config.Setting.LogBasepath))

		if logger == nil {
			logger := util.SetLogger("./system.log")

			if logger == nil {
				fmt.Printf("[!] Failed to setup logger - bailing\n")
				err := cmd.Help()

				if err != nil {
					log.Fatal(err)
					os.Exit(1)
				}

				os.Exit(0)
			}

			fmt.Printf("[!] Failed to setup default logger - using ./system.log instead\n")
		}

		if config.Setting.Debug {
			config.Setting.Print()
		}
	},
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) == 0 {
			err := cmd.Help()

			if err != nil {
				util.Logger.Error(err)
				os.Exit(1)
			}

			os.Exit(0)
		}

		version, _ := cmd.Flags().GetBool("version")

		if version {
			util.Logger.Infof("Version %s\n", VERSION)
			os.Exit(0)
		}
	},
}

func init() {
	RootCmd.AddCommand(ServeCmd)
	RootCmd.AddCommand(ImageCmd)
	RootCmd.AddCommand(DocCmd)

	RootCmd.Flags().BoolP("version", "v", false, "Show the version and exit")

	// Initialize the config and panic on failure
	if err := config.CommandInit(RootCmd); err != nil {
		util.Logger.Error(err)
	}
}
