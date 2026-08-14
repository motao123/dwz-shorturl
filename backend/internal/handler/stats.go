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

func (h *StatsHandler) LinkStats(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}
	result, err := h.svc.LinkStats(id)
	if err != nil {
		pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "short url not found")
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

// Countries returns the global traffic-source country distribution (30 days).
func (h *StatsHandler) Countries(c *gin.Context) {
	limit := 12
	if ns := c.Query("limit"); ns != "" {
		if v, err := strconv.Atoi(ns); err == nil && v > 0 {
			limit = v
		}
	}
	result, err := h.svc.Countries(limit)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "failed to get country distribution")
		return
	}
	pkg.Success(c, result)
}

// ReferrerTypes returns the global referrer-type breakdown (30 days).
func (h *StatsHandler) ReferrerTypes(c *gin.Context) {
	limit := 8
	if ns := c.Query("limit"); ns != "" {
		if v, err := strconv.Atoi(ns); err == nil && v > 0 {
			limit = v
		}
	}
	result, err := h.svc.ReferrerTypes(limit)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "failed to get referrer types")
		return
	}
	pkg.Success(c, result)
}
