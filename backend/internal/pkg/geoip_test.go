package pkg

import "testing"

func TestCountryCodeFromRegion(t *testing.T) {
	cases := map[string]string{
		"中国|0|广东省|深圳市|电信": "CN",
		"美国|0|0|0|0":         "US",
		"日本|0|0|0|0":         "JP",
		"中国香港|0|0|0|0":      "HK",
		"内网IP|0|0|0|内网IP":    "CN",
		"0|0|0|0|0":            "",
		"":                     "",
		"US|0|0|0|0":           "US", // 兜底：双字母英文直接返回
	}
	for in, want := range cases {
		if got := CountryCodeFromRegion(in); got != want {
			t.Errorf("CountryCodeFromRegion(%q) = %q, want %q", in, got, want)
		}
	}
}
