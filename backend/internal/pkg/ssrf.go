package pkg

import (
	"errors"
	"net"
	"net/http"
	"strings"
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
	return newSafeHTTPClient(timeout, defaultMaxRedirects)
}

// defaultMaxRedirects bounds how many hops a server-side fetch may follow. An
// unbounded chain (302 -> 302 -> ...) would otherwise consume the whole timeout
// budget and a connection slot per attempt.
const defaultMaxRedirects = 3

func newSafeHTTPClient(timeout time.Duration, maxRedirects int) *http.Client {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control:   blockPrivateDial,
	}
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:       dialer.DialContext,
			DisableKeepAlives: true,
		},
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return errors.New("stopped after too many redirects")
		}
		// Re-validate every hop: a public URL may redirect to an internal one.
		// The dialer already blocks private IPs at connect time (which covers
		// DNS rebinding); this guard additionally rejects non-HTTP schemes so a
		// redirect cannot pivot to file:// or similar.
		scheme := strings.ToLower(req.URL.Scheme)
		if scheme != "http" && scheme != "https" {
			return errors.New("redirect to unsupported scheme blocked")
		}
		if req.URL.Hostname() == "" {
			return errors.New("redirect without host blocked")
		}
		return nil
	}
	return client
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
