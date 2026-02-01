package util

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

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

func CreateRunWaitSSHContainer(hostVolumnWorkingDir string, createCfg *container.Config, hostCfg *container.HostConfig, networkCfg *network.NetworkingConfig, sshSession ssh.Session) (exitCode int64, err error) {
	var dockerVolumnWorkingDir = fmt.Sprintf("/home/%s", sshSession.User())

	if sshSession.User() == "root" {
		dockerVolumnWorkingDir = "/root/"
	}

	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		panic(err)
	}
	defer dockerClient.Close()

	ctx := context.Background()

	err = BuildFishler(dockerClient, ctx, false)
	if err != nil {
		panic(err)
	}

	exitCode = 255

	hostCfg.Mounts = append(hostCfg.Mounts, mount.Mount{
		ReadOnly: false,
		Type:     mount.TypeBind,
		Source:   hostVolumnWorkingDir,
		Target:   dockerVolumnWorkingDir,
	})

	containerName := sshSession.Context().SessionID()
	Logger.Debugf("Requesting container: %s\n", containerName)

	createResponse, e := dockerClient.ContainerCreate(ctx, createCfg, hostCfg, networkCfg, nil, containerName)
	if e != nil {
		Logger.Error(e)
		return exitCode, e
	}

	containerID := createResponse.ID
	Logger.Debugf("Created ContainerID: %s\n", containerID)

	defer dockerClient.ContainerKill(ctx, containerID, "")

	dockerStream, err := dockerClient.ContainerAttach(
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
		return exitCode, err
	}

	profiletarbuffer, e := GetProfileBuffer(sshSession.User())
	if e != nil {
		Logger.Error(e)
		return exitCode, e
	}

	e = dockerClient.CopyToContainer(ctx, containerID, "/etc/", bytes.NewReader(profiletarbuffer), container.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
		CopyUIDGID:                false,
	})
	if e != nil {
		Logger.Error(e)
		return exitCode, e
	}

	e = dockerClient.ContainerStart(ctx, containerID, container.StartOptions{})
	if e != nil {
		Logger.Error(e)
		return exitCode, e
	}

	execResponse, e := dockerClient.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		User:         "root",
		Tty:          true,
		AttachStdin:  false,
		AttachStderr: false,
		AttachStdout: false,
		Cmd:          []string{"/fixme", sshSession.User()},
	})
	if e != nil {
		Logger.Error(e)
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

	basepath := fmt.Sprintf("/%s/session/", config.Setting.LogBasepath)

	err = os.MkdirAll(basepath, 0750)
	if err != nil {
		Logger.Error(err)
		return exitCode, err
	}

	osRoot, err := os.OpenRoot(basepath)
	if err != nil {
		Logger.Error(err)
		return exitCode, err
	}
	defer osRoot.Close()

	f, err := osRoot.OpenFile(sshSession.Context().SessionID()+".log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		Logger.Error(err)
		return exitCode, err
	}
	defer f.Close()

	mw := io.MultiWriter(sshSession, f)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		bytes, err := io.Copy(mw, dockerStream.Reader)

		if err != nil {
			Logger.Errorf("c->s err: %v", err)
		}

		Logger.WithFields(logrus.Fields{
			"address": sshSession.RemoteAddr().String(),
			"bytes":   ByteCountDecimal(bytes),
		}).Info("metadata container to session")

		sshSession.Close()
	}()

	go func() {
		defer wg.Done()
		defer dockerStream.CloseWrite()

		bytes, err := io.Copy(dockerStream.Conn, sshSession)

		if err != nil {
			Logger.Errorf("s->c err: %v", err)
		}

		Logger.WithFields(logrus.Fields{
			"address": sshSession.RemoteAddr().String(),
			"bytes":   ByteCountDecimal(bytes),
		}).Info("metadata session to container")
	}()

	if createCfg.Tty {
		_, winCh, hasPty := sshSession.Pty()
		if hasPty && winCh != nil {
			wg.Add(1)

			go func() {
				defer wg.Done()
				var height uint = 0
				var width uint = 0

				for win := range winCh {
					width, err = safecast.Convert[uint](win.Width)
					if err != nil {
						width = 1024
						Logger.Errorf("width err: %v", err)
					}
					height, err = safecast.Convert[uint](win.Height)
					if err != nil {
						height = 768
						Logger.Errorf("height err: %v", err)
					}

					err := dockerClient.ContainerResize(ctx, containerID, container.ResizeOptions{
						Height: height,
						Width:  width,
					})

					if err != nil {
						Logger.Errorf("resize err: %v", err)
						break
					}
				}

				Logger.WithFields(logrus.Fields{
					"address": sshSession.RemoteAddr().String(),
					"window":  fmt.Sprintf("%dx%d", width, height),
				}).Info("window event")
			}()
		}
	}

	if len(sshSession.Command()) > 0 {
		dockerStream.Conn.Write([]byte(strings.Join(sshSession.Command(), " ")))
		dockerStream.Conn.Write([]byte("\nexit\n")) // hacky...but meh... TODO: fix?
	}

	wg.Wait()

	time.Sleep(time.Minute * 10)

	resultC, errC := dockerClient.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)

	select {
	case err = <-errC:
		Logger.Error(err)
	case result := <-resultC:
		exitCode = result.StatusCode
	}

	Logger.WithFields(logrus.Fields{
		"address":  sshSession.RemoteAddr().String(),
		"exitcode": exitCode,
	}).Info("exit event")

	return exitCode, nil
}
