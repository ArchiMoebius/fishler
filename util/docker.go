package util

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	config "github.com/archimoebius/fishler/cli/config/root"
	"github.com/ccoveille/go-safecast/v2"
	"github.com/charmbracelet/ssh"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
)

var ErrorContainerNameNotFound = errors.New("container name not found")

func CreateRunWaitSSHContainer(hostVolumnWorkingDir string, createCfg *container.Config, hostCfg *container.HostConfig, networkCfg *network.NetworkingConfig, sess ssh.Session) (exitCode int64, err error) {
	var dockerVolumnWorkingDir = fmt.Sprintf("/home/%s", sess.User())

	if sess.User() == "root" {
		dockerVolumnWorkingDir = "/root/"
	}

	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	err = BuildFishler(dockerClient, ctx, false)
	if err != nil {
		dockerClient.Close()
		panic(err)
	}

	exitCode = 255

	hostCfg.Mounts = append(hostCfg.Mounts, mount.Mount{
		ReadOnly: false,
		Type:     mount.TypeBind,
		Source:   hostVolumnWorkingDir,
		Target:   dockerVolumnWorkingDir,
	})

	containerName := sess.Context().SessionID()
	Logger.Debugf("Requesting container: %s\n", containerName)

	createResponse, e := dockerClient.ContainerCreate(ctx, createCfg, hostCfg, networkCfg, nil, containerName)
	if e != nil {
		Logger.Error(e)
		dockerClient.Close()
		return exitCode, e
	}

	containerID := createResponse.ID
	Logger.Debugf("Created ContainerID: %s\n", containerID)

	profiletarbuffer, e := GetProfileBuffer(sess.User())
	if e != nil {
		Logger.Error(e)
		dockerClient.Close()
		return exitCode, e
	}

	e = dockerClient.CopyToContainer(ctx, containerID, "/etc/", bytes.NewReader(profiletarbuffer), container.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
		CopyUIDGID:                false,
	})
	if e != nil {
		Logger.Error(e)
		dockerClient.Close()
		return exitCode, e
	}

	e = dockerClient.ContainerStart(ctx, containerID, container.StartOptions{})
	if e != nil {
		Logger.Error(e)
		dockerClient.Close()
		return exitCode, e
	}

	execResponse, e := dockerClient.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		User:         "root",
		Tty:          true,
		AttachStdin:  false,
		AttachStderr: false,
		AttachStdout: false,
		Cmd:          []string{"/fixme", sess.User()},
	})
	if e != nil {
		Logger.Error(e)
		_ = dockerClient.ContainerKill(ctx, containerID, "")
		dockerClient.Close()
		return exitCode, e
	}

	hijackedResponse, e := dockerClient.ContainerExecAttach(
		ctx,
		execResponse.ID,
		container.ExecAttachOptions{
			Detach: false,
			Tty:    true,
		},
	)
	if e != nil {
		Logger.Error(e)
		_ = dockerClient.ContainerKill(ctx, containerID, "")
		dockerClient.Close()
		return exitCode, e
	}
	if config.Setting.Debug {
		Logger.Info("Container Exec Init Output: ")
		b := make([]byte, 1)
		for {
			_, err := hijackedResponse.Reader.Read(b)
			fmt.Printf("%s", b)
			if err == io.EOF {
				break
			}
		}
	}
	hijackedResponse.Close()

	stream, err := dockerClient.ContainerAttach(
		ctx,
		containerID,
		container.AttachOptions{
			Stdin:  createCfg.AttachStdin,
			Stdout: createCfg.AttachStdout,
			Stderr: createCfg.AttachStderr,
			Stream: true,
			Logs:   false,
		},
	)
	if err != nil {
		Logger.Error(err)
		_ = dockerClient.ContainerKill(ctx, containerID, "")
		dockerClient.Close()
		return exitCode, err
	}

	logfile := GetSessionFileName(fmt.Sprintf("/%s/session/", config.Setting.LogBasepath), sess.Context().SessionID())
	f, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600) // #nosec

	if err != nil {
		Logger.Error(err)
		stream.Close()
		_ = dockerClient.ContainerKill(ctx, containerID, "")
		dockerClient.Close()
		return exitCode, err
	}

	mw := io.MultiWriter(sess, f)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		// From container to session/logfile
		bytes, err := io.Copy(mw, stream.Reader)

		if err != nil {
			Logger.Error(err)
		}

		Logger.WithFields(logrus.Fields{
			"address": sess.RemoteAddr().String(),
			"bytes":   ByteCountDecimal(bytes),
		}).Info("metadata container to session")
	}()

	go func() {
		defer wg.Done()
		// From session to container
		bytes, err := io.Copy(stream.Conn, sess)

		if err != nil {
			Logger.Error(err)
		}

		Logger.WithFields(logrus.Fields{
			"address": sess.RemoteAddr().String(),
			"bytes":   ByteCountDecimal(bytes),
		}).Info("metadata session to container")
	}()

	if createCfg.Tty {
		_, winCh, hasPty := sess.Pty()
		if hasPty && winCh != nil {
			go func() {
				var height uint = 0
				var width uint = 0

				for win := range winCh {
					width, err = safecast.Convert[uint](win.Width)
					if err != nil {
						width = 1024
					}
					height, err = safecast.Convert[uint](win.Height)
					if err != nil {
						height = 768
					}

					err := dockerClient.ContainerResize(ctx, containerID, container.ResizeOptions{
						Height: height,
						Width:  width,
					})
					if err != nil {
						// Container probably stopped, exit resize loop
						break
					}
				}

				Logger.WithFields(logrus.Fields{
					"address": sess.RemoteAddr().String(),
					"window":  fmt.Sprintf("%dx%d", width, height),
				}).Info("window event")
			}()
		}
	}

	resultC, errC := dockerClient.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)

	select {
	case err = <-errC:
		Logger.Error(err)
	case result := <-resultC:
		exitCode = result.StatusCode
	}

	// Wait for all IO goroutines to complete
	wg.Wait()

	// Close file after all writes complete
	f.Close()

	// Now safe to close stream and docker client
	stream.Close()
	dockerClient.Close()

	Logger.WithFields(logrus.Fields{
		"address":  sess.RemoteAddr().String(),
		"exitcode": exitCode,
	}).Info("exit event")

	return exitCode, nil
}
