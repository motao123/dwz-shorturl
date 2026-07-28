package service

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"dwz-admin/internal/config"
	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

var (
	ErrURLInvalid       = errors.New("url is invalid")
	ErrURLTooLong       = errors.New("url too long")
	ErrSSRFBlocked      = errors.New("url host not allowed")
	ErrCustomCodeFormat = errors.New("custom code format error, must be 6-8 chars of a-z0-5")
	ErrCustomCodeTaken  = errors.New("custom code already taken")
	ErrCodeCollision    = errors.New("short code collision, please retry")
)

type ShortUrlService interface {
	Create(longURL, custom string, expireDays int, createdBy *uint64, source, ip string) (*model.ShortUrl, error)
	BatchCreate(urls []string, createdBy *uint64, ip string) ([]model.ShortUrl, []error)
	GetByID(id uint64) (*model.ShortUrl, error)
	Update(id uint64, longURL, title string, expireDays *int, status *int8, categoryID *uint64) (*model.ShortUrl, error)
	Delete(id uint64) error
	BatchDelete(ids []uint64) error
	List(page, perPage int, filters repository.ShortUrlFilters) ([]model.ShortUrl, int64, error)
	Export(filters repository.ShortUrlFilters) ([]byte, error)
	ResolveByUID(uid string) (*model.ShortUrl, error)
}

type shortUrlService struct {
	repo  repository.ShortUrlRepo
	rdb   *redis.Client
	db    *gorm.DB // raw DB for legacy table fallback queries
}

func NewShortUrlService(repo repository.ShortUrlRepo, rdb *redis.Client, db *gorm.DB) ShortUrlService {
	return &shortUrlService{repo: repo, rdb: rdb, db: db}
}

func (s *shortUrlService) Create(longURL, custom string, expireDays int, createdBy *uint64, source, ip string) (*model.ShortUrl, error) {
	longURL = strings.TrimSpace(longURL)
	if err := validateURL(longURL); err != nil {
		return nil, err
	}

	custom = strings.TrimSpace(custom)
	if custom != "" && !isValidCustomCode(custom) {
		return nil, ErrCustomCodeFormat
	}

	hash := md5Hash(longURL)

	// Check if URL already exists
	existing, err := s.repo.FindByHash(hash)
	if err == nil && existing != nil {
		if custom != "" && custom != existing.UID {
			return nil, errors.New("url already has a short link, cannot use different custom code")
		}
		// Renew if expired
		if existing.ExpireAt != nil && existing.ExpireAt.Before(time.Now()) {
			if expireDays > 0 {
				exp := time.Now().Add(time.Duration(expireDays) * 24 * time.Hour)
				existing.ExpireAt = &exp
			} else {
				existing.ExpireAt = nil
			}
			existing.Status = 1
			_ = s.repo.Update(existing)
		}
		return existing, nil
	}

	var expireAt *time.Time
	if expireDays > 0 {
		exp := time.Now().Add(time.Duration(expireDays) * 24 * time.Hour)
		expireAt = &exp
	}

	attempts := 12
	if custom != "" {
		attempts = 1
	}

	for attempt := 0; attempt < attempts; attempt++ {
		var uid string
		if custom != "" {
			uid = custom
		} else if attempt == 0 {
			uid = pkg.ShortURL(longURL)
		} else {
			salt := fmt.Sprintf("|%d|%s", attempt, pkg.GenerateRandomCode(8))
			uid = pkg.ShortURL(longURL + salt)
		}

		record := model.ShortUrl{
			UID:       uid,
			LongURL:   longURL,
			URLHash:   hash,
			Clicks:    0,
			Status:    1,
			ExpireAt:  expireAt,
			CreatedBy: createdBy,
			Source:    source,
			IP:        ip,
		}

		err := s.repo.Create(&record)
		if err == nil {
			return &record, nil
		}

		// Check if it's a duplicate key error
		if isDuplicateError(err) {
			// Check if the hash already exists (race condition)
			existing, findErr := s.repo.FindByHash(hash)
			if findErr == nil && existing != nil {
				return existing, nil
			}
			if custom != "" {
				return nil, ErrCustomCodeTaken
			}
			continue
		}
		return nil, err
	}

	return nil, ErrCodeCollision
}

