package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dwz-admin/internal/config"
	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
)

type ShortUrlHandler struct {
	svc       service.ShortUrlService
	rl        *pkg.RateLimiter
	rlCfg     config.RateLimitConfig
	auditSvc  service.AuditService
	webhookSvc service.WebhookService
}

func NewShortUrlHandler(svc service.ShortUrlService, rl *pkg.RateLimiter, rlCfg config.RateLimitConfig, auditSvc service.AuditService, webhookSvc service.WebhookService) *ShortUrlHandler {
	return &ShortUrlHandler{svc: svc, rl: rl, rlCfg: rlCfg, auditSvc: auditSvc, webhookSvc: webhookSvc}
}

type CreateShortUrlRequest struct {
	URL        string  `json:"url" binding:"required"`
	Custom     string  `json:"custom"`
	ExpireDays int     `json:"expire_days"`
	DomainID   *uint64 `json:"domain_id"`
	Password   string  `json:"password"`
}

type BatchCreateRequest struct {
	URLs     []string `json:"urls" binding:"required,min=1,max=100"`
	DomainID *uint64  `json:"domain_id"`
}

type UpdateShortUrlRequest struct {
	LongURL    string  `json:"long_url"`
	Title      string  `json:"title"`
	ExpireDays *int    `json:"expire_days"`
	Status     *int8   `json:"status"`
	CategoryID *uint64 `json:"category_id"`
	DomainID   *uint64 `json:"domain_id"`
	// Password: 省略=不修改；"" = 清除密码；非空 = 设置新密码
	Password *string `json:"password"`
}

type BatchDeleteRequest struct {
	IDs []uint64 `json:"ids" binding:"required,min=1"`
}

// CheckLink performs a lightweight HEAD request to the short link's target to
// report whether it is currently reachable. Never mutates anything.
func (h *ShortUrlHandler) CheckLink(c *gin.Context) {
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

	// P1-5: refuse to probe private/internal hosts. The URL is re-validated here
	// (it may predate creation-time checks), and the dialer additionally blocks
	// private IPs in case DNS rebinds between validation and connect.
	if err := service.ValidateURL(record.LongURL); err != nil {
		pkg.Success(c, gin.H{"id": id, "url": record.LongURL, "ok": false, "status": 0, "error": "url not allowed for health check"})
		return
	}
	client := pkg.NewSafeHTTPClient(5 * time.Second)
	req, err := http.NewRequest(http.MethodHead, record.LongURL, nil)
	if err != nil {
		pkg.Success(c, gin.H{"id": id, "url": record.LongURL, "ok": false, "status": 0, "error": err.Error()})
		return
	}
	req.Header.Set("User-Agent", "dwz-shorturl-healthcheck/1.0")
	resp, err := client.Do(req)
	if err != nil {
		pkg.Success(c, gin.H{"id": id, "url": record.LongURL, "ok": false, "status": 0, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	ok := resp.StatusCode >= 200 && resp.StatusCode < 400
	pkg.Success(c, gin.H{"id": id, "url": record.LongURL, "ok": ok, "status": resp.StatusCode})
}

func (h *ShortUrlHandler) Create(c *gin.Context) {
	var req CreateShortUrlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "url is required")
		return
	}

	userID := c.GetUint64("user_id")

	// QUOTA-01: per-user rate limit on single-link creation.
	if h.rl != nil {
		key := "user:" + strconv.FormatUint(userID, 10)
		ok, err := h.rl.Allow(c.Request.Context(), key, h.rlCfg.SingleMax, time.Duration(h.rlCfg.SingleWindow)*time.Second)
		if err != nil {
			pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "rate limit check failed")
			return
		}
		if !ok {
			pkg.Fail(c, http.StatusTooManyRequests, pkg.CodeRateLimit, "请求过于频繁，请稍后再试")
			return
		}
	}

	record, err := h.svc.Create(req.URL, req.Custom, req.ExpireDays, req.DomainID, &userID, "admin", c.ClientIP(), req.Password)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	auditLog(c, h.auditSvc, "short_url", "short_url_create", record.ID, `{"uid":`+strconv.Quote(record.UID)+`}`)
	h.dispatchCreated(record)
	pkg.Success(c, record)
}

