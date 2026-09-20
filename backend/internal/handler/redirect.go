package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dwz-admin/internal/config"
	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"golang.org/x/crypto/bcrypt"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// --- Click event queueing (replaces bare goroutines) ---

// ClickEvent represents a single click to be persisted asynchronously.
type ClickEvent struct {
	ShortUrlID uint64
	UID        string // used as fallback lookup if ShortUrlID is 0
	IP         string
	UserAgent  string
	Referer    string
}

// ClickQueue is a buffered, non-blocking channel-backed click event queue.
type ClickQueue struct {
	ch          chan ClickEvent
	db          *gorm.DB
	logger      *zap.Logger
	geoCountry  func(ip string) string // optional GeoIP country (ISO alpha-2)
	geoCache    sync.Map               // ip → country, bounded by geoCacheCap
	wg          sync.WaitGroup
	stopCtx     context.Context
	cancel      context.CancelFunc
	pending     int64 // atomically tracked count of queued events
	mu          sync.Mutex
	onClick     func(uid string) // optional webhook callback per click

	// Counters exposed through /metrics. They are the ones an operator actually
	// needs to alert on: a rising drop count means the queue is saturated (and
	// clicks are being lost), a rising persisted count shows the drain rate.
	dropped   int64 // events discarded because the queue was full
	persisted int64 // events successfully written to click_logs
	failed    int64 // events whose insert failed (dropped after retries)
}

const (
	clickQueueSize = 2048
	geoCacheCap    = 4096
)

func NewClickQueue(db *gorm.DB, logger *zap.Logger) *ClickQueue {
	ctx, cancel := context.WithCancel(context.Background())
	q := &ClickQueue{
		ch:      make(chan ClickEvent, clickQueueSize),
		db:      db,
		logger:  logger,
		stopCtx: ctx,
		cancel:  cancel,
	}
	q.wg.Add(1)
	go q.worker()
	return q
}

// SetGeoCountry registers the GeoIP country resolver (ip → ISO 3166-1 alpha-2,
// "" when unknown). Results are cached per IP to keep the redirect hot path cheap.
func (q *ClickQueue) SetGeoCountry(fn func(ip string) string) {
	q.geoCountry = fn
}

// countryFor returns the cached (or resolved) country for an IP.
func (q *ClickQueue) countryFor(ip string) string {
	if q.geoCountry == nil || ip == "" {
		return ""
	}
	if v, ok := q.geoCache.Load(ip); ok {
		return v.(string)
	}
	c := q.geoCountry(ip)
	if size := q.cacheLen(); size > geoCacheCap {
		q.geoCache.Range(func(k, _ interface{}) bool {
			q.geoCache.Delete(k)
			return false // 清空后停止遍历
		})
	}
	q.geoCache.Store(ip, c)
	return c
}

func (q *ClickQueue) cacheLen() int {
	n := 0
	q.geoCache.Range(func(_, _ interface{}) bool {
		n++
		return true
	})
	return n
}

// SetOnClick registers a callback invoked once per recorded click (used to
// dispatch link.clicked webhooks). Safe to call before the server starts.
func (q *ClickQueue) SetOnClick(fn func(uid string)) {
	q.mu.Lock()
	q.onClick = fn
	q.mu.Unlock()
}

// Enqueue sends a click event non-blockingly. If the queue is full the
// event is dropped (with a warning log) instead of blocking the request.
func (q *ClickQueue) Enqueue(evt ClickEvent) {
	select {
	case q.ch <- evt:
	default:
		atomic.AddInt64(&q.dropped, 1)
		q.logger.Warn("click queue full, dropping event", zap.String("uid", evt.UID))
	}
}

// ClickQueueStats is a point-in-time snapshot of the queue's counters, used by
// /metrics and the health endpoint.
type ClickQueueStats struct {
	Pending   int
	Capacity  int
	Dropped   int64
	Persisted int64
	Failed    int64
}

