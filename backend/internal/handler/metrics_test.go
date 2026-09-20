package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// The exposition must be parseable by any Prometheus scraper, which means every
// metric line has the form "<name> <value>" and is preceded by HELP/TYPE. A
// malformed line makes the whole scrape fail, so this guards the format rather
// than the individual numbers.
func TestMetricsEndpointExpositionFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// A queue that no Redis/DB is attached to: the handler must still render,
	// reporting the unavailable dependencies as 0 instead of failing.
	q := &ClickQueue{ch: make(chan ClickEvent, 8), logger: zap.NewNop()}
	h := NewMetricsHandler(nil, nil, nil, false, q)

	router := gin.New()
	router.GET("/metrics", h.Metrics)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("unexpected content type %q", ct)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"# TYPE dwz_up gauge",
		"dwz_up 1",
		"dwz_db_up 0",           // nil db -> unavailable
		"dwz_redis_configured 0", // not configured
		"dwz_redis_up 0",        // nil redis -> unavailable
		"dwz_click_queue_capacity 8",
		"# TYPE dwz_clicks_dropped_total counter",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q\n---\n%s", want, body)
		}
	}

	// Every non-comment line must have exactly two fields.
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if fields := strings.Fields(line); len(fields) != 2 {
			t.Errorf("malformed metric line %q (want 2 fields, got %d)", line, len(fields))
		}
	}
}

// Drops are the signal operators alert on, so they must actually be counted.
func TestClickQueueCountsDropsWhenFull(t *testing.T) {
	q := &ClickQueue{ch: make(chan ClickEvent, 1), logger: zap.NewNop()}
	q.Enqueue(ClickEvent{UID: "a"}) // fills the buffer
	q.Enqueue(ClickEvent{UID: "b"}) // must be dropped
	q.Enqueue(ClickEvent{UID: "c"}) // must be dropped

	st := q.Stats()
	if st.Dropped != 2 {
		t.Fatalf("want 2 dropped events, got %d", st.Dropped)
	}
	if st.Pending != 1 {
		t.Fatalf("want 1 pending event, got %d", st.Pending)
	}
	if st.Capacity != 1 {
		t.Fatalf("want capacity 1, got %d", st.Capacity)
	}
}

// Scraping must be cheap and repeatable; a second scrape should succeed and
// report the counter advanced rather than erroring.
func TestMetricsEndpointIsRepeatable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewMetricsHandler(nil, nil, nil, false, nil)
	router := gin.New()
	router.GET("/metrics", h.Metrics)

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("scrape %d: want 200, got %d", i+1, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "dwz_up 1") {
			t.Fatalf("scrape %d: missing dwz_up", i+1)
		}
		time.Sleep(time.Millisecond)
	}
}

// #58: an unconfigured Redis must be distinguishable from a configured-but-down
// one, otherwise the DwzRedisDown alert fires forever on deployments that
// intentionally run without Redis.
func TestMetricsRedisConfiguredDistinguishesUnconfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	render := func(configured bool) string {
		h := NewMetricsHandler(nil, nil, nil, configured, nil)
		router := gin.New()
		router.GET("/metrics", h.Metrics)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		return rec.Body.String()
	}

	if body := render(false); !strings.Contains(body, "dwz_redis_configured 0") {
		t.Errorf("unconfigured Redis should report dwz_redis_configured 0\n%s", body)
	}
	if body := render(true); !strings.Contains(body, "dwz_redis_configured 1") {
		t.Errorf("configured Redis should report dwz_redis_configured 1\n%s", body)
	}
}
