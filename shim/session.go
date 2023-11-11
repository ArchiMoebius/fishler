package shim

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/ArchiMoebius/fishler/util"
	"github.com/anmitsu/go-shlex"
	glssh "github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

const (
	agentRequestType = "auth-agent-req@openssh.com"
	agentChannelType = "auth-agent@openssh.com"

	// agentTempDir    = "auth-agent"
	// agentListenFile = "listener.sock"
)

// Session provides access to information about an SSH session and methods
// to read and write to the SSH channel with an embedded Channel interface from
// crypto/ssh.
//
// When Command() returns an empty slice, the user requested a shell. Otherwise
// the user is performing an exec with those command arguments.
//
// TODO: Signals
type Session interface {
	gossh.Channel

	// User returns the username used when establishing the SSH connection.
	User() string

	// RemoteAddr returns the net.Addr of the client side of the connection.
	RemoteAddr() net.Addr

	// LocalAddr returns the net.Addr of the server side of the connection.
	LocalAddr() net.Addr

	// Environ returns a copy of strings representing the environment set by the
	// user for this session, in the form "key=value".
	Environ() []string

	// Exit sends an exit status and then closes the session.
	Exit(code int) error

	// Command returns a shell parsed slice of arguments that were provided by the
	// user. Shell parsing splits the command string according to POSIX shell rules,
	// which considers quoting not just whitespace.
	Command() []string

	// RawCommand returns the exact command that was provided by the user.
	RawCommand() string

	// Subsystem returns the subsystem requested by the user.
	Subsystem() string

	// PublicKey returns the PublicKey used to authenticate. If a public key was not
	// used it will return nil.
	PublicKey() glssh.PublicKey

	// Context returns the connection's context. The returned context is always
	// non-nil and holds the same data as the Context passed into auth
	// handlers and callbacks.
	//
	// The context is canceled when the client's connection closes or I/O
	// operation fails.
	Context() glssh.Context

	// Permissions returns a copy of the Permissions object that was available for
	// setup in the auth handlers via the Context.
	Permissions() glssh.Permissions

	// Pty returns PTY information, a channel of window size changes, and a boolean
	// of whether or not a PTY was accepted for this session.
	Pty() (glssh.Pty, <-chan glssh.Window, bool)

	// Signals registers a channel to receive signals sent from the client. The
	// channel must handle signal sends or it will block the SSH request loop.
	// Registering nil will unregister the channel from signal sends. During the
	// time no channel is registered signals are buffered up to a reasonable amount.
	// If there are buffered signals when a channel is registered, they will be
	// sent in order on the channel immediately after registering.
	Signals(c chan<- glssh.Signal)

	// Break regisers a channel to receive notifications of break requests sent
	// from the client. The channel must handle break requests, or it will block
	// the request handling loop. Registering nil will unregister the channel.
	// During the time that no channel is registered, breaks are ignored.
	Break(c chan<- bool)
}

// maxSigBufSize is how many signals will be buffered
// when there is no signal channel specified
const maxSigBufSize = 128

func FishlerSessionHandler(srv *glssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx glssh.Context) {
	ch, reqs, err := newChan.Accept()
	if err != nil {
		// TODO: trigger event callback
		return
	}
	sess := &session{
		Channel:           ch,
		conn:              conn,
		handler:           srv.Handler,
		ptyCb:             srv.PtyCallback,
		sessReqCb:         srv.SessionRequestCallback,
		subsystemHandlers: srv.SubsystemHandlers,
		ctx:               ctx,
	}
	sess.handleRequests(reqs)
}

type session struct {
	sync.Mutex
	gossh.Channel
	conn              *gossh.ServerConn
	handler           glssh.Handler
	subsystemHandlers map[string]glssh.SubsystemHandler
	handled           bool
	exited            bool
	pty               *glssh.Pty
	winch             chan glssh.Window
	env               []string
	ptyCb             glssh.PtyCallback
	sessReqCb         glssh.SessionRequestCallback
	rawCmd            string
	subsystem         string
	ctx               glssh.Context
	sigCh             chan<- glssh.Signal
	sigBuf            []glssh.Signal
	breakCh           chan<- bool
}

