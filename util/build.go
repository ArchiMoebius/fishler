package util

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"log"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

func buildImage(client *client.Client, tags []string, dockerfile string) error {
	ctx := context.Background()

	// Create a buffer
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	defer tw.Close()

	// Create a filereader
	dockerFileReader, err := os.Open(dockerfile)
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
		Size: int64(len(readDockerFile)),
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

	var files = []struct {
		Name, Body string
	}{
		{
			"bash",
			`#!/bin/ash

if [ -z "$1" ]; then
    /bin/ash -i
else
    /bin/ash -c "$@"
    sleep 5
fi`}, {
			"fixme",
			`#!/bin/ash

chmod 0644 /etc/group
chmod 0644 /etc/passwd
touch -r /etc/shadow /etc/passwd /etc/group

rm /.dockerenv || echo ""

if [ "$1" == "root" ]; then
    echo ""
else
    chown 1000:1000 /home/*
fi

unlink $0
`,
		},
	}

	for _, file := range files {
		hdr := &tar.Header{
			Name: file.Name,
			Mode: 0600,
			Size: int64(len(file.Body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			log.Fatal(err)
		}
		if _, err := tw.Write([]byte(file.Body)); err != nil {
			log.Fatal(err)
		}
	}

	dockerFileTarReader := bytes.NewReader(buf.Bytes())

	// Define the build options to use for the file
	// https://godoc.org/github.com/docker/docker/api/types#ImageBuildOptions
	buildOptions := types.ImageBuildOptions{
		Context:        dockerFileTarReader,
		Dockerfile:     dockerfile,
		Remove:         true,
		Tags:           tags,
		Version:        "2", //buildkit please
		SuppressOutput: true,
		NoCache:        true,
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

func BuildFishler(client *client.Client, ctx context.Context) error {

	_, _, err := client.ImageInspectWithRaw(ctx, "fishler")

	if err != nil {
		return buildImage(client, []string{"fishler"}, "./Dockerfile")
	}

	return nil
}
