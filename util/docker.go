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
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gliderlabs/ssh"
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
	cleanup = func() {}

	res, err := dockerClient.ContainerCreate(ctx, createCfg, hostCfg, networkCfg, nil, fmt.Sprintf("fishler_%s", strings.Split(sess.RemoteAddr().String(), ":")[0]))
	if err != nil {
		return
	}
	cleanup = func() {
		dockerClient.ContainerRemove(ctx, res.ID, types.ContainerRemoveOptions{Force: true})
	}
	opts := types.ContainerAttachOptions{
		Stdin:  createCfg.AttachStdin,
		Stdout: createCfg.AttachStdout,
		Stderr: createCfg.AttachStderr,
		Stream: true,
	}

	etcpasswdtarbuffer, err := GetPasswdBuffer(sess.User())

	if err != nil {
		Logger.Error(err)
		return
	}

	err = dockerClient.CopyToContainer(ctx, res.ID, "/etc/", bytes.NewReader(etcpasswdtarbuffer), types.CopyToContainerOptions{
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
		Tty:          false,
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
		Tty:    createCfg.Tty,
	}

	_, err = dockerClient.ContainerExecAttach(ctx, execResponse.ID, execAttachConfig)
	if err != nil {
		Logger.Error(err)
		return
	}

	stream, err := dockerClient.ContainerAttach(ctx, res.ID, opts)
	if err != nil {
		Logger.Error(err)
		return
	}
	cleanup = func() {
		dockerClient.ContainerRemove(ctx, res.ID, types.ContainerRemoveOptions{})
		stream.Close()
	}

	outputErr := make(chan error)

	logfile := GetSessionFileName(fmt.Sprintf("/%s/session/", config.GlobalConfig.LogBasepath), res.ID, sess.RemoteAddr())

	f, _ := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	mw := io.MultiWriter(sess, f)
	r, w, _ := os.Pipe()

	//create channel to control exit | will block until all copies are finished
	exit := make(chan bool)

	go func() {
		// copy all reads from pipe to multiwriter, which writes to stdout and file
		_, _ = io.Copy(mw, r)
		// when r or w is closed copy will finish and true will be sent to channel
		exit <- true
	}()

	// function to be deferred in main until program exits
	defer func() {
		// close writer then block on exit channel | this will let mw finish writing before the program exits
		_ = w.Close()
		<-exit
		// close file after all writes have finished
		_ = f.Close()
	}()

	go func() {
		var err error
		if createCfg.Tty {
			_, err = io.Copy(mw, stream.Reader)
		} else {
			_, err = stdcopy.StdCopy(mw, sess.Stderr(), stream.Reader)
		}
		outputErr <- err
	}()

	go func() {
		defer stream.CloseWrite()
		io.Copy(stream.Conn, sess)
	}()

	if createCfg.Tty {
		_, winCh, _ := sess.Pty()
		go func() {
			for win := range winCh {
				err := dockerClient.ContainerResize(ctx, res.ID, types.ResizeOptions{
					Height: uint(win.Height),
					Width:  uint(win.Width),
				})
				if err != nil {
					Logger.Error(err)
					break
				}
			}
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

	Logger.Infof("Session Exit Code: %d", exitCode)

	err = <-outputErr

	if err != nil {
		Logger.Error(err)
	}

	return
}
