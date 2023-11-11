package util

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ArchiMoebius/fishler/cli/config"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

func buildImage(client *client.Client, tags []string, dockerBasepath string) error {
	ctx := context.Background()

	// Create a buffer
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	defer tw.Close()

	dockerfile := fmt.Sprintf("%s/Dockerfile", dockerBasepath)
	docker_rootfs := fmt.Sprintf("%s/rootfs/", dockerBasepath)

	Logger.Infof("Building %s with rootfs %s", dockerfile, docker_rootfs)

	// Create a filereader
	dockerFileReader, err := os.Open(dockerfile) // #nosec
	if err != nil {
		return err
	}

	// Read the actual Dockerfile
	readDockerFile, err := io.ReadAll(dockerFileReader)
	if err != nil {
		return err
	}

	// Make a TAR header for the file
	tarHeader := &tar.Header{
		Name: dockerfile,
		Size: int64(len(readDockerFile)), // #nosec
	}

	// Writes the header described for the TAR file
	err = tw.WriteHeader(tarHeader)
	if err != nil {
		return err
	}

	// Writes the dockerfile data to the TAR file
	_, err = tw.Write(readDockerFile)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	err = os.Chdir(docker_rootfs)
	if err != nil {
		return err
	}

	err = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		filedata, err := os.ReadFile(path) // #nosec
		if err != nil {
			return err
		}

		hdr := &tar.Header{
			Name: fmt.Sprintf("/%s", path),
			Mode: int64(info.Mode()),
			Size: info.Size(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(filedata); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}
	err = os.Chdir(cwd)
	if err != nil {
		return err
	}

	dockerFileTarReader := bytes.NewReader(buf.Bytes())
	labels := make(map[string]string)
	labels[config.GlobalConfig.DockerImagename] = "latest"

	// Define the build options to use for the file
	// https://godoc.org/github.com/docker/docker/api/types#ImageBuildOptions
	buildOptions := types.ImageBuildOptions{
		Context:        dockerFileTarReader,
		Dockerfile:     dockerfile,
		Remove:         true,
		Tags:           tags,
		Version:        types.BuilderBuildKit,
		SuppressOutput: false,
		NoCache:        true,
		Labels:         labels,
	}

	// Build the actual image
	imageBuildResponse, err := client.ImageBuild(
		ctx,
		dockerFileTarReader,
		buildOptions,
	)

	if err != nil {
		return err
	}

	// Read the STDOUT from the build process
	defer imageBuildResponse.Body.Close()
	_, err = io.Copy(os.Stdout, imageBuildResponse.Body)
	if err != nil {
		return err
	}

	return nil
}

func BuildFishler(client *client.Client, ctx context.Context, forceBuild bool) error {
	images, err := GetDockerImage(client, ctx)

	if err != nil {
		return err
	}

	if len(images) == 0 || forceBuild {
		return buildImage(client, []string{config.GlobalConfig.DockerImagename}, config.GlobalConfig.DockerBasepath)
	}

	return nil
}
