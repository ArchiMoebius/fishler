package util

import (
	"encoding/binary"
	"fmt"
	"net"

	eventssh "github.com/ArchiMoebius/uplink/pkg/gen/v1"
	"github.com/ccoveille/go-safecast/v2"
)

// ParseNetAddr parses a net.Addr and populates the SSHConnectionEvent with
// the appropriate source IP and port fields
func ParseNetAddr(addr net.Addr, event *eventssh.SSHConnectionEvent) error {
	// Parse the address - handles both TCP and other address types
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		// Try to parse as string if not a TCPAddr
		host, port, err := net.SplitHostPort(addr.String())
		if err != nil {
			return fmt.Errorf("failed to parse address: %w", err)
		}

		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("invalid IP address: %s", host)
		}

		tcpAddr = &net.TCPAddr{
			IP:   ip,
			Port: parsePort(port),
		}
	}

	sourcePort, err := safecast.Convert[uint32](tcpAddr.Port)
	if err != nil {
		sourcePort = 55555
	}

	event.SourcePort = sourcePort

	// Determine if IPv4 or IPv6 and set appropriate field
	if ipv4 := tcpAddr.IP.To4(); ipv4 != nil {
		// IPv4 address - convert to fixed32
		event.SourceIp = &eventssh.SSHConnectionEvent_SourceIpv4{
			SourceIpv4: binary.BigEndian.Uint32(ipv4),
		}
	} else {
		// IPv6 address - use 16 bytes
		ipv6 := tcpAddr.IP.To16()
		if ipv6 == nil {
			return fmt.Errorf("invalid IP address")
		}
		event.SourceIp = &eventssh.SSHConnectionEvent_SourceIpv6{
			SourceIpv6: ipv6,
		}
	}

	return nil
}

// Helper function to parse port string to int
func parsePort(portStr string) int {
	var port int
	_, err := fmt.Sscanf(portStr, "%d", &port)

	if err != nil {
		return 55555
	}

	return port
}
