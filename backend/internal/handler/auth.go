package handler

import (
	"net/http"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	svc service.AuthService
}

func NewAuthHandler(svc service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
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
	// Token blacklisting can be implemented with Redis in production.
	// For now, we simply return success since JWT is stateless.
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