func (s *shortUrlService) BatchCreate(urls []string, createdBy *uint64, ip string) ([]model.ShortUrl, []error) {
	results := make([]model.ShortUrl, 0, len(urls))
	errs := make([]error, len(urls))

	for i, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		record, err := s.Create(u, "", 0, createdBy, "batch", ip)
		if err != nil {
			errs[i] = err
		} else {
			results = append(results, *record)
		}
	}

	return results, errs
}

func (s *shortUrlService) GetByID(id uint64) (*model.ShortUrl, error) {
	return s.repo.FindByID(id)
}

func (s *shortUrlService) Update(id uint64, longURL, title string, expireDays *int, status *int8, categoryID *uint64) (*model.ShortUrl, error) {
	record, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}

	if longURL != "" {
		if err := validateURL(longURL); err != nil {
			return nil, err
		}
		record.LongURL = longURL
		record.URLHash = md5Hash(longURL)
	}

	if title != "" {
		record.Title = title
	}

	if expireDays != nil {
		if *expireDays > 0 {
			exp := time.Now().Add(time.Duration(*expireDays) * 24 * time.Hour)
			record.ExpireAt = &exp
		} else {
			record.ExpireAt = nil
		}
	}

	if status != nil {
		record.Status = *status
	}

	if categoryID != nil {
		record.CategoryID = categoryID
	}

	if err := s.repo.Update(record); err != nil {
		return nil, err
	}

	// Invalidate cache
	s.invalidateCache(record.UID)

	return record, nil
}

func (s *shortUrlService) Delete(id uint64) error {
	record, err := s.repo.FindByID(id)
	if err != nil {
		return err
	}
	s.invalidateCache(record.UID)
	return s.repo.SoftDelete(id)
}

func (s *shortUrlService) BatchDelete(ids []uint64) error {
	for _, id := range ids {
		record, err := s.repo.FindByID(id)
		if err == nil {
			s.invalidateCache(record.UID)
		}
	}
	return s.repo.BatchDelete(ids)
}

func (s *shortUrlService) List(page, perPage int, filters repository.ShortUrlFilters) ([]model.ShortUrl, int64, error) {
	return s.repo.List(page, perPage, filters)
}

func (s *shortUrlService) Export(filters repository.ShortUrlFilters) ([]byte, error) {
	// Fetch all matching records (up to 10000)
	urls, _, err := s.repo.List(1, 10000, filters)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.WriteString("uid,long_url,title,clicks,status,created_at\n")
	for _, u := range urls {
		longURL := strings.ReplaceAll(u.LongURL, "\"", "\"\"")
		title := strings.ReplaceAll(u.Title, "\"", "\"\"")
		line := fmt.Sprintf("%s,\"%s\",\"%s\",%d,%d,%s\n",
			u.UID, longURL, title, u.Clicks, u.Status, u.CreatedAt.Format(time.RFC3339))
		buf.WriteString(line)
	}

	return buf.Bytes(), nil
}

func (s *shortUrlService) ResolveByUID(uid string) (*model.ShortUrl, error) {
	// Try Redis cache first
	ctx := s.rdb.Context()
	cacheKey := "shorturl:" + uid

	cached, err := s.rdb.Get(ctx, cacheKey).Result()
	if err == nil && cached != "" {
		// Cache hit - build a minimal ShortUrl from cached data
		record := &model.ShortUrl{
			UID:     uid,
			LongURL: cached,
			Status:  1,
		}
		return record, nil
	}

	// Cache miss - query short_urls table first
	record, err := s.repo.FindByUID(uid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Fallback: try legacy wjoy_log table (PHP-era data not yet migrated)
			record = s.resolveFromLegacy(uid)
			if record == nil {
				return nil, errors.New("short url not found")
			}
		} else {
			return nil, err
		}
	}

	// Check expiry
	if record.ExpireAt != nil && record.ExpireAt.Before(time.Now()) {
		return nil, errors.New("short url expired")
	}

	if record.Status != 1 {
		return nil, errors.New("short url disabled")
	}

	// Write to cache with TTL
	s.rdb.Set(ctx, cacheKey, record.LongURL, time.Hour)

	return record, nil
}

