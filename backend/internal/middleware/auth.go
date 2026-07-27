package middleware

import (
	"net/http"
	"strings"

	"dwz-admin/internal/pkg"

	"github.com/gin-gonic/gin"
)

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

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("roles", claims.Roles)
		c.Set("token_jti", claims.ID)

		c.Next()
	}
}
