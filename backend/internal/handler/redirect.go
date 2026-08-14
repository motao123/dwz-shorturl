package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"dwz-admin/internal/config"
	"dwz-admin/internal/model"

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
		q.logger.Warn("click queue full, dropping event", zap.String("uid", evt.UID))
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
		}
	}

	// Increment click counters (group by short_url_id)
	uidSet := make(map[uint64]bool, len(uidToID))
	for _, id := range uidToID {
		uidSet[id] = true
	}
	for id := range uidSet {
		if err := q.db.Model(&model.ShortUrl{}).
			Where("id = ?", id).
			UpdateColumn("clicks", gorm.Expr("clicks + 1")).Error; err != nil {
			q.logger.Error("increment clicks failed", zap.Uint64("id", id), zap.Error(err))
		}
	}
}

// --- Redirect Handler ---

var shortCodeRegex = regexp.MustCompile(`^[a-z0-5]{6,8}$`)

type RedirectHandler struct {
	svc       ShortUrlResolver
	rdb       *redis.Client
	db        *gorm.DB
	logger    *zap.Logger
	clickQueue *ClickQueue
}

// ShortUrlResolver is the minimal interface the redirect handler needs
// (decoupled from the full ShortUrlService to keep redirect.go testable).
type ShortUrlResolver interface {
	ResolveByUID(uid string) (*model.ShortUrl, error)
}

func NewRedirectHandler(svc ShortUrlResolver, rdb *redis.Client, db *gorm.DB, logger *zap.Logger, clickQueue *ClickQueue) *RedirectHandler {
	return &RedirectHandler{
		svc:        svc,
		rdb:        rdb,
		db:         db,
		logger:     logger,
		clickQueue: clickQueue,
	}
}

func (h *RedirectHandler) Redirect(c *gin.Context) {
	code := c.Param("code")

	// Strict short-code format validation (matches PHP's do.php)
	if !shortCodeRegex.MatchString(code) {
		c.String(http.StatusNotFound, "Not Found")
		return
	}

	record, err := h.svc.ResolveByUID(code)
	if err != nil {
		msg := err.Error()
		switch {
		case contains(msg, "expired"):
			c.String(http.StatusGone, "短链已过期")
			return
		case contains(msg, "disabled"):
			c.String(http.StatusGone, "短链已禁用")
			return
		case contains(msg, "invalid"), contains(msg, "not allowed"):
			c.String(http.StatusGone, "短链目标无效")
			return
		default:
			c.String(http.StatusNotFound, "Not Found")
			return
		}
	}

	// Password-protected links: serve an unlock page until the visitor presents
	// the correct password. Clicks are only counted once the link is actually
	// redirected (the enqueue below), so password-page views don't pollute stats.
	if record.PasswordHash != "" {
		if !passwordUnlocked(c, record.UID) {
			if c.Request.Method == http.MethodPost {
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
	title := "请输入访问密码"
	msg := ""
	if errMsg != "" {
		title = errMsg
		msg = `<p style="color:#c0392b;font-size:13px;margin:0 0 14px;">` + errMsg + `</p>`
	}
	return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + title + `</title>
<style>
*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;background:#f2f5f7;font-family:-apple-system,"PingFang SC","Microsoft YaHei",sans-serif;color:#16292b}
.card{width:min(92vw,360px);background:#fff;border:1px solid #e4ecee;border-radius:14px;padding:28px 24px;box-shadow:0 8px 30px rgba(14,110,117,.08)}
.lock{font-size:34px;text-align:center;margin:0 0 6px}
h1{font-size:17px;text-align:center;margin:0 0 6px;font-weight:700}
.sub{font-size:12.5px;text-align:center;color:#6b7f86;margin:0 0 18px}
input{width:100%;padding:11px 12px;border:1px solid #d3e0e3;border-radius:8px;font-size:15px;outline:none}
input:focus{border-color:#0e6e75;box-shadow:0 0 0 3px rgba(14,110,117,.12)}
button{width:100%;margin-top:12px;padding:11px;background:#0e6e75;color:#fff;border:0;border-radius:8px;font-size:15px;font-weight:600;cursor:pointer}
button:hover{background:#0a5a60}
</style></head><body><form class="card" method="post" action="/` + uid + `">
<p class="lock">🔒</p><h1>此链接受密码保护</h1><p class="sub">请输入访问密码以继续</p>
` + msg + `<input type="password" name="password" placeholder="访问密码" required autofocus autocomplete="off">
<button type="submit">解锁访问</button>
</form></body></html>`
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
