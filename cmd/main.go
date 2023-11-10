package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/ArchiMoebius/fishler/shim"
	"github.com/ArchiMoebius/fishler/util"
	futil "github.com/ArchiMoebius/fishler/util"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gliderlabs/ssh"
	glssh "github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

func main() {

	log.Println("starting ssh server on port 2222...")

	s := &glssh.Server{
		Addr: ":2222", // TODO: cli config
		ChannelHandlers: map[string]glssh.ChannelHandler{
			"session": shim.FishlerSessionHandler,
		},
		PasswordHandler: func(ctx ssh.Context, pass string) bool {
			//return pass == "secret"
			return true // TODO cli config, if file list of passwords, if string, just that password
		},
		Handler: func(sess glssh.Session) {
			_, _, isTty := sess.Pty()

			var workingDir = fmt.Sprintf("/home/%s", sess.User())

			if sess.User() == "root" {
				workingDir = "/root/"
			}

			createCfg := &container.Config{
				Image:        "fishler", // TODO: build w/ golang
				Hostname:     "fishy",   // TODO: make cli config
				User:         sess.User(),
				Cmd:          sess.Command(),
				Env:          sess.Environ(),
				Tty:          isTty,
				OpenStdin:    true,
				AttachStderr: true,
				AttachStdin:  true,
				AttachStdout: true,
				StdinOnce:    false,
				WorkingDir:   workingDir,
			}
			hostCfg := &container.HostConfig{
				AutoRemove: true,
				//ReadonlyRootfs: true,
				NetworkMode:   "none",
				DNS:           []string{"127.0.0.1"},
				DNSSearch:     []string{"local"},
				Privileged:    false,
				ShmSize:       4096,
				ReadonlyPaths: []string{"/bin", "/dev", "/lib", "/media", "/mnt", "/opt", "/run", "/sbin", "/srv", "/sys", "/usr", "/var", "/root", "/tmp"},
			}
			networkCfg := &network.NetworkingConfig{}
			status, cleanup, err := dockerRun(createCfg, hostCfg, networkCfg, sess)
			defer cleanup()
			if err != nil {
				fmt.Fprintln(sess, err)
				log.Println(err)
			}
			sess.Exit(int(status))
		},
	}
	pemBytes, err := os.ReadFile("/home/user/.ssh/fishler") //TODO: cli config, if not present, generate one at startup and use it
	if err != nil {
		log.Fatal(err)
	}

	signer, err := gossh.ParsePrivateKey(pemBytes)
	if err != nil {
		log.Fatal(err)
	}

	s.AddHostKey(signer)

	log.Fatal(s.ListenAndServe())
}

func dockerRun(createCfg *container.Config, hostCfg *container.HostConfig, networkCfg *network.NetworkingConfig, sess ssh.Session) (status int64, cleanup func(), err error) {

	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	err = util.BuildFishler(dockerClient, ctx)
	if err != nil {
		panic(err)
	}

	status = 255
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

	etcpasswdtarbuffer, err := util.GetPasswdBuffer(sess.User())

	if err != nil {
		log.Fatal(err)
		return
	}

	err = dockerClient.CopyToContainer(ctx, res.ID, "/etc/", bytes.NewReader(etcpasswdtarbuffer), types.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
		CopyUIDGID:                false,
	})
	if err != nil {
		log.Fatal(err)
		return
	}

	err = dockerClient.ContainerStart(ctx, res.ID, types.ContainerStartOptions{})
	if err != nil {
		return
	}

	// TODO: make cli config, random if set and time not present
	// min := 1.0
	// max := 10.0
	// time.Sleep(time.Duration((min + rand.Float64()*(max-min)) * float64(time.Second)))

	execResponse, err := dockerClient.ContainerExecCreate(ctx, res.ID, types.ExecConfig{
		User:         "root",
		Tty:          false,
		AttachStdin:  false,
		AttachStderr: false,
		AttachStdout: false,
		Cmd:          []string{"/fixme", sess.User()},
	})
	if err != nil {
		log.Fatal(err)
	}

	execAttachConfig := types.ExecStartCheck{
		Detach: false,
		Tty:    createCfg.Tty,
	}

	_, err = dockerClient.ContainerExecAttach(ctx, execResponse.ID, execAttachConfig)
	if err != nil {
		fmt.Print(err.Error())
	}

	stream, err := dockerClient.ContainerAttach(ctx, res.ID, opts)
	if err != nil {
		fmt.Print(err.Error())
		return
	}
	cleanup = func() {
		dockerClient.ContainerRemove(ctx, res.ID, types.ContainerRemoveOptions{})
		stream.Close()
	}

	outputErr := make(chan error)

	logfile := futil.GetSessionFileName(res.ID, sess.RemoteAddr())
	// open file read/write | create if not exist | clear file at open if exists
	f, _ := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)

	// save existing stdout | MultiWriter writes to saved stdout and file
	mw := io.MultiWriter(sess, f)

	// get pipe reader and writer | writes to pipe writer come out pipe reader
	r, w, _ := os.Pipe()

	// writes with log.Print should also write to mw
	log.SetOutput(mw)

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
					log.Println(err)
					break
				}
			}
		}()
	}
	resultC, errC := dockerClient.ContainerWait(ctx, res.ID, container.WaitConditionNotRunning)
	select {
	case err = <-errC:
		return
	case result := <-resultC:
		status = result.StatusCode
	}
	err = <-outputErr
	return
}
