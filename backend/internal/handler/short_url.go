package handler

import (
	"net/http"
	"strconv"
	"time"

	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
)

type ShortUrlHandler struct {
	svc service.ShortUrlService
}

func NewShortUrlHandler(svc service.ShortUrlService) *ShortUrlHandler {
	return &ShortUrlHandler{svc: svc}
}

type CreateShortUrlRequest struct {
	URL        string `json:"url" binding:"required"`
	Custom     string `json:"custom"`
	ExpireDays int    `json:"expire_days"`
}

type BatchCreateRequest struct {
	URLs []string `json:"urls" binding:"required,min=1,max=100"`
}

type UpdateShortUrlRequest struct {
	LongURL    string `json:"long_url"`
	Title      string `json:"title"`
	ExpireDays *int   `json:"expire_days"`
	Status     *int8  `json:"status"`
	CategoryID *uint64 `json:"category_id"`
}

type BatchDeleteRequest struct {
	IDs []uint64 `json:"ids" binding:"required,min=1"`
}

func (h *ShortUrlHandler) Create(c *gin.Context) {
	var req CreateShortUrlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "url is required")
		return
	}

	userID := c.GetUint64("user_id")
	record, err := h.svc.Create(req.URL, req.Custom, req.ExpireDays, &userID, "admin", c.ClientIP())
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	pkg.Success(c, record)
}

func (h *ShortUrlHandler) BatchCreate(c *gin.Context) {
	var req BatchCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "urls array is required (1-100 items)")
		return
	}

	userID := c.GetUint64("user_id")
	results, errs := h.svc.BatchCreate(req.URLs, &userID, c.ClientIP())

	// Build error list
	errList := make([]string, 0)
	for i, e := range errs {
		if e != nil {
			errList = append(errList, strconv.Itoa(i)+": "+e.Error())
		}
	}

	pkg.Success(c, gin.H{
		"results": results,
		"errors":  errList,
		"total":   len(results),
	})
}

func (h *ShortUrlHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	record, err := h.svc.GetByID(id)
	if err != nil {
		pkg.Fail(c, http.StatusNotFound, pkg.CodeNotFound, "short url not found")
		return
	}

	pkg.Success(c, record)
}

func (h *ShortUrlHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	var req UpdateShortUrlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "invalid request body")
		return
	}

	record, err := h.svc.Update(id, req.LongURL, req.Title, req.ExpireDays, req.Status, req.CategoryID)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	pkg.Success(c, record)
}

func (h *ShortUrlHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	if err := h.svc.Delete(id); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	pkg.Success(c, nil)
}

func (h *ShortUrlHandler) BatchDelete(c *gin.Context) {
	var req BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "ids array is required")
		return
	}

	if err := h.svc.BatchDelete(req.IDs); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	pkg.Success(c, nil)
}

func (h *ShortUrlHandler) List(c *gin.Context) {
	page, perPage := pkg.ParsePagination(c)

	filters := repository.ShortUrlFilters{
		Keyword: c.Query("keyword"),
	}

	if s := c.Query("status"); s != "" {
		v, err := strconv.ParseInt(s, 10, 8)
		if err == nil {
			status := int8(v)
			filters.Status = &status
		}
	}

	if cid := c.Query("category_id"); cid != "" {
		v, err := strconv.ParseUint(cid, 10, 64)
		if err == nil {
			filters.CategoryID = &v
		}
	}

	if df := c.Query("date_from"); df != "" {
		t, err := time.Parse("2006-01-02", df)
		if err == nil {
			filters.DateFrom = &t
		}
	}

	if dt := c.Query("date_to"); dt != "" {
		t, err := time.Parse("2006-01-02", dt)
		if err == nil {
			// Include the full day
			end := t.Add(24*time.Hour - time.Millisecond)
			filters.DateTo = &end
		}
	}

	filters.Sort = c.Query("sort")
	filters.Order = c.Query("order")

	list, total, err := h.svc.List(page, perPage, filters)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
		return
	}

	pkg.Paginated(c, list, total, page, perPage)
}

func (h *ShortUrlHandler) Export(c *gin.Context) {
	filters := repository.ShortUrlFilters{
		Keyword: c.Query("keyword"),
	}

	if s := c.Query("status"); s != "" {
		v, err := strconv.ParseInt(s, 10, 8)
		if err == nil {
			status := int8(v)
			filters.Status = &status
		}
	}

	data, err := h.svc.Export(filters)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "export failed")
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=short_urls_export.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", data)
}
