package handler

import (
	"errors"
	"net/http"
	"time"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

type AuthHandler struct {
	svc service.AuthService
	rdb *redis.Client
}

func NewAuthHandler(svc service.AuthService, rdb *redis.Client) *AuthHandler {
	return &AuthHandler{svc: svc, rdb: rdb}
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	TotpCode string `json:"totp_code"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "请填写用户名和密码")
		return
	}

	result, err := h.svc.Login(req.Username, req.Password, req.TotpCode, c.ClientIP())
	if err != nil {
		// 2FA 账户未提供验证码：返回特殊标记，前端据此展示 TOTP 输入框。
		if errors.Is(err, pkg.ErrTotpRequired) {
			pkg.FailWithData(c, http.StatusUnauthorized, pkg.CodeUnauthorized, "需要两步验证", gin.H{"totp_required": true})
			return
		}
		pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, err.Error())
		return
	}

	pkg.Success(c, result)
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "refresh_token 不能为空")
		return
	}

	result, err := h.svc.Refresh(req.RefreshToken)
	if err != nil {
		pkg.Fail(c, http.StatusUnauthorized, pkg.CodeUnauthorized, err.Error())
		return
	}

	pkg.Success(c, result)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	// P1-4: revoke the current access token by blacklisting its jti until it
	// would naturally expire. The Auth middleware rejects blacklisted jtis.
	if h.rdb != nil {
		jti := c.GetString("token_jti")
		if jti != "" {
			if exp, ok := c.Get("token_exp"); ok {
				if t, ok2 := exp.(time.Time); ok2 {
					if ttl := time.Until(t); ttl > 0 {
						h.rdb.Set(c.Request.Context(), "jwt:blacklist:"+jti, 1, ttl)
					}
				}
			}
		}
	}

	// #7: also revoke the refresh token. Blacklisting only the access jti left a
	// logged-out session able to mint a brand-new access token from the refresh
	// token for up to refresh_expiry (7 days by default) — "log out" did not log
	// the session out.
	if h.rdb != nil {
		if rt := c.Query("refresh_token"); rt != "" {
			if jti, ttl, err := pkg.RefreshTokenIdentity(rt); err == nil && jti != "" && ttl > 0 {
				h.rdb.Set(c.Request.Context(), "jwt:blacklist:"+jti, 1, ttl)
			}
		}
	}
	pkg.Success(c, nil)
}

func (h *AuthHandler) GetMe(c *gin.Context) {
	userID := c.GetUint64("user_id")

	info, err := h.svc.GetMe(userID)
	if err != nil {
		pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "user not found")
		return
	}

	pkg.Success(c, info)
}