func (sess *session) Write(p []byte) (n int, err error) {
	if sess.pty != nil {
		m := len(p)
		// normalize \n to \r\n when pty is accepted.
		// this is a hardcoded shortcut since we don't support terminal modes.
		p = bytes.Replace(p, []byte{'\n'}, []byte{'\r', '\n'}, -1)
		p = bytes.Replace(p, []byte{'\r', '\r', '\n'}, []byte{'\r', '\n'}, -1)
		n, err = sess.Channel.Write(p)
		if n > m {
			n = m
		}
		return
	}
	return sess.Channel.Write(p)
}

func (sess *session) PublicKey() glssh.PublicKey {
	sessionkey := sess.ctx.Value(glssh.ContextKeyPublicKey)
	if sessionkey == nil {
		return nil
	}
	return sessionkey.(glssh.PublicKey)
}

func (sess *session) Permissions() glssh.Permissions {
	// use context permissions because its properly
	// wrapped and easier to dereference
	perms := sess.ctx.Value(glssh.ContextKeyPermissions).(*glssh.Permissions)
	return *perms
}

func (sess *session) Context() glssh.Context {
	return sess.ctx
}

func (sess *session) Exit(code int) error {
	// _, _ = sess.Channel.Write([]byte("k, bye\n\n"))
	sess.Lock()
	defer sess.Unlock()
	if sess.exited {
		return errors.New("Session.Exit called multiple times")
	}
	sess.exited = true

	status := struct{ Status uint32 }{uint32(code)}
	_, err := sess.SendRequest("exit-status", false, gossh.Marshal(&status))
	if err != nil {
		return err
	}
	return sess.Close()
}

func (sess *session) User() string {
	return sess.conn.User()
}

func (sess *session) RemoteAddr() net.Addr {
	return sess.conn.RemoteAddr()
}

func (sess *session) LocalAddr() net.Addr {
	return sess.conn.LocalAddr()
}

func (sess *session) Environ() []string {
	return append([]string(nil), sess.env...)
}

func (sess *session) RawCommand() string {
	return sess.rawCmd
}

func (sess *session) Command() []string {
	cmd, _ := shlex.Split(sess.rawCmd, true)
	return append([]string(nil), cmd...)
}

func (sess *session) Subsystem() string {
	return sess.subsystem
}

func (sess *session) Pty() (glssh.Pty, <-chan glssh.Window, bool) {
	if sess.pty != nil {
		return *sess.pty, sess.winch, true
	}
	return glssh.Pty{}, sess.winch, false
}

func (sess *session) Signals(c chan<- glssh.Signal) {
	sess.Lock()
	defer sess.Unlock()
	sess.sigCh = c
	if len(sess.sigBuf) > 0 {
		go func() {
			for _, sig := range sess.sigBuf {
				sess.sigCh <- sig
			}
		}()
	}
}

func (sess *session) Break(c chan<- bool) {
	sess.Lock()
	defer sess.Unlock()
	sess.breakCh = c
}

