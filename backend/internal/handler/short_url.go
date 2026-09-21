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
	svc        service.ShortUrlService
	rl         *pkg.RateLimiter
	rlCfg      config.RateLimitConfig
	auditSvc   service.AuditService
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
	// Redirects are capped at 3 hops: an unbounded 302 chain would otherwise
	// pin a connection slot until the 5s timeout expires.
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
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "url 不能为空")
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
		"short_url": record.ShortURL,
	})
}

// CreatePublic creates a short URL through the API-key-authenticated public
// endpoint. The caller is not a logged-in admin user, so created_by is nil and
// source is "api".
func (h *ShortUrlHandler) CreatePublic(c *gin.Context) {
	var req CreateShortUrlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "url 不能为空")
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
		"short_url":  record.ShortURL,
		"long_url":   record.LongURL,
		"expire_at":  record.ExpireAt,
		"created_at": record.CreatedAt,
	})
}

// batchResultRow pairs a created short URL with its position in the request
// input. The embedded ShortUrl keeps the previous response shape (record fields
// are inlined) while Index lets callers locate the row unambiguously.
type batchResultRow struct {
	Index int `json:"index"`
	*model.ShortUrl
}

// BatchCreatePublic batch-creates short URLs through the API-key-authenticated
// public endpoint. Returns per-row results and errors.
func (h *ShortUrlHandler) BatchCreatePublic(c *gin.Context) {
	var req BatchCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "urls 数组不能为空（1-100 条）")
		return
	}

	outcomes := h.svc.BatchCreatePublicAPI(req.URLs, req.DomainID, c.ClientIP())

	out := make([]gin.H, 0, len(outcomes))
	errItems := make([]gin.H, 0)
	errList := make([]string, 0)
	for _, o := range outcomes {
		if o.Err != nil {
			// 同时返回结构化 error_items（携带输入下标）与兼容的字符串列表，
			// 便于调用方按行定位失败原因。
			errItems = append(errItems, gin.H{"index": o.Index, "message": o.Err.Error()})
			errList = append(errList, strconv.Itoa(o.Index)+": "+o.Err.Error())
			continue
		}
		if o.Record == nil {
			continue
		}
		out = append(out, gin.H{
			"index":     o.Index,
			"uid":       o.Record.UID,
			"short_url": o.Record.ShortURL,
			"long_url":  o.Record.LongURL,
		})
		h.dispatchCreated(o.Record)
	}
	pkg.Success(c, gin.H{
		"results":     out,
		"errors":      errList,
		"error_items": errItems,
		"total":       len(out),
	})
}

func (h *ShortUrlHandler) BatchCreate(c *gin.Context) {
	var req BatchCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "urls 数组不能为空（1-100 条）")
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

	outcomes := h.svc.BatchCreate(req.URLs, req.DomainID, &userID, c.ClientIP())

	// results 仅包含成功行，但每项带 index 明确对应输入下标；
	// error_items 同样带 index，调用方无需依赖两个数组的下标对齐。
	// 记录体保持原有完整字段（仅额外增加 index），不破坏既有调用方。
	results := make([]batchResultRow, 0, len(outcomes))
	errList := make([]string, 0)
	errItems := make([]gin.H, 0)
	for _, o := range outcomes {
		if o.Err != nil {
			errItems = append(errItems, gin.H{"index": o.Index, "message": o.Err.Error()})
			errList = append(errList, strconv.Itoa(o.Index)+": "+o.Err.Error())
			continue
		}
		if o.Record == nil {
			continue
		}
		results = append(results, batchResultRow{Index: o.Index, ShortUrl: o.Record})
		h.dispatchCreated(o.Record)
	}

	pkg.Success(c, gin.H{
		"results":     results,
		"errors":      errList,
		"error_items": errItems,
		"total":       len(results),
	})
	auditLog(c, h.auditSvc, "short_url", "short_url_batch_create", 0, `{"count":`+strconv.Itoa(len(results))+`}`)
}

