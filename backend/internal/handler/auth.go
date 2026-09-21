package handler

import (
	"errors"
	"net/http"
	"time"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	svc service.AuthService
	// rev 是登出的吊销出口（键名与 TTL 只在 service 里定义一次）。
	rev *service.SessionRevocation
}

func NewAuthHandler(svc service.AuthService, rev *service.SessionRevocation) *AuthHandler {
	return &AuthHandler{svc: svc, rev: rev}
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
	// would naturally expire. The Auth middleware rejects revoked jtis.
	if h.rev != nil {
		jti := c.GetString("token_jti")
		if exp, ok := c.Get("token_exp"); ok && jti != "" {
			if t, ok2 := exp.(time.Time); ok2 {
				_ = h.rev.BlacklistToken(c.Request.Context(), jti, t)
			}
		}
	}

	// #7: also revoke the refresh token. Blacklisting only the access jti left a
	// logged-out session able to mint a brand-new access token from the refresh
	// token for up to refresh_expiry (7 days by default) — "log out" did not log
	// the session out.
	if h.rev != nil {
		if rt := c.Query("refresh_token"); rt != "" {
			if jti, ttl, err := pkg.RefreshTokenIdentity(rt); err == nil && jti != "" && ttl > 0 {
				_ = h.rev.BlacklistToken(c.Request.Context(), jti, time.Now().Add(ttl))
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
