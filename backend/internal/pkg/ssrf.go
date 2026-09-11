package pkg

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// defaultMaxRedirects bounds how many redirect hops a server-side fetch may follow.
// Without a limit a malicious target can chain redirects until the client
// timeout, tying up the connection/worker for the full timeout window.
const defaultMaxRedirects = 3

// NewSafeHTTPClient returns an *http.Client whose transport refuses to dial
// loopback/private/link-local addresses at connection time. It complements the
// DNS-level check in service.validateURL: even if a hostname resolves to a
// public IP at validation time and is later rebound to an internal address
// (DNS rebinding), the dial is rejected. Use it for every server-side fetch —
// link health checks, title scraping, and similar.
//
// Redirects are capped at defaultMaxRedirects hops and every hop must use
// http(s) and a public host: a redirect target is a fresh request that would
// otherwise bypass the pre-flight ValidateURL / validateURL check.
func NewSafeHTTPClient(timeout time.Duration) *http.Client {
	return newSafeHTTPClient(timeout, defaultMaxRedirects)
}

// newSafeHTTPClient builds the client with an explicit redirect budget. The
// parameter exists so tests can exercise the hop cap without depending on the
// production default.
func newSafeHTTPClient(timeout time.Duration, maxRedirects int) *http.Client {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control:   blockPrivateDial,
	}
	max := maxRedirects
	if max <= 0 {
		max = defaultMaxRedirects
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:       dialer.DialContext,
			DisableKeepAlives: true,
		},
		CheckRedirect: redirectPolicy(max),
	}
}

// redirectPolicy enforces the hop limit and re-validates each redirect target
// (scheme + host) so an internal address cannot be reached via 302.
func redirectPolicy(maxRedirects int) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after too many redirects (%d)", maxRedirects)
		}
		if err := ValidatePublicHTTPURL(req.URL); err != nil {
			return err
		}
		// Keep the caller's User-Agent/Accept headers on every hop (Go drops
		// them for cross-host redirects otherwise). via may be empty (no
		// preceding request), in which case the new request already carries its
		// own headers.
		if len(via) > 0 {
			for _, h := range []string{"User-Agent", "Accept"} {
				if v := via[0].Header.Get(h); v != "" {
					req.Header.Set(h, v)
				}
			}
		}
		return nil
	}
}

// ValidatePublicHTTPURL rejects non-http(s) schemes and hosts that resolve to
// loopback/private/link-local addresses. It is the redirect-time counterpart of
// service.ValidateURL (kept here to avoid an import cycle).
func ValidatePublicHTTPURL(u *url.URL) error {
	if u == nil {
		return errors.New("empty redirect target")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("redirect to unsupported scheme %q blocked", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("redirect target has no host")
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return errors.New("redirect to private network address blocked")
		}
		return nil
	}
	// Hostname: resolve and reject when any address is private. The dialer
	// re-checks at connect time, so a rebind after this check is still blocked.
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("redirect target resolution failed: %w", err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return errors.New("redirect to private network address blocked")
		}
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
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
	if isBlockedIP(ip) {
		return errors.New("connection to private network address blocked")
	}
	return nil
}