type ImportRequest struct {
	Format   string  `json:"format" binding:"required,oneof=csv json"`
	Content  string  `json:"content" binding:"required"`
	DomainID *uint64 `json:"domain_id"`
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

	outcomes := h.svc.BatchImport(items, req.DomainID, &userID, c.ClientIP())
	results := make([]batchResultRow, 0, len(outcomes))
	errList := make([]string, 0)
	errItems := make([]gin.H, 0)
	for _, o := range outcomes {
		if o.Err != nil {
			errItems = append(errItems, gin.H{"index": o.Index, "message": o.Err.Error()})
			errList = append(errList, strconv.Itoa(o.Index)+": "+o.Err.Error())
			continue
		}
		if o.Record == nil {
			continue
		}
		results = append(results, batchResultRow{Index: o.Index, ShortUrl: o.Record})
		h.dispatchCreated(o.Record)
	}
	pkg.Success(c, gin.H{
		"results":     results,
		"errors":      errList,
		"error_items": errItems,
		"total":       len(results),
	})
	auditLog(c, h.auditSvc, "short_url", "short_url_import", 0, `{"count":`+strconv.Itoa(len(results))+`}`)
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
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "ids 不能为空")
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
		// A PublicSyncError means the admin-side delete succeeded but the public
		// wjoy_log mirror did not. Report it as a partial success so the operator
		// sees the affected UID and knows the periodic reconcile will retry,
		// instead of an opaque 400 that hides the successful local delete.
		var syncErr *service.PublicSyncError
		if errors.As(err, &syncErr) {
			auditLog(c, h.auditSvc, "short_url", "short_url_delete", id, `{"public_sync_failed":true}`)
			pkg.Success(c, gin.H{
				"deleted":            true,
				"public_sync_failed": true,
				"sync_failed_uids":   syncErr.UIDs,
				"warning":            syncErr.Error(),
			})
			return
		}
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	auditLog(c, h.auditSvc, "short_url", "short_url_delete", id, "")
	pkg.Success(c, gin.H{"deleted": true, "public_sync": true})
}

func (h *ShortUrlHandler) BatchDelete(c *gin.Context) {
	var req BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "ids 数组不能为空")
		return
	}

	if err := h.svc.BatchDelete(req.IDs); err != nil {
		// Local rows are already deleted; only the public wjoy_log mirror failed.
		// Return a partial success carrying the affected UIDs so the console can
		// report "local delete done, public sync pending" instead of a 400.
		var syncErr *service.PublicSyncError
		if errors.As(err, &syncErr) {
			auditLog(c, h.auditSvc, "short_url", "short_url_batch_delete", 0, `{"count":`+strconv.Itoa(len(req.IDs))+`,"public_sync_failed":true}`)
			pkg.Success(c, gin.H{
				"deleted":            true,
				"public_sync_failed": true,
				"sync_failed_uids":   syncErr.UIDs,
				"warning":            syncErr.Error(),
			})
			return
		}
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeBadRequest, err.Error())
		return
	}

	auditLog(c, h.auditSvc, "short_url", "short_url_batch_delete", 0, `{"count":`+strconv.Itoa(len(req.IDs))+`}`)
	pkg.Success(c, gin.H{"deleted": true, "public_sync": true})
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

// parseShortUrlFilters is the ONE place admin short-link filters are read from
// the query string. List and Export must share it: Export used to parse only
// keyword+status, so an operator who filtered by date or category and exported
// got the whole table instead of the rows on screen, and the "共 N 条" in the UI
// never matched the file (#26).
func parseShortUrlFilters(c *gin.Context) repository.ShortUrlFilters {
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
	return filters
}

func (h *ShortUrlHandler) List(c *gin.Context) {
	page, perPage := pkg.ParsePagination(c)
	filters := parseShortUrlFilters(c)

	list, total, err := h.svc.List(page, perPage, filters)
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "query failed")
		return
	}

	pkg.Paginated(c, list, total, page, perPage)
}

func (h *ShortUrlHandler) Export(c *gin.Context) {
	// #26: 与 List 共用同一份筛选解析，导出必须等于屏幕上看到的那些行。
	data, err := h.svc.Export(parseShortUrlFilters(c))
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "export failed")
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=short_urls_export.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", data)
}
