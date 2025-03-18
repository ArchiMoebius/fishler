package util

import (
	"context"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

func GetDockerImage(client *client.Client, ctx context.Context) ([]image.Summary, error) {
	// https://docs.docker.com/engine/api/v1.42/#tag/Image/operation/ImageList
	images, err := client.ImageList(
		ctx,
		image.ListOptions{
			All: true,
			Filters: filters.NewArgs(
				filters.Arg("reference", "fishler"),
			),
		},
	)

	if err != nil {
		return nil, err
	}

	return images, err
}