// dispatchCreated fires the link.created webhook event for a newly created link.
func (h *ShortUrlHandler) dispatchCreated(record *model.ShortUrl) {
	if h.webhookSvc == nil {
		return
	}
	h.webhookSvc.Dispatch("link.created", map[string]interface{}{
		"id":        record.ID,
		"uid":       record.UID,
		"long_url":  record.LongURL,
		"short_url": service.PublicShortURL(record.UID),
	})
}

// CreatePublic creates a short URL through the API-key-authenticated public
// endpoint. The caller is not a logged-in admin user, so created_by is nil and
// source is "api".
func (h *ShortUrlHandler) CreatePublic(c *gin.Context) {
	var req CreateShortUrlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "url is required")
		return
	}

	record, err := h.svc.CreatePublicAPI(req.URL, req.Custom, req.ExpireDays, req.DomainID, c.ClientIP(), req.Password)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	h.dispatchCreated(record)
	pkg.Success(c, gin.H{
		"uid":        record.UID,
		"short_url":  service.PublicShortURL(record.UID),
		"long_url":   record.LongURL,
		"expire_at":  record.ExpireAt,
		"created_at": record.CreatedAt,
	})
}

// BatchCreatePublic batch-creates short URLs through the API-key-authenticated
// public endpoint. Returns per-row results and errors.
func (h *ShortUrlHandler) BatchCreatePublic(c *gin.Context) {
	var req BatchCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "urls array is required (1-100 items)")
		return
	}

	results, errs := h.svc.BatchCreatePublicAPI(req.URLs, req.DomainID, c.ClientIP())
	errList := make([]string, 0, len(errs))
	for i, e := range errs {
		if e != nil {
			errList = append(errList, strconv.Itoa(i)+": "+e.Error())
		}
	}

	out := make([]gin.H, 0, len(results))
	for _, r := range results {
		out = append(out, gin.H{
			"uid":       r.UID,
			"short_url": service.PublicShortURL(r.UID),
			"long_url":  r.LongURL,
		})
	}
	for i := range results {
		h.dispatchCreated(&results[i])
	}
	pkg.Success(c, gin.H{"results": out, "errors": errList, "total": len(results)})
}

func (h *ShortUrlHandler) BatchCreate(c *gin.Context) {
	var req BatchCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "urls array is required (1-100 items)")
		return
	}

	userID := c.GetUint64("user_id")

	// QUOTA-01: per-user batch rate limit, charging one token per URL.
	if h.rl != nil {
		key := "batch:" + strconv.FormatUint(userID, 10)
		ok, err := h.rl.AllowN(c.Request.Context(), key, h.rlCfg.BatchMax, len(req.URLs), time.Duration(h.rlCfg.BatchWindow)*time.Second)
		if err != nil {
			pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "rate limit check failed")
			return
		}
		if !ok {
			pkg.Fail(c, http.StatusTooManyRequests, pkg.CodeRateLimit, "批量请求过于频繁，请稍后再试")
			return
		}
	}

	results, errs := h.svc.BatchCreate(req.URLs, req.DomainID, &userID, c.ClientIP())

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
	auditLog(c, h.auditSvc, "short_url", "short_url_batch_create", 0, `{"count":`+strconv.Itoa(len(results))+`}`)
	for i := range results {
		h.dispatchCreated(&results[i])
	}
}

type ImportRequest struct {
	Format   string               `json:"format" binding:"required,oneof=csv json"`
	Content  string               `json:"content" binding:"required"`
	DomainID *uint64              `json:"domain_id"`
}

