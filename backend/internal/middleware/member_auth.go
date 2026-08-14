package middleware

import (
	"net/http"
	"strings"

	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"

	"github.com/gin-gonic/gin"
)

// MemberAuth authenticates a public member via a member JWT (issued by the PHP
// frontend on login). The token is read from the Authorization: Bearer header
// or the X-Member-Token header. When getMember is provided the member's
// token_version is verified so logged-out JWTs are rejected.
func MemberAuth(getMember func(uint64) (*model.Member, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := c.GetHeader("X-Member-Token")
		if tokenStr == "" {
			auth := c.GetHeader("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				tokenStr = strings.TrimPrefix(auth, "Bearer ")
			}
		}
		if tokenStr == "" {
			pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "missing member token")
			c.Abort()
			return
		}
		claims, err := pkg.ParseMemberToken(tokenStr)
		if err != nil {
			pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "invalid member token")
			c.Abort()
			return
		}
		if getMember != nil {
			member, err := getMember(claims.MemberID)
			if err != nil || member.TokenVersion != claims.TokenVersion || member.Status != 1 {
				pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "invalid member token")
				c.Abort()
				return
			}
		}
		c.Set("member_id", claims.MemberID)
		c.Set("member_username", claims.Username)
		c.Next()
	}
}