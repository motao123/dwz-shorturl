package middleware

import (
	"net/http"
	"net/url"
	"time"

	"dwz-admin/internal/pkg"

	"github.com/gin-gonic/gin"
)

// RateLimitByIP applies a per-IP rate limit using the shared Redis-backed
// limiter. Used for sensitive endpoints such as admin login to slow brute-force
// attempts. When rateLimiter is nil the limiter is a no-op.
func RateLimitByIP(rateLimiter *pkg.RateLimiter, prefix string, max int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rateLimiter == nil {
			c.Next()
			return
		}
		ip := c.ClientIP()
		allowed, err := rateLimiter.Allow(c.Request.Context(), prefix+":"+ip, max, window)
		if err != nil || !allowed {
			if err != nil {
				// limiter unavailable: fail open rather than block the whole site
				c.Next()
				return
			}
			pkg.Fail(c, http.StatusTooManyRequests, pkg.CodeRateLimit, "尝试过于频繁，请稍后再试")
			c.Abort()
			return
		}
		c.Next()
	}
}

// RateLimitKey is a small helper to build a namespaced limiter key from an
// arbitrary string (used by callers that need a stable key for a value).
func RateLimitKey(prefix, value string) string {
	return prefix + ":" + url.QueryEscape(value)
}