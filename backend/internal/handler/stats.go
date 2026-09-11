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
	dateFrom, dateTo := queryDateRange(c)

	result, err := h.svc.Trend(granularity, dateFrom, dateTo)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "failed to get trend")
		return
	}

	pkg.Success(c, result)
}

// queryLimit reads a list-size parameter, accepting both `limit` (used by the
// frontend) and the legacy `n` name. Without this the frontend's `limit` was
// silently dropped and Top/Recent always returned the default size.
func queryLimit(c *gin.Context, fallback int) int {
	for _, key := range []string{"limit", "n"} {
		if raw := c.Query(key); raw != "" {
			if v, err := strconv.Atoi(raw); err == nil && v > 0 {
				return v
			}
		}
	}
	return fallback
}

// queryDateRange parses the shared date_from/date_to (YYYY-MM-DD) filters.
func queryDateRange(c *gin.Context) (dateFrom, dateTo *time.Time) {
	if df := c.Query("date_from"); df != "" {
		if t, err := time.Parse("2006-01-02", df); err == nil {
			dateFrom = &t
		}
	}
	if dt := c.Query("date_to"); dt != "" {
		if t, err := time.Parse("2006-01-02", dt); err == nil {
			end := t.Add(24*time.Hour - time.Millisecond)
			dateTo = &end
		}
	}
	return dateFrom, dateTo
}

func (h *StatsHandler) TopN(c *gin.Context) {
	n := queryLimit(c, 10)
	dateFrom, dateTo := queryDateRange(c)

	result, err := h.svc.TopN(n, dateFrom, dateTo)
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
	n := queryLimit(c, 20)

	result, err := h.svc.Recent(n)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "failed to get recent urls")
		return
	}

	pkg.Success(c, result)
}

// Countries returns the global traffic-source country distribution (30 days).
func (h *StatsHandler) Countries(c *gin.Context) {
	limit := queryLimit(c, 12)
	result, err := h.svc.Countries(limit)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "failed to get country distribution")
		return
	}
	pkg.Success(c, result)
}

// ReferrerTypes returns the global referrer-type breakdown (30 days).
func (h *StatsHandler) ReferrerTypes(c *gin.Context) {
	limit := queryLimit(c, 8)
	result, err := h.svc.ReferrerTypes(limit)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "failed to get referrer types")
		return
	}
	pkg.Success(c, result)
}
