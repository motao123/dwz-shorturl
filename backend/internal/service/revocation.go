package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"dwz-admin/internal/config"
	"dwz-admin/internal/pkg"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

const (
	// jtiBlacklistKeyPrefix 沿用登出已在写的键名，避免升级后旧吊销记录读不到。
	jtiBlacklistKeyPrefix = "jwt:blacklist:"
	// userRevokeKeyPrefix 存「该用户在此时刻之前签发的凭证一律作废」的水位线。
	// 按用户而不是按 jti 记账，才能覆盖「禁用/降权/改密」这类并没有具体 jti 的场景。
	userRevokeKeyPrefix = "jwt:revoked_before:"

	// revokeClockMargin：iat 只有秒级精度，水位线抬高 1 秒，保证与吊销发生在同一秒的
	// 旧凭证也被作废；代价是同一秒内新签的凭证会被误杀，而这类操作都需要人点一下。
	revokeClockMargin = time.Second
)

// Revoker 让 user service 在改动凭证效力时不必知道背后是 Redis。
type Revoker interface {
	RevokeUser(ctx context.Context, userID uint64) error
}

// SessionRevocation 是吊销记录的存储与判定：jti 黑名单（登出）+ 用户级水位线（禁用/删除/改密/改角色）。
type SessionRevocation struct {
	rdb *redis.Client
	// configured 与「rdb 是否为 nil」不是一回事：initRedis 无论如何都会返回客户端，
	// 地址留空的部署只是没启用吊销能力。把两者混为一谈，fail-closed 就会在没有 Redis
	// 的部署上把所有后台请求拒掉。
	configured bool
	logger     *zap.Logger
}

func NewSessionRevocation(rdb *redis.Client, configured bool) *SessionRevocation {
	return &SessionRevocation{rdb: rdb, configured: configured}
}

func (s *SessionRevocation) WithLogger(logger *zap.Logger) *SessionRevocation {
	s.logger = logger
	return s
}

// revokeTTL 必须长于它可能要作废的最长凭证：水位线只活 2 小时的话，早于吊销时刻签发的
// refresh token（默认 7 天）会在键过期后重新可用。
func (s *SessionRevocation) revokeTTL() time.Duration {
	if cfg := config.Get(); cfg != nil && cfg.JWT.RefreshExpiry > 0 {
		return cfg.JWT.RefreshExpiry
	}
	return 7 * 24 * time.Hour
}

func (s *SessionRevocation) enabled() bool {
	return s != nil && s.configured && s.rdb != nil
}

// Check 实现 pkg.SessionChecker。未启用吊销能力时恒为 SessionActive。
func (s *SessionRevocation) Check(ctx context.Context, userID uint64, jti string, issuedAt time.Time) pkg.SessionState {
	if !s.enabled() {
		return pkg.SessionActive
	}
	if jti != "" {
		n, err := s.rdb.Exists(ctx, jtiBlacklistKeyPrefix+jti).Result()
		if err != nil {
			s.logUnavailable("jti_blacklist_lookup", err)
			return pkg.SessionUnavailable
		}
		if n > 0 {
			return pkg.SessionRevoked
		}
	}
	if userID == 0 {
		return pkg.SessionActive
	}

	ts, err := s.rdb.Get(ctx, userRevokeKeyPrefix+strconv.FormatUint(userID, 10)).Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return pkg.SessionActive // 从未被吊销过
		}
		s.logUnavailable("revocation_watermark_lookup", err)
		return pkg.SessionUnavailable
	}
	if issuedAt.IsZero() {
		// 缺 iat 的旧 token 无法与水位线比较。按吊销处理会让升级前签发的存量会话集体掉线，
		// 而这类 token 也带不上新的角色声明，风险由 jti 黑名单与 access token 自然过期兜住。
		return pkg.SessionActive
	}
	if issuedAt.Unix() < ts {
		return pkg.SessionRevoked
	}
	return pkg.SessionActive
}

// RevokeUser 把该用户当前签发的全部 access/refresh token 作废到自然过期为止。
func (s *SessionRevocation) RevokeUser(ctx context.Context, userID uint64) error {
	return s.revokeUserAt(ctx, userID, time.Now())
}

func (s *SessionRevocation) revokeUserAt(ctx context.Context, userID uint64, at time.Time) error {
	if !s.enabled() || userID == 0 {
		return nil
	}
	watermark := at.Add(revokeClockMargin).Unix()
	return s.rdb.Set(ctx, userRevokeKeyPrefix+strconv.FormatUint(userID, 10), watermark, s.revokeTTL()).Err()
}

// BlacklistToken 供登出使用：只作废这一个 jti，TTL 取它自己的剩余寿命。
func (s *SessionRevocation) BlacklistToken(ctx context.Context, jti string, expiresAt time.Time) error {
	if !s.enabled() || jti == "" {
		return nil
	}
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return nil // 本来就过期了，不必占键位
	}
	return s.rdb.Set(ctx, jtiBlacklistKeyPrefix+jti, 1, ttl).Err()
}

// Checker 返回可直接注册给中间件/服务的闭包。
func (s *SessionRevocation) Checker() pkg.SessionChecker {
	return func(userID uint64, jti string, issuedAt time.Time) pkg.SessionState {
		return s.Check(context.Background(), userID, jti, issuedAt)
	}
}

func (s *SessionRevocation) logUnavailable(op string, err error) {
	if s.logger == nil {
		return
	}
	s.logger.Error("session revocation backend unavailable, request denied",
		zap.String("op", op), zap.Error(err))
}
