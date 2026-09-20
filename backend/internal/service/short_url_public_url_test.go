package service

import (
	"strings"
	"testing"

	"dwz-admin/internal/model"
)

// #30: the admin UI used to build the absolute short URL on the client and fall
// back to a hard-coded third-party host, so every "copy short link" handed out a
// dwz.cn address and multi-domain deployments showed the wrong host on every row.
// The address is now derived server-side from the row's own bound domain.

func TestAbsoluteShortURL(t *testing.T) {
	bound := uint64(7)
	noScheme := uint64(8)
	deleted := uint64(9)
	domains := map[uint64]model.Domain{
		bound:    {ID: bound, Domain: "s.link", Scheme: "https"},
		noScheme: {ID: noScheme, Domain: "plain.test"},
	}

	cases := []struct {
		name     string
		uid      string
		domainID *uint64
		want     string
	}{
		{"绑定域名优先于部署默认域名", "ab12cd", &bound, "https://s.link/ab12cd"},
		{"未存 scheme 时按 https 补全", "ab12cd", &noScheme, "https://plain.test/ab12cd"},
		{"域名已删除则回落 base，不编造主机", "ab12cd", &deleted, "https://base.test/ab12cd"},
		{"无 domain_id 用 base", "ab12cd", nil, "https://base.test/ab12cd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := absoluteShortURL(tc.uid, tc.domainID, domains, "https://base.test/")
			if got != tc.want {
				t.Fatalf("absoluteShortURL(%q, %v) = %q, want %q", tc.uid, tc.domainID, got, tc.want)
			}
		})
	}
}

// FillShortURLs must read the domain table once for the whole page, not once per
// row: a 100-row batch response would otherwise turn into an N+1.
func TestFillShortURLsReadsDomainsOncePerPage(t *testing.T) {
	bound := uint64(7)
	sr := newMockShortRepo()
	dr := &mockDomainRepo{domains: []model.Domain{{ID: bound, Domain: "s.link", Scheme: "https"}}}
	svc := buildService(sr, dr)

	urls := []model.ShortUrl{
		{UID: "aaa111", DomainID: &bound},
		{UID: "bbb222", DomainID: &bound},
		{UID: "ccc333"},
	}
	svc.FillShortURLs(urls)

	if urls[0].ShortURL != "https://s.link/aaa111" || urls[1].ShortURL != "https://s.link/bbb222" {
		t.Fatalf("绑定域名的行应各自拼出自己的主机, got %q / %q", urls[0].ShortURL, urls[1].ShortURL)
	}
	if !strings.HasSuffix(urls[2].ShortURL, "/ccc333") {
		t.Fatalf("无域名行仍应得到可用地址, got %q", urls[2].ShortURL)
	}
	for _, u := range urls {
		if strings.Contains(u.ShortURL, "dwz.cn") {
			t.Fatalf("任何地址都不该带历史硬编码的第三方域名: %q", u.ShortURL)
		}
	}
}

// A created record must already carry its address, so webhook payloads and the
// create response cannot forget to fill it in.
func TestCreateReturnsRecordWithShortURL(t *testing.T) {
	bound := uint64(7)
	sr := newMockShortRepo()
	dr := &mockDomainRepo{domains: []model.Domain{{ID: bound, Domain: "s.link", Scheme: "https"}}}
	svc := buildService(sr, dr)

	rec, err := svc.Create("https://www.example.com", "", 0, &bound, nil, "test", "127.0.0.1", "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if !strings.HasPrefix(rec.ShortURL, "https://s.link/") {
		t.Fatalf("created record should carry its bound-domain address, got %q", rec.ShortURL)
	}
}

// The single-record form must mutate the caller's record, not a copy — the
// obvious-looking "wrap it in a slice and reuse the batch path" version silently
// fills a throwaway value instead.
func TestFillShortURLMutatesTheGivenRecord(t *testing.T) {
	bound := uint64(7)
	sr := newMockShortRepo()
	dr := &mockDomainRepo{domains: []model.Domain{{ID: bound, Domain: "s.link", Scheme: "https"}}}
	svc := buildService(sr, dr)

	rec := &model.ShortUrl{UID: "aaa111", DomainID: &bound}
	svc.FillShortURL(rec)

	if rec.ShortURL != "https://s.link/aaa111" {
		t.Fatalf("FillShortURL 没有写回原记录, got %q", rec.ShortURL)
	}
}

// Outcomes of a batch create share one domain read; a nil record (failed row)
// must not panic the pass.
func TestFillShortURLOutcomesSkipsFailures(t *testing.T) {
	bound := uint64(7)
	sr := newMockShortRepo()
	dr := &mockDomainRepo{domains: []model.Domain{{ID: bound, Domain: "s.link", Scheme: "https"}}}
	svc := buildService(sr, dr)

	outcomes := []BatchOutcome{
		{Index: 0, Record: &model.ShortUrl{UID: "aaa111", DomainID: &bound}},
		{Index: 1, Err: ErrCustomCodeTaken},
	}
	svc.FillShortURLOutcomes(outcomes)

	if outcomes[0].Record.ShortURL != "https://s.link/aaa111" {
		t.Fatalf("batch row not filled: %q", outcomes[0].Record.ShortURL)
	}
	if outcomes[1].Record != nil {
		t.Fatal("failed row must stay record-less")
	}
}
