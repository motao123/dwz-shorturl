package handler

import (
	"net/http"
	"strconv"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	svc      service.UserService
	auditSvc service.AuditService
}

func NewUserHandler(svc service.UserService, auditSvc service.AuditService) *UserHandler {
	return &UserHandler{svc: svc, auditSvc: auditSvc}
}

type CreateUserRequest struct {
	Username    string `json:"username" binding:"required,min=2,max=32"`
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=6"`
	DisplayName string `json:"display_name"`
}

type UpdateUserRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	Status      *int8  `json:"status"`
}

type ResetPasswordRequest struct {
	Password string `json:"password" binding:"required,min=6"`
}

type AssignRolesRequest struct {
	RoleIDs []uint64 `json:"role_ids" binding:"required"`
}

func (h *UserHandler) Create(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "invalid request: "+err.Error())
		return
	}

	user, err := h.svc.Create(req.Username, req.Email, req.Password, req.DisplayName)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	auditLog(c, h.auditSvc, "user", "user_create", user.ID, `{"username":`+strconv.Quote(user.Username)+`}`)
	pkg.Success(c, user)
}

func (h *UserHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "invalid request body")
		return
	}

	user, err := h.svc.Update(id, req.Email, req.DisplayName, req.AvatarURL, req.Status)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	auditLog(c, h.auditSvc, "user", "user_update", id, "")
	pkg.Success(c, user)
}

func (h *UserHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	if err := h.svc.Delete(id); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	auditLog(c, h.auditSvc, "user", "user_delete", id, "")
	pkg.Success(c, nil)
}

func (h *UserHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	user, err := h.svc.GetByID(id)
	if err != nil {
		pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "user not found")
		return
	}

	pkg.Success(c, user)
}

func (h *UserHandler) List(c *gin.Context) {
	page, perPage := pkg.ParsePagination(c)
	keyword := c.Query("keyword")

	list, total, err := h.svc.List(page, perPage, keyword)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
		return
	}

	pkg.Paginated(c, list, total, page, perPage)
}

func (h *UserHandler) ResetPassword(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "password is required (min 6 chars)")
		return
	}

	if err := h.svc.ResetPassword(id, req.Password); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	auditLog(c, h.auditSvc, "user", "user_password_reset", id, "")
	pkg.Success(c, nil)
}

func (h *UserHandler) AssignRoles(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	var req AssignRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "role_ids is required")
		return
	}

	if err := h.svc.AssignRoles(id, req.RoleIDs); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	auditLog(c, h.auditSvc, "user", "user_roles", id, "")
	pkg.Success(c, nil)
}
