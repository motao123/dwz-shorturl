package handler

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// MetricsHandler exposes a minimal Prometheus text-format endpoint.
//
// Why hand-rolled instead of prometheus/client_golang: the project needs six
// numbers scraped by an external agent, not a full instrumentation framework.
// Pulling in the client library would add a dependency tree (and a second
// registry to keep in sync) for what is a ~60-line string builder. The output
// below is the standard text exposition format, so any Prometheus-compatible
// scraper reads it unchanged.
//
// Scraped metrics (all prefixed dwz_):
//
//	dwz_up                       1 when the process is serving
//	dwz_goroutines               current goroutine count
//	dwz_db_up                    1 when the admin DB answers a ping
//	dwz_public_db_up             1 when the public DB answers a ping (same as db_up in single-DB mode)
//	dwz_redis_configured         1 when Redis is configured (a Redis address is set)
//	dwz_redis_up                 1 when Redis answers a ping
//	dwz_click_queue_pending      events waiting to be flushed
//	dwz_click_queue_capacity     queue buffer size
//	dwz_clicks_dropped_total     clicks lost because the queue was full
//	dwz_clicks_persisted_total   clicks successfully written
//	dwz_clicks_failed_total      clicks whose insert failed
//
// Suggested alerts (see deploy/prometheus-alerts.yml):
//   - dwz_clicks_dropped_total increasing  → queue saturated, clicks are being lost
//   - dwz_up == 0                          → process down
//   - dwz_db_up == 0 for 1m                → admin DB unreachable
type MetricsHandler struct {
	db         *gorm.DB
	publicDB   *gorm.DB
	rdb        *redis.Client
	// redisConfigured distinguishes "Redis was intentionally not configured"
	// from "Redis is configured but unreachable" (#58). Before this, the
	// dwz_redis_up gauge read 0 in both cases, so the RedisDown alert fired
	// permanently on deployments without Redis — alert fatigue that trains
	// operators to ignore the rule (and the real outage hides behind it).
	redisConfigured bool
	clickQueue *ClickQueue
	startedAt  time.Time
	scrapes    int64
}

func NewMetricsHandler(db, publicDB *gorm.DB, rdb *redis.Client, redisConfigured bool, clickQueue *ClickQueue) *MetricsHandler {
	return &MetricsHandler{
		db:              db,
		publicDB:        publicDB,
		rdb:             rdb,
		redisConfigured: redisConfigured,
		clickQueue:      clickQueue,
		startedAt:       time.Now(),
	}
}

// Metrics renders the exposition. It is deliberately cheap: one DB ping and one
// Redis ping, both short-timeout, so a scrape never piles up.
func (h *MetricsHandler) Metrics(c *gin.Context) {
	atomic.AddInt64(&h.scrapes, 1)

	var b strings.Builder
	writeMetric := func(name, help, typ string, value float64, labels string) {
		fmt.Fprintf(&b, "# HELP %s %s\n", name, help)
		fmt.Fprintf(&b, "# TYPE %s %s\n", name, typ)
		if labels != "" {
			fmt.Fprintf(&b, "%s{%s} %g\n", name, labels, value)
		} else {
			fmt.Fprintf(&b, "%s %g\n", name, value)
		}
	}

	writeMetric("dwz_up", "1 when the process is serving requests.", "gauge", 1, "")
	writeMetric("dwz_goroutines", "Current number of goroutines.", "gauge", float64(runtime.NumGoroutine()), "")
	writeMetric("dwz_uptime_seconds", "Seconds since the process started serving.", "gauge", time.Since(h.startedAt).Seconds(), "")

	// DB reachability: the single most useful alert signal (the redirect path is
	// useless without the admin DB even though the process stays "up").
	dbUp := 0.0
	if h.db != nil {
		if sqlDB, err := h.db.DB(); err == nil && sqlDB != nil {
			if pingErr := sqlDB.Ping(); pingErr == nil {
				dbUp = 1
			}
		}
	}
	writeMetric("dwz_db_up", "1 when the admin database answers a ping.", "gauge", dbUp, "")

	// Public DB reachability (#58). In single-DB mode this is the same handle,
	// so it reads the same value; in split-DB mode a public-DB outage (which
	// silently breaks redirects/stats) is otherwise invisible on /metrics.
	if h.publicDB != nil && h.publicDB != h.db {
		pubUp := 0.0
		if sqlDB, err := h.publicDB.DB(); err == nil && sqlDB != nil {
			if pingErr := sqlDB.Ping(); pingErr == nil {
				pubUp = 1
			}
		}
		writeMetric("dwz_public_db_up", "1 when the public database answers a ping.", "gauge", pubUp, "")
	}

	// Redis reachability, split into "configured" and "up" (#58) so an
	// unconfigured Redis is not mistaken for an outage.
	redisConfigured := 0.0
	if h.redisConfigured {
		redisConfigured = 1
	}
	writeMetric("dwz_redis_configured", "1 when Redis is configured (address set).", "gauge", redisConfigured, "")

	redisUp := 0.0
	if h.rdb != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := h.rdb.Ping(ctx).Err(); err == nil {
			redisUp = 1
		}
	}
	writeMetric("dwz_redis_up", "1 when Redis answers a ping (0 when unconfigured/unreachable).", "gauge", redisUp, "")

	// Click queue: drops are the actionable signal, so they are exported as a
	// counter (alert on rate) rather than a gauge.
	if h.clickQueue != nil {
		st := h.clickQueue.Stats()
		writeMetric("dwz_click_queue_pending", "Click events waiting to be flushed.", "gauge", float64(st.Pending), "")
		writeMetric("dwz_click_queue_capacity", "Click queue buffer size.", "gauge", float64(st.Capacity), "")
		writeMetric("dwz_clicks_dropped_total", "Clicks dropped because the queue was full.", "counter", float64(st.Dropped), "")
		writeMetric("dwz_clicks_persisted_total", "Clicks successfully written to click_logs.", "counter", float64(st.Persisted), "")
		writeMetric("dwz_clicks_failed_total", "Clicks whose database insert failed.", "counter", float64(st.Failed), "")
	}

	c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	c.String(http.StatusOK, b.String())
}
