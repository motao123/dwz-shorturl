package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func originEngine(siteURL string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequireSameSiteOrigin(func() string { return siteURL }))
	r.POST("/member/api/links", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/member/api/links", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.PUT("/member/api/links/1", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.DELETE("/member/api/links/1", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	return r
}

const testHost = "links.test"

func newRequest(method, target, origin, referer, contentType string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	// httptest 默认 Host 是 example.com；不显式设成本站主机，"同源"就永远不成立。
	req.Host = testHost
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req
}

func do(r *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func post(r *gin.Engine, target, origin, referer, contentType string) *httptest.ResponseRecorder {
	return do(r, newRequest(http.MethodPost, target, origin, referer, contentType))
}

// #36：会员 API 可以用 cookie 完成鉴权，所以跨站请求只要浏览器愿意发出来就带着凭据。
// Origin 是无状态就能查的那一环。
func TestRequireSameSiteOriginAllowsSameSite(t *testing.T) {
	r := originEngine("")

	if w := post(r, "/member/api/links", "http://links.test", "", "application/json"); w.Code != http.StatusOK {
		t.Fatalf("同源 JSON POST 应放行, got %d", w.Code)
	}
	// 真实的攻击形状：CORS 简单请求不需要预检，text/plain 直达，但 Origin 一定会带。
	if w := post(r, "/member/api/links", "http://links.test", "", "text/plain"); w.Code != http.StatusOK {
		t.Fatalf("同源 text/plain 简单请求应放行, got %d", w.Code)
	}
	if w := post(r, "/member/api/links", "", "http://links.test/member/", ""); w.Code != http.StatusOK {
		t.Fatalf("只有 Referer 的同源请求应放行, got %d", w.Code)
	}
	if w := post(r, "/member/api/links", "HTTP://LINKS.TEST", "", ""); w.Code != http.StatusOK {
		t.Fatalf("Origin 大小写不应影响判定, got %d", w.Code)
	}
}

func TestRequireSameSiteOriginBlocksCrossSite(t *testing.T) {
	r := originEngine("")

	cases := map[string]string{
		"外站 Origin":            "http://evil.test",
		"后缀伪装成本站的恶意域": "https://links.test.evil.test",
		"端口不符":                "http://links.test:9999",
	}
	for name, origin := range cases {
		if w := post(r, "/member/api/links", origin, "", "text/plain"); w.Code != http.StatusForbidden {
			t.Errorf("%s 应被拒, got %d (%s)", name, w.Code, origin)
		}
	}
	if w := post(r, "/member/api/links", "", "http://evil.test/attack", ""); w.Code != http.StatusForbidden {
		t.Errorf("Referer 指向外站应被拒, got %d", w.Code)
	}
	// 查询串与路径不能成为绕过的入口。
	if w := post(r, "/member/api/links", "http://links.test/?x=evil", "", ""); w.Code == http.StatusForbidden {
		t.Errorf("同源带路径的 Origin 不该被拒")
	}
}

// 不带 Origin/Referer 的是非浏览器客户端（curl、服务端到服务端、探针），
// 跨站页面无法伪造出这种请求，因此必须放行——否则会打断合法集成。
func TestRequireSameSiteOriginAllowsHeaderlessClients(t *testing.T) {
	r := originEngine("")
	targets := map[string]string{
		http.MethodPost:   "/member/api/links",
		http.MethodPut:    "/member/api/links/1",
		http.MethodDelete: "/member/api/links/1",
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		if w := do(r, newRequest(m, targets[m], "", "", "")); w.Code != http.StatusOK {
			t.Errorf("%s 无 Origin 的客户端应放行, got %d", m, w.Code)
			return
		}
	}
}

// GET 一类的安全方法不参与判定（会员中心大量依赖只读接口）。
func TestRequireSameSiteOriginIgnoresSafeMethods(t *testing.T) {
	r := originEngine("")
	if w := do(r, newRequest(http.MethodGet, "/member/api/links", "http://evil.test", "", "")); w.Code != http.StatusOK {
		t.Fatalf("GET 不应受同源限制, got %d", w.Code)
	}
}

// CDN/自定义域前置时，Host 与运维配置的站点地址会不同：两个都要认。
func TestRequireSameSiteOriginAcceptsConfiguredSite(t *testing.T) {
	r := originEngine("https://public.cdn.test")
	if w := post(r, "/member/api/links", "https://public.cdn.test", "", ""); w.Code != http.StatusOK {
		t.Fatalf("配置的站点地址应被接受, got %d", w.Code)
	}
	if w := post(r, "/member/api/links", "https://other.test", "", ""); w.Code != http.StatusForbidden {
		t.Fatalf("既不匹配 Host 也不匹配站点地址时应拒绝, got %d", w.Code)
	}
}
