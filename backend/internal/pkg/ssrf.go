package pkg

import (
	"errors"
	"net"
	"net/http"
	"syscall"
	"time"
)

// NewSafeHTTPClient returns an *http.Client whose transport refuses to dial
// loopback/private/link-local addresses at connection time. It complements the
// DNS-level check in service.validateURL: even if a hostname resolves to a
// public IP at validation time and is later rebound to an internal address
// (DNS rebinding), the dial is rejected. Use it for every server-side fetch —
// link health checks, title scraping, and similar.
func NewSafeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control:   blockPrivateDial,
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:     dialer.DialContext,
			DisableKeepAlives: true,
		},
	}
}

func blockPrivateDial(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return errors.New("invalid IP address")
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return errors.New("connection to private network address blocked")
	}
	return nil
}
