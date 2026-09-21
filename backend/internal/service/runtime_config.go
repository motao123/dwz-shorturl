package service

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"dwz-admin/internal/repository"

	"go.uber.org/zap"
)

// RuntimeConfig resolves the governance switches an administrator changes in
// the admin UI ("系统配置") into values the Go request path actually reads.
//
// Why this exists (#4): the admin UI exposed at least four switches —
// shorturl.allow_custom_code, shorturl.require_registration, batch.max_urls and
// short_url.allowed_expire_days — that PHP read but the Go side never consulted.
// An operator could turn custom codes off, watch the toggle show "关闭", and the
// member API would keep accepting them. A control plane that lies is worse than
// no control plane, so the switches are now resolved here and consulted by
// short_url.go / member_api.go.
//
// The values are cached for a short TTL: these are read on the hot create path,
// and a 60s staleness window is the right trade for not issuing a query per
// request. A read failure is deliberately NOT fail-open in the dangerous
// direction — see each getter for its default.
type RuntimeConfig struct {
	repo   repository.ConfigRepo
	logger *zap.Logger

	ttl time.Duration

	mu       sync.RWMutex
	loadedAt time.Time
	values   map[string]string
	loadErr  error
}

// Defaults mirror the PHP side (includes/function.php, api.php, batch.php) and
// the seed data, so a deployment that has never opened the config page behaves
// the same on both stacks.
const (
	defaultAllowCustomCode     = true
	defaultRequireRegistration = false
	defaultBatchMaxURLs        = 100
	defaultAllowedExpireDays   = "0,1,7,30,365"

	// Member API quota defaults (#8/#37). 120 requests / 60s per member token is
	// deliberately generous: it must not break the console's own burst (a page
	// load issues several calls), while still bounding an authenticated client
	// that loops the create endpoint. Operators can tune both from the config
	// page; 0 in api_rate_max disables the limit.
	defaultMemberRateMax = 120
	// defaultMemberMaxLinks is the per-member live-link cap when the operator
	// hasn't set member.max_links (#41). Finite on purpose: "unlimited by
	// default" is what let fresh registrations mass-produce phishing links.
	defaultMemberMaxLinks   = 1000
	defaultMemberRateWindow = 60
	minMemberRateWindow     = 1
	maxMemberRateWindow     = 3600

	// Guard rails for the switches an operator can set. A zero/negative batch
	// size would reject every batch request, so it falls back to the default.
	minBatchMaxURLs = 1
	maxBatchMaxURLs = 1000
)

// System config keys, shared with the admin UI catalog.
const (
	KeyAllowCustomCode     = "shorturl.allow_custom_code"
	KeyRequireRegistration = "shorturl.require_registration"
	KeyBatchMaxURLs        = "batch.max_urls"
	KeyAllowedExpireDays   = "short_url.allowed_expire_days"
	KeyMemberRateMax       = "member.api_rate_max"
	// KeyMemberMaxLinks caps how many live links one member account may hold (#41).
	KeyMemberMaxLinks   = "member.max_links"
	KeyMemberRateWindow = "member.api_rate_window"
)

// NewRuntimeConfig builds a resolver. A nil repo yields defaults for everything,
// which keeps tests and any deployment without the admin DB working.
func NewRuntimeConfig(repo repository.ConfigRepo, logger *zap.Logger) *RuntimeConfig {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RuntimeConfig{
		repo:   repo,
		logger: logger,
		ttl:    60 * time.Second,
		values: map[string]string{},
	}
}

// Invalidate forces the next read to hit the database. The admin config handler
// calls this after a successful update so a toggle takes effect immediately
// instead of up to one TTL later.
func (r *RuntimeConfig) Invalidate() {
	r.mu.Lock()
	r.loadedAt = time.Time{}
	r.mu.Unlock()
}

func (r *RuntimeConfig) snapshot() map[string]string {
	r.mu.RLock()
	loaded := !r.loadedAt.IsZero()
	fresh := loaded && time.Since(r.loadedAt) < r.ttl
	values := r.values
	err := r.loadErr
	r.mu.RUnlock()
	if fresh && err == nil {
		return values
	}

	next := map[string]string{}
	var loadErr error
	if r.repo != nil {
		configs, dbErr := r.repo.GetAll()
		if dbErr != nil {
			loadErr = dbErr
		} else {
			for _, c := range configs {
				next[c.ConfigKey] = c.ConfigValue
			}
		}
	}

	r.mu.Lock()
	r.values = next
	r.loadErr = loadErr
	r.loadedAt = time.Now()
	r.mu.Unlock()

	if loadErr != nil {
		// Loud, not silent: a degraded read is exactly the condition that used to
		// make the admin UI silently fail-open without a trace.
		r.logger.Error("读取 system_configs 失败，治理开关回退到默认值（最严）",
			zap.Error(loadErr))
		return nil
	}
	return next
}

// lookup distinguishes "row absent" from "row present but empty" so callers can
// apply their own default without an empty string masquerading as "off".
func (r *RuntimeConfig) lookup(key string) (string, bool) {
	values := r.snapshot()
	if values == nil {
		return "", false
	}
	v, ok := values[key]
	return strings.TrimSpace(v), ok
}

