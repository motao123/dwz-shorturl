package pkg

import (
	"os"
	"testing"
)

// TestIp2RegionLive verifies the v1 reader against a real database file. It is
// skipped unless IP2REGION_DB points at an ip2region.db file:
//
//	IP2REGION_DB=/path/to/ip2region.db go test ./internal/pkg/ -run TestIp2RegionLive -v
func TestIp2RegionLive(t *testing.T) {
	path := os.Getenv("IP2REGION_DB")
	if path == "" {
		t.Skip("IP2REGION_DB not set")
	}
	r, err := NewIp2Region(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	expect := []struct {
		ip   string
		want string // ISO code, "" = only check no error
	}{
		{"8.8.8.8", "US"},
		{"1.1.1.1", "AU"},
		{"114.114.114.114", "CN"},
		{"223.5.5.5", "CN"},
		{"202.96.128.86", "CN"},
	}
	for _, c := range expect {
		region, err := r.Search(c.ip)
		if err != nil {
			t.Errorf("%s: %v", c.ip, err)
			continue
		}
		code := CountryCodeFromRegion(region)
		t.Logf("%s -> %s (ISO %s)", c.ip, region, code)
		if c.want != "" && code != c.want {
			t.Errorf("%s: got ISO %q want %q (region %q)", c.ip, code, c.want, region)
		}
	}
}
