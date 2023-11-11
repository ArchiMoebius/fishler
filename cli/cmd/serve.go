package cmd

import (
	"github.com/archimoebius/fishler/app"
	"github.com/archimoebius/fishler/cli/config"
	"github.com/archimoebius/fishler/util"
	"github.com/leebenson/conform"
	"github.com/spf13/cobra"
)

var ServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start an SSH server on --port configured to as desired; serving a docker container on authentication success.",
	Long:  `Leveraging the goodness of Golang, Docker, and SSH - create a container for any user that authenticates with success to the SSH server - recording the session and credentials used.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		config.ReadGlobalConfig()
	},
	Run: func(cmd *cobra.Command, args []string) {

		config.GlobalConfig.Print()
		config.SetupAuthentication(cmd)

		err := app.NewApplication().Start()

		if err != nil {
			util.Logger.Error(err)
		}
	},
}

// init is called before main
func init() {
	// A custom sanitizer to redact sensitive data by defining a struct tag= named "redact".
	conform.AddSanitizer("redact", func(_ string) string { return "*****" })

	// Initialize the config and panic on failure
	if err := config.ConfigInit(ServeCmd); err != nil {
		util.Logger.Error(err)
	}
}
