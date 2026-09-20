package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
)

type ApiKeyHandler struct {
	svc      service.ApiKeyService
	auditSvc service.AuditService
}

func NewApiKeyHandler(svc service.ApiKeyService, auditSvc service.AuditService) *ApiKeyHandler {
	return &ApiKeyHandler{svc: svc, auditSvc: auditSvc}
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
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "请填写名称")
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

	// #25: 发放密钥要留痕。只记 id/名称/前缀——result.ApiKey 与 result.PlainText
	// 是明文密钥本身，写进审计表就等于把凭据复制到另一个可读位置。
	auditLog(c, h.auditSvc, "api_key", "api_key_create", result.ID,
		`{"name":`+strconv.Quote(result.Name)+`,"key_prefix":`+strconv.Quote(result.KeyPrefix)+`}`)
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

	// #40: scope the operation to the acting administrator. Revoke alone took a
	// bare id, so any holder of api_keys.revoke could enumerate ids and revoke
	// every API key on the platform.
	actorID := c.GetUint64("user_id")
	if err := h.svc.RevokeAs(actorID, id); err != nil {
		apiKeyFail(c, err)
		return
	}

	// #25: 吊销是会让集成方立刻 401 的破坏性动作，必须可追溯是谁做的。
	auditLog(c, h.auditSvc, "api_key", "api_key_revoke", id, "")
	pkg.Success(c, nil)
}

func (h *ApiKeyHandler) GetStats(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	// #40: same ownership constraint as Revoke — the stats payload exposes key
	// metadata (prefix, quota, usage) that belongs to a single account.
	actorID := c.GetUint64("user_id")
	key, err := h.svc.GetByIDAs(actorID, id)
	if err != nil {
		apiKeyFail(c, err)
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

// apiKeyFail maps an API-key service error to an HTTP response.
//
// An ownership violation is 403 (authenticated, but not allowed) while a
// missing key is 404. The distinction is for logs and for operators; neither
// discloses whether an id belonging to somebody else exists, because the 404
// branch is only reachable when the key does not exist for anyone.
func apiKeyFail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrApiKeyForbidden):
		pkg.Fail(c, http.StatusForbidden, pkg.CodeForbidden, "无权操作该 API Key")
	case errors.Is(err, service.ErrApiKeyNotFound):
		pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "API Key 不存在")
	default:
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
	}
}
