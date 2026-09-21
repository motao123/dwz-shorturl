package pkg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// forceReload 关掉节流，让下一次读取一定去 stat 文件。
func forceReload() {
	rulesMu.Lock()
	rulesCheckedAt = time.Time{}
	rulesMu.Unlock()
}

// restoreEmbeddedRules 把全局状态放回内嵌词库，避免本测试污染同包其它断言
// （热加载状态是包级全局的，测试顺序不该改变结果）。
func restoreEmbeddedRules(t *testing.T) {
	t.Helper()
	r, err := parseViolationRules(violationRulesJSON)
	if err != nil {
		t.Fatalf("内嵌词库解析失败: %v", err)
	}
	rulesMu.Lock()
	defer rulesMu.Unlock()
	rulesCur = &violationRuleSet{suffixes: r.DomainSuffixes, keywords: r.Keywords, source: "embedded"}
	rulesMTime = time.Time{}
	rulesCheckedAt = time.Time{}
	rulesErr = nil
}

func writeRules(t *testing.T, path string, suffixes, keywords []string, mtime time.Time) {
	t.Helper()
	b, err := json.Marshal(violationRules{DomainSuffixes: suffixes, Keywords: keywords})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// #33：Go 侧此前只有 go:embed + sync.Once，应急加黑名单时 PHP 立即生效、
// Go 要重编译重启才生效——两栈"内容一致但时效不一致"。
// 这里验证运维文件确实能改变裁决，且改坏文件不会把防御摘掉。
func TestViolationRulesHotReloadFromOperatorFile(t *testing.T) {
	t.Cleanup(func() { restoreEmbeddedRules(t) })
	restoreEmbeddedRules(t)

	dir := t.TempDir()
	file := filepath.Join(dir, "violation_rules.json")
	t.Setenv(ViolationRulesEnv, file)

	t1 := time.Now().Add(-2 * time.Hour)
	writeRules(t, file, []string{"bad-a.example"}, []string{"kw-a"}, t1)
	forceReload()

	if got := BlockedDomainSuffixes(); len(got) != 1 || got[0] != "bad-a.example" {
		t.Fatalf("运维文件应覆盖内嵌词库, got %v", got)
	}
	if src := ViolationRulesSource(); src != file {
		t.Errorf("来源应报告文件路径, got %q", src)
	}
	if res := CheckURLViolation("https://bad-a.example/x"); res.Status != ViolationBlocked {
		t.Errorf("新黑名单域名应被拦截, got %+v", res)
	}
	if res := CheckURLViolation("https://still-ok.example/x"); res.Status == ViolationBlocked {
		t.Errorf("未列入黑名单的地址不应被拦, got %+v", res)
	}

	// 换内容 + 换 mtime：应被拾起
	t2 := t1.Add(time.Hour)
	writeRules(t, file, []string{"bad-a.example", "bad-b.example"}, []string{"kw-a"}, t2)
	forceReload()
	if got := BlockedDomainSuffixes(); len(got) != 2 {
		t.Fatalf("mtime 变化后应重载, got %v", got)
	}
	if res := CheckURLViolation("https://bad-b.example/x"); res.Status != ViolationBlocked {
		t.Errorf("新增条目应立即生效, got %+v", res)
	}

	// 改坏文件：必须保住上一份好词库，并把错误暴露出来
	if err := os.WriteFile(file, []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	t3 := t2.Add(time.Hour)
	if err := os.Chtimes(file, t3, t3); err != nil {
		t.Fatal(err)
	}
	forceReload()
	if got := BlockedDomainSuffixes(); len(got) != 2 {
		t.Fatalf("词库文件损坏时必须沿用上一份好规则, got %v", got)
	}
	if ViolationRulesError() == nil {
		t.Error("词库文件损坏必须可通过 ViolationRulesError() 读到，否则坏编辑不可见")
	}
	if res := CheckURLViolation("https://bad-b.example/x"); res.Status != ViolationBlocked {
		t.Errorf("坏编辑期间仍须拦截已知违规域名, got %+v", res)
	}
}

// 文件没变化时不该反复重解析（热路径上每次建链都会调检查）。
func TestViolationRulesReloadIsThrottled(t *testing.T) {
	t.Cleanup(func() { restoreEmbeddedRules(t) })
	restoreEmbeddedRules(t)

	dir := t.TempDir()
	file := filepath.Join(dir, "rules.json")
	t.Setenv(ViolationRulesEnv, file)
	writeRules(t, file, []string{"one.example"}, []string{"k"}, time.Now().Add(-time.Hour))

	rulesMu.Lock()
	rulesCheckedAt = time.Time{}
	rulesMu.Unlock()
	if len(BlockedDomainSuffixes()) != 1 {
		t.Fatal("首次读取未生效")
	}

	// 只改内容、不改 mtime：节流窗口内不该重新 stat，也就该沿用当前集合。
	// （真正的应急改法会写新文件，mtime 必然变化，见上一个测试。）
	rulesMu.Lock()
	keep := rulesCheckedAt
	rulesMu.Unlock()
	if keep.IsZero() {
		t.Fatal("读取时间未被记录，节流失效")
	}
	_ = os.WriteFile(file, []byte(`{"domain_suffixes":["a","b","c"],"keywords":[]}`), 0o600)
	if got := len(BlockedDomainSuffixes()); got != 1 {
		t.Errorf("节流窗口内不应重载（否则每次建链都要重解析文件）, got %d 条", got)
	}
}

// 两栈必须看同一个路径：PHP 侧 includes/function.php 的候选顺序里必须有同一份。
// 这条断言防的是"只改一边"——那正是 #33 的原始形状。
func TestViolationRulesPathMatchesPHPLoader(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "includes", "function.php"))
	if err != nil {
		t.Fatal(err)
	}
	php := string(b)
	if !strings.Contains(php, "'"+ViolationRulesPath+"'") {
		t.Fatalf("PHP 词库候选路径里没有 %q：两栈会读不同文件（#33）", ViolationRulesPath)
	}
	if !strings.Contains(php, "$violation_rules_file") {
		t.Error("PHP 侧不再支持 $violation_rules_file 覆盖，Go 的 DWZ_VIOLATION_RULES_FILE 就成了单边能力")
	}
}