// Import parses CSV or JSON content and batch-creates short URLs.
func (h *ShortUrlHandler) Import(c *gin.Context) {
	var req ImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "format (csv|json) and content are required")
		return
	}
	userID := c.GetUint64("user_id")

	items, err := parseImportRows(req.Format, req.Content)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, err.Error())
		return
	}
	if len(items) == 0 {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "no valid rows to import")
		return
	}
	if len(items) > 500 {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "max 500 rows per import")
		return
	}

	if h.rl != nil {
		key := "import:" + strconv.FormatUint(userID, 10)
		ok, err := h.rl.AllowN(c.Request.Context(), key, h.rlCfg.BatchMax, len(items), time.Duration(h.rlCfg.BatchWindow)*time.Second)
		if err != nil {
			pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "rate limit check failed")
			return
		}
		if !ok {
			pkg.Fail(c, http.StatusTooManyRequests, pkg.CodeRateLimit, "导入过于频繁，请稍后再试")
			return
		}
	}

	results, errs := h.svc.BatchImport(items, req.DomainID, &userID, c.ClientIP())
	errList := make([]string, 0, len(errs))
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
	auditLog(c, h.auditSvc, "short_url", "short_url_import", 0, `{"count":`+strconv.Itoa(len(results))+`}`)
	for i := range results {
		h.dispatchCreated(&results[i])
	}
}

// parseImportRows converts CSV/JSON content into import items.
func parseImportRows(format, content string) ([]service.ImportItem, error) {
	if format == "json" {
		var items []service.ImportItem
		if err := json.Unmarshal([]byte(content), &items); err != nil {
			return nil, errors.New("invalid JSON: " + err.Error())
		}
		return items, nil
	}
	// CSV: header optional; columns url,title,custom,expire_days
	lines := strings.Split(strings.TrimSpace(content), "\n")
	items := make([]service.ImportItem, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ",")
		it := service.ImportItem{URL: strings.TrimSpace(fields[0])}
		if len(fields) > 1 {
			it.Title = strings.TrimSpace(fields[1])
		}
		if len(fields) > 2 {
			it.Custom = strings.TrimSpace(fields[2])
		}
		if len(fields) > 3 {
			if days, err := strconv.Atoi(strings.TrimSpace(fields[3])); err == nil {
				it.ExpireDays = days
			}
		}
		items = append(items, it)
	}
	return items, nil
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

type BatchUpdateRequest struct {
	IDs        []uint64 `json:"ids" binding:"required,min=1"`
	Status     *int8    `json:"status"`
	ExpireDays *int     `json:"expire_days"`
}

func (h *ShortUrlHandler) BatchUpdate(c *gin.Context) {
	var req BatchUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "ids is required")
		return
	}
	updated, err := h.svc.BatchUpdate(req.IDs, req.Status, req.ExpireDays)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}
	auditLog(c, h.auditSvc, "short_url", "short_url_batch_update", 0, `{"count":`+strconv.FormatInt(updated, 10)+`}`)
	pkg.Success(c, gin.H{"updated": updated})
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

	record, err := h.svc.Update(id, req.LongURL, req.Title, req.ExpireDays, req.Status, req.CategoryID, req.DomainID, req.Password)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	auditLog(c, h.auditSvc, "short_url", "short_url_update", id, `{"uid":`+strconv.Quote(record.UID)+`}`)
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

	auditLog(c, h.auditSvc, "short_url", "short_url_delete", id, "")
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

	auditLog(c, h.auditSvc, "short_url", "short_url_batch_delete", 0, `{"count":`+strconv.Itoa(len(req.IDs))+`}`)
	pkg.Success(c, nil)
}

// Restore undeletes a soft-deleted short URL (回收站恢复).
func (h *ShortUrlHandler) Restore(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, "invalid id")
		return
	}

	record, err := h.svc.Restore(id)
	if err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	auditLog(c, h.auditSvc, "short_url", "short_url_restore", id, `{"uid":`+strconv.Quote(record.UID)+`}`)
	pkg.Success(c, record)
}

func (h *ShortUrlHandler) List(c *gin.Context) {
	page, perPage := pkg.ParsePagination(c)

	filters := repository.ShortUrlFilters{
		Keyword:        c.Query("keyword"),
		IncludeDeleted: c.Query("include_deleted") == "1",
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

	if did := c.Query("domain_id"); did != "" {
		v, err := strconv.ParseUint(did, 10, 64)
		if err == nil {
			filters.DomainID = &v
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
