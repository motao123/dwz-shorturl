package handler

import (
	"net/http"
	"strconv"
	"strings"

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

// SetPermissionsRequest accepts the permission set in either of the two shapes
// clients have used:
//
//	{"permission_ids": [1,2,3]}                    // canonical
//	{"permissions": ["short_urls.read", "stats.read"]}  // what the admin UI sent
//
// The UI's tree is keyed by `resource.action` strings; the backend's join table
// is keyed by numeric ids. Neither side was wrong on its own, but the contract
// between them was: the UI sent `permissions` while the handler bound
// `permission_ids` with binding:"required", so every save returned 400 ("保存权限
// 失败") and the authorization feature was 100% unusable (#21). Accept both and
// resolve names to ids server-side rather than forcing a flag day.
type SetPermissionsRequest struct {
	PermissionIDs []uint64 `json:"permission_ids"`
	Permissions   []string `json:"permissions"`
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
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "invalid request body")
		return
	}

	ids := req.PermissionIDs
	if len(ids) == 0 && len(req.Permissions) > 0 {
		// Resolve `resource.action` names to ids; unknown names are rejected
		// rather than silently dropped, so a typo cannot grant less than intended
		// without anyone noticing.
		all, err := h.svc.GetAllPermissions()
		if err != nil {
			pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
			return
		}
		byName := make(map[string]uint64, len(all))
		for _, p := range all {
			byName[p.Resource+"."+p.Action] = p.ID
		}
		for _, name := range req.Permissions {
			// The UI's tree also carries resource parent nodes ("short_urls");
			// they are not real permissions, so skip them.
			if !strings.Contains(name, ".") {
				continue
			}
			pid, ok := byName[name]
			if !ok {
				pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "未知权限点: "+name)
				return
			}
			ids = append(ids, pid)
		}
	}

	if len(ids) == 0 {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "permission_ids 不能为空")
		return
	}

	if err := h.svc.SetPermissionsAs(actorID(c), id, ids); err != nil {
		pkg.Fail(c, guardStatus(err), pkg.CodeForbidden, err.Error())
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
