package handler

import (
	"context"
	"net/http"
	"regexp"
	"runtime"
	"sync"
	"time"

	"dwz-admin/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
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
	ch      chan ClickEvent
	db      *gorm.DB
	logger  *zap.Logger
	wg      sync.WaitGroup
	stopCtx context.Context
	cancel  context.CancelFunc
	pending int64 // atomically tracked count of queued events
	mu      sync.Mutex
}

const clickQueueSize = 2048

func NewClickQueue(db *gorm.DB, logger *zap.Logger) *ClickQueue {
	ctx, cancel := context.WithCancel(context.Background())
	q := &ClickQueue{
		ch:     make(chan ClickEvent, clickQueueSize),
		db:     db,
		logger: logger,
		stopCtx: ctx,
		cancel:  cancel,
	}
	q.wg.Add(1)
	go q.worker()
	return q
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
		})
		uidToID[e.UID] = e.ShortUrlID
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
