package cmd

import (
	"context"

	cmd "github.com/ArchiMoebius/fishler/cli/cmd/image"
	"github.com/ArchiMoebius/fishler/util"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"
)

var ImageCmd = &cobra.Command{
	Use:   "image",
	Short: "Build, list, delete fishler Docker image",
	Long:  `Manage the fishler images`,
	Run: func(cmd *cobra.Command, args []string) {

		dockerClient, err := client.NewClientWithOpts(
			client.FromEnv,
			client.WithAPIVersionNegotiation(),
		)

		if err != nil {
			util.Logger.Error(err)
		}

		ctx := context.Background()

		images, err := util.GetDockerImage(dockerClient, ctx)

		if err != nil {
			util.Logger.Error(err)
			return
		}

		if len(images) <= 0 {
			util.Logger.Info("No image's found - try building one!")
		}

		for _, image := range images {
			util.Logger.Infof("Size: %s Image: %v ID: %s", util.ByteCountDecimal(image.Size), image.RepoTags, image.ID)
		}

	},
}

func init() {
	ImageCmd.AddCommand(cmd.BuildCmd)
	ImageCmd.AddCommand(cmd.RootImageRemoveCmd)
}
