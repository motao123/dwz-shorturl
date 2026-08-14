package pkg

import (
	"net/url"
	"strings"
)

// ViolationStatus is the review state of a shortened URL.
type ViolationStatus string

const (
	ViolationPending  ViolationStatus = "pending"  // detection not yet run
	ViolationPassed   ViolationStatus = "passed"   // looks safe
	ViolationReview   ViolationStatus = "review"   // suspicious, needs human review
	ViolationBlocked  ViolationStatus = "blocked"  // clearly violating, do not serve
)

// ViolationResult carries the outcome of a synchronous violation check.
type ViolationResult struct {
	Status ViolationStatus
	Reason string
}

// blockedDomainSuffixes are exact-match or suffix-match domains that are banned
// outright (gambling, scam, phishing, spam). Suffix match is against the
// fully-qualified host, so "example.com" also matches "www.example.com".
var blockedDomainSuffixes = []string{
	// gambling / 菠菜
	"bet365.com", "188bet.com", "bwin.com", "dafabet.com", "m88.com",
	"18win.com", "betway.com", "w88.com", "fun88.com", "12bet.com",
	"10bet.com", "365bet.com", "betflik.com", "ufa999.com", "pgslot.com",
	"slotxo.com", "milton88.com", "boss888.com", "mgm888.com", "jackpot.com",
	// scam / phishing / 仿冒
	"phishing.com", "scam.com", "bitcoin-cash.cc", "secure-login-verify.com",
	"account-verify.com", "paypal-verify.com", "appleid-verify.com",
	// spam / malicious download
	"malware.com", "trojan-download.com", "spam-site.com",
}

// blockedKeywords are case-insensitive substring rules fired on the full URL.
// They catch obvious gambling / scam intent even when the domain is not listed.
var blockedKeywords = []string{
	// 菠菜 / gambling
	"betting", "casino", "poker", "slot", "gambling", "老虎机", "百家乐",
	"轮盘", "德州扑克", "棋牌充值", "博彩", "赌场", "网络赌博", "太阳城",
	"澳门娱乐场", "六合彩", "外围赌", "重庆时时彩", "彩票投注",
	// 仿冒 / phishing
	"password reset please login", "verify your account immediately",
	"登录验证您的账户", "您的账户已被冻结", "账户异常请立即验证",
	// 恶意下载 / malware
	"download-virus", "free-crack", "keygen", "盗版激活码",
	// 诈骗 / scam
	"刷单返利", "稳赚不赔", "高额回报", "保本保息", "先付后取",
}

// CheckURLViolation performs a synchronous, rule-based violation check on a
// destination URL. It NEVER performs network fetches (the URL is not resolved
// here), so it is safe and fast. Detection returns one of:
//   - ViolationBlocked: clearly violating, the link must be rejected.
//   - ViolationReview:  suspicious pattern, flag for human review.
//   - ViolationPassed:  no obvious violation found.
func CheckURLViolation(rawURL string) ViolationResult {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ViolationResult{Status: ViolationBlocked, Reason: "url is invalid"}
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	lower := strings.ToLower(rawURL)

	// Cloud metadata endpoints are always blocked (SSRF defence-in-depth).
	if host == "169.254.169.254" ||
		strings.HasSuffix(host, ".169.254.169.254") ||
		host == "metadata.google.internal" ||
		strings.HasSuffix(host, "metadata.google.internal") {
		return ViolationResult{Status: ViolationBlocked, Reason: "cloud metadata address is not allowed"}
	}

	// TODO: fetch the page and keyword-scan its body (async deep detection).
	// The synchronous check below only inspects the URL itself.
	_ = lower

	// Exact / suffix domain blacklist.
	if host != "" {
		for _, d := range blockedDomainSuffixes {
			if host == d || strings.HasSuffix(host, "."+d) {
				return ViolationResult{Status: ViolationBlocked, Reason: "domain is blocked"}
			}
		}
	}

	// Substring keyword rules on the full URL.
	for _, kw := range blockedKeywords {
		if strings.Contains(lower, kw) {
			return ViolationResult{Status: ViolationBlocked, Reason: "url matches a blocked keyword"}
		}
	}

	return ViolationResult{Status: ViolationPassed, Reason: ""}
}