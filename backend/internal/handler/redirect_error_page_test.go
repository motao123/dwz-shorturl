package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

// 品牌化错误页：不再是裸文本，且必须带 noindex（短码是跳板，不应被索引）。
func TestRenderErrorPageIsBrandedHTML(t *testing.T) {
	cases := []struct {
		kind       errorPageKind
		status     int
		wantTitle  string
		wantMarker string
	}{
		{errorPageNotFound, http.StatusNotFound, "短链不存在", "返回首页"},
		{errorPageExpired, http.StatusGone, "这个短链已过期", "返回首页"},
		{errorPageDisabled, http.StatusGone, "这个短链已被停用", "返回首页"},
		{errorPageInvalidTarget, http.StatusGone, "这个短链暂时无法访问", "返回首页"},
	}

	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		renderErrorPage(c, tc.status, tc.kind)

		if w.Code != tc.status {
			t.Fatalf("kind %d: status = %d, want %d", tc.kind, w.Code, tc.status)
		}
		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Fatalf("kind %d: content-type = %q, want text/html", tc.kind, ct)
		}
		body := w.Body.String()
		if !strings.Contains(body, tc.wantTitle) {
			t.Errorf("kind %d: body missing title %q", tc.kind, tc.wantTitle)
		}
		if !strings.Contains(body, tc.wantMarker) {
			t.Errorf("kind %d: body missing marker %q", tc.kind, tc.wantMarker)
		}
		for _, want := range []string{"<html", "prefers-color-scheme", "noindex"} {
			if !strings.Contains(body, want) {
				t.Errorf("kind %d: body missing %q", tc.kind, want)
			}
		}
	}
}

// 过期/停用/目标无效 必须是三种不同文案，避免恢复成笼统的「Not Found」。
func TestErrorPageContentDistinguishesKinds(t *testing.T) {
	seen := map[string]errorPageKind{}
	for _, kind := range []errorPageKind{errorPageNotFound, errorPageExpired, errorPageDisabled, errorPageInvalidTarget} {
		title, desc, _ := errorPageContent(kind)
		if title == "" || desc == "" {
			t.Fatalf("kind %d: empty title/description", kind)
		}
		if prev, dup := seen[title]; dup {
			t.Fatalf("kind %d and %d share title %q", prev, kind, title)
		}
		seen[title] = kind
	}
}

// 密码页必须带暗色适配，且错误态与正常态都包含品牌化返回入口。
func TestRenderPasswordPageDarkModeAndBrand(t *testing.T) {
	body := renderPasswordPage("abc12345", "密码错误，请重试")
	for _, want := range []string{"prefers-color-scheme", "密码错误，请重试", "返回短网址首页", "noindex"} {
		if !strings.Contains(body, want) {
			t.Errorf("password page missing %q", want)
		}
	}
	// 正常态不应出现错误提示
	if ok := renderPasswordPage("abc12345", ""); strings.Contains(ok, "密码错误") {
		t.Error("password page without error should not render the error message")
	}
}

// 密码页必须转义 UID，避免未来调用方传入不可信字符串时被注入。
func TestRenderPasswordPageEscapesUID(t *testing.T) {
	body := renderPasswordPage(`"><script>alert(1)</script>`, "")
	if strings.Contains(body, "<script>") {
		t.Error("password page did not escape the interpolated uid")
	}
}

// 无 Redis 时密码尝试限流必须放行（可用性优先），且带限流器时可拦下超限请求。
func TestPasswordAttemptLimiterDegradesGracefully(t *testing.T) {
	h := &RedirectHandler{}
	if !h.passwordAttemptAllowed(newTestContext(), "abc12345") {
		t.Fatal("nil limiter must allow all attempts")
	}
}

func newTestContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/abc12345", nil)
	c.Request.RemoteAddr = "203.0.113.9:1234"
	return c
}
