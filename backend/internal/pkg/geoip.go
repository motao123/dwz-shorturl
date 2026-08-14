package pkg

import (
	"encoding/binary"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
)

// Ip2Region is a minimal reader for the ip2region v1 binary database
// (data/ip2region.db). It performs a binary search over the index blocks and
// returns the raw region string. The whole file is kept in memory; lookups are
// O(log N) and safe for concurrent use.
type Ip2Region struct {
	mu            sync.Mutex
	data          []byte
	firstIndexPtr uint32
	lastIndexPtr  uint32
	totalBlocks   uint32
}

// NewIp2Region loads an ip2region v1 .db file into memory.
func NewIp2Region(path string) (*Ip2Region, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) < 8 {
		return nil, errors.New("invalid ip2region db (too small)")
	}
	first := binary.LittleEndian.Uint32(b[0:4])
	last := binary.LittleEndian.Uint32(b[4:8])
	if first == 0 || last <= first || last >= uint32(len(b)) {
		return nil, errors.New("invalid ip2region db header")
	}
	return &Ip2Region{
		data:          b,
		firstIndexPtr: first,
		lastIndexPtr:  last,
		totalBlocks:   (last - first)/12 + 1,
	}, nil
}

// Search returns the raw region string for an IPv4 string, e.g.
// "中国|0|广东省|深圳市|电信" or "美国|0|0|0|0". Returns an error for
// non-IPv4 input or when the IP is not covered.
func (r *Ip2Region) Search(ip string) (string, error) {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil || parsed.To4() == nil {
		return "", errors.New("not an ipv4 address")
	}
	ipInt := binary.BigEndian.Uint32(parsed.To4())

	r.mu.Lock()
	defer r.mu.Unlock()

	lo, hi := uint32(0), r.totalBlocks-1
	for lo <= hi {
		mid := lo + (hi-lo)/2
		off := r.firstIndexPtr + mid*12
		if int(off+12) > len(r.data) {
			return "", errors.New("index out of range")
		}
		startIP := binary.LittleEndian.Uint32(r.data[off : off+4])
		endIP := binary.LittleEndian.Uint32(r.data[off+4 : off+8])

		switch {
		case ipInt < startIP:
			hi = mid - 1
		case ipInt > endIP:
			lo = mid + 1
		default:
			// 索引块第 3 字段为打包指针：高 8 位=数据长度(含 4 字节 city_id)，低 24 位=文件偏移
			packed := binary.LittleEndian.Uint32(r.data[off+8 : off+12])
			dataLen := int((packed >> 24) & 0xff)
			dataPos := int(packed & 0x00ffffff)
			if dataLen < 4 || dataPos+dataLen > len(r.data) {
				return "", errors.New("data block out of range")
			}
			// 数据块：city_id(4 字节) + "国家|省份|城市|ISP" UTF-8 字符串
			return string(r.data[dataPos+4 : dataPos+dataLen]), nil
		}
	}
	return "", errors.New("ip not found")
}

// countryNameToCode maps ip2region's Chinese country field to ISO 3166-1
// alpha-2 codes (the click_logs.country column is varchar(2)). Entries cover the
// long tail seen on a Chinese-facing short URL service.
var countryNameToCode = map[string]string{
	"中国": "CN", "内网IP": "CN", "中国香港": "HK", "中国澳门": "MO", "中国台湾": "TW",
	"美国": "US", "日本": "JP", "韩国": "KR", "朝鲜": "KP", "新加坡": "SG",
	"英国": "GB", "德国": "DE", "法国": "FR", "意大利": "IT", "西班牙": "ES",
	"俄罗斯": "RU", "乌克兰": "UA", "荷兰": "NL", "比利时": "BE", "瑞士": "CH",
	"瑞典": "SE", "挪威": "NO", "芬兰": "FI", "丹麦": "DK", "波兰": "PL",
	"奥地利": "AT", "葡萄牙": "PT", "希腊": "GR", "爱尔兰": "IE", "捷克": "CZ",
	"澳大利亚": "AU", "新西兰": "NZ", "加拿大": "CA", "墨西哥": "MX",
	"印度": "IN", "印度尼西亚": "ID", "马来西亚": "MY", "泰国": "TH", "越南": "VN",
	"菲律宾": "PH", "巴基斯坦": "PK", "孟加拉国": "BD", "斯里兰卡": "LK",
	"尼泊尔": "NP", "缅甸": "MM", "柬埔寨": "KH", "老挝": "LA",
	"巴西": "BR", "阿根廷": "AR", "智利": "CL", "秘鲁": "PE", "哥伦比亚": "CO",
	"埃及": "EG", "南非": "ZA", "尼日利亚": "NG", "肯尼亚": "KE",
	"直布罗陀": "GI", "冰岛": "IS", "匈牙利": "HU", "罗马尼亚": "RO", "保加利亚": "BG",
	"克罗地亚": "HR", "塞尔维亚": "RS", "斯洛伐克": "SK", "斯洛文尼亚": "SI",
	"立陶宛": "LT", "拉脱维亚": "LV", "爱沙尼亚": "EE", "卢森堡": "LU",
	"摩纳哥": "MC", "马耳他": "MT", "塞浦路斯": "CY", "巴拿马": "PA",
	"哥斯达黎加": "CR", "乌拉圭": "UY", "厄瓜多尔": "EC", "委内瑞拉": "VE",
	"古巴": "CU", "牙买加": "JM", "多米尼加": "DO", "波多黎各": "PR",
	"摩洛哥": "MA", "阿尔及利亚": "DZ", "突尼斯": "TN", "利比亚": "LY",
	"埃塞俄比亚": "ET", "坦桑尼亚": "TZ", "乌干达": "UG", "加纳": "GH",
	"科特迪瓦": "CI", "喀麦隆": "CM", "安哥拉": "AO", "莫桑比克": "MZ",
	"赞比亚": "ZM", "津巴布韦": "ZW", "博茨瓦纳": "BW", "纳米比亚": "NA",
	"斐济": "FJ", "巴布亚新几内亚": "PG", "约旦": "JO", "伊拉克": "IQ",
	"科威特": "KW", "卡塔尔": "QA", "巴林": "BH", "阿曼": "OM", "黎巴嫩": "LB",
	"阿富汗": "AF", "格鲁吉亚": "GE", "亚美尼亚": "AM", "阿塞拜疆": "AZ",
	"白俄罗斯": "BY", "摩尔多瓦": "MD", "波黑": "BA", "北马其顿": "MK",
}

// CountryCode extracts an ISO 3166-1 alpha-2 code from an ip2region region
// string. Returns "" when the country is unknown or the input is empty.
func CountryCodeFromRegion(region string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		return ""
	}
	country := strings.Split(region, "|")[0]
	if country == "" || country == "0" {
		return ""
	}
	if code, ok := countryNameToCode[country]; ok {
		return code
	}
	// 兜底：ip2region 对部分境外 IP 直接用英文国名（如 "United States"）
	upper := strings.ToUpper(country)
	if len(upper) == 2 {
		return upper
	}
	return ""
}
