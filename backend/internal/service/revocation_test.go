package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dwz-admin/internal/pkg"

	"github.com/go-redis/redis/v8"
)

// deadRedis 指向一个必然连不上的端口。用它是为了把"吊销存储不可用"这条分支跑成真代码，
// 而不是只在注释里声称处理过——#38 的原始形态正是"读不到就当没吊销"。
func deadRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 80 * time.Millisecond,
		MaxRetries:  -1,
	})
}

// TestRevocation_NotConfiguredStaysOpen 盯的是另一个方向：Redis 没配置的部署里，
// fail-closed 不能变成"后台整个锁死"。
func TestRevocation_NotConfiguredStaysOpen(t *testing.T) {
	s := NewSessionRevocation(deadRedis(), false)

	if got := s.Check(context.Background(), 7, "some-jti", time.Now()); got != pkg.SessionActive {
		t.Errorf("未启用吊销能力时 Check = %v，应为 SessionActive（否则无 Redis 的部署会全部 503）", got)
	}
	if err := s.RevokeUser(context.Background(), 7); err != nil {
		t.Errorf("未启用时 RevokeUser 应为无操作，得到 %v", err)
	}
	if err := s.BlacklistToken(context.Background(), "jti", time.Now().Add(time.Hour)); err != nil {
		t.Errorf("未启用时 BlacklistToken 应为无操作，得到 %v", err)
	}
}

func TestRevocation_StoreUnavailableIsDenied(t *testing.T) {
	s := NewSessionRevocation(deadRedis(), true)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if got := s.Check(ctx, 7, "some-jti", time.Now()); got != pkg.SessionUnavailable {
		t.Errorf("Redis 不可用时 Check = %v，应为 SessionUnavailable；按 Active 处理即登出/禁用绕过（#38）", got)
	}
	// jti 为空也不能退化成"查不到=没吊销"：此时靠的是用户水位线。
	if got := s.Check(ctx, 7, "", time.Now()); got != pkg.SessionUnavailable {
		t.Errorf("无 jti 且 Redis 不可用时 Check = %v，应为 SessionUnavailable", got)
	}
}

func TestRevocation_ExpiredCredentialNeedsNoBlacklistKey(t *testing.T) {
	s := NewSessionRevocation(deadRedis(), true)
	if err := s.BlacklistToken(context.Background(), "jti", time.Now().Add(-time.Minute)); err != nil {
		t.Errorf("已过期凭证应直接跳过写黑名单（不占键位，也不该去碰连不上的 Redis），得到 %v", err)
	}
}

// TestRevocation_WatermarkOutlivesRefreshToken：水位线只活 2 小时的话，早于吊销时刻
// 签发的 refresh token（默认 7 天）会在键过期后重新可用。
func TestRevocation_WatermarkOutlivesRefreshToken(t *testing.T) {
	if ttl := NewSessionRevocation(nil, true).revokeTTL(); ttl < 168*time.Hour {
		t.Errorf("revokeTTL = %v，必须不短于 refresh_expiry，否则吊销会自己失效", ttl)
	}
}

// TestWiring_RevocationIsInstalled 是源码级断言：吊销能力靠构造点接线才生效，
// 漏接一处不会有任何报错——禁用账号就静默不吊销会话。新增构造点时这条会红。
func TestWiring_RevocationIsInstalled(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "cmd", "server", "main.go"))
	if err != nil {
		t.Fatalf("读取 cmd/server/main.go 失败: %v", err)
	}
	text := string(src)
	for _, want := range []string{
		"NewUserService(userRepo).WithSessionRevocation(",
		"middleware.SetSessionRevocationCheck(",
		".WithSessionCheck(",
		// 「配了 Redis 却连不上」才拒绝；「压根没配」必须继续放行。
		// 写成字面量断言是因为这个参数一旦被改掉，故障方向就悄悄反了。
		"NewSessionRevocation(rdb, cfg.Redis.Addr != \"\")",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("cmd/server/main.go 里找不到 %q：会话吊销的接线被拆开或漏接", want)
		}
	}
}
