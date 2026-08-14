package handler

import (
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

	result, err := h.svc.Login(req.Username, req.Password, c.ClientIP())
	if err != nil {
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
