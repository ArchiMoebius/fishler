package shim

import (
	// #nosec 501 -- MD5 is used for non-security purposes (e.g. hashing IDs)
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
)

// HASSHInfo contains the HASSH fingerprint and related data
type HASSHInfo struct {
	Hash            string
	Algorithms      string
	ClientID        string
	KexAlgorithms   []string
	Ciphers         []string
	MACs            []string
	CompressionAlgs []string
	RemoteAddr      net.Addr
}

// parseNameList reads an SSH name-list from the buffer
func parseNameList(data []byte, offset int) ([]string, int, error) {
	if offset+4 > len(data) {
		return nil, offset, fmt.Errorf("buffer too short")
	}

	length := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if offset+int(length) > len(data) {
		return nil, offset, fmt.Errorf("buffer too short")
	}

	if length == 0 {
		return []string{}, offset, nil
	}

	nameListStr := string(data[offset : offset+int(length)])
	offset += int(length)

	return strings.Split(nameListStr, ","), offset, nil
}

// parseKexInit extracts algorithm lists from SSH_MSG_KEXINIT packet
// RFC 4253 Section 7.1 - Algorithm Negotiation
//
// SSH_MSG_KEXINIT structure:
//
//	byte         SSH_MSG_KEXINIT (20)
//	byte[16]     cookie (random bytes)
//	name-list    kex_algorithms
//	name-list    server_host_key_algorithms
//	name-list    encryption_algorithms_client_to_server
//	name-list    encryption_algorithms_server_to_client
//	name-list    mac_algorithms_client_to_server
//	name-list    mac_algorithms_server_to_client
//	name-list    compression_algorithms_client_to_server
//	name-list    compression_algorithms_server_to_client
//	name-list    languages_client_to_server
//	name-list    languages_server_to_client
//	boolean      first_kex_packet_follows
//	uint32       0 (reserved for future extension)
func parseKexInit(payload []byte) (kex, ciphers, macs, compression []string, err error) {
	if len(payload) < 17 || payload[0] != 20 {
		return nil, nil, nil, nil, fmt.Errorf("invalid KEXINIT")
	}

	// Skip: 1 byte (message type) + 16 bytes (cookie) = 17 bytes
	offset := 17

	// kex_algorithms
	kex, offset, err = parseNameList(payload, offset)
	if err != nil {
		return
	}

	// server_host_key_algorithms (skip)
	_, offset, err = parseNameList(payload, offset)
	if err != nil {
		return
	}

	// encryption_algorithms_client_to_server
	ciphers, offset, err = parseNameList(payload, offset)
	if err != nil {
		return
	}

	// encryption_algorithms_server_to_client (skip)
	_, offset, err = parseNameList(payload, offset)
	if err != nil {
		return
	}

	// mac_algorithms_client_to_server
	macs, offset, err = parseNameList(payload, offset)
	if err != nil {
		return
	}

	// mac_algorithms_server_to_client (skip)
	_, offset, err = parseNameList(payload, offset)
	if err != nil {
		return
	}

	// compression_algorithms_client_to_server
	compression, offset, err = parseNameList(payload, offset)
	return
}

// calculateHASSH generates the HASSH fingerprint
// see:
//   - https://engineering.salesforce.com/open-sourcing-hassh-abed3ae5044c/
//   - https://github.com/corelight/hassh
//   - https://cyberchef.org/security-hassh-fingerprint.html
func calculateHASSH(kex, ciphers, macs, compression []string) string {
	hasshAlgorithms := fmt.Sprintf("%s;%s;%s;%s",
		strings.Join(kex, ","),
		strings.Join(ciphers, ","),
		strings.Join(macs, ","),
		strings.Join(compression, ","))

	fmt.Println(hasshAlgorithms)

	// #nosec G401 -- MD5 is used for non-security purposes (e.g. hashing IDs)
	hassh := md5.Sum([]byte(hasshAlgorithms))

	return hex.EncodeToString(hassh[:])
}

// HASSHConnectionWrapper wraps a connection to capture SSH handshake
type HASSHConnectionWrapper struct {
	net.Conn
	OnCapture   func(*HASSHInfo)
	Buffer      []byte
	Captured    bool
	VersionRead bool
	ClientID    string
}

func (c *HASSHConnectionWrapper) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)

	if n > 0 && !c.Captured {
		c.Buffer = append(c.Buffer, b[:n]...)
		c.parse()
	}

	return n, err
}

func (c *HASSHConnectionWrapper) parse() {
	if !c.VersionRead {
		data := string(c.Buffer)

		if idx := strings.Index(data, "SSH-"); idx >= 0 {
			if endIdx := strings.Index(data[idx:], "\r\n"); endIdx > 0 {
				c.ClientID = data[idx : idx+endIdx]
				c.VersionRead = true
				c.Buffer = c.Buffer[idx+endIdx+2:]
			}
		}
	}

	if c.VersionRead && !c.Captured && len(c.Buffer) >= 5 {
		packetLen := uint32(c.Buffer[0])<<24 | uint32(c.Buffer[1])<<16 |
			uint32(c.Buffer[2])<<8 | uint32(c.Buffer[3])

		if packetLen < 1 || packetLen > 35000 || len(c.Buffer) < int(4+packetLen) {
			return
		}

		paddingLen := int(c.Buffer[4])
		payloadLen := int(packetLen) - paddingLen - 1

		if payloadLen <= 0 || 5+payloadLen > len(c.Buffer) {
			return
		}

		payload := c.Buffer[5 : 5+payloadLen]

		if len(payload) > 0 && payload[0] == 20 {
			kex, ciphers, macs, compression, err := parseKexInit(payload)

			if err == nil {
				info := &HASSHInfo{
					Hash:            calculateHASSH(kex, ciphers, macs, compression),
					ClientID:        c.ClientID,
					KexAlgorithms:   kex,
					Ciphers:         ciphers,
					MACs:            macs,
					CompressionAlgs: compression,
					RemoteAddr:      c.Conn.RemoteAddr(),
					Algorithms: fmt.Sprintf("%s;%s;%s;%s",
						strings.Join(kex, ","),
						strings.Join(ciphers, ","),
						strings.Join(macs, ","),
						strings.Join(compression, ",")),
				}

				c.Captured = true

				if c.OnCapture != nil {
					c.OnCapture(info)
				}
			}
		}
	}
}
