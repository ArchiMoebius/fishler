package util

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ArchiMoebius/fishler/cli/config"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/gliderlabs/ssh"
	"github.com/sirupsen/logrus"
)

func CreateRunWaitSSHContainer(createCfg *container.Config, hostCfg *container.HostConfig, networkCfg *network.NetworkingConfig, sess ssh.Session) (exitCode int64, cleanup func(), err error) {

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
		panic(err)
	}

	exitCode = 255

	res, err := dockerClient.ContainerCreate(ctx, createCfg, hostCfg, networkCfg, nil, fmt.Sprintf("fishler_%s", strings.Split(sess.RemoteAddr().String(), ":")[0]))
	if err != nil {
		Logger.Error(err)
		return
	}

	profiletarbuffer, err := GetProfileBuffer(sess.User())

	if err != nil {
		Logger.Error(err)
		return
	}
	err = dockerClient.CopyToContainer(ctx, res.ID, "/etc/", bytes.NewReader(profiletarbuffer), types.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
		CopyUIDGID:                false,
	})
	if err != nil {
		Logger.Error(err)
		return
	}

	err = dockerClient.ContainerStart(ctx, res.ID, types.ContainerStartOptions{})
	if err != nil {
		Logger.Error(err)
		return
	}

	execResponse, err := dockerClient.ContainerExecCreate(ctx, res.ID, types.ExecConfig{
		User:         "root",
		Tty:          true,
		AttachStdin:  false,
		AttachStderr: false,
		AttachStdout: false,
		Cmd:          []string{"/fixme", sess.User()},
	})
	if err != nil {
		Logger.Error(err)
		return
	}

	execAttachConfig := types.ExecStartCheck{
		Detach: false,
		Tty:    true,
	}

	hijackedResponse, err := dockerClient.ContainerExecAttach(ctx, execResponse.ID, execAttachConfig)
	if err != nil {
		Logger.Error(err)
		return
	}
	hijackedResponse.Close()

	stream, err := dockerClient.ContainerAttach(
		ctx,
		res.ID,
		types.ContainerAttachOptions{
			Stdin:  createCfg.AttachStdin,
			Stdout: createCfg.AttachStdout,
			Stderr: createCfg.AttachStderr,
			Stream: true,
			Logs:   true,
		},
	)
	if err != nil {
		Logger.Error(err)
		return
	}

	logfile := GetSessionFileName(fmt.Sprintf("/%s/session/", config.GlobalConfig.LogBasepath), res.ID, sess.RemoteAddr())
	f, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600) // #nosec

	if err != nil {
		Logger.Error(err)
	}

	cleanup = func() {
		err = f.Close()

		if err != nil {
			Logger.Error(err)
		}

		err := CleanIfContainerExists(ctx, dockerClient, res.ID)

		if err != nil {
			Logger.Error(err)
			return
		}
		stream.Close()
	}

	mw := io.MultiWriter(sess, f)

	go func() {
		// From container to session/logfile - Copy copies from src (sess) to dst (stream.Conn) until either EOF is reached on src or an error occurs.
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
		defer stream.CloseWrite()
		// From session to container - Copy copies from src (sess) to dst (stream.Conn) until either EOF is reached on src or an error occurs.
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
		_, winCh, _ := sess.Pty()
		go func() {
			var height = 0
			var width = 0

			for win := range winCh {
				width = win.Width
				height = win.Height

				err := dockerClient.ContainerResize(ctx, res.ID, types.ResizeOptions{
					Height: uint(height),
					Width:  uint(width),
				})
				if err != nil {
					Logger.Error(err)
					break
				}
			}

			Logger.WithFields(logrus.Fields{
				"address": sess.RemoteAddr().String(),
				"window":  fmt.Sprintf("%dx%d", width, height),
			}).Info("window event")
		}()
	}

	resultC, errC := dockerClient.ContainerWait(ctx, res.ID, container.WaitConditionNotRunning)

	select {
	case err = <-errC:
		Logger.Error(err)
		return
	case result := <-resultC:
		exitCode = result.StatusCode
	}

	Logger.WithFields(logrus.Fields{
		"address":  sess.RemoteAddr().String(),
		"exitcode": exitCode,
	}).Info("exit event")

	return
}

func CleanIfContainerExists(ctx context.Context, client *client.Client, containerID string) error {
	_, err := client.ContainerInspect(ctx, containerID)

	if err == nil { // container exists

		err = client.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true})
		if err != nil {
			return err
		}
	}

	return nil
}