// Stats returns the current queue counters.
func (q *ClickQueue) Stats() ClickQueueStats {
	return ClickQueueStats{
		Pending:   len(q.ch),
		Capacity:  cap(q.ch),
		Dropped:   atomic.LoadInt64(&q.dropped),
		Persisted: atomic.LoadInt64(&q.persisted),
		Failed:    atomic.LoadInt64(&q.failed),
	}
}

func (q *ClickQueue) PendingCount() int {
	return len(q.ch)
}

func (q *ClickQueue) Stop() {
	q.cancel()
	close(q.ch)
	q.wg.Wait()
}

// worker consumes events from the channel in batches.
func (q *ClickQueue) worker() {
	defer q.wg.Done()

	const (
		batchSize     = 100
		flushInterval = 2 * time.Second
	)

	batch := make([]ClickEvent, 0, batchSize)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		q.persist(batch)
		batch = batch[:0] // reuse underlying array
	}

	for {
		select {
		case evt, ok := <-q.ch:
			if !ok {
				flush()
				return
			}
			batch = append(batch, evt)
			if len(batch) >= batchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}

// persist writes a batch of click events to the database.
func (q *ClickQueue) persist(events []ClickEvent) {
	// Resolve any zero IDs via uid lookup
	for i := range events {
		if events[i].ShortUrlID == 0 && events[i].UID != "" {
			var id uint64
			if err := q.db.Model(&model.ShortUrl{}).
				Where("uid = ?", events[i].UID).
				Limit(1).
				Pluck("id", &id).Error; err == nil && id > 0 {
				events[i].ShortUrlID = id
			}
		}
	}

	// Batch insert click logs
	clickLogs := make([]*model.ClickLog, 0, len(events))
	uidToID := make(map[string]uint64, len(events))
	for _, e := range events {
		if e.ShortUrlID == 0 {
			continue // could not resolve, skip
		}
		clickLogs = append(clickLogs, &model.ClickLog{
			ShortUrlID: e.ShortUrlID,
			IP:         e.IP,
			UserAgent:  e.UserAgent,
			Referer:    e.Referer,
			Country:    q.countryFor(e.IP),
		})
		uidToID[e.UID] = e.ShortUrlID
	}

	// Fire the per-click webhook callback (link.clicked) for each recorded click.
	q.mu.Lock()
	onClick := q.onClick
	q.mu.Unlock()
	if onClick != nil {
		for uid := range uidToID {
			onClick(uid)
		}
	}

	if len(clickLogs) > 0 {
		if err := q.db.CreateInBatches(clickLogs, 100).Error; err != nil {
			q.logger.Error("batch insert click logs failed", zap.Error(err))
			atomic.AddInt64(&q.failed, int64(len(clickLogs)))
		} else {
			atomic.AddInt64(&q.persisted, int64(len(clickLogs)))
		}
	}

	// Increment click counters. Multiple clicks on the same link may land in
	// the same batch, so count them per short_url_id and add the real number
	// (clicks + n). Deduplicating ids and incrementing by 1 each would
	// permanently undercount the counter relative to the click_logs rows.
	for id, n := range countClicksPerURL(events) {
		if err := q.db.Model(&model.ShortUrl{}).
			Where("id = ?", id).
			UpdateColumn("clicks", gorm.Expr("clicks + ?", n)).Error; err != nil {
			q.logger.Error("increment clicks failed",
				zap.Uint64("id", id), zap.Uint32("count", n), zap.Error(err))
		}
	}
}

// countClicksPerURL aggregates a batch of click events into
// short_url_id -> number of clicks. Events without a resolvable id are skipped.
func countClicksPerURL(events []ClickEvent) map[uint64]uint32 {
	counts := make(map[uint64]uint32, len(events))
	for _, e := range events {
		if e.ShortUrlID != 0 {
			counts[e.ShortUrlID]++
		}
	}
	return counts
}

// --- Redirect Handler ---

var shortCodeRegex = regexp.MustCompile(`^[a-z0-5]{6,8}$`)

type RedirectHandler struct {
	svc        ShortUrlResolver
	rdb        *redis.Client
	db         *gorm.DB
	logger     *zap.Logger
	clickQueue *ClickQueue
	// 密码保护短链的尝试限流（P2-12）：此前可无限次尝试，没有任何节流。
	// 复用限流器实现（Redis 计数），Redis 不可用时退化为不限流（保证跳转可用）。
	pwLimiter *pkg.RateLimiter
}

// passwordAttemptMax / passwordAttemptWindow：每个 IP + 短码 60 秒内最多 5 次。
// 阈值取「够正常人手误」且「远低于暴力破解成本」。
const (
	passwordAttemptMax    = 5
	passwordAttemptWindow = time.Minute
)

// ShortUrlResolver is the minimal interface the redirect handler needs
// (decoupled from the full ShortUrlService to keep redirect.go testable).
type ShortUrlResolver interface {
	ResolveByUID(uid string) (*model.ShortUrl, error)
}

func NewRedirectHandler(svc ShortUrlResolver, rdb *redis.Client, db *gorm.DB, logger *zap.Logger, clickQueue *ClickQueue) *RedirectHandler {
	h := &RedirectHandler{
		svc:        svc,
		rdb:        rdb,
		db:         db,
		logger:     logger,
		clickQueue: clickQueue,
	}
	if rdb != nil {
		h.pwLimiter = pkg.NewRateLimiter(rdb)
	}
	return h
}

// passwordAttemptAllowed consumes one attempt for (client IP, short code).
// A nil limiter (no Redis) means "unlimited" so the redirect path never breaks.
func (h *RedirectHandler) passwordAttemptAllowed(c *gin.Context, uid string) bool {
	if h.pwLimiter == nil {
		return true
	}
	key := "pwtry:" + c.ClientIP() + ":" + uid
	ok, err := h.pwLimiter.Allow(c.Request.Context(), key, passwordAttemptMax, passwordAttemptWindow)
	if err != nil {
		// 限流器故障不应把访客挡在门外（可用性优先），记录后放行
		if h.logger != nil {
			h.logger.Warn("password attempt limiter failed", zap.Error(err))
		}
		return true
	}
	return ok
}

func (h *RedirectHandler) Redirect(c *gin.Context) {
	code := c.Param("code")

	// Strict short-code format validation (matches PHP's do.php)
	if !shortCodeRegex.MatchString(code) {
		renderErrorPage(c, http.StatusNotFound, errorPageNotFound)
		return
	}

	record, err := h.svc.ResolveByUID(code)
	if err != nil {
		msg := err.Error()
		switch {
		case contains(msg, "expired"):
			renderErrorPage(c, http.StatusGone, errorPageExpired)
			return
		case contains(msg, "disabled"):
			renderErrorPage(c, http.StatusGone, errorPageDisabled)
			return
		case contains(msg, "invalid"), contains(msg, "not allowed"):
			// 面向站长：目标 URL 未通过安全校验，与「已过期」区分文案
			renderErrorPage(c, http.StatusGone, errorPageInvalidTarget)
			return
		default:
			renderErrorPage(c, http.StatusNotFound, errorPageNotFound)
			return
		}
	}

	// Password-protected links: serve an unlock page until the visitor presents
	// the correct password. Clicks are only counted once the link is actually
	// redirected (the enqueue below), so password-page views don't pollute stats.
	if record.PasswordHash != "" {
		if !passwordUnlocked(c, record.UID) {
			if c.Request.Method == http.MethodPost {
				if !h.passwordAttemptAllowed(c, record.UID) {
					c.Header("Retry-After", strconv.Itoa(int(passwordAttemptWindow.Seconds())))
					c.Data(http.StatusTooManyRequests, "text/html; charset=utf-8",
						[]byte(renderPasswordPage(record.UID, "尝试次数过多，请稍后再试")))
					return
				}
				pw := c.PostForm("password")
				if bcrypt.CompareHashAndPassword([]byte(record.PasswordHash), []byte(pw)) == nil {
					setPasswordUnlockCookie(c, record.UID)
					// Re-request as GET; the freshly set cookie authorises the redirect.
					c.Redirect(http.StatusFound, c.Request.URL.Path)
					return
				}
				c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(renderPasswordPage(record.UID, "密码错误，请重试")))
				return
			}
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(renderPasswordPage(record.UID, "")))
			return
		}
	}

	// Defensive re-check: never serve a soft-deleted or expired record even if
	// the cache/DB layer handed one back (mirrors PHP do.php's explicit checks).
	if record.Status != 1 {
		renderErrorPage(c, http.StatusNotFound, errorPageNotFound)
		return
	}
	if record.ExpireAt != nil && !record.ExpireAt.After(time.Now()) {
		renderErrorPage(c, http.StatusGone, errorPageExpired)
		return
	}

	// Enqueue click event (non-blocking)
	h.clickQueue.Enqueue(ClickEvent{
		ShortUrlID: record.ID,
		UID:        code,
		IP:         c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
		Referer:    c.GetHeader("Referer"),
	})

	c.Redirect(http.StatusFound, record.LongURL)
}

