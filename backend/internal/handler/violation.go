package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
)

type ViolationHandler struct {
	svc      service.ViolationService
	auditSvc service.AuditService
}

func NewViolationHandler(svc service.ViolationService, auditSvc service.AuditService) *ViolationHandler {
	return &ViolationHandler{svc: svc, auditSvc: auditSvc}
}

type MarkReviewedRequest struct {
	Note string `json:"note"`
}

func (h *ViolationHandler) List(c *gin.Context) {
	page, perPage := pkg.ParsePagination(c)
	keyword := c.Query("keyword")
	var reviewed *int8
	if s := c.Query("reviewed"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 8); err == nil {
			st := int8(v)
			reviewed = &st
		}
	}

	list, total, err := h.svc.List(page, perPage, reviewed, keyword)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
		return
	}

	pkg.Paginated(c, list, total, page, perPage)
}

func (h *ViolationHandler) MarkReviewed(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	var req MarkReviewedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "invalid request body")
		return
	}

	if err := h.svc.MarkReviewed(id, req.Note); err != nil {
		if errors.Is(err, service.ErrViolationNotFound) {
			pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "record not found")
			return
		}
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	h.audit(c, "violation_review", id, fmt.Sprintf(`{"note":%s}`, strconv.Quote(req.Note)))
	pkg.Success(c, nil)
}

func (h *ViolationHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	if err := h.svc.Delete(id); err != nil {
		if errors.Is(err, service.ErrViolationNotFound) {
			pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "record not found")
			return
		}
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	h.audit(c, "violation_delete", id, `{"action":"delete"}`)
	pkg.Success(c, nil)
}

// audit records the action to the audit log (best-effort).
func (h *ViolationHandler) audit(c *gin.Context, action string, resourceID uint64, detail string) {
	if h.auditSvc == nil {
		return
	}
	uid := c.GetUint64("user_id")
	_ = h.auditSvc.Log(&uid, action, "violation", strconv.FormatUint(resourceID, 10), detail, c.ClientIP(), c.GetHeader("User-Agent"))
}