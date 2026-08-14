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

type MemberHandler struct {
	svc      service.MemberService
	auditSvc service.AuditService
}

func NewMemberHandler(svc service.MemberService, auditSvc service.AuditService) *MemberHandler {
	return &MemberHandler{svc: svc, auditSvc: auditSvc}
}

type UpdateMemberStatusRequest struct {
	Status int8 `json:"status" binding:"oneof=0 1"`
}

type ResetMemberPasswordRequest struct {
	Password string `json:"password" binding:"required,min=6"`
}

func (h *MemberHandler) List(c *gin.Context) {
	page, perPage := pkg.ParsePagination(c)
	keyword := c.Query("keyword")
	var status *int8
	if s := c.Query("status"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 8); err == nil {
			st := int8(v)
			status = &st
		}
	}

	list, total, err := h.svc.List(page, perPage, keyword, status)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
		return
	}

	pkg.Paginated(c, list, total, page, perPage)
}

func (h *MemberHandler) UpdateStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	var req UpdateMemberStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "invalid status")
		return
	}

	if err := h.svc.UpdateStatus(id, req.Status); err != nil {
		if errors.Is(err, service.ErrMemberNotFound) {
			pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "member not found")
			return
		}
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	h.audit(c, "member_status", id, fmt.Sprintf(`{"status":%d}`, req.Status))
	pkg.Success(c, nil)
}

func (h *MemberHandler) ResetPassword(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	var req ResetMemberPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "password is required (min 6 chars)")
		return
	}

	if err := h.svc.ResetPassword(id, req.Password); err != nil {
		if errors.Is(err, service.ErrMemberNotFound) {
			pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "member not found")
			return
		}
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	h.audit(c, "member_password_reset", id, `{"action":"reset"}`)
	pkg.Success(c, nil)
}

func (h *MemberHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	if err := h.svc.Delete(id); err != nil {
		if errors.Is(err, service.ErrMemberNotFound) {
			pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "member not found")
			return
		}
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	h.audit(c, "member_delete", id, `{"action":"delete"}`)
	pkg.Success(c, nil)
}

// audit records the action to the audit log (best-effort).
func (h *MemberHandler) audit(c *gin.Context, action string, resourceID uint64, detail string) {
	if h.auditSvc == nil {
		return
	}
	uid := c.GetUint64("user_id")
	_ = h.auditSvc.Log(&uid, action, "member", strconv.FormatUint(resourceID, 10), detail, c.ClientIP(), c.GetHeader("User-Agent"))
}