// unlockCookieName returns the per-link password-unlock cookie name.
func unlockCookieName(uid string) string {
	return "dwz_plink_" + uid
}

const unlockCookieTTL = 30 * 24 * time.Hour

// setPasswordUnlockCookie issues a signed "unlocked" cookie for a link. The HMAC
// is keyed with the member secret shared between PHP and Go, so both redirect
// paths honour the same cookie.
func setPasswordUnlockCookie(c *gin.Context, uid string) {
	secret := config.Get().JWT.MemberSecret
	if secret == "" {
		return
	}
	expiry := time.Now().Add(unlockCookieTTL).Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s.%d", uid, expiry)
	token := fmt.Sprintf("%d.%x", expiry, mac.Sum(nil))
	c.SetCookie(unlockCookieName(uid), token, int(unlockCookieTTL.Seconds()), "/",
		"", c.Request.TLS != nil, true)
}

// passwordUnlocked reports whether the request carries a valid unlock cookie.
func passwordUnlocked(c *gin.Context, uid string) bool {
	secret := config.Get().JWT.MemberSecret
	if secret == "" {
		return false
	}
	raw, err := c.Cookie(unlockCookieName(uid))
	if err != nil || raw == "" {
		return false
	}
	parts := strings.SplitN(raw, ".", 2)
	if len(parts) != 2 {
		return false
	}
	expiry, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s.%d", uid, expiry)
	expected := fmt.Sprintf("%d.%x", expiry, mac.Sum(nil))
	return hmac.Equal([]byte(raw), []byte(expected))
}

