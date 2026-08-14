package handler

import (
	"net/http"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
)

type MonitorHandler struct {
	svc service.MonitorService
}

func NewMonitorHandler(svc service.MonitorService) *MonitorHandler {
	return &MonitorHandler{svc: svc}
}

func (h *MonitorHandler) Status(c *gin.Context) {
	status, err := h.svc.Status()
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "monitor query failed")
		return
	}
	pkg.Success(c, status)
}

type RunTaskRequest struct {
	Name string `json:"name" binding:"required"`
}

// RunTask triggers a cron task on demand (admin op tool).
func (h *MonitorHandler) RunTask(c *gin.Context) {
	var req RunTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "task name is required")
		return
	}
	ok, err := h.svc.RunTask(req.Name)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	pkg.Success(c, gin.H{"task": req.Name, "ran": ok})
}