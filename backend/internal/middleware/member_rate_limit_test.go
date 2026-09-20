package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"dwz-admin/internal/pkg"

	"github.com/gin-gonic/gin"
)

// memCounter is an in-memory pkg.Counter so the limiter can be exercised without
// Redis. It mirrors the fake used in pkg's own tests but lives here because
// pkg.fakeCounter is unexported.
type memCounter struct {
	mu     sync.Mutex
	counts map[string]int64
}

func newMemCounter() *memCounter {
	return &memCounter{counts: map[string]int64{}}
}

func (m *memCounter) IncrBy(_ context.Context, key string, n int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counts[key] += n
	return m.counts[key], nil
}

func (m *memCounter) Expire(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

func (m *memCounter) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.counts, key)
	return nil
}

func (m *memCounter) Get(_ context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.counts[key], nil
}

func newMemberLimiter(t *testing.T, max int, window time.Duration) (*MemberRateLimiter, *memCounter) {
	t.Helper()
	mc := newMemCounter()
	limiter := pkg.NewRateLimiterWithCounter(mc)
	return NewMemberRateLimiter(limiter, func() (int, time.Duration) { return max, window }), mc
}

func runRequest(t *testing.T, rl *MemberRateLimiter, memberID uint64, ip string) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if memberID > 0 {
			c.Set("member_id", memberID)
		}
		c.Next()
	})
	r.Use(RateLimitMember(rl))
	r.GET("/member/api/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/member/api/test", nil)
	req.RemoteAddr = ip + ":12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

// TestMemberRateLimit_PerTokenNotPerIP is the core contract of #8/#37: the quota
// is spent per member identity, so two members sharing one (NAT) IP do not
// consume each other's budget.
func TestMemberRateLimit_PerTokenNotPerIP(t *testing.T) {
	rl, _ := newMemberLimiter(t, 2, time.Minute)
	const sharedIP = "203.0.113.9"

	if code := runRequest(t, rl, 1, sharedIP); code != http.StatusOK {
		t.Fatalf("member 1 first call: want 200, got %d", code)
	}
	if code := runRequest(t, rl, 1, sharedIP); code != http.StatusOK {
		t.Fatalf("member 1 second call: want 200, got %d", code)
	}
	// Member 1 is now over quota...
	if code := runRequest(t, rl, 1, sharedIP); code != http.StatusTooManyRequests {
		t.Fatalf("member 1 third call: want 429, got %d", code)
	}
	// ...but member 2 on the SAME IP still has a full budget. Under an
	// IP-scoped limiter this would already be a 429.
	if code := runRequest(t, rl, 2, sharedIP); code != http.StatusOK {
		t.Fatalf("member 2 first call on shared IP: want 200, got %d", code)
	}
}

// TestMemberRateLimit_FallsBackToIPWithoutIdentity covers the public auth routes
// (forgot-password etc.) where MemberAuth has not run: with no member_id the
// limiter must still bound the caller by IP instead of being a no-op.
func TestMemberRateLimit_FallsBackToIPWithoutIdentity(t *testing.T) {
	rl, _ := newMemberLimiter(t, 1, time.Minute)

	if code := runRequest(t, rl, 0, "198.51.100.4"); code != http.StatusOK {
		t.Fatalf("first anonymous call: want 200, got %d", code)
	}
	if code := runRequest(t, rl, 0, "198.51.100.4"); code != http.StatusTooManyRequests {
		t.Fatalf("second anonymous call: want 429, got %d", code)
	}
	// A different IP is unaffected.
	if code := runRequest(t, rl, 0, "198.51.100.5"); code != http.StatusOK {
		t.Fatalf("other IP: want 200, got %d", code)
	}
}

// TestMemberRateLimit_ZeroDisables documents the operator opt-out: max 0 means
// "unlimited" (the config page exposes it), not "reject everything".
func TestMemberRateLimit_ZeroDisables(t *testing.T) {
	rl, _ := newMemberLimiter(t, 0, time.Minute)
	for i := 0; i < 10; i++ {
		if code := runRequest(t, rl, 7, "203.0.113.7"); code != http.StatusOK {
			t.Fatalf("call %d with max=0: want 200 (unlimited), got %d", i, code)
		}
	}
}

// TestMemberRateLimit_NilIsNoOp keeps the "deployment without Redis" case from
// panicking: a nil limiter must pass traffic through.
func TestMemberRateLimit_NilIsNoOp(t *testing.T) {
	if code := runRequest(t, nil, 0, "203.0.113.8"); code != http.StatusOK {
		t.Fatalf("nil limiter: want 200, got %d", code)
	}
}
