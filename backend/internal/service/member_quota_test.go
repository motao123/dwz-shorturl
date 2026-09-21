package service

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// #41：会员建链路径既不校验 domain_id，也没有总量配额——第 7 批收口的
// member.api_rate_max 只限制"一秒能发多少请求"，摊长时间窗后仍可无限累积短链。

func TestCheckMemberQuotaBlocksAtCap(t *testing.T) {
	sr := newMockShortRepo()
	svc := buildService(sr, &mockDomainRepo{})

	// 未达上限：放行。
	sr.memberCount = 999
	if err := svc.CheckMemberQuota(7); err != nil {
		t.Fatalf("未达上限不应拒绝: %v", err)
	}
	// 达到默认上限 1000：拒绝。
	sr.memberCount = 1000
	if err := svc.CheckMemberQuota(7); err != ErrMemberQuotaExceeded {
		t.Fatalf("达到上限应返回配额错误, got %v", err)
	}
}

// 域名与配额必须在每一条会员建链入口上生效。用源码断言而非只测 CreateLink：
// 批量与导入同样经 createLink，但若日后新增第三条入口绕过它，行为测试不会发现。
func TestMemberCreatePathsEnforceDomainAndQuota(t *testing.T) {
	root := repoRootForService(t)
	src, err := os.ReadFile(filepath.Join(root, "backend", "internal", "service", "member_api.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	re := regexp.MustCompile(`(?s)func \(s \*memberApiService\) createLink\(.*?\n}`)
	m := re.FindString(body)
	if m == "" {
		t.Fatal("找不到 createLink；若重命名请同步本测试")
	}
	for _, want := range []string{"ValidateDomain(", "CheckMemberQuota("} {
		if !strings.Contains(m, want) {
			t.Errorf("createLink 不再调用 %s：会员建链的域名校验/配额又成了空话（#41）", want)
		}
	}
	// 配额必须在校验"同一 URL 已存在"之后：复用已有短链不该占额度。
	existing := strings.Index(m, "FindByHash(")
	quota := strings.Index(m, "CheckMemberQuota(")
	if existing >= 0 && quota > 0 && quota < existing {
		t.Error("配额检查排在去重之前：同一长链重复提交会白占额度（#41）")
	}
}
