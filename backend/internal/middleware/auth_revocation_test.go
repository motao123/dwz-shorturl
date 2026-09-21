package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dwz-admin/internal/pkg"

	"github.com/gin-gonic/gin"
)

// TestAuthMiddleware_SessionRevocationStates 锁住三态各自的响应：
// 可用→放行，已吊销→401，吊销存储不可用→503。
// 第三态是关键：按布尔实现时"查不到"会被读成"没吊销"，于是把 Redis 打挂就能让
// 登出与禁用记录全体失效（#38）。
func TestAuthMiddleware_SessionRevocationStates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestConfig(t)

	original := sessionCheck
	t.Cleanup(func() { sessionCheck = original })

	access, _, err := pkg.GenerateTokens(42, "operator", []string{"admin"})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := pkg.ParseToken(access)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		state      pkg.SessionState
		wantStatus int
		wantAbort  bool
	}{
		{"可用会话放行", pkg.SessionActive, http.StatusOK, false},
		{"已登出/已吊销的会话 401", pkg.SessionRevoked, http.StatusUnauthorized, true},
		{"吊销存储不可用时拒绝而不是放行", pkg.SessionUnavailable, http.StatusServiceUnavailable, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotUser uint64
			var gotJTI string
			var gotIssuedAt time.Time
			called := 0
			sessionCheck = func(userID uint64, jti string, issuedAt time.Time) pkg.SessionState {
				called++
				gotUser, gotJTI, gotIssuedAt = userID, jti, issuedAt
				return tc.state
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/admin/api/short_urls", nil)
			c.Request.Header.Set("Authorization", "Bearer "+access)
			Auth()(c)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d，期望 %d", w.Code, tc.wantStatus)
			}
			if c.IsAborted() != tc.wantAbort {
				t.Errorf("aborted = %v，期望 %v", c.IsAborted(), tc.wantAbort)
			}
			if called != 1 {
				t.Fatalf("吊销检查被调用 %d 次，期望 1 次", called)
			}
			// 中间件必须把身份三要素传对：水位线按 user 记，黑名单按 jti 记。
			if gotUser != 42 || gotJTI != claims.ID {
				t.Errorf("传给检查的标识 = (%d, %q)，期望 (%d, %q)", gotUser, gotJTI, 42, claims.ID)
			}
			if gotIssuedAt.IsZero() {
				t.Error("iat 没有传进吊销检查，用户级水位线将永远比不出结果")
			}
		})
	}
}

// TestAuthMiddleware_NoRevocationStoreKeepsWorking：没有 Redis 的部署不注册检查，
// 鉴权行为与引入吊销能力之前一致。
func TestAuthMiddleware_NoRevocationStoreKeepsWorking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestConfig(t)

	original := sessionCheck
	t.Cleanup(func() { sessionCheck = original })
	sessionCheck = nil

	access, _, err := pkg.GenerateTokens(7, "admin", nil)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/api/short_urls", nil)
	c.Request.Header.Set("Authorization", "Bearer "+access)
	Auth()(c)

	if c.IsAborted() || w.Code != http.StatusOK {
		t.Errorf("未注册吊销检查时应照常放行，得到 status=%d aborted=%v", w.Code, c.IsAborted())
	}
}
