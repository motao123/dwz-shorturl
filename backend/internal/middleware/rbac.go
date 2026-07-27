package middleware

import (
	"net/http"

	"dwz-admin/internal/pkg"

	"github.com/gin-gonic/gin"
)

// RequirePermission returns a middleware that checks whether the current user
// has the specified permission. The user's permissions must be set in the
// gin.Context under the key "permissions" (set during auth or a preceding middleware).
func RequirePermission(resource, action string) gin.HandlerFunc {
	required := resource + "." + action
	return func(c *gin.Context) {
		// Super admin bypass
		roles, _ := c.Get("roles")
		if roleSlice, ok := roles.([]string); ok {
			for _, r := range roleSlice {
				if r == "super_admin" {
					c.Next()
					return
				}
			}
		}

		perms, exists := c.Get("permissions")
		if !exists {
			pkg.Fail(c, http.StatusForbidden, pkg.CodeForbidden, "permission denied")
			c.Abort()
			return
		}

		permSlice, ok := perms.([]string)
		if !ok {
			pkg.Fail(c, http.StatusForbidden, pkg.CodeForbidden, "permission denied")
			c.Abort()
			return
		}

		for _, p := range permSlice {
			if p == required || p == "*" {
				c.Next()
				return
			}
		}

		pkg.Fail(c, http.StatusForbidden, pkg.CodeForbidden, "permission denied")
		c.Abort()
	}
}

// LoadPermissions is a middleware that loads user permissions from the repository
// and sets them in the context. It should be placed after Auth middleware.
func LoadPermissions(getPerms func(userID uint64) ([]string, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDVal, exists := c.Get("user_id")
		if !exists {
			c.Next()
			return
		}

		userID, ok := userIDVal.(uint64)
		if !ok {
			c.Next()
			return
		}

		perms, err := getPerms(userID)
		if err != nil {
			c.Set("permissions", []string{})
			c.Next()
			return
		}

		c.Set("permissions", perms)
		c.Next()
	}
}
