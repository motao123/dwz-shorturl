package middleware

import (
	"net/http"
	"strings"
	"time"

	"dwz-admin/internal/pkg"

	"github.com/gin-gonic/gin"
)

// sessionCheck reports whether a signed, unexpired token is still usable
// (revoked at logout, or by disabling the account / resetting its password /
// changing its roles). Registered once at startup by main; unset means the
// deployment has no revocation store, which keeps the middleware working
// without Redis.
var sessionCheck pkg.SessionChecker

// SetSessionRevocationCheck registers the revocation check used by the Auth
// middleware.
func SetSessionRevocationCheck(fn pkg.SessionChecker) {
	sessionCheck = fn
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

		// P1-4 / #42：拒绝已吊销的会话——登出作废单个 jti，禁用账号、重置密码、
		// 改角色则按用户水位线作废其全部在授凭证。
		var issuedAt time.Time
		if claims.IssuedAt != nil {
			issuedAt = claims.IssuedAt.Time
		}
		if sessionCheck != nil {
			switch sessionCheck(claims.UserID, claims.ID, issuedAt) {
			case pkg.SessionRevoked:
				pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "会话已失效，请重新登录")
				c.Abort()
				return
			case pkg.SessionUnavailable:
				// 吊销存储不可用 ≠ 没有吊销。放行等于「把 Redis 打挂就能让所有登出与
				// 禁用记录集体失效」，正是要堵的那条绕过（#38）。用 503 而非 401，是为了
				// 让 Redis 抖动不必把全体在线管理员踢成重新登录。
				pkg.Fail(c, http.StatusServiceUnavailable, pkg.CodeInternalError, "登录状态校验暂时不可用，请稍后重试")
				c.Abort()
				return
			}
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
