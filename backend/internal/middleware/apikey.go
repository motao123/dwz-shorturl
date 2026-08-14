package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"

	"github.com/gin-gonic/gin"
)

// RequireApiKey authenticates API-key-driven public endpoints. The key is read
// from the X-API-Key header (or Authorization: Bearer <key>), hashed with
// SHA-256, and looked up in the api_keys table. Disabled/expired keys are
// rejected. A per-key rate limit is applied when rateLimiter is provided.
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
			pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "missing api key")
			c.Abort()
			return
		}

		hash := sha256.Sum256([]byte(key))
		keyHash := hex.EncodeToString(hash[:])
		record, err := apiKeyRepo.FindByHash(keyHash)
		if err != nil {
			pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "invalid api key")
			c.Abort()
			return
		}
		if record.Status != 1 {
			pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "api key revoked")
			c.Abort()
			return
		}
		if record.ExpiresAt != nil && record.ExpiresAt.Before(time.Now()) {
			pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "api key expired")
			c.Abort()
			return
		}

		if rateLimiter != nil {
			allowed, err := rateLimiter.Allow(c.Request.Context(), "apikey:"+record.KeyPrefix, record.RateLimit, time.Minute)
			if err != nil || !allowed {
				pkg.Fail(c, http.StatusTooManyRequests, pkg.CodeRateLimit, "rate limit exceeded")
				c.Abort()
				return
			}
		}

		c.Set("api_key_id", record.ID)
		_ = apiKeyRepo.UpdateLastUsed(record.ID)
		c.Next()
	}
}