package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ArchiMoebius/fishler/shim"
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
	glssh.Handle(func(sess glssh.Session) {
		_, _, isTty := sess.Pty()
		createCfg := &container.Config{
			Image:        "fishler",
			Cmd:          sess.Command(),
			Env:          sess.Environ(),
			Tty:          isTty,
			OpenStdin:    true,
			AttachStderr: true,
			AttachStdin:  true,
			AttachStdout: true,
			StdinOnce:    true,
			Volumes:      make(map[string]struct{}),
		}
		hostCfg := &container.HostConfig{
			AutoRemove: true,
			//ReadonlyRootfs: true,
			NetworkMode:   "none",
			DNS:           []string{"127.0.0.1"},
			DNSSearch:     []string{"local"},
			Privileged:    false,
			ShmSize:       4096,
			ReadonlyPaths: []string{"/bin", "/dev", "/etc", "/lib", "/media", "/mnt", "/opt", "/run", "/sbin", "/srv", "/sys", "/usr", "/var", "/root", "/tmp"},
		}
		networkCfg := &network.NetworkingConfig{}
		status, cleanup, err := dockerRun(createCfg, hostCfg, networkCfg, sess)
		defer cleanup()
		if err != nil {
			fmt.Fprintln(sess, err)
			log.Println(err)
		}
		sess.Exit(int(status))
	})

	log.Println("starting ssh server on port 2222...")

	s := &glssh.Server{
		Addr: ":2222",
		ChannelHandlers: map[string]glssh.ChannelHandler{
			"session": shim.FishlerSessionHandler,
		},
	}
	pemBytes, err := os.ReadFile("/home/user/.ssh/fishler")
	if err != nil {
		log.Fatal(err)
	}

	signer, err := gossh.ParsePrivateKey(pemBytes)
	if err != nil {
		log.Fatal(err)
	}

	s.AddHostKey(signer)
	// s.AddHostKey(hostKeySigner)

	log.Fatal(s.ListenAndServe())

	// log.Fatal(ssh.ListenAndServe(":2222", nil,
	// 	ssh.PasswordAuth(func(ctx ssh.Context, pass string) bool {
	// 		fmt.Printf("%s : %s", ctx.User(), pass)
	// 		return pass == "secret"
	// 	}),
	// ))
}

func dockerRun(createCfg *container.Config, hostCfg *container.HostConfig, networkCfg *network.NetworkingConfig, sess ssh.Session) (status int64, cleanup func(), err error) {
	docker, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		panic(err)
	}
	status = 255
	cleanup = func() {}
	ctx := context.Background()
	res, err := docker.ContainerCreate(ctx, createCfg, hostCfg, networkCfg, nil, "")
	if err != nil {
		return
	}
	cleanup = func() {
		docker.ContainerRemove(ctx, res.ID, types.ContainerRemoveOptions{})
	}
	opts := types.ContainerAttachOptions{
		Stdin:  createCfg.AttachStdin,
		Stdout: createCfg.AttachStdout,
		Stderr: createCfg.AttachStderr,
		Stream: true,
	}
	stream, err := docker.ContainerAttach(ctx, res.ID, opts)
	if err != nil {
		return
	}
	cleanup = func() {
		docker.ContainerRemove(ctx, res.ID, types.ContainerRemoveOptions{})
		stream.Close()
	}

	outputErr := make(chan error)
	datetime := strings.ReplaceAll(strings.ReplaceAll(fmt.Sprintf("%v", time.Now().Format(time.RFC3339)), ":", ""), "-", "")
	ipaddr := strings.Split(strings.Replace(sess.RemoteAddr().String(), ".", "-", -1), ":")[0]
	logfile := fmt.Sprintf("/tmp/fishler_%s_%s_%s.log", res.ID, datetime, ipaddr)
	// open file read/write | create if not exist | clear file at open if exists
	f, _ := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)

	// save existing stdout | MultiWriter writes to saved stdout and file
	mw := io.MultiWriter(sess, f)

	// get pipe reader and writer | writes to pipe writer come out pipe reader
	r, w, _ := os.Pipe()

	// replace stdout,stderr with pipe writer | all writes to stdout, stderr will go through pipe instead (fmt.print, log)
	os.Stdout = w
	os.Stderr = w

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

	err = docker.ContainerStart(ctx, res.ID, types.ContainerStartOptions{})
	if err != nil {
		return
	}
	if createCfg.Tty {
		_, winCh, _ := sess.Pty()
		go func() {
			for win := range winCh {
				err := docker.ContainerResize(ctx, res.ID, types.ResizeOptions{
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
	resultC, errC := docker.ContainerWait(ctx, res.ID, container.WaitConditionNotRunning)
	select {
	case err = <-errC:
		return
	case result := <-resultC:
		status = result.StatusCode
	}
	err = <-outputErr
	return
}
