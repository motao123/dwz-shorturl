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
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "username and password are required")
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
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "refresh_token is required")
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
