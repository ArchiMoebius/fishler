package cli

import (
	"context"

	"github.com/archimoebius/fishler/util"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"
)

var RootImageRemoveCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove the fishler Docker image",
	Long:  `If the fishler Docker image is not yet built, build it. Use --force to rebuild.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		CallPersistentPreRun(cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		forceRemove, _ := cmd.Flags().GetBool("force")

		dockerClient, err := client.NewClientWithOpts(
			client.FromEnv,
			client.WithAPIVersionNegotiation(),
		)

		if err != nil {
			util.Logger.Error(err)
			return
		}

		ctx := context.Background()

		images, err := util.GetDockerImage(dockerClient, ctx)

		if err != nil {
			util.Logger.Error(err)
			return
		}

		if len(images) == 0 {
			util.Logger.Info("No fishler image's found for removal")
			return
		}

		for _, image := range images {
			_, err = dockerClient.ImageRemove(ctx, image.ID, types.ImageRemoveOptions{
				Force:         forceRemove,
				PruneChildren: true,
			})

			if err != nil {
				util.Logger.Errorf("Failed to remove image %s %v", image.ID, err)
				continue
			}

			util.Logger.Infof("Image Removed %s", image.ID)
		}
	},
}

func init() {
	RootImageRemoveCmd.Flags().BoolP("force", "f", false, "Force the fishler image to be removed")
}
