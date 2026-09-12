package handler

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 品牌页（错误页 / 密码页）在 PHP（do.php + includes/function.php）与
// Go（redirect.go）两条跳转路径上各有一份实现。两份实现**无法做到字节一致**
// （Go 用反引号模板含真实换行，PHP 是单行拼接），因此这里锁定的是
// 「可验证的结构契约」：同一套 CSS 变量、暗色适配、noindex、品牌入口。
//
// 任何一侧改动如果破坏了契约（例如暗色变量被删、noindex 被去掉、
// 文案被改成笼统的 Not Found），这个测试会失败，避免两条路径悄悄漂移。
//
// 若日后改为共享模板（例如把样式抽成静态 CSS 由两侧引用），可把本测试
// 升级为对产物的字节比对。

// cssVars 是从两份实现中必须共同出现的品牌令牌。
var brandPageCSSVars = []string{
	"--ep-page:", "--ep-card:", "--ep-line:", "--ep-text:",
	"--ep-dim:", "--ep-brand:", "--ep-brand-hover",
}

var passwordPageCSSVars = []string{
	"--pw-page:", "--pw-card:", "--pw-line:", "--pw-text:",
	"--pw-dim:", "--pw-input:", "--pw-brand:", "--pw-brand-hover",
}

func phpSource(t *testing.T, rel string) string {
	t.Helper()
	root := filepath.Join("..", "..", "..")
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// PHP 侧错误页必须包含与 Go 侧相同的品牌变量与暗色适配。
func TestPHPErrorPageMatchesGoBrandContract(t *testing.T) {
	php := phpSource(t, "do.php")
	for _, v := range brandPageCSSVars {
		if !strings.Contains(php, v) {
			t.Errorf("do.php error page missing brand CSS var %q", v)
		}
	}
	for _, want := range []string{"prefers-color-scheme:dark", "X-Robots-Tag: noindex", "返回首页"} {
		if !strings.Contains(php, want) {
			t.Errorf("do.php error page missing %q", want)
		}
	}

	goSrc := phpSource(t, filepath.Join("backend", "internal", "handler", "redirect.go"))
	for _, v := range brandPageCSSVars {
		if !strings.Contains(goSrc, v) {
			t.Errorf("redirect.go error page missing brand CSS var %q", v)
		}
	}
	for _, want := range []string{"prefers-color-scheme:dark", "noindex", "返回首页"} {
		if !strings.Contains(goSrc, want) {
			t.Errorf("redirect.go error page missing %q", want)
		}
	}
}

// PHP 侧密码页必须与 Go 侧共享同一套 --pw-* 变量、暗色适配与品牌入口。
func TestPHPPasswordPageMatchesGoBrandContract(t *testing.T) {
	php := phpSource(t, filepath.Join("includes", "function.php"))
	for _, v := range passwordPageCSSVars {
		if !strings.Contains(php, v) {
			t.Errorf("function.php password page missing brand CSS var %q", v)
		}
	}
	for _, want := range []string{"prefers-color-scheme:dark", "返回短网址首页"} {
		if !strings.Contains(php, want) {
			t.Errorf("function.php password page missing %q", want)
		}
	}

	private := renderPasswordPage("abc12345", "")
	for _, v := range passwordPageCSSVars {
		if !strings.Contains(private, v) {
			t.Errorf("Go password page missing brand CSS var %q", v)
		}
	}
	if !strings.Contains(private, "返回短网址首页") {
		t.Error("Go password page missing brand back-link")
	}
}

// 错误页与密码页必须使用同一品牌色板。两份页面的深色值可以不同（深色本来就是
// 深色），因此这里只比对**浅色基准值**：同一语义令牌（page/card/line/text/dim/
// brand/brand-hover）在两个页面上的浅色取值必须一致，避免出现「错误页暗色适配
// 但密码页仍是白底」这类漂移。
func TestBrandPagesShareSamePalette(t *testing.T) {
	redirect := phpSource(t, filepath.Join("backend", "internal", "handler", "redirect.go"))

	valueRe := regexp.MustCompile(`(--(ep|pw)-[a-z-]+):(#[0-9a-fA-F]{3,8})`)

	// 取每个变量在源码中的**首个**取值。无论是密码页还是错误页，其浅色 :root
	// 声明都在自家 @media (prefers-color-scheme:dark) 之前，因此首次出现即浅色值。
	firstLight := func(prefix string) map[string]string {
		got := map[string]string{}
		for _, m := range valueRe.FindAllStringSubmatch(redirect, -1) {
			if !strings.HasPrefix(m[1], prefix) {
				continue
			}
			name := strings.TrimPrefix(m[1], prefix)
			if _, ok := got[name]; !ok {
				got[name] = m[3]
			}
		}
		return got
	}

	errLight := firstLight("--ep-")
	pwLight := firstLight("--pw-")
	if len(errLight) == 0 || len(pwLight) == 0 {
		t.Fatalf("failed to extract palettes: ep=%d pw=%d", len(errLight), len(pwLight))
	}

	for _, name := range []string{"page", "card", "line", "text", "dim", "brand", "brand-hover"} {
		ep, okEp := errLight[name]
		pw, okPw := pwLight[name]
		if !okEp || !okPw {
			continue
		}
		if ep != pw {
			t.Errorf("palette drift for %q: error page=%s password page=%s", name, ep, pw)
		}
	}
}
