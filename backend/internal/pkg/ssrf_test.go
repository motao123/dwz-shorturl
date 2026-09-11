package pkg

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestSafeRedirectPolicyBlocksInternalTargets covers the redirect-time guard on
// NewSafeHTTPClient: every hop must use http(s) and a public host.
func TestSafeRedirectPolicyBlocksInternalTargets(t *testing.T) {
	c := NewSafeHTTPClient(time.Second)
	if c.CheckRedirect == nil {
		t.Fatal("expected a redirect policy to be installed")
	}
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

// TestSafeHTTPClientCapsRedirects verifies the hop cap is enforced: once the
// number of followed redirects reaches the budget the chain is aborted, so an
// endless 302 loop cannot consume the whole request timeout.
//
// The cap is exercised through the installed CheckRedirect policy rather than
// an httptest chain: httptest always binds to a loopback address, and the
// production policy (correctly) rejects such redirect targets first, so the
// limit error would never be the one surfaced.
func TestSafeHTTPClientCapsRedirects(t *testing.T) {
	client := newSafeHTTPClient(2*time.Second, defaultMaxRedirects)
	if client.CheckRedirect == nil {
		t.Fatal("expected a redirect policy to be installed")
	}

	via := make([]*http.Request, 0, defaultMaxRedirects)
	for i := 0; i < defaultMaxRedirects; i++ {
		via = append(via, &http.Request{URL: mustURL(t, "https://93.184.216.34/next")})
	}
	// len(via) already at the budget: the next hop must be refused.
	req := &http.Request{URL: mustURL(t, "https://93.184.216.34/next")}
	err := client.CheckRedirect(req, via)
	if err == nil {
		t.Fatal("expected the redirect chain to be capped")
	}
	if !strings.Contains(err.Error(), "too many redirects") {
		t.Fatalf("expected redirect-limit error, got %v", err)
	}
	// One hop fewer than the budget is still allowed.
	if err := client.CheckRedirect(req, via[:defaultMaxRedirects-1]); err != nil {
		t.Fatalf("expected %d hops to be allowed, got %v", defaultMaxRedirects-1, err)
	}
}
