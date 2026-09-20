package service

import (
	"net"
	"testing"
)

// TestSSRFParityWithPHP pins the address classes the Go path must reject so the
// two create paths cannot drift again (#63).
//
// PHP uses filter_var(..., FILTER_FLAG_NO_PRIV_RANGE | FILTER_FLAG_NO_RES_RANGE);
// Go's net.IP helpers cover loopback/private/link-local but NOT the reserved
// block, so 0.0.0.0/8, 100.64.0.0/10, 192.0.2.0/24, 198.18.0.0/15 and
// 240.0.0.0/4 were accepted by Go and refused by PHP — the same URL got
// different security verdicts depending on which stack created it.
//
// If this fails after a Go/range edit, update reservedRanges AND the PHP side in
// the same commit; the whole point is that they agree.
func TestSSRFParityWithPHP(t *testing.T) {
	blocked := []string{
		// loopback / unspecified
		"127.0.0.1", "127.1.2.3", "0.0.0.0", "::1", "::",
		// RFC1918
		"10.0.0.1", "172.16.0.1", "192.168.1.1",
		// link-local, incl. cloud metadata
		"169.254.169.254", "fe80::1",
		// reserved / documentation / CGNAT: these are the ones Go used to accept
		"100.64.0.1", "192.0.0.1", "192.0.2.5", "198.18.0.1",
		"198.51.100.7", "203.0.113.9", "240.0.0.1", "255.255.255.255",
		"2001:db8::1",
		// multicast
		"224.0.0.1",
	}
	for _, s := range blocked {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("test fixture %q is not a valid IP", s)
		}
		if !isPrivateIP(ip) {
			t.Errorf("isPrivateIP(%s) = false; PHP rejects it, so Go must too", s)
		}
	}

	allowed := []string{
		"1.1.1.1", "8.8.8.8", "93.184.216.34", "2606:4700:4700::1111",
	}
	for _, s := range allowed {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("test fixture %q is not a valid IP", s)
		}
		if isPrivateIP(ip) {
			t.Errorf("isPrivateIP(%s) = true; public address must stay allowed", s)
		}
	}
}

// TestClassifyHostLocalNames covers the non-IP rejections the PHP side also does
// (localhost, *.local, empty).
func TestClassifyHostLocalNames(t *testing.T) {
	for _, host := range []string{"", "localhost", "LOCALHOST", "printer.local", "2001:db8::1"} {
		priv, err := classifyHost(host)
		if err != nil {
			t.Fatalf("classifyHost(%q) returned error %v", host, err)
		}
		if !priv {
			t.Errorf("classifyHost(%q) = false, want true", host)
		}
	}
}
