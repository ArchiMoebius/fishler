package app

import (
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/ArchiMoebius/fishler/cli/config"
	"github.com/ArchiMoebius/fishler/shim"
	"github.com/ArchiMoebius/fishler/util"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/gliderlabs/ssh"
	"github.com/sirupsen/logrus"
)

// Application is the interface for the application
type Application interface {
	Start() error
}

// app is the implementation of the application
type app struct {
}

// NewApplication creates a new application
func NewApplication() Application {
	return &app{}
}

func (a *app) Start() error {
	s := &ssh.Server{
		Addr: fmt.Sprintf(":%d", config.GlobalConfig.Port),
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"session": shim.FishlerSessionHandler,
		},
		PasswordHandler: func(ctx ssh.Context, password string) bool {

			if config.GlobalConfig.RandomConnectionSleepCount > 0 {
				min := 1.0
				max := float64(config.GlobalConfig.RandomConnectionSleepCount)

				time.Sleep(time.Duration((min + rand.Float64()*(max-min)) * float64(time.Second))) // #nosec
			}

			authenticated := config.Authenticate(ctx.User(), password)

			util.Logger.WithFields(logrus.Fields{
				"address":  ctx.RemoteAddr().String(),
				"username": ctx.User(),
				"password": password,
				"success":  authenticated,
			}).Info("authentication event")

			return authenticated
		},
		Handler: func(sess ssh.Session) {
			_, _, isTty := sess.Pty()

			util.Logger.WithFields(logrus.Fields{
				"address":     sess.RemoteAddr().String(),
				"username":    sess.User(),
				"command":     sess.RawCommand(),
				"environment": sess.Environ(),
				"version":     sess.Context().ClientVersion(),
				"pty":         isTty,
				"publickey":   sess.PublicKey(),
				"subsystem":   sess.Subsystem(),
			}).Info("session event")

			var workingDir = fmt.Sprintf("/home/%s", sess.User())

			if sess.User() == "root" {
				workingDir = "/root/"
			}

			createCfg := &container.Config{
				Image:        config.GlobalConfig.DockerImagename,
				Hostname:     config.GlobalConfig.DockerHostname,
				User:         sess.User(),
				Cmd:          sess.Command(),
				Env:          sess.Environ(),
				Tty:          true,
				OpenStdin:    true,
				AttachStderr: true,
				AttachStdin:  true,
				AttachStdout: true,
				StdinOnce:    false,
				WorkingDir:   workingDir,
			}
			hostCfg := &container.HostConfig{
				AutoRemove:    false,
				NetworkMode:   "none",
				DNS:           []string{"127.0.0.1"},
				DNSSearch:     []string{"local"},
				Privileged:    false,
				ShmSize:       4096,
				ReadonlyPaths: []string{"/bin", "/dev", "/lib", "/media", "/mnt", "/opt", "/run", "/sbin", "/srv", "/sys", "/usr", "/var", "/root", "/tmp"},
				Resources: container.Resources{
					Memory: 1024 * 1024 * int64(config.GlobalConfig.DockerMemoryLimit),
				},
			}

			if len(config.GlobalConfig.Volumns) > 0 {
				hostCfg.Binds = config.GlobalConfig.Volumns
			}

			networkCfg := &network.NetworkingConfig{}
			status, cleanup, err := util.CreateRunWaitSSHContainer(createCfg, hostCfg, networkCfg, sess)
			defer cleanup()

			if err != nil {
				util.Logger.Error(err)
			}

			err = sess.Exit(int(status))

			if err != nil {
				util.Logger.Error(err)
			}
		},
	}

	signer, err := util.GetKeySigner()
	if err != nil {
		util.Logger.Error(err)
	}

	s.AddHostKey(signer)

	addr := s.Addr
	if addr == "" {
		addr = ":22"
	}
	ln, err := net.Listen("tcp", addr)

	if err != nil {
		return err
	}

	util.Logger.Infof("Listening on %s", ln.Addr().String())

	return s.Serve(ln)
}
