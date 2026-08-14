package handler

import (
	"net/http"
	"strconv"
	"time"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
)

type AuditHandler struct {
	svc service.AuditService
}

func NewAuditHandler(svc service.AuditService) *AuditHandler {
	return &AuditHandler{svc: svc}
}

func (h *AuditHandler) List(c *gin.Context) {
	page, perPage := pkg.ParsePagination(c)

	filters := repository.AuditFilters{
		Action: c.Query("action"),
	}

	if uid := c.Query("user_id"); uid != "" {
		v, err := strconv.ParseUint(uid, 10, 64)
		if err == nil {
			filters.UserID = &v
		}
	}

	if df := c.Query("date_from"); df != "" {
		t, err := time.Parse("2006-01-02", df)
		if err == nil {
			filters.DateFrom = &t
		}
	}

	if dt := c.Query("date_to"); dt != "" {
		t, err := time.Parse("2006-01-02", dt)
		if err == nil {
			end := t.Add(24*time.Hour - time.Millisecond)
			filters.DateTo = &end
		}
	}

	list, total, err := h.svc.List(page, perPage, filters)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
		return
	}

	pkg.Paginated(c, list, total, page, perPage)
}

func (h *AuditHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	log, err := h.svc.GetByID(id)
	if err != nil {
		pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "audit log not found")
		return
	}

	pkg.Success(c, log)
}

// auditLog records an admin action to the audit log (best-effort). It is safe
// to call when auditSvc is nil.
func auditLog(c *gin.Context, auditSvc service.AuditService, resource, action string, resourceID uint64, detail string) {
	if auditSvc == nil {
		return
	}
	uid := c.GetUint64("user_id")
	_ = auditSvc.Log(&uid, action, resource, strconv.FormatUint(resourceID, 10), detail, c.ClientIP(), c.GetHeader("User-Agent"))
}
