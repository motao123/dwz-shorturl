package pkg

import (
	"net/url"
	"strings"
)

// Referer types used across stats aggregation.
const (
	RefererDirect  = "直接访问"
	RefererSearch  = "搜索引擎"
	RefererSocial  = "社交媒体"
	RefererOther   = "其他网站"
)

// searchDomains are hostname substrings that identify search-engine referrers.
var searchDomains = []string{
	"baidu.com", "google.", "bing.com", "sogou.com", "so.com", "sm.cn",
	"yandex.", "duckduckgo.com", "yahoo.", "360.cn", "ecosia.org", "search.brave.com",
}

// socialDomains are hostname substrings that identify social-media referrers.
var socialDomains = []string{
	"weibo.com", "weibo.cn", "weixin.qq.com", "qq.com", "douban.com", "zhihu.com",
	"x.com", "twitter.com", "facebook.", "instagram.com", "tiktok.com",
	"douyin.com", "bilibili.com", "reddit.com", "telegram.", "linkedin.com",
	"youtube.com", "xiaohongshu.com", "kuaishou.com", "meituan.com",
}

// ClassifyReferer buckets a raw HTTP Referer header into a coarse traffic
// source. Empty referers are "直接访问"; the rest fall into search, social or
// "其他网站" by hostname pattern.
func ClassifyReferer(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return RefererDirect
	}
	host := ref
	if u, err := url.Parse(ref); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	host = strings.ToLower(host)

	for _, d := range searchDomains {
		if strings.Contains(host, d) {
			return RefererSearch
		}
	}
	for _, d := range socialDomains {
		if strings.Contains(host, d) {
			return RefererSocial
		}
	}
	return RefererOther
}
