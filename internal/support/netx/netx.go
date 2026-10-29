// Package netx holds the small network checks shared by the front ends.
package netx

import (
	"net"
	"net/url"
	"strings"
)

// Loopback reports whether target names this host's loopback interface.
// It accepts a host, a host:port pair, or an http(s) URL. An empty host,
// an unknown name, and a URL with another scheme are not loopback.
func Loopback(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	if strings.Contains(target, "://") {
		u, err := url.Parse(target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return false
		}
		return Loopback(u.Hostname())
	}
	if host, _, err := net.SplitHostPort(target); err == nil {
		target = host
	}
	target = strings.Trim(target, "[]")
	if strings.EqualFold(target, "localhost") {
		return true
	}
	ip := net.ParseIP(target)
	return ip != nil && ip.IsLoopback()
}
