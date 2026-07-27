package handler

import (
	"net/http"
	"strconv"
	"time"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
)

type ApiKeyHandler struct {
	svc service.ApiKeyService
}

func NewApiKeyHandler(svc service.ApiKeyService) *ApiKeyHandler {
	return &ApiKeyHandler{svc: svc}
}

type CreateApiKeyRequest struct {
	Name        string `json:"name" binding:"required"`
	Permissions string `json:"permissions"`
	RateLimit   int    `json:"rate_limit"`
	ExpiresAt   string `json:"expires_at"`
}

func (h *ApiKeyHandler) Create(c *gin.Context) {
	var req CreateApiKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "name is required")
		return
	}

	userID := c.GetUint64("user_id")

	var expiresAt *time.Time
	if req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "invalid expires_at format, use RFC3339")
			return
		}
		expiresAt = &t
	}

	result, err := h.svc.Create(userID, req.Name, req.Permissions, req.RateLimit, expiresAt)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "failed to create api key")
		return
	}

	pkg.Success(c, result)
}

func (h *ApiKeyHandler) List(c *gin.Context) {
	userID := c.GetUint64("user_id")

	keys, err := h.svc.List(userID)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
		return
	}

	pkg.Success(c, keys)
}

func (h *ApiKeyHandler) Revoke(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	if err := h.svc.Revoke(id); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	pkg.Success(c, nil)
}

func (h *ApiKeyHandler) GetStats(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	key, err := h.svc.GetByID(id)
	if err != nil {
		pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "api key not found")
		return
	}

	pkg.Success(c, gin.H{
		"id":           key.ID,
		"name":         key.Name,
		"key_prefix":   key.KeyPrefix,
		"rate_limit":   key.RateLimit,
		"status":       key.Status,
		"last_used_at": key.LastUsedAt,
		"expires_at":   key.ExpiresAt,
		"created_at":   key.CreatedAt,
	})
}
