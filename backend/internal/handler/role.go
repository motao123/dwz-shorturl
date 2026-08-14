package handler

import (
	"net/http"
	"strconv"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
)

type RoleHandler struct {
	svc      service.RoleService
	auditSvc service.AuditService
}

func NewRoleHandler(svc service.RoleService, auditSvc service.AuditService) *RoleHandler {
	return &RoleHandler{svc: svc, auditSvc: auditSvc}
}

type CreateRoleRequest struct {
	Name        string `json:"name" binding:"required,min=2,max=32"`
	DisplayName string `json:"display_name" binding:"required"`
	Description string `json:"description"`
}

type UpdateRoleRequest struct {
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
}

type SetPermissionsRequest struct {
	PermissionIDs []uint64 `json:"permission_ids" binding:"required"`
}

func (h *RoleHandler) Create(c *gin.Context) {
	var req CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "invalid request: "+err.Error())
		return
	}

	role, err := h.svc.Create(req.Name, req.DisplayName, req.Description)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	auditLog(c, h.auditSvc, "role", "role_create", role.ID, `{"name":`+strconv.Quote(req.Name)+`}`)
	pkg.Success(c, role)
}

func (h *RoleHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	var req UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "invalid request body")
		return
	}

	role, err := h.svc.Update(id, req.DisplayName, req.Description)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	auditLog(c, h.auditSvc, "role", "role_update", id, "")
	pkg.Success(c, role)
}

func (h *RoleHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	if err := h.svc.Delete(id); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	auditLog(c, h.auditSvc, "role", "role_delete", id, "")
	pkg.Success(c, nil)
}

func (h *RoleHandler) List(c *gin.Context) {
	roles, err := h.svc.GetAll()
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
		return
	}

	pkg.Success(c, roles)
}

func (h *RoleHandler) SetPermissions(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	var req SetPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "permission_ids is required")
		return
	}

	if err := h.svc.SetPermissions(id, req.PermissionIDs); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	auditLog(c, h.auditSvc, "role", "role_permissions", id, "")
	pkg.Success(c, nil)
}

func (h *RoleHandler) GetPermissions(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	perms, err := h.svc.GetPermissions(id)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
		return
	}

	pkg.Success(c, perms)
}

func (h *RoleHandler) GetAllPermissions(c *gin.Context) {
	perms, err := h.svc.GetAllPermissions()
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
		return
	}

	pkg.Success(c, perms)
}
