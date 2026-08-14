package middleware

import (
	"net/http"
	"strings"

	"dwz-admin/internal/config"

	"github.com/gin-gonic/gin"
)

// CORS returns a middleware that handles Cross-Origin Resource Sharing.
//
// Only origins listed in cfg.CORS.AllowedOrigins are allowed to make
// credentialed cross-origin requests (Access-Control-Allow-Credentials is only
// set for whitelisted origins). Any other origin is served without CORS headers
// so the browser blocks the request. If the whitelist is empty, all cross-origin
// requests are refused (same-origin only).
func CORS(cfg *config.Config) gin.HandlerFunc {
	allowed := make(map[string]bool, len(cfg.CORS.AllowedOrigins))
	for _, o := range cfg.CORS.AllowedOrigins {
		allowed[strings.TrimRight(o, "/")] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		normalized := strings.TrimRight(origin, "/")
		if !allowed[normalized] {
			// Not whitelisted: set no CORS headers and let the request through.
			// The browser enforces the same-origin policy, so an unlisted origin
			// cannot read the response; hard-rejecting here would also break
			// legitimate non-browser clients (curl/scripts) that send Origin.
			c.Next()
			return
		}

		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With, X-API-Key, X-Member-Token")
		c.Header("Access-Control-Expose-Headers", "Content-Length, Content-Disposition")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}