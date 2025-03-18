package cli

import (
	"context"

	"github.com/archimoebius/fishler/util"
	"github.com/docker/docker/api/types/image"
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

		for _, img := range images {
			_, err = dockerClient.ImageRemove(ctx, img.ID, image.RemoveOptions{
				Force:         forceRemove,
				PruneChildren: true,
			})

			if err != nil {
				util.Logger.Errorf("Failed to remove image %s %v", img.ID, err)
				continue
			}

			util.Logger.Infof("Image Removed %s", img.ID)
		}
	},
}

func init() {
	RootImageRemoveCmd.Flags().BoolP("force", "f", false, "Force the fishler image to be removed")
}