// resolveFromLegacy queries the PHP-era wjoy_log table as a fallback.
// Columns: uid, longurl (no underscore), expire_at, clicks.
func (s *shortUrlService) resolveFromLegacy(uid string) *model.ShortUrl {
	if s.db == nil {
		return nil
	}

	type legacyRow struct {
		LongURL  string `gorm:"column:longurl"`
		ExpireAt *time.Time
	}
	var row legacyRow
	// Check if wjoy_log table exists before querying (avoids noise on fresh installs)
	var tableExists int64
	s.db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'wjoy_log'").Scan(&tableExists)
	if tableExists == 0 {
		return nil
	}

	err := s.db.Table("wjoy_log").
		Select("longurl, expire_at").
		Where("uid = ?", uid).
		Limit(1).
		Scan(&row).Error
	if err != nil || row.LongURL == "" {
		return nil
	}

	// Handle base64-encoded legacy URLs
	if dec, decErr := base64decodeSafe(row.LongURL); decErr == nil && dec != "" {
		row.LongURL = dec
	}

	return &model.ShortUrl{
		UID:      uid,
		LongURL:  row.LongURL,
		Status:   1,
		ExpireAt: row.ExpireAt,
		Source:   "legacy",
	}
}

func (s *shortUrlService) invalidateCache(uid string) {
	ctx := s.rdb.Context()
	s.rdb.Del(ctx, "shorturl:"+uid)
}

// validateURL performs SSRF-safe validation of a URL.
func validateURL(rawURL string) error {
	if rawURL == "" {
		return ErrURLInvalid
	}
	if len(rawURL) > 2048 {
		return ErrURLTooLong
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ErrURLInvalid
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ErrURLInvalid
	}

	if parsed.Hostname() == "" {
		return ErrURLInvalid
	}

	if parsed.User != nil {
		return ErrURLInvalid
	}

	if parsed.Port() != "" {
		// Port is present, it's fine as long as it's valid (url.Parse handles this)
	}

	if isPrivateHost(parsed.Hostname()) {
		return ErrSSRFBlocked
	}

	return nil
}

func isPrivateHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".local") {
		return true
	}

	ip := net.ParseIP(host)
	if ip != nil {
		return isPrivateIP(ip)
	}

	// Resolve DNS
	addrs, err := net.LookupHost(host)
	if err != nil || len(addrs) == 0 {
		return true
	}

	for _, addr := range addrs {
		resolved := net.ParseIP(addr)
		if resolved == nil || isPrivateIP(resolved) {
			return true
		}
	}

	return false
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	return false
}

func isValidCustomCode(code string) bool {
	if len(code) < 6 || len(code) > 8 {
		return false
	}
	for _, c := range code {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '5')) {
			return false
		}
	}
	return true
}

func md5Hash(input string) string {
	h := md5.Sum([]byte(input))
	return hex.EncodeToString(h[:])
}

func isDuplicateError(err error) bool {
	return strings.Contains(err.Error(), "Duplicate entry") || strings.Contains(err.Error(), "1062")
}

// base64decodeSafe decodes a base64 string and returns the decoded value
// only if it looks like a valid http(s) URL. Used for legacy wjoy_log rows.
func base64decodeSafe(s string) (string, error) {
	dec, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	result := string(dec)
	if !strings.HasPrefix(result, "http://") && !strings.HasPrefix(result, "https://") {
		return "", fmt.Errorf("not a valid URL")
	}
	return result, nil
}

// PublicShortURL builds the full public short URL from a uid.
func PublicShortURL(uid string) string {
	cfg := config.Get()
	base := strings.TrimRight(cfg.Public.BaseURL, "/")
	return base + "/" + uid
}
