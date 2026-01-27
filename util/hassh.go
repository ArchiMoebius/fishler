package util

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"net"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

// Conn wraps a net.Conn and parses SSH packet framing to extract SSH_MSG_KEXINIT
type Conn struct {
	net.Conn
	buf     bytes.Buffer
	once    sync.Once
	OnHASSH func(string)
}

// Wrap wraps an existing net.Conn so that the first SSH_MSG_KEXINIT
// from the client is parsed and a HASSH fingerprint string is sent
// to the OnHASSH callback.
func Wrap(c net.Conn, onHASSH func(string)) net.Conn {
	return &Conn{
		Conn:    c,
		OnHASSH: onHASSH,
	}
}

func (c *Conn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 && c.OnHASSH != nil {
		c.buf.Write(p[:n])
		c.tryParse()
	}
	return n, err
}

func (c *Conn) tryParse() {
	c.once.Do(func() {
		b := c.buf.Bytes()

		for len(b) >= 5 {
			pktLen := int(binary.BigEndian.Uint32(b[0:4]))
			if len(b) < pktLen+4 {
				return
			}

			paddingLen := int(b[4])
			payloadStart := 5
			payloadEnd := 4 + pktLen - paddingLen

			if payloadEnd <= payloadStart {
				return
			}

			payload := b[payloadStart:payloadEnd]
			if len(payload) > 0 && payload[0] == 20 { // SSH_MSG_KEXINIT
				fp := parseHASSH(payload[1:])
				if c.OnHASSH != nil {
					c.OnHASSH(fp)
				}
				c.OnHASSH = nil
				return
			}

			b = b[4+pktLen:]
		}
	})
}

func parseHASSH(b []byte) string {
	if len(b) < 16 {
		return ""
	}
	b = b[16:]

	kex, b := readNameList(b)
	enc, b := readNameList(b)
	mac, b := readNameList(b)
	comp, _ := readNameList(b)

	raw := strings.Join([]string{
		strings.Join(kex, ","),
		strings.Join(enc, ","),
		strings.Join(mac, ","),
		strings.Join(comp, ","),
	}, ";")

	sum := md5.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func readNameList(b []byte) ([]string, []byte) {
	if len(b) < 4 {
		return nil, nil
	}
	l := int(binary.BigEndian.Uint32(b[:4]))
	if len(b) < 4+l {
		return nil, nil
	}
	raw := string(b[4 : 4+l])
	rest := b[4+l:]
	if raw == "" {
		return nil, rest
	}
	return strings.Split(raw, ","), rest
}

type HASSHListener struct {
	net.Listener
}

func (hl *HASSHListener) Accept() (net.Conn, error) {
	conn, err := hl.Listener.Accept()
	if err != nil {
		return nil, err
	}
	var hassh string
	wrapped := Wrap(conn, func(fp string) {
		hassh = fp
	})

	Logger.WithFields(logrus.Fields{
		"address":      conn.RemoteAddr().String(),
		"client_hassh": hassh,
	}).Info("ssh connection fingerprint")

	return wrapped, nil
}