// AllowCustomCode reports whether custom short codes are accepted.
//
// On a config-read failure this returns false (the restrictive side): if we
// cannot tell whether the gate is closed, we do not accept a caller-supplied
// code. Callers that must distinguish the two conditions can check LoadError.
func (r *RuntimeConfig) AllowCustomCode() bool {
	v, ok := r.lookup(KeyAllowCustomCode)
	if !ok || v == "" {
		if r.LoadError() != nil {
			return false
		}
		return defaultAllowCustomCode
	}
	return parseBool(v, defaultAllowCustomCode)
}

// RequireRegistration reports whether anonymous (unregistered) callers are
// barred from creating links.
func (r *RuntimeConfig) RequireRegistration() bool {
	v, ok := r.lookup(KeyRequireRegistration)
	if !ok || v == "" {
		if r.LoadError() != nil {
			// Treat "unknown" as "registration required": the safe direction for a
			// governance gate. A false negative here lets anonymous abuse through.
			return true
		}
		return defaultRequireRegistration
	}
	return parseBool(v, false)
}

// BatchMaxURLs is the per-request cap for the batch endpoints, clamped to a
// sane range. The admin UI used to accept 1..1000 here while the frontend
// hard-coded 100, so raising it had no effect (#50); the number is now resolved
// once and handed back to callers instead of duplicated.
func (r *RuntimeConfig) BatchMaxURLs() int {
	v, ok := r.lookup(KeyBatchMaxURLs)
	if !ok || v == "" {
		return defaultBatchMaxURLs
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < minBatchMaxURLs || n > maxBatchMaxURLs {
		r.logger.Warn("batch.max_urls 取值非法，使用默认值",
			zap.String("value", v), zap.Int("default", defaultBatchMaxURLs))
		return defaultBatchMaxURLs
	}
	return n
}

// AllowedExpireDays returns the whitelist of permitted expiry values (in days,
// 0 = permanent). It mirrors the PHP whitelist that only existed on that side
// (#62), so the two create paths agree on what "允许的有效期" means.
func (r *RuntimeConfig) AllowedExpireDays() []int {
	v, ok := r.lookup(KeyAllowedExpireDays)
	if !ok || v == "" {
		return defaultExpireDaysList()
	}
	days, parsed := parseExpireDays(v)
	if !parsed || len(days) == 0 {
		r.logger.Warn("short_url.allowed_expire_days 取值非法，使用默认白名单",
			zap.String("value", v))
		return defaultExpireDaysList()
	}
	return days
}

// IsExpireDaysAllowed reports whether the requested value is on the whitelist.
func (r *RuntimeConfig) IsExpireDaysAllowed(days int) bool {
	for _, d := range r.AllowedExpireDays() {
		if d == days {
			return true
		}
	}
	return false
}

// LoadError exposes the last config-read error so callers can surface a degraded
// state (e.g. an admin banner) instead of silently running on defaults.
func (r *RuntimeConfig) LoadError() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.values == nil {
		return errors.New("system_configs 不可用")
	}
	return r.loadErr
}

func defaultExpireDaysList() []int { return []int{0, 1, 7, 30, 365} }

// parseExpireDays accepts both the JSON-array form stored by the config catalog
// (`[0,1,7,30,365]`) and the comma form used by PHP (`0,1,7,30,365`).
func parseExpireDays(raw string) ([]int, bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil, false
	}
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// MemberRateLimit returns the per-member quota as (max, window). max <= 0 means
// unlimited. Values outside the sane range fall back to the default rather than
// silently disabling the gate, and the window is clamped to 1..3600s so a typo
// cannot produce a 1ns window that rejects everything.
func (r *RuntimeConfig) MemberRateLimit() (int, time.Duration) {
	max := defaultMemberRateMax
	if v, ok := r.lookup(KeyMemberRateMax); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			r.logger.Warn("member.api_rate_max 取值非法，使用默认值",
				zap.String("value", v), zap.Int("default", defaultMemberRateMax))
		} else {
			max = n
		}
	}

	window := defaultMemberRateWindow
	if v, ok := r.lookup(KeyMemberRateWindow); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < minMemberRateWindow || n > maxMemberRateWindow {
			r.logger.Warn("member.api_rate_window 取值非法，使用默认值",
				zap.String("value", v), zap.Int("default", defaultMemberRateWindow))
		} else {
			window = n
		}
	}

	return max, time.Duration(window) * time.Second
}

// MemberMaxLinks is the per-member cap on live links (0 = unlimited). A value
// that isn't a number falls back to the default rather than silently lifting the
// cap, matching how MemberRateLimit treats a typo.
func (r *RuntimeConfig) MemberMaxLinks() int {
	if r == nil {
		return defaultMemberMaxLinks
	}
	v, ok := r.lookup(KeyMemberMaxLinks)
	if !ok || v == "" {
		return defaultMemberMaxLinks
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		r.logger.Warn("member.max_links 取值非法，使用默认值",
			zap.String("value", v), zap.Int("default", defaultMemberMaxLinks))
		return defaultMemberMaxLinks
	}
	return n
}

func parseBool(v string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes", "启用":
		return true
	case "0", "false", "off", "no", "禁用":
		return false
	}
	return fallback
}
