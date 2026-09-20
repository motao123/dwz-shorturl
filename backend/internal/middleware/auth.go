package middleware

import (
	"net/http"
	"strings"

	"dwz-admin/internal/pkg"

	"github.com/gin-gonic/gin"
)

// tokenBlacklistCheck reports whether a token jti has been revoked (logout).
// Registered once at startup by main; no-op when unset so the middleware keeps
// working without a blacklist backend.
var tokenBlacklistCheck func(jti string) bool

// SetTokenBlacklistCheck registers the Redis-backed revocation check used by
// the Auth middleware and the Logout handler.
func SetTokenBlacklistCheck(fn func(jti string) bool) {
	tokenBlacklistCheck = fn
}

// Auth is a JWT authentication middleware. It extracts the Bearer token from
// the Authorization header, validates it, and sets user context values.
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "missing authorization header")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "invalid authorization format")
			c.Abort()
			return
		}

		tokenStr := parts[1]
		claims, err := pkg.ParseToken(tokenStr)
		if err != nil {
			if err == pkg.ErrTokenExpired {
				pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "token expired")
			} else {
				pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "invalid token")
			}
			c.Abort()
			return
		}

		// #7: a refresh token must never authenticate an API call. Access and
		// refresh tokens were signed with the same key and had the same shape, and
		// nothing here looked at TokenType, so a refresh token — which lives 7 days,
		// is stored in localStorage by the frontend, and was not revoked by logout
		// or by a password change — worked as a full admin credential.
		//
		// Tokens minted before the field existed carry no type; they predate the
		// refresh flow and are treated as access tokens for backward compatibility.
		if claims.TokenType != "" && claims.TokenType != pkg.TokenTypeAccess {
			pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "invalid token type")
			c.Abort()
			return
		}

		// P1-4: reject tokens that were revoked via logout.
		if tokenBlacklistCheck != nil && claims.ID != "" && tokenBlacklistCheck(claims.ID) {
			pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "token has been revoked")
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("roles", claims.Roles)
		c.Set("token_jti", claims.ID)
		if claims.ExpiresAt != nil {
			c.Set("token_exp", claims.ExpiresAt.Time)
		}

		c.Next()
	}
}
