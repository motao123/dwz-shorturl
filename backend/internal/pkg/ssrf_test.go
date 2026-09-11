package pkg

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestNewSafeHTTPClientCapsRedirects(t *testing.T) {
	c := NewSafeHTTPClient(2 * time.Second)
	if c.CheckRedirect == nil {
		t.Fatal("expected a redirect policy to be installed")
	}
	// Build a chain of len(via) == maxSafeRedirects requests: must be rejected.
	via := make([]*http.Request, maxSafeRedirects)
	for i := range via {
		via[i] = &http.Request{URL: mustURL(t, "https://example.com/a")}
	}
	req := &http.Request{URL: mustURL(t, "https://example.com/b")}
	if err := c.CheckRedirect(req, via); err == nil {
		t.Fatalf("expected redirect chain of %d hops to be blocked", maxSafeRedirects)
	}
	// One hop fewer is allowed.
	if err := c.CheckRedirect(req, via[:maxSafeRedirects-1]); err != nil {
		t.Fatalf("expected %d hops to be allowed, got %v", maxSafeRedirects-1, err)
	}
}

func TestSafeRedirectPolicyBlocksInternalTargets(t *testing.T) {
	c := NewSafeHTTPClient(time.Second)
	cases := []struct {
		name string
		raw  string
	}{
		{"loopback", "http://127.0.0.1/admin"},
		{"private", "http://10.0.0.5/"},
		{"link-local metadata", "http://169.254.169.254/latest/meta-data/"},
		{"file scheme", "file:///etc/passwd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &http.Request{URL: mustURL(t, tc.raw)}
			if err := c.CheckRedirect(req, nil); err == nil {
				t.Fatalf("expected redirect to %s to be blocked", tc.raw)
			}
		})
	}
	// Public target on the first hop is fine.
	req := &http.Request{URL: mustURL(t, "https://93.184.216.34/")}
	if err := c.CheckRedirect(req, nil); err != nil {
		t.Fatalf("expected public redirect target to be allowed, got %v", err)
	}
}

func TestValidatePublicHTTPURLRejectsBadSchemes(t *testing.T) {
	if err := ValidatePublicHTTPURL(nil); err == nil {
		t.Fatal("nil URL should be rejected")
	}
	if err := ValidatePublicHTTPURL(mustURL(t, "ftp://example.com/x")); err == nil {
		t.Fatal("non-http(s) scheme should be rejected")
	}
	if err := ValidatePublicHTTPURL(mustURL(t, "http://[::1]/")); err == nil {
		t.Fatal("IPv6 loopback should be rejected")
	}
	if err := ValidatePublicHTTPURL(mustURL(t, "https://93.184.216.34/")); err != nil {
		t.Fatalf("public IP should be allowed, got %v", err)
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("bad test URL %q: %v", raw, err)
	}
	return u
}
