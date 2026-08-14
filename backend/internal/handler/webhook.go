package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
)

type WebhookHandler struct {
	svc      service.WebhookService
	auditSvc service.AuditService
}

func NewWebhookHandler(svc service.WebhookService, auditSvc service.AuditService) *WebhookHandler {
	return &WebhookHandler{svc: svc, auditSvc: auditSvc}
}

type CreateWebhookRequest struct {
	Name   string   `json:"name" binding:"required,min=1,max=64"`
	URL    string   `json:"url" binding:"required,url"`
	Events []string `json:"events" binding:"required,min=1"`
	Secret string   `json:"secret"`
}

func (h *WebhookHandler) List(c *gin.Context) {
	subs, err := h.svc.List()
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
		return
	}
	pkg.Success(c, subs)
}

func (h *WebhookHandler) ListDeliveries(c *gin.Context) {
	page, perPage := pkg.ParsePagination(c)

	filters := repository.WebhookDeliveryFilters{
		Event: c.Query("event"),
	}
	if wid := c.Query("webhook_id"); wid != "" {
		v, err := strconv.ParseUint(wid, 10, 64)
		if err == nil {
			filters.WebhookID = &v
		}
	}
	if res := c.Query("result"); res == "success" || res == "failed" {
		v := int8(0)
		if res == "success" {
			v = 1
		}
		filters.Success = &v
	}

	list, total, err := h.svc.ListDeliveries(page, perPage, filters)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
		return
	}
	pkg.Paginated(c, list, total, page, perPage)
}

func (h *WebhookHandler) Create(c *gin.Context) {
	var req CreateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "name, url and events are required")
		return
	}
	events, err := json.Marshal(req.Events)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "invalid events")
		return
	}
	userID := c.GetUint64("user_id")
	sub, err := h.svc.Create(req.Name, req.URL, string(events), req.Secret, &userID)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	auditLog(c, h.auditSvc, "webhook", "webhook_create", sub.ID, `{"name":`+strconv.Quote(sub.Name)+`}`)
	pkg.Success(c, sub)
}

func (h *WebhookHandler) TestPing(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}
	delivery, err := h.svc.TestPing(id)
	if err != nil {
		if errors.Is(err, service.ErrWebhookNotFound) {
			pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "webhook not found")
			return
		}
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "ping failed")
		return
	}
	auditLog(c, h.auditSvc, "webhook", "webhook_ping", id, "")
	pkg.Success(c, delivery)
}

func (h *WebhookHandler) RetryDelivery(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}
	delivery, err := h.svc.RetryDelivery(id)
	if err != nil {
		if errors.Is(err, service.ErrWebhookDeliveryNotFound) {
			pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "delivery not found")
			return
		}
		if errors.Is(err, service.ErrWebhookNotFound) {
			pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "webhook not found")
			return
		}
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "retry failed")
		return
	}
	auditLog(c, h.auditSvc, "webhook", "webhook_retry", id, "")
	pkg.Success(c, delivery)
}

func (h *WebhookHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}
	if err := h.svc.Delete(id); err != nil {
		if errors.Is(err, service.ErrWebhookNotFound) {
			pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "webhook not found")
			return
		}
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	auditLog(c, h.auditSvc, "webhook", "webhook_delete", id, "")
	pkg.Success(c, nil)
}