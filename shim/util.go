package shim

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"

	"github.com/ccoveille/go-safecast/v2"
	glssh "github.com/charmbracelet/ssh"
	"golang.org/x/crypto/ssh"
)

func generateSigner() (ssh.Signer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(key)
}

func parsePtyRequest(s []byte) (pty glssh.Pty, ok bool) {
	term, s, ok := parseString(s)
	if !ok {
		return
	}
	width32, s, ok := parseUint32(s)
	if !ok {
		return
	}
	height32, _, ok := parseUint32(s)
	if !ok {
		return
	}
	pty = glssh.Pty{
		Term: term,
		Window: glssh.Window{
			Width:  int(width32),
			Height: int(height32),
		},
	}
	return
}

func parseWindow(s []byte) (win glssh.Window, rem []byte, ok bool) {
	// 6.7.  Window Dimension Change Message
	// When the window (terminal) size changes on the client side, it MAY
	// send a message to the other side to inform it of the new dimensions.

	//   byte      SSH_MSG_CHANNEL_REQUEST
	//   uint32    recipient channel
	//   string    "window-change"
	//   boolean   FALSE
	//   uint32    terminal width, columns
	//   uint32    terminal height, rows
	//   uint32    terminal width, pixels
	//   uint32    terminal height, pixels
	wCols, rem, ok := parseUint32(s)
	if !ok {
		return
	}
	hRows, rem, ok := parseUint32(rem)
	if !ok {
		return
	}
	wPixels, rem, ok := parseUint32(rem)
	if !ok {
		return
	}
	hPixels, rem, ok := parseUint32(rem)
	if !ok {
		return
	}
	win = glssh.Window{
		Width:        int(wCols),
		Height:       int(hRows),
		WidthPixels:  int(wPixels),
		HeightPixels: int(hPixels),
	}
	return
}

func parseString(in []byte) (out string, rest []byte, ok bool) {
	if len(in) < 4 {
		return
	}
	length := binary.BigEndian.Uint32(in)
	maxLength, err := safecast.Convert[int](4 + length)
	if err != nil {
		maxLength = 4
	}
	if len(in) < maxLength {
		return
	}
	out = string(in[4 : 4+length])
	rest = in[4+length:]
	ok = true
	return
}

func parseUint32(in []byte) (uint32, []byte, bool) {
	if len(in) < 4 {
		return 0, nil, false
	}
	return binary.BigEndian.Uint32(in), in[4:], true
}
