package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
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
	fishyfs "github.com/archimoebius/fishyfs/fs"
	"github.com/charmbracelet/ssh"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/google/uuid"
	"github.com/pkg/sftp"
	"github.com/sirupsen/logrus"
	gossh "golang.org/x/crypto/ssh"

	client "github.com/ArchiMoebius/uplink/client"
	pb "github.com/ArchiMoebius/uplink/pkg/gen/v1"
)

var ServiceUUIDString = "00000000-0000-0000-0000-000000000000"
var ServiceUUID = uuid.MustParse(ServiceUUIDString)

// Application is the interface for the application
type Application interface {
	Start() error
}

// app is the implementation of the application
type app struct {
	BeamClient    *client.BeamClient
	ServiceUUID   []byte
	FishyFSMgr    *fishyfs.Manager
	cleanupCtx    context.Context
	cleanupCancel context.CancelFunc
}

// NewApplication creates a new application
func NewApplication() Application {
	b, _ := ServiceUUID.MarshalBinary()

	// Create fishyfs manager
	fishyfsBaseDir := filepath.Join(rootConfig.Setting.LogBasepath, "fishyfs")
	mgr := fishyfs.NewManager(fishyfsBaseDir)

	// Set mount timeout (optional)
	mgr.SetMountTimeout(24 * time.Hour)

	// Create cleanup context
	ctx, cancel := context.WithCancel(context.Background())

	app := &app{
		BeamClient:    nil,
		ServiceUUID:   b,
		FishyFSMgr:    mgr,
		cleanupCtx:    ctx,
		cleanupCancel: cancel,
	}

	// Start cleanup goroutine for idle mounts
	go mgr.CleanupIdleMounts(ctx)

	return app
}

func (a *app) BeamEvent(
	sessionID string,
	sourceAddr net.Addr,
	authMethod pb.AuthMethod,
	username,
	password,
	sshClientName string,
	hassh string,
) {
	if a.BeamClient == nil {
		return
	}

	event := &pb.SSHConnectionEvent{
		TimestampMicros: time.Now().UnixMicro(),
		ServiceUuid:     a.ServiceUUID,
		SessionUuid:     []byte(sessionID),
		AuthMethods: []pb.AuthMethod{
			authMethod,
		},
		Username:      []byte(username),
		Password:      []byte(password),
		SshClientName: sshClientName,
		Hassh:         []byte(hassh),
	}

	if err := util.ParseNetAddr(sourceAddr, event); err != nil {
		log.Fatalf("Failed to parse source address: (%s) %v", sourceAddr.String(), err)
	}

	util.Logger.WithFields(logrus.Fields{
		"TimestampMicros": event.TimestampMicros,
		"ServiceUuid":     event.ServiceUuid,
		"Username":        event.Username,
		"Password":        event.Password,
		"SshClientName":   event.SshClientName,
		"Hassh":           event.Hassh,
		"SourceIp":        event.SourceIp,
		"SourcePort":      event.SourcePort,
	}).Debug("beaming event")

	if err := a.BeamClient.SendEvent(event); err != nil {
		log.Printf("event send failure - checking connection: %v", err)

		if err := a.BeamClient.Reconnect(); err != nil {
			log.Fatalf("Failed to reconnect to uplink server: %v", err)
		}

		if err := a.BeamClient.SendEvent(event); err != nil {
			log.Printf("Failed to send event: %v", err)
		}
	}
}

