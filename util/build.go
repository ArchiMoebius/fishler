package util

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/archimoebius/fishler/asset"
	"github.com/archimoebius/fishler/cli/config"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

func buildImage(client *client.Client, tags []string, dockerBasepath string) error {
	ctx := context.Background()

	reader, err := client.ImagePull(ctx, "docker.io/library/alpine", types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	_, err = io.Copy(os.Stdout, reader)

	if err != nil {
		return err
	}

	var dockerFileContent []byte

	// Create a buffer
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	defer tw.Close()

	dockerfile := fmt.Sprintf("%s/Dockerfile", dockerBasepath)
	docker_rootfs := fmt.Sprintf("%s/rootfs/", dockerBasepath)

	// Create a filereader
	dockerFileReader, err := os.Open(dockerfile) // #nosec

	if errors.Is(err, fs.ErrNotExist) {
		dockerFileContent, err = asset.DockerFolder.ReadFile(dockerfile)

		Logger.Infof("Building %s with embedFS %s", dockerfile, docker_rootfs)
	}

	if err != nil {
		return err
	}

	if len(dockerFileContent) == 0 {
		Logger.Infof("Building %s with rootFS %s", dockerfile, docker_rootfs)

		// Read the actual Dockerfile
		dockerFileContent, err = io.ReadAll(dockerFileReader)
		if err != nil {
			return err
		}
	}

	// Make a TAR header for the file
	tarHeader := &tar.Header{
		Name: dockerfile,
		Size: int64(len(dockerFileContent)), // #nosec
	}

	// Writes the header described for the TAR file
	err = tw.WriteHeader(tarHeader)
	if err != nil {
		return err
	}

	// Writes the dockerfile data to the TAR file
	_, err = tw.Write(dockerFileContent)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	var has_rootfs = true

	err = os.Chdir(docker_rootfs)
	if errors.Is(err, fs.ErrNotExist) {
		has_rootfs = false

		err = fs.WalkDir(asset.DockerFolder, ".", func(path string, info fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			if strings.HasSuffix(path, "Dockerfile") {
				return nil // no need to embed the Dockerfile in the docker image :P
			}

			filedata, err := asset.DockerFolder.ReadFile(path)
			if err != nil {
				return err
			}

			if !strings.HasPrefix(path, "docker/rootfs/") {
				return nil // no need to embed anything in the docker image that shouldn't be in the rootfs
			}

			var mode = int64(0o0644)

			// this script is invoked by root at container creation to make any last second changes to the environment before the user is provided a shell
			if strings.HasSuffix(path, "fixme") {
				mode = int64(0o0700)
			}

			// not really bash :P
			if strings.HasSuffix(path, "bash") {
				mode = int64(0o0755)
			}

			hdr := &tar.Header{
				Name: fmt.Sprintf("/%s", strings.ReplaceAll(path, "docker/rootfs/", "")),
				Mode: mode,
				Size: int64(len(filedata)),
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
	} else {
		return err
	}

	if has_rootfs {
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