// renderPasswordPage returns a lightweight, mobile-friendly unlock page. The
// form posts the password back to the same short URL; no third-party assets.
func renderPasswordPage(uid, errMsg string) string {
	// Escape every interpolation: callers may pass arbitrary strings in the
	// future, so treat them as untrusted (PHP's password_page_html does the same).
	safeUID := html.EscapeString(uid)
	safeErr := html.EscapeString(errMsg)
	title := "请输入访问密码"
	msg := ""
	if errMsg != "" {
		title = safeErr
		msg = `<p class="err">` + safeErr + `</p>`
	}
	// 与 PHP 侧 password_page_html 使用同一套 CSS 变量 + prefers-color-scheme，
	// 保证两条跳转路径观感一致（暗色环境下不再出现刺眼白卡）。
	return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + title + ` - 短网址</title>
<meta name="robots" content="noindex">
<style>
:root{--pw-page:#f2f5f7;--pw-card:#fff;--pw-line:#e4ecee;--pw-text:#16292b;--pw-dim:#6b7f86;--pw-input:#d3e0e3;--pw-brand:#0e6e75;--pw-brand-hover:#0a5a60;--pw-danger:#c0392b}
@media (prefers-color-scheme:dark){:root{--pw-page:#0d1b20;--pw-card:#122027;--pw-line:#23343b;--pw-text:#e6edf0;--pw-dim:#9aa9ae;--pw-input:#31454d;--pw-brand:#12909a;--pw-brand-hover:#0e6e75;--pw-danger:#f87171}}
*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;background:var(--pw-page);font-family:-apple-system,"PingFang SC","Microsoft YaHei",sans-serif;color:var(--pw-text)}
.card{width:min(92vw,360px);background:var(--pw-card);border:1px solid var(--pw-line);border-radius:14px;padding:28px 24px;box-shadow:0 8px 30px rgba(14,110,117,.08)}
.lock{font-size:34px;text-align:center;margin:0 0 6px}
h1{font-size:17px;text-align:center;margin:0 0 6px;font-weight:700}
.sub{font-size:12.5px;text-align:center;color:var(--pw-dim);margin:0 0 18px}
.err{color:var(--pw-danger);font-size:13px;margin:0 0 14px}
input{width:100%;padding:11px 12px;border:1px solid var(--pw-input);border-radius:8px;font-size:15px;outline:none;background:var(--pw-card);color:var(--pw-text)}
input:focus{border-color:var(--pw-brand);box-shadow:0 0 0 3px rgba(14,110,117,.12)}
button{width:100%;margin-top:12px;padding:11px;background:var(--pw-brand);color:#fff;border:0;border-radius:8px;font-size:15px;font-weight:600;cursor:pointer}
button:hover{background:var(--pw-brand-hover)}
.brand{display:block;margin-top:16px;text-align:center;font-size:12.5px;color:var(--pw-dim);text-decoration:none}
.brand:hover{color:var(--pw-brand)}
</style></head><body><form class="card" method="post" action="/` + safeUID + `">
<p class="lock">🔒</p><h1>此链接受密码保护</h1><p class="sub">请输入访问密码以继续</p>
` + msg + `<input type="password" name="password" placeholder="访问密码" required autofocus autocomplete="off">
<button type="submit">解锁访问</button>
<a class="brand" href="/">← 返回短网址首页</a>
</form></body></html>`
}

