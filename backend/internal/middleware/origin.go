package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"dwz-admin/internal/pkg"

	"github.com/gin-gonic/gin"
)

// RequireSameSiteOrigin blocks cross-site writes on cookie-authenticated APIs.
//
// MemberAuth falls back to the member JWT cookie when the Authorization header
// is absent, so a request forged by another page can carry the victim's
// credential as long as the browser attaches that cookie (#36). SameSite=Lax
// stops top-level cross-site POSTs in current browsers, but the guarantee is
// only as good as the engine: older kernels and embedded WebViews have shipped
// without "Lax as default", and those sessions are exactly the ones where a
// silent link edit or delete is most useful to an attacker.
//
// Origin (or, failing that, Referer) is the cheap, stateless check: browsers
// always send Origin on POST/PUT/PATCH/DELETE, including for CORS-safelisted
// simple requests such as a text/plain fetch, so requiring it to be our own
// host closes the form/one-click path without a token round-trip.
//
// Requests with neither header are allowed through: those are non-browser
// clients (curl, server-to-server, health probes) that a cross-site page
// cannot produce.
func RequireSameSiteOrigin(siteURL func() string) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}

		raw := strings.TrimSpace(c.GetHeader("Origin"))
		if raw == "" {
			raw = strings.TrimSpace(c.GetHeader("Referer"))
		}
		if raw == "" {
			c.Next()
			return
		}

		requester, ok := hostOf(raw)
		if !ok {
			pkg.Fail(c, http.StatusForbidden, pkg.CodeForbidden, "来源无法识别，请刷新页面后重试")
			c.Abort()
			return
		}

		if strings.EqualFold(requester, normalizeHost(c.Request.Host)) {
			c.Next()
			return
		}
		// A deployment may be reached under a different public origin than the
		// Host header (CDN in front, custom domain pool): honour the configured
		// site origin as well.
		if siteURL != nil {
			if configured, ok := hostOf(strings.TrimSpace(siteURL())); ok && strings.EqualFold(requester, configured) {
				c.Next()
				return
			}
		}

		pkg.Fail(c, http.StatusForbidden, pkg.CodeForbidden, "跨站请求已被拒绝，请从本站页面操作")
		c.Abort()
	}
}

// hostOf extracts a comparable host[:port] from an absolute URL, tolerating both
// a bare origin ("https://x.test") and a full referer ("https://x.test/a/b").
func hostOf(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	if !strings.Contains(raw, "://") {
		// A bare host[:port] is accepted where it is unambiguous.
		return normalizeHost(raw), true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", false
	}
	return normalizeHost(u.Host), true
}

func normalizeHost(h string) string {
	h = strings.TrimSpace(strings.ToLower(h))
	// IPv6 literals arrive bracketed; keep the bracket form so a later equality
	// check against c.Request.Host stays meaningful.
	if strings.HasPrefix(h, "[") {
		return h
	}
	return strings.TrimSuffix(h, ".")
}
