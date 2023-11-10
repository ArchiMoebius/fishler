package util

import (
	"fmt"
	"net"
	"strings"
	"time"
)

func GetSessionFileName(containerID string, sessionRemoteAddress net.Addr) string {
	datetime := strings.ReplaceAll(strings.ReplaceAll(fmt.Sprintf("%v", time.Now().Format(time.RFC3339)), ":", ""), "-", "")
	ipaddr := strings.Split(strings.Replace(sessionRemoteAddress.String(), ".", "-", -1), ":")[0]
	return fmt.Sprintf("/tmp/fishler_%s_%s_%s.log", containerID, datetime, ipaddr)
}