func (sess *session) handleRequests(reqs <-chan *gossh.Request) {
	for req := range reqs {
		switch req.Type {
		case "shell", "exec":
			if sess.handled {
				err := req.Reply(false, nil)
				if err != nil {
					util.Logger.Error(err)
				}
				continue
			}

			if len(req.Payload) > 0 {
				var payload = struct{ Value string }{}
				err := gossh.Unmarshal(req.Payload, &payload)

				if err != nil {
					util.Logger.Error(err)
				}

				sess.rawCmd = payload.Value
			}

			// If there's a session policy callback, we need to confirm before
			// accepting the session.
			if sess.sessReqCb != nil && !sess.sessReqCb(sess, req.Type) {
				sess.rawCmd = ""
				err := req.Reply(false, nil)
				if err != nil {
					util.Logger.Error(err)
				}

				continue
			}

			sess.handled = true
			err := req.Reply(true, nil)
			if err != nil {
				util.Logger.Error(err)
			}

			go func() {
				sess.handler(sess)
			}()
		case "subsystem":
			if sess.handled {
				err := req.Reply(false, nil)
				if err != nil {
					util.Logger.Error(err)
				}

				continue
			}

			var payload = struct{ Value string }{}
			err := gossh.Unmarshal(req.Payload, &payload)
			if err != nil {
				util.Logger.Error(err)
			}

			sess.subsystem = payload.Value

			// If there's a session policy callback, we need to confirm before
			// accepting the session.
			if sess.sessReqCb != nil && !sess.sessReqCb(sess, req.Type) {
				sess.rawCmd = ""
				err = req.Reply(false, nil)
				if err != nil {
					util.Logger.Error(err)
				}

				continue
			}

			handler := sess.subsystemHandlers[payload.Value]
			if handler == nil {
				handler = sess.subsystemHandlers["default"]
			}
			if handler == nil {
				err = req.Reply(false, nil)
				if err != nil {
					util.Logger.Error(err)
				}

				continue
			}

			sess.handled = true
			err = req.Reply(true, nil)
			if err != nil {
				util.Logger.Error(err)
			}

			go func() {
				handler(sess)
				err = sess.Exit(0)
				if err != nil {
					util.Logger.Error(err)
				}

			}()
		case "env":
			if sess.handled {
				err := req.Reply(false, nil)
				if err != nil {
					util.Logger.Error(err)
				}

				continue
			}
			var kv struct{ Key, Value string }
			err := gossh.Unmarshal(req.Payload, &kv)
			if err != nil {
				util.Logger.Error(err)
			}

			sess.env = append(sess.env, fmt.Sprintf("%s=%s", kv.Key, kv.Value))
			err = req.Reply(true, nil)
			if err != nil {
				util.Logger.Error(err)
			}

		case "signal":
			var payload struct{ Signal string }
			err := gossh.Unmarshal(req.Payload, &payload)
			if err != nil {
				util.Logger.Error(err)
			}

			sess.Lock()
			if sess.sigCh != nil {
				sess.sigCh <- glssh.Signal(payload.Signal)
			} else {
				if len(sess.sigBuf) < maxSigBufSize {
					sess.sigBuf = append(sess.sigBuf, glssh.Signal(payload.Signal))
				}
			}
			sess.Unlock()
		case "pty-req":
			if sess.handled || sess.pty != nil {
				err := req.Reply(false, nil)
				if err != nil {
					util.Logger.Error(err)
				}

				continue
			}
			ptyReq, ok := parsePtyRequest(req.Payload)
			if !ok {
				err := req.Reply(false, nil)
				if err != nil {
					util.Logger.Error(err)
				}

				continue
			}
			if sess.ptyCb != nil {
				ok := sess.ptyCb(sess.ctx, ptyReq)
				if !ok {
					err := req.Reply(false, nil)
					if err != nil {
						util.Logger.Error(err)
					}

					continue
				}
			}
			sess.pty = &ptyReq
			sess.winch = make(chan glssh.Window, 1)
			sess.winch <- ptyReq.Window
			defer func() {
				// when reqs is closed
				close(sess.winch)
			}()
			err := req.Reply(ok, nil)
			if err != nil {
				util.Logger.Error(err)
			}

		case "window-change":
			if sess.pty == nil {
				err := req.Reply(false, nil)
				if err != nil {
					util.Logger.Error(err)
				}

				continue
			}
			win, ok := parseWinchRequest(req.Payload)
			if ok {
				sess.pty.Window = win
				sess.winch <- win
			}
			err := req.Reply(ok, nil)
			if err != nil {
				util.Logger.Error(err)
			}

		case agentRequestType:
			// TODO: option/callback to allow agent forwarding
			glssh.SetAgentRequested(sess.ctx)
			err := req.Reply(true, nil)
			if err != nil {
				util.Logger.Error(err)
			}

		case "break":
			ok := false
			sess.Lock()
			if sess.breakCh != nil {
				sess.breakCh <- true
				ok = true
			}
			err := req.Reply(ok, nil)
			if err != nil {
				util.Logger.Error(err)
			}

			sess.Unlock()
		default:
			err := req.Reply(false, nil)
			if err != nil {
				util.Logger.Error(err)
			}

		}
	}
}
