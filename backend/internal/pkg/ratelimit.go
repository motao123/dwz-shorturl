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

// AllowN atomically consumes cost tokens for key within the window. A max <= 0
// means unlimited.
//
// B8 hardening:
//   - The window TTL is (re)set on EVERY increment, not only on the first one.
//     EXPIRE is idempotent and cheap; setting it unconditionally guarantees a
//     key can never end up without an expiry (which would otherwise permanently
//     ban the key after a Redis restart or a key deletion that lost the TTL).
//   - When the request is over the limit the cost is refunded, so rejected
//     requests no longer consume budget and block subsequent legitimate calls
//     within the same window.
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
	// 无论是否为窗口首次自增，都重申 TTL，确保计数器不会永不过期。
	_, _ = r.counter.Expire(ctx, ckey, window)
	if n <= int64(max) {
		return true, nil
	}
	// 超额：退还本次消耗，避免被拒请求连带阻塞窗口内后续合法请求。
	// 退款同样受 TTL 保护，不会生成不过期的 key。
	if _, rerr := r.counter.IncrBy(ctx, ckey, -int64(cost)); rerr != nil {
		return false, rerr
	}
	return false, nil
}