// Error page kinds. Kept as an enum so both the handler and tests can reference
// them without string matching.
type errorPageKind int

const (
	errorPageNotFound errorPageKind = iota
	errorPageExpired
	errorPageDisabled
	errorPageInvalidTarget
)

// errorPageContent maps a kind to the visitor-facing (title, description) pair.
// Expired/disabled are visitor-facing; invalid target is intended for the site
// owner but is shown to the visitor too (the owner is usually the one testing).
func errorPageContent(kind errorPageKind) (string, string, string) {
	switch kind {
	case errorPageExpired:
		return "这个短链已过期", "链接的有效期已结束。请联系分享者重新生成一条。", "⏳"
	case errorPageDisabled:
		return "这个短链已被停用", "分享者或平台已停用该链接。如有疑问请联系分享者。", "🚫"
	case errorPageInvalidTarget:
		return "这个短链暂时无法访问", "目标地址未通过安全检查（可能指向内网或非法站点）。如果这是你自己的链接，请重新创建。", "⚠️"
	default:
		return "短链不存在", "这个短码没有对应的链接，可能输入有误或已被删除。", "🔍"
	}
}

// renderErrorPage writes a branded, dark-mode-aware error page instead of a bare
// text body. The short-link landing page is the first screen a visitor sees
// after clicking a shared link, so it carries the same brand as the homepage.
func renderErrorPage(c *gin.Context, status int, kind errorPageKind) {
	title, desc, icon := errorPageContent(kind)
	body := `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + html.EscapeString(title) + ` - 短网址</title>
<meta name="robots" content="noindex">
<meta name="referrer" content="no-referrer">
<style>
:root{--ep-page:#f2f5f7;--ep-card:#fff;--ep-line:#e4ecee;--ep-text:#16292b;--ep-dim:#6b7f86;--ep-brand:#0e6e75;--ep-brand-hover:#0a5a60}
@media (prefers-color-scheme:dark){:root{--ep-page:#0d1b20;--ep-card:#122027;--ep-line:#23343b;--ep-text:#e6edf0;--ep-dim:#9aa9ae;--ep-brand:#12909a;--ep-brand-hover:#0e6e75}}
*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;background:var(--ep-page);font-family:-apple-system,"PingFang SC","Microsoft YaHei",sans-serif;color:var(--ep-text)}
.card{width:min(92vw,400px);background:var(--ep-card);border:1px solid var(--ep-line);border-radius:14px;padding:32px 26px;text-align:center;box-shadow:0 8px 30px rgba(14,110,117,.08)}
.icon{font-size:36px;margin:0 0 10px}
h1{font-size:18px;margin:0 0 10px;font-weight:700}
p{font-size:13.5px;line-height:1.7;color:var(--ep-dim);margin:0 0 22px}
.actions{display:flex;gap:10px;flex-direction:column}
a.btn{display:block;padding:11px;border-radius:8px;font-size:14px;font-weight:600;text-decoration:none;background:var(--ep-brand);color:#fff}
a.btn:hover{background:var(--ep-brand-hover)}
a.btn.ghost{background:transparent;color:var(--ep-brand);border:1px solid var(--ep-line)}
.slogan{margin-top:18px;font-size:12px;color:var(--ep-dim)}
</style></head><body><div class="card">
<p class="icon">` + icon + `</p>
<h1>` + html.EscapeString(title) + `</h1>
<p>` + html.EscapeString(desc) + `</p>
<div class="actions">
<a class="btn" href="/">返回首页</a>
<a class="btn ghost" href="/#single">重新生成短链</a>
</div>
<p class="slogan">短网址 · 一次生成，随处链接</p>
</div></body></html>`
	c.Data(status, "text/html; charset=utf-8", []byte(body))
}

// Health returns detailed system health information.
func (h *RedirectHandler) Health(c *gin.Context) {
	status := gin.H{
		"status":    "ok",
		"goroutines": runtime.NumGoroutine(),
		"time":       time.Now().UTC().Format(time.RFC3339),
	}

	// DB health
	sqlDB, err := h.db.DB()
	if err == nil && sqlDB != nil {
		dbStats := sqlDB.Stats()
		status["db"] = gin.H{
			"healthy":        true,
			"open_conns":     dbStats.OpenConnections,
			"in_use":         dbStats.InUse,
			"idle":           dbStats.Idle,
		}
	} else {
		status["db"] = gin.H{"healthy": false}
	}

	// Redis health
	if h.rdb != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := h.rdb.Ping(ctx).Err(); err == nil {
			status["redis"] = gin.H{"healthy": true}
		} else {
			status["redis"] = gin.H{"healthy": false, "error": err.Error()}
		}
	} else {
		status["redis"] = gin.H{"healthy": false, "error": "not configured"}
	}

	// Click queue pending count
	if h.clickQueue != nil {
		status["click_queue_pending"] = h.clickQueue.PendingCount()
	}

	c.JSON(http.StatusOK, status)
}

// --- helpers ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsFold(s, substr))
}

func containsFold(s, substr string) bool {
	ls := []byte(s)
	lsub := []byte(substr)
	for i := 0; i+len(lsub) <= len(ls); i++ {
		match := true
		for j := 0; j < len(lsub); j++ {
			if toLower(ls[i+j]) != toLower(lsub[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func toLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}
