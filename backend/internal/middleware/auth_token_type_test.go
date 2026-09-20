package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"dwz-admin/internal/config"
	"dwz-admin/internal/pkg"

	"github.com/gin-gonic/gin"
)

var initOnce sync.Once

// setupTestConfig initialises the global config from a throwaway YAML file. The
// package-level config is a sync.Once, so this can only run once per process.
func setupTestConfig(t *testing.T) {
	t.Helper()
	initOnce.Do(func() {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		body := []byte("jwt:\n  secret: test-secret-for-token-type\n  access_expiry: 2h\n  refresh_expiry: 168h\n")
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if err := config.Init(path); err != nil {
			t.Fatalf("config.Init: %v", err)
		}
	})
	if config.Get() == nil {
		t.Fatal("config not initialised")
	}
}

// TestAuthMiddleware_RejectsRefreshToken is the regression test for #7.
//
// Access and refresh tokens are signed with the same key and have the same
// shape. The middleware only checked the signature, so a refresh token — which
// lives 7 days, is kept in localStorage by the frontend, and survived both
// logout and password changes — authenticated admin API calls just as well as
// an access token. The middleware must reject it.
func TestAuthMiddleware_RejectsRefreshToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestConfig(t)

	access, refresh, err := pkg.GenerateTokens(7, "admin", []string{"super_admin"})
	if err != nil {
		t.Fatal(err)
	}

	run := func(token string) (int, bool) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/admin/api/auth/me", nil)
		c.Request.Header.Set("Authorization", "Bearer "+token)
		Auth()(c)
		return w.Code, c.IsAborted()
	}

	if code, aborted := run(refresh); !aborted || code != http.StatusUnauthorized {
		t.Errorf("refresh token was accepted by the Auth middleware (status %d, aborted %v); "+
			"it must be rejected as a non-access credential", code, aborted)
	}
	if _, aborted := run(access); aborted {
		t.Error("access token must pass the Auth middleware")
	}
}

// TestRefreshTokenIdentity_RejectsAccessToken makes sure logout cannot be
// tricked into blacklisting the wrong credential class.
func TestRefreshTokenIdentity_RejectsAccessToken(t *testing.T) {
	setupTestConfig(t)

	access, refresh, err := pkg.GenerateTokens(1, "u", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pkg.RefreshTokenIdentity(access); err == nil {
		t.Error("RefreshTokenIdentity accepted an access token")
	}
	jti, ttl, err := pkg.RefreshTokenIdentity(refresh)
	if err != nil || jti == "" || ttl <= 0 {
		t.Errorf("RefreshTokenIdentity(refresh) = (%q, %v, %v), want a jti and positive ttl", jti, ttl, err)
	}
}
