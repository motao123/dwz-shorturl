package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// #26：导出曾经只读 keyword/status，运维按日期或分组筛完点导出，拿到的是全量表格，
// 屏幕上的「共 N 条」和文件行数永远对不上。现在两处共用一份解析。
func TestParseShortUrlFiltersReadsEveryFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet,
		"/admin/api/short-urls?keyword=abc&status=1&category_id=7&domain_id=3"+
			"&date_from=2026-01-02&date_to=2026-03-04&include_deleted=1&sort=clicks&order=asc", nil)

	f := parseShortUrlFilters(c)
	if f.Keyword != "abc" {
		t.Errorf("keyword 未解析: %q", f.Keyword)
	}
	if f.Status == nil || *f.Status != 1 {
		t.Errorf("status 未解析: %v", f.Status)
	}
	if f.CategoryID == nil || *f.CategoryID != 7 {
		t.Errorf("category_id 未解析: %v", f.CategoryID)
	}
	if f.DomainID == nil || *f.DomainID != 3 {
		t.Errorf("domain_id 未解析: %v", f.DomainID)
	}
	if !f.IncludeDeleted {
		t.Error("include_deleted 未解析")
	}
	if f.Sort != "clicks" || f.Order != "asc" {
		t.Errorf("排序未解析: %q/%q", f.Sort, f.Order)
	}
	if f.DateFrom == nil || f.DateFrom.Format("2006-01-02") != "2026-01-02" {
		t.Errorf("date_from 未解析: %v", f.DateFrom)
	}
	// date_to 要覆盖整天：23:59:59.999
	if f.DateTo == nil || !strings.HasSuffix(f.DateTo.Format("2006-01-02 15:04:05"), "03-04 23:59:59") {
		t.Errorf("date_to 应含整日, got %v", f.DateTo)
	}
}

// repoRootForHandler resolves the repository root from the package test dir
// (backend/internal/handler -> repo root).
func repoRootForHandler(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// 防止有人把两处再拆开：Export 与 List 都必须经由同一个解析函数。
// 断言源码而不是行为，是因为行为测试只覆盖今天存在的参数组合，而这条约束要防的
// 正是"下次给 List 加新筛选时忘了 Export"。
func TestListAndExportShareFilterParsing(t *testing.T) {
	root := repoRootForHandler(t)
	src, err := os.ReadFile(filepath.Join(root, "backend", "internal", "handler", "short_url.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	for _, fn := range []string{"func (h *ShortUrlHandler) List(", "func (h *ShortUrlHandler) Export("} {
		i := strings.Index(body, fn)
		if i < 0 {
			t.Fatalf("找不到 %s", fn)
		}
		end := strings.Index(body[i+10:], "\nfunc ")
		if end < 0 {
			end = len(body) - i - 10
		}
		if !strings.Contains(body[i:i+10+end], "parseShortUrlFilters(c)") {
			t.Errorf("%s 未使用 parseShortUrlFilters：两处筛选解析会漂移（#26）", fn)
		}
	}
}
