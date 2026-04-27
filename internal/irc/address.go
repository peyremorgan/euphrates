package irc

import (
	"net"
	"strings"
)

// HostOnly extracts the host portion of an IRC server address.
//
// Accepted input forms include:
//   - host
//   - host:port
//   - [ipv6]
//   - [ipv6]:port
//
// If parsing is ambiguous (for example an unbracketed IPv6 literal), the
// original input is returned unchanged.
func HostOnly(addr string) string {
	if addr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(addr, "["), "]")
	}
	if strings.Count(addr, ":") == 1 {
		if i := strings.LastIndexByte(addr, ':'); i > 0 {
			return addr[:i]
		}
	}
	return addr
}

func hostOnly(addr string) string {
	return HostOnly(addr)
}
