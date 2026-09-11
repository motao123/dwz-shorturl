package pkg

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSafeHTTPClientCapsRedirects verifies that an endless 302 chain is stopped
// after defaultMaxRedirects hops instead of running until the timeout.
func TestSafeHTTPClientCapsRedirects(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	defer srv.Close()

	client := newSafeHTTPClient(2*time.Second, defaultMaxRedirects)
	// Override the dial guard: httptest binds to a loopback address, which the
	// production dialer intentionally blocks.
	client.Transport = srv.Client().Transport

	_, err := client.Get(srv.URL)
	if err == nil {
		t.Fatal("expected an error after exceeding the redirect limit")
	}
	if !strings.Contains(err.Error(), "too many redirects") {
		t.Fatalf("expected redirect-limit error, got %v", err)
	}
	// The initial request plus one per allowed hop.
	if want := defaultMaxRedirects; hits != want {
		t.Fatalf("expected %d requests before aborting, got %d", want, hits)
	}
}
