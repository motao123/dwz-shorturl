package pkg

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

// Counter abstracts the atomic counter operations needed by the rate limiter.
// It is satisfied by the Redis-backed implementation and by an in-memory fake
// in tests, so the limiter logic is unit-testable without a live Redis.
type Counter interface {
	IncrBy(ctx context.Context, key string, n int64) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) (bool, error)
}

type redisCounter struct {
	rdb *redis.Client
}

func (c redisCounter) IncrBy(ctx context.Context, key string, n int64) (int64, error) {
	return c.rdb.IncrBy(ctx, key, n).Result()
}

func (c redisCounter) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return c.rdb.Expire(ctx, key, ttl).Result()
}

// RateLimiter implements a fixed-window rate limiter over a shared counter
// (normally Redis). Each key is an atomic counter reset when the window elapses.
type RateLimiter struct {
	counter Counter
}

// NewRateLimiter returns a RateLimiter backed by a Redis client.
func NewRateLimiter(rdb *redis.Client) *RateLimiter {
	return &RateLimiter{counter: redisCounter{rdb: rdb}}
}

// NewRateLimiterWithCounter returns a RateLimiter backed by an arbitrary
// Counter implementation (used by tests).
func NewRateLimiterWithCounter(c Counter) *RateLimiter {
	return &RateLimiter{counter: c}
}

// Allow consumes one token for key within the window and reports whether the
// request may proceed. A max <= 0 means unlimited.
func (r *RateLimiter) Allow(ctx context.Context, key string, max int, window time.Duration) (bool, error) {
	return r.AllowN(ctx, key, max, 1, window)
}

// AllowN atomically consumes cost tokens for key within the window. The window
// TTL is (re)set on the first increment of a fresh window; subsequent calls
// within the same window only bump the counter. A max <= 0 means unlimited.
func (r *RateLimiter) AllowN(ctx context.Context, key string, max, cost int, window time.Duration) (bool, error) {
	if max <= 0 {
		return true, nil
	}
	if cost <= 0 {
		cost = 1
	}
	ckey := "rl:" + key
	n, err := r.counter.IncrBy(ctx, ckey, int64(cost))
	if err != nil {
		return false, err
	}
	// Best-effort TTL: only meaningful on the first increment of a window.
	if n == int64(cost) {
		_, _ = r.counter.Expire(ctx, ckey, window)
	}
	return n <= int64(max), nil
}