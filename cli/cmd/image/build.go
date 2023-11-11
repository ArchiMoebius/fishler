package cmd

import (
	"context"

	"github.com/ArchiMoebius/fishler/cli/config"
	"github.com/ArchiMoebius/fishler/util"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"
)

var BuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the fishler Docker image",
	Long:  `If the fishler Docker image is not yet built, build it. Use --force to rebuild.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		config.ReadGlobalConfig()
	},
	Run: func(cmd *cobra.Command, args []string) {
		forceBuild, _ := cmd.Flags().GetBool("force")

		dockerClient, err := client.NewClientWithOpts(
			client.FromEnv,
			client.WithAPIVersionNegotiation(),
		)

		if err != nil {
			util.Logger.Error(err)
		}

		ctx := context.Background()

		err = util.BuildFishler(dockerClient, ctx, forceBuild)
		if err != nil {
			util.Logger.Error(err)
		}

	},
}

func init() {
	BuildCmd.Flags().BoolP("force", "f", false, "Force the fishler image to be re-built if it exists")
}
