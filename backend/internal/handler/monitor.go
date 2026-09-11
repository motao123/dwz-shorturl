package handler

import (
	"net/http"
	"strconv"

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

// EnsurePartitions creates any missing click_logs monthly partitions up to the
// requested horizon (defaults to the cron job's own 2-month target).
func (h *MonitorHandler) EnsurePartitions(c *gin.Context) {
	months := 0
	if raw := c.Query("months"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 || v > 24 {
			pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "months must be an integer between 0 and 24")
			return
		}
		months = v
	}
	created, err := h.svc.EnsurePartitions(months)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	pkg.Success(c, gin.H{"created": created})
}
