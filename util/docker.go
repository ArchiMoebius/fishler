package util

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	config "github.com/archimoebius/fishler/cli/config/root"
	configServe "github.com/archimoebius/fishler/cli/config/serve"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/gliderlabs/ssh"
	"github.com/sirupsen/logrus"
)

var ErrorContainerNameNotFound = errors.New("container name not found")
var containerConnectionMap = make(map[string]int)

func GetContainerName(remoteIP string) string {
	return fmt.Sprintf("fishler_%s", strings.Split(remoteIP, ":")[0])
}

func CreateRunWaitSSHContainer(createCfg *container.Config, hostCfg *container.HostConfig, networkCfg *network.NetworkingConfig, sess ssh.Session) (exitCode int64, cleanup func(), err error) {
	var hostVolumnWorkingDir = GetSessionVolumnDirectory(config.Setting.LogBasepath, "data", sess.RemoteAddr())
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
		panic(err)
	}

	exitCode = 255
	cleanup = func() {}

	hostCfg.Mounts = append(hostCfg.Mounts, mount.Mount{
		ReadOnly: false,
		Type:     mount.TypeBind,
		Source:   hostVolumnWorkingDir,
		Target:   dockerVolumnWorkingDir,
	})

	containerName := GetContainerName(sess.RemoteAddr().String())

	containerID, err := getContainerIdFromName(ctx, dockerClient, containerName)
	if err != nil && !errors.Is(err, ErrorContainerNameNotFound) {
		Logger.Error(err)
		return exitCode, cleanup, err
	}

	if containerID == "" { // container does not exist exists
		Logger.Infof("create container event %s", containerID)

		createResponse, e := dockerClient.ContainerCreate(ctx, createCfg, hostCfg, networkCfg, nil, containerName)
		if e != nil {
			Logger.Error(e)
			return exitCode, cleanup, e
		}

		containerID = createResponse.ID
		Logger.Debugf("Created ContainerID: %s\n", containerID)

		profiletarbuffer, e := GetProfileBuffer(sess.User())
		if e != nil {
			Logger.Error(e)
			return exitCode, cleanup, e
		}

		e = dockerClient.CopyToContainer(ctx, containerID, "/etc/", bytes.NewReader(profiletarbuffer), types.CopyToContainerOptions{
			AllowOverwriteDirWithFile: true,
			CopyUIDGID:                false,
		})
		if e != nil {
			Logger.Error(e)
			return exitCode, cleanup, e
		}

		e = dockerClient.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
		if e != nil {
			Logger.Error(e)
			return exitCode, cleanup, e
		}

		execResponse, e := dockerClient.ContainerExecCreate(ctx, containerID, types.ExecConfig{
			User:         "root",
			Tty:          true,
			AttachStdin:  false,
			AttachStderr: false,
			AttachStdout: false,
			Cmd:          []string{"/fixme", sess.User()},
		})
		if e != nil {
			Logger.Error(e)
			return exitCode, cleanup, e
		}

		hijackedResponse, e := dockerClient.ContainerExecAttach(
			ctx,
			execResponse.ID,
			types.ExecStartCheck{
				Detach: false,
				Tty:    true,
			},
		)
		if e != nil {
			Logger.Error(err)
			return exitCode, cleanup, e
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

		if _, found := containerConnectionMap[containerID]; found {
			containerConnectionMap[containerID] = 1
		}
	} else {
		Logger.Infof("container reconnect event %s", containerID)

		if _, found := containerConnectionMap[containerID]; found {
			containerConnectionMap[containerID] += 1
		}
	}

	stream, err := dockerClient.ContainerAttach(
		ctx,
		containerID,
		types.ContainerAttachOptions{
			Stdin:  createCfg.AttachStdin,
			Stdout: createCfg.AttachStdout,
			Stderr: createCfg.AttachStderr,
			Stream: true,
			Logs:   false, // TODO: include all of STDOUT from 1'st connection for 2'nd connection or no
		},
	)
	if err != nil {
		Logger.Error(err)
		return
	}

	logfile := GetSessionFileName(fmt.Sprintf("/%s/session/", config.Setting.LogBasepath), containerID, sess.RemoteAddr())
	f, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600) // #nosec

	if err != nil {
		Logger.Error(err)
	}

	cleanup = func() {
		err = f.Close()

		if err != nil {
			Logger.Error(err)
		}

		containerConnectionMap[containerID] -= 1

		if containerConnectionMap[containerID] <= 0 {
			delete(containerConnectionMap, containerID)

			err := CleanIfContainerExists(ctx, dockerClient, containerID)

			if err != nil {
				Logger.Error(err)
				return
			}
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

				err := dockerClient.ContainerResize(ctx, containerID, types.ResizeOptions{
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

	if configServe.Setting.IdleTimeout > 0 || configServe.Setting.MaxTimeout > 0 {
		go func() {
			i := 0
			for {
				i += 1
				select {
				case <-time.After(time.Second):
					continue
				case <-sess.Context().Done():

					Logger.WithFields(logrus.Fields{
						"address":  sess.RemoteAddr().String(),
						"username": sess.User(),
						"duration": i,
					}).Info("metadata exit event")

					cleanup()
					return
				}
			}
		}()
	}

	resultC, errC := dockerClient.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)

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

func getContainerIdFromName(ctx context.Context, client *client.Client, containerName string) (string, error) {

	filters := filters.NewArgs()
	filters.Add("label", "fishler")

	containers, err := client.ContainerList(context.Background(), types.ContainerListOptions{
		Size:    false,
		All:     false,
		Filters: filters,
	})

	if err != nil {
		return "", err
	}

	var cname = fmt.Sprintf("/%s", containerName)

	for _, container := range containers {
		fmt.Printf("%s %s (%s)\n", container.ID, container.Image, container.Names[0])

		if strings.EqualFold(cname, container.Names[0]) {
			return container.ID, nil
		}
	}

	return "", ErrorContainerNameNotFound
}
