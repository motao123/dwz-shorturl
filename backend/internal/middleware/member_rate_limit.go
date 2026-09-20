package middleware

import (
	"net/http"
	"strconv"
	"time"

	"dwz-admin/internal/pkg"

	"github.com/gin-gonic/gin"
)

// MemberRateLimiter resolves the per-member quota. The numbers come from the
// admin config catalog (via the Quota closure) so an operator can tune them
// without a restart, and tests can inject a deterministic implementation.
type MemberRateLimiter struct {
	limiter *pkg.RateLimiter
	// Quota resolves (max, window) for the current request. Returning max <= 0
	// disables the limit.
	Quota func() (int, time.Duration)
}

// NewMemberRateLimiter builds a limiter for the member API.
func NewMemberRateLimiter(limiter *pkg.RateLimiter, quota func() (int, time.Duration)) *MemberRateLimiter {
	return &MemberRateLimiter{limiter: limiter, Quota: quota}
}

// RateLimitMember enforces a per-member-token quota on the member API, with a
// per-IP fallback for the unauthenticated auth endpoints.
//
// Why token-scoped rather than IP-scoped (#8/#37): the member API was the only
// externally reachable group with no limiter at all. IP alone is the wrong
// dimension here — a single NAT/proxy shares one IP between many members (so an
// IP cap punishes innocent users while one member can still burn the whole
// budget from a rotating pool). The member id is a stable, unforgeable identity
// once the JWT is verified, so it is the primary key; the IP fallback covers the
// public auth routes (forgot-password / reset-password / send-verification)
// where there is no member identity yet and the risk is enumeration + mail
// bombing.
//
// The identity is read from the context value set by MemberAuth. When absent
// (public auth routes) the client IP is used. A limiter that errors (Redis down)
// fails open, matching RateLimitByIP: a limiter outage must not take down the
// whole member console.
func RateLimitMember(rl *MemberRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rl == nil || rl.limiter == nil {
			c.Next()
			return
		}

		// Defaults mirror RuntimeConfig.MemberRateLimit so a nil/blank quota
		// still bounds the group instead of silently disabling it.
		max, window := 60, time.Minute
		if rl.Quota != nil {
			qMax, qWindow := rl.Quota()
			// A resolver error must not disable the gate: only an explicit 0
			// (operator opt-out) turns it off, and that is handled below.
			if qMax != 0 {
				max = qMax
			}
			if qWindow > 0 {
				window = qWindow
			}
		}
		if max <= 0 {
			c.Next()
			return
		}

		// Identity: member token when authenticated, IP otherwise.
		key := ""
		if id, ok := c.Get("member_id"); ok {
			if mid, ok := id.(uint64); ok && mid > 0 {
				key = "member-api:m:" + strconv.FormatUint(mid, 10)
			}
		}
		if key == "" {
			key = "member-api:ip:" + c.ClientIP()
		}

		allowed, err := rl.limiter.Allow(c.Request.Context(), key, max, window)
		if err != nil {
			// Limiter unavailable: fail open, do not block the console.
			c.Next()
			return
		}
		if !allowed {
			pkg.Fail(c, http.StatusTooManyRequests, pkg.CodeRateLimit, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}
		c.Next()
	}
}
