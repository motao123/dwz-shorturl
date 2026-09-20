package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"

	"github.com/gin-gonic/gin"
)

// ---------- 测试替身 ----------

type fakeApiKeyRepo struct {
	record *model.ApiKey
	err    error
	// lastUsedCalls counts UpdateLastUsed invocations so we can assert the
	// reject paths do not touch the row.
	lastUsedCalls int
}

func (f *fakeApiKeyRepo) Create(*model.ApiKey) error { return nil }
func (f *fakeApiKeyRepo) FindByHash(string) (*model.ApiKey, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.record, nil
}
func (f *fakeApiKeyRepo) FindByPrefix(string) ([]model.ApiKey, error) { return nil, nil }
func (f *fakeApiKeyRepo) FindByID(uint64) (*model.ApiKey, error)      { return nil, nil }
func (f *fakeApiKeyRepo) ListByUser(uint64) ([]model.ApiKey, error)   { return nil, nil }
func (f *fakeApiKeyRepo) Revoke(uint64) error                         { return nil }
func (f *fakeApiKeyRepo) UpdateLastUsed(uint64) error {
	f.lastUsedCalls++
	return nil
}

// failingCounter makes the limiter report an infrastructure failure, standing
// in for "Redis is unreachable".
type failingCounter struct{}

func (failingCounter) IncrBy(_ context.Context, _ string, _ int64) (int64, error) {
	return 0, errors.New("redis: connection refused")
}
func (failingCounter) Expire(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return false, errors.New("redis: connection refused")
}
func (failingCounter) Delete(_ context.Context, _ string) error {
	return errors.New("redis: connection refused")
}
func (failingCounter) Get(_ context.Context, _ string) (int64, error) {
	return 0, errors.New("redis: connection refused")
}

// ---------- 权限解析 ----------

func TestParseApiKeyPermissions(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"正常数组", `["short_urls.create"]`, []string{"short_urls.create"}},
		{"多个范围", `["short_urls.create","short_urls.batch"]`, []string{"short_urls.create", "short_urls.batch"}},
		{"空字符串→空集（拒绝，不是放行）", ``, []string{}},
		{"空白→空集", `   `, []string{}},
		{"JSON null→空集", `null`, []string{}},
		{"非法 JSON→空集（畸形值不得变成放行）", `{oops`, []string{}},
		{"非数组→空集", `"short_urls.create"`, []string{}},
		{"数组元素去空白与空项", `[" short_urls.create ", ""]`, []string{"short_urls.create"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseApiKeyPermissions(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("ParseApiKeyPermissions(%q) = %v, want %v", tc.raw, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("ParseApiKeyPermissions(%q) = %v, want %v", tc.raw, got, tc.want)
				}
			}
		})
	}
}

// ---------- 范围判定 ----------

func TestApiKeyHasScope(t *testing.T) {
	cases := []struct {
		name  string
		perms []string
		scope ApiKeyScope
		want  bool
	}{
		{"精确命中", []string{"short_urls.create"}, ScopeShortURLsCreate, true},
		{"无权限→拒绝", []string{}, ScopeShortURLsCreate, false},
		{"不相关权限→拒绝", []string{"short_urls.read"}, ScopeShortURLsCreate, false},
		{"通配放行一切", []string{"*"}, ScopeShortURLsBatch, true},
		{"create 蕴含 batch（可循环调用单条达到同样效果）", []string{"short_urls.create"}, ScopeShortURLsBatch, true},
		{"batch 不蕴含 create（反向不成立）", []string{"short_urls.batch"}, ScopeShortURLsCreate, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := apiKeyHasScope(tc.perms, tc.scope); got != tc.want {
				t.Fatalf("apiKeyHasScope(%v, %q) = %v, want %v", tc.perms, tc.scope, got, tc.want)
			}
		})
	}
}

// ---------- ScopeApiKey 中间件 ----------

func newScopeEngine(perms any, withContext bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/probe", func(c *gin.Context) {
		if withContext {
			if p, ok := perms.([]string); ok {
				c.Set("api_key_permissions", p)
			} else {
				c.Set("api_key_permissions", perms)
			}
		}
		c.Next()
	}, ScopeApiKey(ScopeShortURLsCreate), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

func TestScopeApiKey_AllowsKeyWithScope(t *testing.T) {
	r := newScopeEngine([]string{"short_urls.create"}, true)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("带权限的密钥应放行，got %d", w.Code)
	}
}

