package pkg

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeCounter is an in-memory Counter for exercising the limiter without Redis.
type fakeCounter struct {
	mu     sync.Mutex
	counts map[string]int64
	ttl    map[string]time.Time
}

func newFakeCounter() *fakeCounter {
	return &fakeCounter{counts: map[string]int64{}, ttl: map[string]time.Time{}}
}

func (f *fakeCounter) IncrBy(_ context.Context, key string, n int64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Simulate window expiry: if the stored TTL has passed, reset to 0.
	if exp, ok := f.ttl[key]; ok && time.Now().After(exp) {
		f.counts[key] = 0
	}
	// A key with no TTL yet behaves like a fresh window.
	f.counts[key] += n
	return f.counts[key], nil
}

func (f *fakeCounter) Expire(_ context.Context, key string, ttl time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ttl[key] = time.Now().Add(ttl)
	return true, nil
}

func newTestLimiter() (*RateLimiter, *fakeCounter) {
	fc := newFakeCounter()
	return NewRateLimiterWithCounter(fc), fc
}

func TestAllow_WithinLimit(t *testing.T) {
	rl, _ := newTestLimiter()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		ok, err := rl.Allow(ctx, "ip:1.2.3.4", 5, time.Minute)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("request %d should be allowed within limit 5", i+1)
		}
	}
}

func TestAllow_OverLimit_Rejected(t *testing.T) {
	rl, _ := newTestLimiter()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		ok, _ := rl.Allow(ctx, "ip:1.2.3.4", 5, time.Minute)
		if !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	// 6th request exceeds the fixed window.
	ok, err := rl.Allow(ctx, "ip:1.2.3.4", 5, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("6th request should be rejected when limit is 5")
	}
}

// TestAllow_KeysAreIsolated: different keys must have independent budgets.
func TestAllow_KeysAreIsolated(t *testing.T) {
	rl, _ := newTestLimiter()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if ok, _ := rl.Allow(ctx, "ip:1.1.1.1", 5, time.Minute); !ok {
			t.Fatalf("ip 1.1.1.1 request %d should be allowed", i+1)
		}
	}
	// A different IP is unaffected.
	if ok, _ := rl.Allow(ctx, "ip:2.2.2.2", 5, time.Minute); !ok {
		t.Fatalf("ip 2.2.2.2 should have its own budget")
	}
}

// TestAllowN_Cost_ConsumesTokens: a batch of cost N consumes N tokens.
func TestAllowN_Cost_ConsumesTokens(t *testing.T) {
	rl, _ := newTestLimiter()
	ctx := context.Background()
	// Two batches of cost 3 against a limit of 5.
	if ok, _ := rl.AllowN(ctx, "user:7", 5, 3, time.Minute); !ok {
		t.Fatalf("first batch of 3 should be allowed")
	}
	if ok, _ := rl.AllowN(ctx, "user:7", 5, 3, time.Minute); ok {
		t.Fatalf("second batch of 3 would reach 6 > 5, should be rejected")
	}
}

// TestAllowN_ZeroMax_Unlimited: a zero/negative limit means no throttling.
func TestAllowN_ZeroMax_Unlimited(t *testing.T) {
	rl, _ := newTestLimiter()
	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		if ok, _ := rl.AllowN(ctx, "user:1", 0, 100, time.Minute); !ok {
			t.Fatalf("unlimited limiter must never reject")
		}
	}
}

// TestAllowN_WindowExpiry_ResetsBudget: after the window elapses the budget resets.
func TestAllowN_WindowExpiry_ResetsBudget(t *testing.T) {
	rl, fc := newTestLimiter()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if ok, _ := rl.Allow(ctx, "ip:9.9.9.9", 5, time.Minute); !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if ok, _ := rl.Allow(ctx, "ip:9.9.9.9", 5, time.Minute); ok {
		t.Fatalf("should be over limit before expiry")
	}
	// Force the stored window to expire.
	fc.mu.Lock()
	fc.ttl["rl:ip:9.9.9.9"] = time.Now().Add(-time.Second)
	fc.mu.Unlock()
	if ok, _ := rl.Allow(ctx, "ip:9.9.9.9", 5, time.Minute); !ok {
		t.Fatalf("budget should reset after window expiry")
	}
}

// TestAllowN_OverLimit_RefundsCost: a rejected request must not consume budget,
// so subsequent legitimate calls within the same window are not blocked (B8).
func TestAllowN_OverLimit_RefundsCost(t *testing.T) {
	rl, fc := newTestLimiter()
	ctx := context.Background()
	// Use 4 of a 5 budget.
	if ok, _ := rl.AllowN(ctx, "ip:refund", 5, 4, time.Minute); !ok {
		t.Fatalf("first cost-4 request should be allowed")
	}
	// A cost-3 request would reach 7 > 5 and must be rejected without consuming.
	if ok, _ := rl.AllowN(ctx, "ip:refund", 5, 3, time.Minute); ok {
		t.Fatalf("over-limit request must be rejected")
	}
	// The counter must still be 4, leaving room for one more single token.
	if ok, _ := rl.Allow(ctx, "ip:refund", 5, time.Minute); !ok {
		t.Fatalf("refunded budget should still allow one more token (5th)")
	}
	// Now at 5/5, the next one is legitimately rejected.
	if ok, _ := rl.Allow(ctx, "ip:refund", 5, time.Minute); ok {
		t.Fatalf("6th request should be rejected")
	}
	_ = fc
}

// TestAllowN_TTLSetOnEveryCall: the window TTL is refreshed on every increment,
// so a key can never lose its expiry and become permanently banned (B8).
func TestAllowN_TTLSetOnEveryCall(t *testing.T) {
	rl, fc := newTestLimiter()
	ctx := context.Background()
	if ok, _ := rl.Allow(ctx, "ip:ttl", 10, time.Minute); !ok {
		t.Fatalf("first request should be allowed")
	}
	// Simulate the TTL being lost (e.g. Redis restart losing TTL on the key).
	fc.mu.Lock()
	delete(fc.ttl, "rl:ip:ttl")
	countBefore := fc.counts["rl:ip:ttl"]
	fc.mu.Unlock()
	// A subsequent call must re-establish the TTL.
	if ok, _ := rl.Allow(ctx, "ip:ttl", 10, time.Minute); !ok {
		t.Fatalf("second request should be allowed")
	}
	fc.mu.Lock()
	_, hasTTL := fc.ttl["rl:ip:ttl"]
	countAfter := fc.counts["rl:ip:ttl"]
	fc.mu.Unlock()
	if !hasTTL {
		t.Fatalf("TTL must be re-established on every call")
	}
	if countAfter != countBefore+1 {
		t.Fatalf("counter should advance by 1, got %d -> %d", countBefore, countAfter)
	}
}
