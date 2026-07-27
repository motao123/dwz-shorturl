package handler

import (
	"net/http"
	"strconv"
	"time"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
)

type StatsHandler struct {
	svc service.StatsService
}

func NewStatsHandler(svc service.StatsService) *StatsHandler {
	return &StatsHandler{svc: svc}
}

func (h *StatsHandler) Overview(c *gin.Context) {
	result, err := h.svc.Overview()
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "failed to get overview")
		return
	}

	pkg.Success(c, result)
}

func (h *StatsHandler) Trend(c *gin.Context) {
	granularity := c.DefaultQuery("granularity", "day")

	var dateFrom, dateTo *time.Time
	if df := c.Query("date_from"); df != "" {
		t, err := time.Parse("2006-01-02", df)
		if err == nil {
			dateFrom = &t
		}
	}
	if dt := c.Query("date_to"); dt != "" {
		t, err := time.Parse("2006-01-02", dt)
		if err == nil {
			end := t.Add(24*time.Hour - time.Millisecond)
			dateTo = &end
		}
	}

	result, err := h.svc.Trend(granularity, dateFrom, dateTo)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "failed to get trend")
		return
	}

	pkg.Success(c, result)
}

func (h *StatsHandler) TopN(c *gin.Context) {
	n := 10
	if ns := c.Query("n"); ns != "" {
		if v, err := strconv.Atoi(ns); err == nil && v > 0 {
			n = v
		}
	}

	result, err := h.svc.TopN(n)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "failed to get top urls")
		return
	}

	pkg.Success(c, result)
}

func (h *StatsHandler) Recent(c *gin.Context) {
	n := 20
	if ns := c.Query("n"); ns != "" {
		if v, err := strconv.Atoi(ns); err == nil && v > 0 {
			n = v
		}
	}

	result, err := h.svc.Recent(n)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "failed to get recent urls")
		return
	}

	pkg.Success(c, result)
}
