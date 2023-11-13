package util

import (
	"context"
	"os"
	"testing"

	config "github.com/archimoebius/fishler/cli/config/root"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{}

func init() {
	os.Setenv("FISHLER_LOG_BASEPATH", "/tmp/")
	config.CommandInit(RootCmd)
	config.Load()
	SetLogger("/tmp/testing.log")
}

func TestBuildImage(t *testing.T) {

	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	if BuildFishler(dockerClient, ctx, false) != nil {
		t.Fatalf("Failed to build fishler image")
	}

	if BuildFishler(dockerClient, ctx, true) != nil {
		t.Fatalf("Failed to force build fishler image")
	}
}
