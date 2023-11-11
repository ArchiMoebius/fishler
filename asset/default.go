package asset

import (
	"embed"
)

// content holds the default docker image contents
//
//go:embed docker
var DockerFolder embed.FS
