package pkg

import "time"

// SessionState 是一个签名有效的 token「现在还能不能用」的三态判定。
//
// 之所以不是布尔：吊销记录存在 Redis 里，而「查不到」和「没吊销」是两回事。
// 按布尔实现时 Redis 一超时就被读成「没吊销」，于是登出、禁用账号、改密、降权
// 的吊销记录全体静默失效——想让吊销失效，只要把吊销存储打挂（#38）。
type SessionState int

const (
	// SessionActive：凭证可用（含「本部署没启用吊销能力」这一情形）。
	SessionActive SessionState = iota
	// SessionRevoked：已被明确吊销，调用方必须拒绝。
	SessionRevoked
	// SessionUnavailable：吊销存储不可用。调用方同样必须拒绝——放行的正是上面那条绕过路径。
	// 注意「本部署没启用吊销能力」不算不可用，那种情形是 SessionActive。
	SessionUnavailable
)

// SessionChecker 由吊销后端实现，供鉴权中间件与刷新流程调用。
// issuedAt 取自 token 的 iat，缺失时传时间零值。
type SessionChecker func(userID uint64, jti string, issuedAt time.Time) SessionState
