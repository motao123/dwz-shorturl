package handler

import "testing"

// GetAll() 把敏感项显示成 ******，所以任何一次往返（运维碰了那个输入框、
// 或 API 客户端把整份 GetAll() 原样 PUT 回来）都会把真实 SMTP 口令覆盖成
// 字面量 "******" —— 静默且不可恢复。掩码必须被当成"未修改"。
func TestResolveConfigValueKeepsSecretOnMaskRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		key      string
		incoming string
		stored   string
		want     string
	}{
		{"敏感项收到掩码则保留原值", "smtp.password", configValueMask, "s3cr3t-smtp", "s3cr3t-smtp"},
		{"敏感项收到新值则写入", "smtp.password", "new-pass", "s3cr3t-smtp", "new-pass"},
		{"密钥类同样受保护", "jwt.secret", configValueMask, "abc123", "abc123"},
		{"非敏感项的 ****** 是用户真实输入，照写", "site.name", configValueMask, "旧站名", configValueMask},
		{"清空敏感项仍按提交值处理（留空是显式动作）", "smtp.password", "", "s3cr3t-smtp", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveConfigValue(tc.key, tc.incoming, tc.stored); got != tc.want {
				t.Fatalf("resolveConfigValue(%q,%q,%q) = %q, want %q",
					tc.key, tc.incoming, tc.stored, got, tc.want)
			}
		})
	}
}
