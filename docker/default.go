package docker

import "embed"

// content holds the default docker image contents
//
//go:embed *
var DockerFolder embed.FS
