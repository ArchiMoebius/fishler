package app

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	rootConfig "github.com/archimoebius/fishler/cli/config/root"
	configServe "github.com/archimoebius/fishler/cli/config/serve"
	"github.com/archimoebius/fishler/shim"
	"github.com/archimoebius/fishler/util"
	FishlerSFTP "github.com/archimoebius/fishler/util/sftp"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/gliderlabs/ssh"
	"github.com/google/uuid"
	"github.com/pkg/sftp"
	"github.com/sirupsen/logrus"

	gossh "golang.org/x/crypto/ssh"
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
		Version: configServe.Setting.Banner,
		Addr:    fmt.Sprintf("%s:%d", configServe.Setting.IP, configServe.Setting.Port),
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"session": shim.FishlerSessionHandler,
		},
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": func(sess ssh.Session) {

				var hostVolumnWorkingDir = util.GetSessionVolumnDirectory(rootConfig.Setting.LogBasepath, "data", sess.RemoteAddr())
				var hostVolumnTrashDir = util.GetSessionVolumnDirectory(rootConfig.Setting.LogBasepath, "trash", sess.RemoteAddr())
				var dockerVolumnWorkingDir = fmt.Sprintf("/home/%s", sess.User())

				if sess.User() == "root" {
					dockerVolumnWorkingDir = "/root/"
				}

				options := []sftp.RequestServerOption{
					sftp.WithStartDirectory(dockerVolumnWorkingDir),
				}

				p := FishlerSFTP.FishlerFS{
					HasDiskSpace: func(fs FishlerSFTP.FishlerFS) bool {
						var dirSize int64 = 0

						readSize := func(path string, file os.FileInfo, err error) error {
							if !file.IsDir() {
								dirSize += file.Size()
							}

							return nil
						}

						filepath.Walk(hostVolumnWorkingDir, readSize)

						sizeMB := dirSize / 1024 / 1024

						return sizeMB <= configServe.Setting.DockerDiskLimit
					},
					GetDockerVolumnPath: func(fs FishlerSFTP.FishlerFS, p string, trash bool) (string, error) {
						var replace = filepath.Clean(hostVolumnWorkingDir)

						if trash { // copy anything the user rm/rmdir to a 'trash' folder - UUID to mitigate collisions
							replace = filepath.Join(hostVolumnTrashDir, uuid.New().String())
							os.MkdirAll(replace, 0750)
						}

						p = filepath.Clean(strings.ReplaceAll(filepath.Clean(p), dockerVolumnWorkingDir, replace))

						if !strings.HasPrefix(p, replace) {
							return "", errors.New("bad path")
						}

						return p, nil
					},
					Lock:     &sync.Mutex{},
					User:     sess.User(),
					RemoteIP: sess.RemoteAddr().String(),
				}

				requestServer := sftp.NewRequestServer(
					sess,
					sftp.Handlers{
						FileGet:  p,
						FilePut:  p,
						FileCmd:  p,
						FileList: p,
					},
					options...,
				)

				defer requestServer.Close()

				if err := requestServer.Serve(); err == io.EOF {

					util.Logger.WithFields(logrus.Fields{
						"address":  sess.RemoteAddr().String(),
						"username": sess.User(),
					}).Info("sftp exit event")
				} else if err != nil {
					util.Logger.WithFields(logrus.Fields{
						"address":  sess.RemoteAddr().String(),
						"username": sess.User(),
						"error":    err,
					}).Error("sftp error event")
				}
			},
		},
		PasswordHandler: func(ctx ssh.Context, password string) bool {

			if configServe.Setting.RandomConnectionSleepCount > 0 {
				min := 1.0
				max := float64(configServe.Setting.RandomConnectionSleepCount)

				time.Sleep(time.Duration((min + rand.Float64()*(max-min)) * float64(time.Second))) // #nosec
			}

			authenticated := configServe.Setting.Authenticate(ctx.User(), password)

			util.Logger.WithFields(logrus.Fields{
				"address":        ctx.RemoteAddr().String(),
				"username":       ctx.User(),
				"password":       password,
				"client_version": ctx.ClientVersion(),
				"session_id":     ctx.SessionID(),
				"success":        authenticated,
			}).Info("password authentication event")

			return authenticated
		},
		PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
			util.Logger.WithFields(logrus.Fields{
				"address":        ctx.RemoteAddr().String(),
				"username":       ctx.User(),
				"key":            key.Marshal(),
				"client_version": ctx.ClientVersion(),
				"session_id":     ctx.SessionID(),
			}).Info("public-key authentication event")
			return false
		},
		KeyboardInteractiveHandler: func(ctx ssh.Context, challenger gossh.KeyboardInteractiveChallenge) bool {
			util.Logger.WithFields(logrus.Fields{
				"address":        ctx.RemoteAddr().String(),
				"username":       ctx.User(),
				"client_version": ctx.ClientVersion(),
				"session_id":     ctx.SessionID(),
			}).Info("keyboard-interactive authentication event")
			return false
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
				Image:        rootConfig.Setting.DockerImagename,
				Hostname:     configServe.Setting.DockerHostname,
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
				Labels:       map[string]string{"fishler": "fishler"},
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
					Memory: 1024 * 1024 * int64(configServe.Setting.DockerMemoryLimit),
				},
			}

			if len(configServe.Setting.Volumns) > 0 {
				hostCfg.Binds = configServe.Setting.Volumns
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
		LocalPortForwardingCallback: func(ctx ssh.Context, destinationHost string, destinationPort uint32) bool {
			util.Logger.WithFields(logrus.Fields{
				"address":        ctx.RemoteAddr().String(),
				"username":       ctx.User(),
				"client_version": ctx.ClientVersion(),
				"session_id":     ctx.SessionID(),
				"host":           destinationHost,
				"port":           destinationPort,
			}).Info("ssh local port forward request")
			return false
		},
		ReversePortForwardingCallback: func(ctx ssh.Context, bindHost string, bindPort uint32) bool {
			util.Logger.WithFields(logrus.Fields{
				"address":        ctx.RemoteAddr().String(),
				"username":       ctx.User(),
				"client_version": ctx.ClientVersion(),
				"session_id":     ctx.SessionID(),
				"host":           bindHost,
				"port":           bindPort,
			}).Info("ssh reverse port forward request")
			return false
		},
		ConnectionFailedCallback: func(conn net.Conn, err error) {
			util.Logger.WithFields(logrus.Fields{
				"address": conn.RemoteAddr().String(),
				"error":   err,
			}).Info("connection failed error")
		},
	}

	if configServe.Setting.MaxTimeout > 0 {
		s.MaxTimeout = configServe.Setting.MaxTimeout
	}

	if configServe.Setting.IdleTimeout > 0 {
		s.IdleTimeout = configServe.Setting.IdleTimeout
	}

	signer, err := util.GetKeySigner()
	if err != nil {
		util.Logger.Error(err)
	}

	s.AddHostKey(signer)

	addr := s.Addr
	if addr == "" {
		addr = ":2222"
	}
	ln, err := net.Listen("tcp4", addr)

	if err != nil {
		return err
	}

	util.Logger.Infof("Listening on %s", ln.Addr().String())

	return s.Serve(ln)
}
