package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"

	"github.com/gin-gonic/gin"
)

// ApiKeyScope is the permission string an API key must carry to call a given
// public endpoint. It follows the same `resource.action` convention as the
// admin RBAC permissions (middleware.RequirePermission), so an operator who
// understands one already understands the other.
//
// The zero value means "no scope required" — used only where there is no
// meaningful scope to check.
type ApiKeyScope string

const (
	// ScopeShortURLsCreate is required to create a single short URL.
	ScopeShortURLsCreate ApiKeyScope = "short_urls.create"
	// ScopeShortURLsBatch is required to create short URLs in bulk.
	ScopeShortURLsBatch ApiKeyScope = "short_urls.batch"
)

// wildcardScope grants every scope, mirroring the RBAC middleware's "*".
const wildcardScope = "*"

// RequireApiKey authenticates API-key-driven public endpoints. The key is read
// from the X-API-Key header (or Authorization: Bearer <key>), hashed with
// SHA-256, and looked up in the api_keys table. Disabled/expired keys are
// rejected. A per-key rate limit is applied when rateLimiter is provided.
//
// The granted scope(s) are stored in the context under "api_key_permissions"
// so handlers or a preceding ScopeApiKey middleware can enforce them.
func RequireApiKey(apiKeyRepo repository.ApiKeyRepo, rateLimiter *pkg.RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")
		if key == "" {
			auth := c.GetHeader("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				key = strings.TrimPrefix(auth, "Bearer ")
			}
		}
		if key == "" {
			pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "缺少 API Key")
			c.Abort()
			return
		}

		hash := sha256.Sum256([]byte(key))
		keyHash := hex.EncodeToString(hash[:])
		record, err := apiKeyRepo.FindByHash(keyHash)
		if err != nil {
			pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "API Key 无效")
			c.Abort()
			return
		}
		if record.Status != 1 {
			pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "API Key 已吊销")
			c.Abort()
			return
		}
		if record.ExpiresAt != nil && record.ExpiresAt.Before(time.Now()) {
			pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "API Key 已过期")
			c.Abort()
			return
		}

		if rateLimiter != nil {
			allowed, err := rateLimiter.Allow(c.Request.Context(), "apikey:"+record.KeyPrefix, record.RateLimit, time.Minute)
			if err != nil {
				// Limiter unavailable (Redis down): fail OPEN, matching every other
				// limiter in this codebase (RateLimitByIP, RateLimitMember).
				//
				// This was previously fail-closed (`err != nil || !allowed`), which
				// made a Redis outage take the whole public API hard-down: a limiter
				// is a protection against abuse, not an authentication factor, so its
				// failure must not remove a working feature. The asymmetry mattered
				// because the two sibling limiters on the same code path already
				// failed open — the strictest behavior sat on the least privileged
				// entry point, which is exactly backwards.
				//
				// Note this only relaxes the *rate* gate. Authentication and scope
				// checks below still run, so an outage never grants access that the
				// key does not already hold.
			} else if !allowed {
				pkg.Fail(c, http.StatusTooManyRequests, pkg.CodeRateLimit, "请求过于频繁，请稍后再试")
				c.Abort()
				return
			}
		}

		c.Set("api_key_id", record.ID)
		c.Set("api_key_permissions", ParseApiKeyPermissions(record.Permissions))
		_ = apiKeyRepo.UpdateLastUsed(record.ID)
		c.Next()
	}
}

// ParseApiKeyPermissions decodes the api_keys.permissions JSON column into a
// permission slice. Anything unparseable yields an EMPTY set (deny), never a
// wildcard: a malformed value must not be the thing that grants the most.
//
// A NULL/empty column is treated as empty for the same reason. Callers that
// want "no scopes required" must say so explicitly rather than relying on a
// blank value.
func ParseApiKeyPermissions(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return []string{}
	}
	var perms []string
	if err := json.Unmarshal([]byte(trimmed), &perms); err != nil {
		return []string{}
	}
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ScopeApiKey rejects the request unless the authenticated API key holds the
// required scope. It must be placed after RequireApiKey, which is what puts the
// decoded permissions into the context.
//
// Why this exists: the api_keys table has carried a `permissions` JSON column
// since it was introduced, the admin UI sends it on create, and the create
// service writes it — but nothing ever read it. A key minted with
// `permissions: []` could therefore call every public endpoint. That is the
// same "control plane that lies" pattern as the runtime-config switches fixed
// earlier: the switch is displayed, persisted, and ignored.
//
// A missing context value is a programming error (middleware ordering), and is
// treated as deny so a future route that forgets RequireApiKey fails closed
// rather than open.
func ScopeApiKey(scope ApiKeyScope) gin.HandlerFunc {
	return func(c *gin.Context) {
		if scope == "" {
			c.Next()
			return
		}

		val, exists := c.Get("api_key_permissions")
		if !exists {
			pkg.Fail(c, http.StatusForbidden, pkg.CodeForbidden, "API Key 缺少权限声明")
			c.Abort()
			return
		}
		perms, ok := val.([]string)
		if !ok {
			pkg.Fail(c, http.StatusForbidden, pkg.CodeForbidden, "API Key 缺少权限声明")
			c.Abort()
			return
		}

		if apiKeyHasScope(perms, scope) {
			c.Next()
			return
		}
		pkg.Fail(c, http.StatusForbidden, pkg.CodeForbidden, "API Key 无此操作权限")
		c.Abort()
	}
}

// apiKeyHasScope reports whether the granted permission list satisfies scope.
// "*" grants everything; the batch scope is also satisfied by the single-create
// scope because batch creation is a superset operation the caller could already
// perform by looping the single endpoint. The reverse is NOT true.
func apiKeyHasScope(perms []string, scope ApiKeyScope) bool {
	for _, p := range perms {
		switch p {
		case wildcardScope:
			return true
		case string(scope):
			return true
		case string(ScopeShortURLsCreate):
			// Holding single-create implies batch-create (see doc comment).
			if scope == ScopeShortURLsBatch {
				return true
			}
		}
	}
	return false
}