func TestScopeApiKey_RejectsKeyWithoutScope(t *testing.T) {
	r := newScopeEngine([]string{}, true)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("无权限的密钥应 403，got %d", w.Code)
	}
}

func TestScopeApiKey_MissingContextFailsClosed(t *testing.T) {
	// 中间件顺序写错（忘了 RequireApiKey）时必须拒绝，不能放行。
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/probe", ScopeApiKey(ScopeShortURLsCreate), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("缺少权限上下文应 403（fail-closed），got %d", w.Code)
	}
}

func TestScopeApiKey_WrongTypeFailsClosed(t *testing.T) {
	r := newScopeEngine("not-a-slice", true)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("上下文类型不对应 403，got %d", w.Code)
	}
}

func TestScopeApiKey_EmptyScopeIsNoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/probe", ScopeApiKey(""), func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("空 scope 表示不校验，应放行，got %d", w.Code)
	}
}

// ---------- RequireApiKey：限流故障时的失败方向 ----------

// TestRequireApiKey_LimiterErrorFailsOpen 钉住本轮修的行为：
// 限流器（Redis）故障不能让整个公开 API 硬停。
// 反向验证方式：把 apikey.go 里的 `if err != nil { } else if !allowed`
// 改回 `if err != nil || !allowed`，本用例立刻变红（期望 200，实得 429）。
func TestRequireApiKey_LimiterErrorFailsOpen(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeApiKeyRepo{record: &model.ApiKey{
		ID: 1, KeyPrefix: "dwz_abcd", KeyHash: "x", RateLimit: 100, Status: 1,
		Permissions: `["short_urls.create"]`,
	}}
	// 注入一个必定报错的计数器，模拟 Redis 不可用。
	rl := pkg.NewRateLimiterWithCounter(&failingCounter{})

	r := gin.New()
	r.POST("/probe", RequireApiKey(repo, rl), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodPost, "/probe", nil)
	req.Header.Set("X-API-Key", "dwz_abcdefgh")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("限流器故障时应 fail-open（保护≠鉴权），got %d body=%s", w.Code, w.Body.String())
	}
}

// TestRequireApiKey_LimiterErrorDoesNotBypassAuth 确认 fail-open 仅放开限流，
// 鉴权与范围校验照旧生效：认证失败即便限流器挂了也要 401。
func TestRequireApiKey_LimiterErrorDoesNotBypassAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeApiKeyRepo{err: errors.New("no such key")}
	rl := pkg.NewRateLimiterWithCounter(&failingCounter{})

	r := gin.New()
	r.POST("/probe", RequireApiKey(repo, rl), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodPost, "/probe", nil)
	req.Header.Set("X-API-Key", "dwz_bogus")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("无效密钥应 401，got %d", w.Code)
	}
}

// TestRequireApiKey_LoadsPermissionsIntoContext 确认解析后的权限进了上下文，
// 否则 ScopeApiKey 无从校验（会直接 fail-closed，功能全废）。
func TestRequireApiKey_LoadsPermissionsIntoContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeApiKeyRepo{record: &model.ApiKey{
		ID: 1, KeyPrefix: "dwz_abcd", RateLimit: 100, Status: 1,
		Permissions: `["short_urls.create"]`,
	}}
	var got []string
	r := gin.New()
	r.POST("/probe", RequireApiKey(repo, nil), func(c *gin.Context) {
		v, _ := c.Get("api_key_permissions")
		got, _ = v.([]string)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/probe", nil)
	req.Header.Set("X-API-Key", "dwz_abcdefgh")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("应放行，got %d", w.Code)
	}
	if len(got) != 1 || got[0] != "short_urls.create" {
		t.Fatalf("权限未正确载入上下文：%v", got)
	}
}

// TestRequireApiKey_ExpiredKeyRejected 确认过期密钥被拒（回归保护）。
func TestRequireApiKey_ExpiredKeyRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	past := time.Now().Add(-time.Hour)
	repo := &fakeApiKeyRepo{record: &model.ApiKey{
		ID: 1, KeyPrefix: "dwz_abcd", RateLimit: 100, Status: 1, ExpiresAt: &past,
	}}
	r := gin.New()
	r.POST("/probe", RequireApiKey(repo, nil), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodPost, "/probe", nil)
	req.Header.Set("X-API-Key", "dwz_abcdefgh")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("过期密钥应 401，got %d", w.Code)
	}
	if repo.lastUsedCalls != 0 {
		t.Fatalf("被拒请求不应更新 last_used_at，got %d", repo.lastUsedCalls)
	}
}