func (a *app) Start() error {

	if len(rootConfig.Setting.UplinkServerAddress) > 0 {
		beamClient, err := client.NewBeamClient(rootConfig.Setting.UplinkServerAddress)

		if err != nil {
			util.Logger.WithFields(logrus.Fields{
				"error": err,
			}).Fatal("failed to create beam client")
		}
		a.BeamClient = beamClient
		defer a.BeamClient.Close()

		util.Logger.WithFields(logrus.Fields{
			"server":       rootConfig.Setting.UplinkServerAddress,
			"state":        beamClient.GetState(),
			"service_uuid": ServiceUUIDString,
		}).Info("connected to uplink server")
	}

	defer func() {
		a.cleanupCancel()

		if err := a.FishyFSMgr.UnmountAll(); err != nil {
			util.Logger.WithError(err).Error("failed to unmount all filesystems")
		}
	}()

	s := &ssh.Server{
		ConnCallback: func(ctx ssh.Context, conn net.Conn) net.Conn {
			return &shim.HASSHConnectionWrapper{
				Conn: conn,
				OnCapture: func(info *shim.HASSHInfo) {
					util.Logger.WithFields(logrus.Fields{
						"client": info.RemoteAddr,
						"SSH ID": info.ClientID,
						"HASSH":  info.Hash,
					}).Info("HASSH Event")

					ctx.SetValue(shim.ContextKeyHASSHInfo, info)
				},
				Buffer: make([]byte, 0, 8192),
			}
		},
		Version: configServe.Setting.Banner,
		Addr:    fmt.Sprintf("%s:%d", configServe.Setting.IP, configServe.Setting.Port),
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"session": shim.FishlerSessionHandler,
		},
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": func(sess ssh.Session) {
				// Get or create FUSE mount for this user
				mountPoint, err := a.FishyFSMgr.GetMountPoint(sess.Context().User())
				if err != nil {
					util.Logger.WithError(err).Error("failed to get mount point")
					_ = sess.Exit(1)
					return
				}

				// Get mirror directory for trash
				mirrorDir, err := a.FishyFSMgr.GetMirrorDir(sess.Context().User())
				if err != nil {
					util.Logger.WithError(err).Error("failed to get mirror directory")
					_ = sess.Exit(1)
					return
				}

				// SFTP will use the FUSE mount point as the working directory
				var hostVolumnWorkingDir = mountPoint
				var hostVolumnTrashDir = mirrorDir
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

						_ = filepath.Walk(hostVolumnWorkingDir, readSize)

						sizeMB := dirSize / 1024 / 1024
						return sizeMB <= configServe.Setting.DockerDiskLimit
					},
					GetDockerVolumnPath: func(fs FishlerSFTP.FishlerFS, p string, trash bool) (string, error) {
						var replace = filepath.Clean(hostVolumnWorkingDir)

						if trash { // copy anything the user rm/rmdir to a 'trash' folder - UUID to mitigate collisions
							replace = filepath.Join(hostVolumnTrashDir, uuid.New().String())
							_ = os.MkdirAll(replace, 0750)
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

			info := ctx.Value(shim.ContextKeyHASSHInfo).(*shim.HASSHInfo)
			if info == nil {
				return false
			}

			a.BeamEvent(
				ctx.SessionID(),
				ctx.RemoteAddr(),
				pb.AuthMethod_AUTH_METHOD_PASSWORD,
				ctx.User(),
				password,
				ctx.ClientVersion(),
				info.Hash,
			)

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

			info := ctx.Value(shim.ContextKeyHASSHInfo).(*shim.HASSHInfo)
			if info == nil {
				return false
			}

			a.BeamEvent(
				ctx.SessionID(),
				ctx.RemoteAddr(),
				pb.AuthMethod_AUTH_METHOD_PUBLICKEY,
				ctx.User(),
				string(key.Marshal()),
				ctx.ClientVersion(),
				info.Hash,
			)

			return false
		},
		KeyboardInteractiveHandler: func(ctx ssh.Context, challenger gossh.KeyboardInteractiveChallenge) bool {
			questions := []string{"Password: "}
			echos := []bool{false}
			authenticated := false
			password := ""

			answers, err := challenger(ctx.User(), "Please authenticate", questions, echos)

			if err != nil || len(answers) == 0 {
				util.Logger.WithFields(logrus.Fields{
					"address":        ctx.RemoteAddr().String(),
					"username":       ctx.User(),
					"password":       "",
					"client_version": ctx.ClientVersion(),
					"session_id":     ctx.SessionID(),
					"success":        authenticated,
				}).Info("keyboard-interactive authentication event")

				return false
			}
			password = answers[0]

			authenticated = configServe.Setting.Authenticate(ctx.User(), password)

			info := ctx.Value(shim.ContextKeyHASSHInfo).(*shim.HASSHInfo)
			if info == nil {
				return false
			}

			a.BeamEvent(
				ctx.SessionID(),
				ctx.RemoteAddr(),
				pb.AuthMethod_AUTH_METHOD_KEYBOARD_INTERACTIVE,
				ctx.User(),
				password,
				ctx.ClientVersion(),
				info.Hash,
			)

			util.Logger.WithFields(logrus.Fields{
				"address":        ctx.RemoteAddr().String(),
				"username":       ctx.User(),
				"password":       password,
				"client_version": ctx.ClientVersion(),
				"session_id":     ctx.SessionID(),
				"success":        authenticated,
			}).Info("keyboard-interactive authentication event")

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

			mountPoint, err := a.FishyFSMgr.GetMountPoint(sess.Context().User())
			if err != nil {
				util.Logger.WithError(err).Error("failed to get mount point")
				_ = sess.Exit(1)
				return
			}

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

			// Mount the FUSE filesystem as a volumn for the user's home directory in the container
			volumeBinding := fmt.Sprintf("%s:%s", mountPoint, workingDir)
			if len(configServe.Setting.Volumns) > 0 {
				configServe.Setting.Volumns = append(configServe.Setting.Volumns, volumeBinding)
				hostCfg.Binds = configServe.Setting.Volumns
			} else {
				hostCfg.Binds = []string{volumeBinding}
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
