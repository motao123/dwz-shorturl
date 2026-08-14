package service

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"dwz-admin/internal/config"
	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"

	"github.com/go-redis/redis/v8"
	"golang.org/x/crypto/bcrypt"
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
	Create(longURL, custom string, expireDays int, domainID *uint64, createdBy *uint64, source, ip, password string) (*model.ShortUrl, error)
	CreatePublicAPI(longURL, custom string, expireDays int, domainID *uint64, ip, password string) (*model.ShortUrl, error)
	BatchCreatePublicAPI(urls []string, domainID *uint64, ip string) ([]model.ShortUrl, []error)
	BatchCreate(urls []string, domainID *uint64, createdBy *uint64, ip string) ([]model.ShortUrl, []error)
	BatchImport(items []ImportItem, domainID *uint64, createdBy *uint64, ip string) ([]model.ShortUrl, []error)
	GetByID(id uint64) (*model.ShortUrl, error)
	// Restore undeletes a soft-deleted link (回收站恢复).
	Restore(id uint64) (*model.ShortUrl, error)
	Update(id uint64, longURL, title string, expireDays *int, status *int8, categoryID *uint64, domainID *uint64, password *string) (*model.ShortUrl, error)
	Delete(id uint64) error
	BatchDelete(ids []uint64) error
	BatchUpdate(ids []uint64, status *int8, expireDays *int) (int64, error)
	List(page, perPage int, filters repository.ShortUrlFilters) ([]model.ShortUrl, int64, error)
	Export(filters repository.ShortUrlFilters) ([]byte, error)
	ResolveByUID(uid string) (*model.ShortUrl, error)
}

type shortUrlService struct {
	repo       repository.ShortUrlRepo
	rdb        *redis.Client
	db         *gorm.DB // raw DB for legacy table fallback queries
	wjoyLog    repository.WjoyLogRepo
	domainRepo repository.DomainRepo
	violation  repository.ViolationRepo
	sf         singleflightGroup
	// legacyTable caches whether the admin DB exposes a wjoy_log table, so cache
	// misses on unknown short codes don't hammer information_schema every time.
	legacyTable legacyTableCheck
}

type legacyTableCheck struct {
	once   sync.Once
	exists bool
}

func (c *legacyTableCheck) tableExists(db *gorm.DB) bool {
	c.once.Do(func() {
		var n int64
		db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'wjoy_log'").Scan(&n)
		c.exists = n > 0
	})
	return c.exists
}

func NewShortUrlService(repo repository.ShortUrlRepo, rdb *redis.Client, db *gorm.DB, wjoyLog repository.WjoyLogRepo, domainRepo repository.DomainRepo, violationRepo repository.ViolationRepo) ShortUrlService {
	return &shortUrlService{repo: repo, rdb: rdb, db: db, wjoyLog: wjoyLog, domainRepo: domainRepo, violation: violationRepo}
}

// checkViolation runs the synchronous violation rules on a destination URL and
// returns an error for clearly-blocked links. Suspicious URLs are queued for
// human review in violation_reviews (best-effort).
func (s *shortUrlService) checkViolation(longURL, source, ip string) error {
	vr := pkg.CheckURLViolation(longURL)
	switch vr.Status {
	case pkg.ViolationBlocked:
		return errors.New("url blocked: " + vr.Reason)
	case pkg.ViolationReview:
		if s.violation != nil {
			_ = s.violation.Create(&model.ViolationReview{
				URL:    longURL,
				Reason: vr.Reason,
				IP:     ip,
				Source: source,
			})
		}
	}
	return nil
}

// validateDomain checks that a domain reference is present and active. It is a
// no-op when domainID is nil. A nil domainRepo treats the reference as valid.
func (s *shortUrlService) validateDomain(domainID *uint64) error {
	if domainID == nil || s.domainRepo == nil {
		return nil
	}
	d, err := s.domainRepo.FindByID(*domainID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDomainNotFound
		}
		return err
	}
	if d.Status != 1 {
		return ErrDomainInvalid
	}
	return nil
}

func (s *shortUrlService) Create(longURL, custom string, expireDays int, domainID *uint64, createdBy *uint64, source, ip, password string) (*model.ShortUrl, error) {
	longURL = strings.TrimSpace(longURL)
	if err := validateURL(longURL); err != nil {
		return nil, err
	}

	// P0-6: reject clearly-blocked URLs and queue suspicious ones for review.
	if err := s.checkViolation(longURL, source, ip); err != nil {
		return nil, err
	}

	custom = strings.TrimSpace(custom)
	if custom != "" && !isValidCustomCode(custom) {
		return nil, ErrCustomCodeFormat
	}

	if err := s.validateDomain(domainID); err != nil {
		return nil, err
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

	// P0-7: a soft-deleted row still occupies the unique url_hash index, so a
	// re-created link would collide forever. Resurrect it instead of failing.
	if resurrected, rErr := s.resurrectDeleted(hash, expireDays, domainID, createdBy, source, ip); rErr == nil && resurrected != nil {
		return resurrected, nil
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return nil, err
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
			UID:          uid,
			LongURL:      longURL,
			URLHash:      hash,
			DomainID:     domainID,
			Clicks:       0,
			Status:       1,
			ExpireAt:     expireAt,
			PasswordHash: passwordHash,
			CreatedBy:    createdBy,
			Source:       source,
			IP:           ip,
		}

		err := s.repo.CreateWithDomainCount(&record)
		if err == nil {
			// Dual-write to the public wjoy_log table so the link is redirectable
			// via the primary PHP do.php path (/{uid}) as well as the Go /r/{uid}.
			if s.wjoyLog != nil {
				_ = s.wjoyLog.Create(record.UID, record.LongURL, record.URLHash, record.ExpireAt, record.PasswordHash)
			}
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

// CreatePublicAPI creates a short URL through the API-key-authenticated public
// endpoint. The base Create dual-writes to wjoy_log, so the link is redirectable
// via both the primary PHP do.php path and the Go /r/{uid} path.
func (s *shortUrlService) CreatePublicAPI(longURL, custom string, expireDays int, domainID *uint64, ip, password string) (*model.ShortUrl, error) {
	return s.Create(longURL, custom, expireDays, domainID, nil, "api", ip, password)
}

// resurrectDeleted revives a soft-deleted row with the same url_hash so the URL
// can be shortened again. The unique index on url_hash keeps a deleted row in
// place, so without this a re-creation would fail with a duplicate-key error
// forever. Returns nil when there is nothing to resurrect.
func (s *shortUrlService) resurrectDeleted(hash string, expireDays int, domainID *uint64, createdBy *uint64, source, ip string) (*model.ShortUrl, error) {
	deleted, err := s.repo.FindByHashIncludingDeleted(hash)
	if err != nil || !deleted.DeletedAt.Valid {
		return nil, nil
	}

	var expireAt *time.Time
	if expireDays > 0 {
		exp := time.Now().Add(time.Duration(expireDays) * 24 * time.Hour)
		expireAt = &exp
	}

	deleted.DeletedAt = gorm.DeletedAt{}
	deleted.ExpireAt = expireAt
	deleted.Status = 1
	deleted.DomainID = domainID
	deleted.CreatedBy = createdBy
	deleted.Source = source
	deleted.IP = ip
	// Restore the derived domain counter that was decremented on delete.
	if err := s.repo.UpdateWithDomainCount(deleted, nil); err != nil {
		return nil, err
	}
	s.invalidateCache(deleted.UID)
	return deleted, nil
}

// BatchCreatePublicAPI batch-creates short URLs through the API-key-authenticated
// public endpoint, dual-writing each to wjoy_log so the links are redirectable.
func (s *shortUrlService) BatchCreatePublicAPI(urls []string, domainID *uint64, ip string) ([]model.ShortUrl, []error) {
	results := make([]model.ShortUrl, 0, len(urls))
	errs := make([]error, len(urls))
	// 批量内按 URL 去重：同一批中重复 URL 直接复用首个结果，避免重复 DNS 校验
	// 与重复 INSERT（全局 url_hash 去重本就会返回同一条短链）。
	seen := make(map[string]*model.ShortUrl, len(urls))
	seenErr := make(map[string]error, len(urls))

	for i, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if rec, ok := seen[u]; ok {
			results = append(results, *rec)
			continue
		}
		if e, ok := seenErr[u]; ok {
			errs[i] = e
			continue
		}
		record, err := s.CreatePublicAPI(u, "", 0, domainID, ip, "")
		if err != nil {
			errs[i] = err
			seenErr[u] = err
		} else {
			results = append(results, *record)
			seen[u] = record
		}
	}
	return results, errs
}

func (s *shortUrlService) BatchCreate(urls []string, domainID *uint64, createdBy *uint64, ip string) ([]model.ShortUrl, []error) {
	results := make([]model.ShortUrl, 0, len(urls))
	errs := make([]error, len(urls))
	// 批量内去重：重复 URL 复用首个结果
	seen := make(map[string]*model.ShortUrl, len(urls))
	seenErr := make(map[string]error, len(urls))

	for i, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if rec, ok := seen[u]; ok {
			results = append(results, *rec)
			continue
		}
		if e, ok := seenErr[u]; ok {
			errs[i] = e
			continue
		}
		record, err := s.Create(u, "", 0, domainID, createdBy, "batch", ip, "")
		if err != nil {
			errs[i] = err
			seenErr[u] = err
		} else {
			results = append(results, *record)
			seen[u] = record
		}
	}

	return results, errs
}

// ImportItem is a single row from a CSV/JSON import.
type ImportItem struct {
	URL        string `json:"url"`
	Title      string `json:"title"`
	Custom     string `json:"custom"`
	ExpireDays int    `json:"expire_days"`
}

// BatchImport creates short URLs from CSV/JSON import rows, preserving each
// row's title. Returns parallel results and per-row errors.
func (s *shortUrlService) BatchImport(items []ImportItem, domainID *uint64, createdBy *uint64, ip string) ([]model.ShortUrl, []error) {
	results := make([]model.ShortUrl, 0, len(items))
	errs := make([]error, len(items))
	// 批量内去重：重复 URL 复用首个结果（与全局 url_hash 去重语义一致）
	seen := make(map[string]*model.ShortUrl, len(items))
	seenErr := make(map[string]error, len(items))

	for i, it := range items {
		it.URL = strings.TrimSpace(it.URL)
		if it.URL == "" {
			errs[i] = errors.New("url is empty")
			continue
		}
		if rec, ok := seen[it.URL]; ok {
			results = append(results, *rec)
			continue
		}
		if e, ok := seenErr[it.URL]; ok {
			errs[i] = e
			continue
		}
		record, err := s.createWithTitle(it.URL, it.Title, it.Custom, it.ExpireDays, domainID, createdBy, "import", ip, "")
		if err != nil {
			errs[i] = err
			seenErr[it.URL] = err
		} else {
			results = append(results, *record)
			seen[it.URL] = record
		}
	}

	return results, errs
}

// createWithTitle mirrors Create but also sets the record title.
func (s *shortUrlService) createWithTitle(longURL, title, custom string, expireDays int, domainID *uint64, createdBy *uint64, source, ip, password string) (*model.ShortUrl, error) {
	longURL = strings.TrimSpace(longURL)
	if err := validateURL(longURL); err != nil {
		return nil, err
	}
	// P0-6: reject clearly-blocked URLs and queue suspicious ones for review.
	if err := s.checkViolation(longURL, source, ip); err != nil {
		return nil, err
	}
	custom = strings.TrimSpace(custom)
	if custom != "" && !isValidCustomCode(custom) {
		return nil, ErrCustomCodeFormat
	}
	hash := md5Hash(longURL)

	if existing, err := s.repo.FindByHash(hash); err == nil && existing != nil {
		if custom != "" && custom != existing.UID {
			return nil, errors.New("url already has a short link, cannot use different custom code")
		}
		return existing, nil
	}

	// P0-7: resurrect a soft-deleted row instead of colliding with its unique
	// url_hash index forever.
	if resurrected, rErr := s.resurrectDeleted(hash, expireDays, domainID, createdBy, source, ip); rErr == nil && resurrected != nil {
		return resurrected, nil
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return nil, err
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
			UID:          uid,
			LongURL:      longURL,
			URLHash:      hash,
			Title:        title,
			DomainID:     domainID,
			Status:       1,
			ExpireAt:     expireAt,
			PasswordHash: passwordHash,
			CreatedBy:    createdBy,
			Source:       source,
			IP:           ip,
		}
		if err := s.repo.CreateWithDomainCount(&record); err == nil {
			// Dual-write to the public wjoy_log table so the link is redirectable
			// via the primary PHP do.php path (/{uid}) as well as the Go /r/{uid}.
			if s.wjoyLog != nil {
				_ = s.wjoyLog.Create(record.UID, record.LongURL, record.URLHash, record.ExpireAt, record.PasswordHash)
			}
			return &record, nil
		} else if !isDuplicateError(err) {
			return nil, err
		}
		if existing, findErr := s.repo.FindByHash(hash); findErr == nil && existing != nil {
			return existing, nil
		}
		if custom != "" {
			return nil, ErrCustomCodeTaken
		}
	}
	return nil, ErrCodeCollision
}

func (s *shortUrlService) GetByID(id uint64) (*model.ShortUrl, error) {
	rec, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	rec.HasPassword = rec.PasswordHash != ""
	return rec, nil
}

// Restore undeletes a soft-deleted link and re-enables its public row so the
// PHP redirect path serves it again.
func (s *shortUrlService) Restore(id uint64) (*model.ShortUrl, error) {
	record, err := s.repo.FindByIDIncludingDeleted(id)
	if err != nil {
		return nil, err
	}
	if !record.DeletedAt.Valid {
		return nil, errors.New("short url is not deleted")
	}
	if err := s.repo.RestoreWithDomainCount(record); err != nil {
		return nil, err
	}
	// Re-enable the public wjoy_log row (was disabled on delete).
	if s.wjoyLog != nil {
		_ = s.wjoyLog.SetStatus(record.UID, 1)
	}
	s.invalidateCache(record.UID)
	record.HasPassword = record.PasswordHash != ""
	return record, nil
}

func (s *shortUrlService) Update(id uint64, longURL, title string, expireDays *int, status *int8, categoryID *uint64, domainID *uint64, password *string) (*model.ShortUrl, error) {
	record, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}

	if err := s.validateDomain(domainID); err != nil {
		return nil, err
	}

	// password: nil = keep current, "" = clear protection, value = set new hash.
	if password != nil {
		ph, err := hashPassword(*password)
		if err != nil {
			return nil, err
		}
		record.PasswordHash = ph
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
			// Re-enable a link that was marked expired/disabled by the cron.
			if status == nil && record.Status == 2 {
				record.Status = 1
			}
		} else {
			record.ExpireAt = nil
		}
		// 续期/改有效期后允许再次触发到期提醒
		record.ReminderSentAt = nil
	}

	if status != nil {
		record.Status = *status
	}

	if categoryID != nil {
		record.CategoryID = categoryID
	}

	// Track domain change for link_count adjustment.
	var oldDomainID *uint64
	if record.DomainID != nil {
		v := *record.DomainID
		oldDomainID = &v
	}
	if domainID != nil && !sameUint64Ptr(oldDomainID, domainID) {
		record.DomainID = domainID
	}

	if err := s.repo.UpdateWithDomainCount(record, oldDomainID); err != nil {
		return nil, err
	}

	// Keep the public wjoy_log row in sync so the primary PHP path serves the
	// updated target/expiry. Only relevant when the URL, expiry or password
	// changed.
	if s.wjoyLog != nil && (longURL != "" || expireDays != nil || password != nil) {
		if err := s.wjoyLog.Update(record.UID, record.LongURL, record.URLHash, record.ExpireAt, password); err != nil {
			log.Printf("sync wjoy_log on update failed: uid=%s err=%v", record.UID, err)
		}
		// Re-enable the public row when the admin renews an expired link.
		if expireDays != nil && *expireDays > 0 && record.Status == 1 {
			_ = s.wjoyLog.SetStatus(record.UID, 1)
		}
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
	if err := s.repo.SoftDeleteWithDomainCount(record); err != nil {
		return err
	}
	// Disable the public wjoy_log row so the primary PHP path also stops serving it.
	if s.wjoyLog != nil {
		_ = s.wjoyLog.SetStatus(record.UID, 0)
	}
	return nil
}

// BatchUpdate applies status and/or expiry to a set of short URLs. Returns the
// number successfully updated; keeps wjoy_log in sync per row.
func (s *shortUrlService) BatchUpdate(ids []uint64, status *int8, expireDays *int) (int64, error) {
	if len(ids) == 0 {
		return 0, errors.New("ids is empty")
	}
	if len(ids) > 200 {
		return 0, errors.New("too many ids; maximum is 200")
	}
	var updated int64
	for _, id := range ids {
		if _, err := s.Update(id, "", "", expireDays, status, nil, nil, nil); err == nil {
			updated++
		}
	}
	return updated, nil
}

func (s *shortUrlService) BatchDelete(ids []uint64) error {
	// Read records first to collect the exact set that will be deleted and to
	// invalidate their redirect caches.
	records := make([]model.ShortUrl, 0, len(ids))
	for _, id := range ids {
		record, err := s.repo.FindByID(id)
		if err != nil {
			return err
		}
		s.invalidateCache(record.UID)
		records = append(records, *record)
	}
	if err := s.repo.BatchDeleteWithDomainCount(records); err != nil {
		return err
	}
	// Disable the public wjoy_log rows so the primary PHP path stops serving them.
	if s.wjoyLog != nil {
		for _, r := range records {
			_ = s.wjoyLog.SetStatus(r.UID, 0)
		}
	}
	return nil
}

func (s *shortUrlService) List(page, perPage int, filters repository.ShortUrlFilters) ([]model.ShortUrl, int64, error) {
	urls, total, err := s.repo.List(page, perPage, filters)
	if err != nil {
		return nil, 0, err
	}
	for i := range urls {
		urls[i].HasPassword = urls[i].PasswordHash != ""
	}
	return urls, total, nil
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
		line := fmt.Sprintf("%s,\"%s\",\"%s\",%d,%d,%s\n",
			u.UID, csvSafeCell(u.LongURL), csvSafeCell(u.Title), u.Clicks, u.Status, u.CreatedAt.Format(time.RFC3339))
		buf.WriteString(line)
	}

	return buf.Bytes(), nil
}

// csvSafeCell neutralises CSV formula injection: cells beginning with =, +, -,
// @ (or tab/CR) are prefixed with a single quote so spreadsheet apps treat them
// as text instead of executing a formula.
func csvSafeCell(v string) string {
	v = strings.ReplaceAll(v, "\"", "\"\"")
	if v == "" {
		return v
	}
	switch v[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + v
	}
	return v
}

func (s *shortUrlService) ResolveByUID(uid string) (*model.ShortUrl, error) {
	// Try Redis cache first (status-aware).
	cacheKey := "shorturl:" + uid
	if cached, err := s.rdb.Get(s.rdb.Context(), cacheKey).Result(); err == nil && cached != "" {
		var rec cachedShortURL
		if json.Unmarshal([]byte(cached), &rec) == nil && rec.UID != "" && rec.Status == 1 && !rec.isExpired() {
			return rec.toShortUrl(), nil
		}
		// Cache is stale (expired/disabled) — fall through to a fresh lookup.
	}

	// Cache miss: singleflight dedupes concurrent lookups for the same uid so a
	// cache stampede on a hot link does not hammer the database.
	record, err := s.sf.Do(uid, func() (*model.ShortUrl, error) {
		return s.resolveAndCache(uid)
	})
	return record, err
}

// resolveAndCache loads a short URL from the database (with legacy fallback),
// validates it, and writes a status-aware cache entry.
func (s *shortUrlService) resolveAndCache(uid string) (*model.ShortUrl, error) {
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

	if err := s.validateRecord(record); err != nil {
		return nil, err
	}

	// P0-6: a blocked destination must never be served, even for links created
	// before detection was wired in. CheckURLViolation is a pure CPU rule scan
	// (no DNS, no HTTP), so running it on cache misses is cheap.
	if vr := pkg.CheckURLViolation(record.LongURL); vr.Status == pkg.ViolationBlocked {
		return nil, errors.New("short url blocked: " + vr.Reason)
	}

	// Cache the status-aware record. TTL is capped at 1h but shortened when the
	// link expires sooner so expired links are not served from cache.
	ttl := time.Hour
	if record.ExpireAt != nil {
		until := time.Until(*record.ExpireAt)
		if until > 0 && until < ttl {
			ttl = until
		}
	}
	rec := cachedShortURL{UID: record.UID, LongURL: record.LongURL, Status: record.Status, ExpireAt: record.ExpireAt, PasswordHash: record.PasswordHash}
	if data, err := json.Marshal(&rec); err == nil {
		s.rdb.Set(s.rdb.Context(), "shorturl:"+uid, data, ttl)
	}
	return record, nil
}

// validateRecord checks expiry/status and returns nil if the record is
// servable, or a typed error (expired/disabled) if not.
func (s *shortUrlService) validateRecord(record *model.ShortUrl) error {
	if record.ExpireAt != nil && record.ExpireAt.Before(time.Now()) {
		return errors.New("short url expired")
	}
	if record.Status != 1 {
		return errors.New("short url disabled")
	}
	return nil
}

// cachedShortURL is the status-aware, JSON-serialisable cache payload.
type cachedShortURL struct {
	UID          string     `json:"uid"`
	LongURL      string     `json:"long_url"`
	Status       int8       `json:"status"`
	ExpireAt     *time.Time `json:"expire_at"`
	PasswordHash string     `json:"password_hash,omitempty"`
}

func (c cachedShortURL) isExpired() bool {
	return c.ExpireAt != nil && c.ExpireAt.Before(time.Now())
}

func (c cachedShortURL) toShortUrl() *model.ShortUrl {
	return &model.ShortUrl{
		UID:          c.UID,
		LongURL:      c.LongURL,
		Status:       c.Status,
		ExpireAt:     c.ExpireAt,
		PasswordHash: c.PasswordHash,
		HasPassword:  c.PasswordHash != "",
	}
}

// singleflightGroup coalesces concurrent lookups for the same key.
type singleflightGroup struct {
	mu sync.Mutex
	m  map[string]*singleflightCall
}

type singleflightCall struct {
	wg  sync.WaitGroup
	val *model.ShortUrl
	err error
}

func (g *singleflightGroup) Do(key string, fn func() (*model.ShortUrl, error)) (*model.ShortUrl, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*singleflightCall)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := &singleflightCall{}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()
	return c.val, c.err
}

// resolveFromLegacy queries the PHP-era wjoy_log table as a fallback.
// Columns: uid, longurl (no underscore), expire_at, clicks.
func (s *shortUrlService) resolveFromLegacy(uid string) *model.ShortUrl {
	if s.db == nil {
		return nil
	}

	type legacyRow struct {
		LongURL      string `gorm:"column:longurl"`
		ExpireAt     *time.Time
		PasswordHash string
	}
	var row legacyRow
	// Check if wjoy_log table exists before querying (avoids noise on fresh
	// installs). The result is cached for the process lifetime.
	if !s.legacyTable.tableExists(s.db) {
		return nil
	}

	err := s.db.Table("wjoy_log").
		Select("longurl, expire_at, password_hash").
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
		UID:          uid,
		LongURL:      row.LongURL,
		Status:       1,
		ExpireAt:     row.ExpireAt,
		PasswordHash: row.PasswordHash,
		Source:       "legacy",
	}
}

func (s *shortUrlService) invalidateCache(uid string) {
	ctx := s.rdb.Context()
	s.rdb.Del(ctx, "shorturl:"+uid)
}

// ValidateURL is the exported form of validateURL for handler-side outbound
// requests (link health checks, title scraping) that need SSRF-safe validation.
func ValidateURL(rawURL string) error { return validateURL(rawURL) }

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

// privateHostCache caches DNS resolution results per hostname (5-minute TTL) so
// a 100-URL batch sharing a few domains does not perform hundreds of blocking
// DNS lookups. Bounded: cleared when it grows past 2048 entries.
var privateHostCache = struct {
	mu    sync.Mutex
	items map[string]privateHostEntry
}{items: make(map[string]privateHostEntry)}

type privateHostEntry struct {
	private bool
	expiry  time.Time
}

const privateHostCacheTTL = 5 * time.Minute

func isPrivateHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".local") {
		return true
	}

	// Literal IP: cheap, no DNS, no caching needed.
	if ip := net.ParseIP(host); ip != nil {
		return isPrivateIP(ip)
	}

	// Cached DNS result.
	privateHostCache.mu.Lock()
	if e, ok := privateHostCache.items[host]; ok && time.Now().Before(e.expiry) {
		privateHostCache.mu.Unlock()
		return e.private
	}
	privateHostCache.mu.Unlock()

	private := isPrivateHostResolve(host)

	privateHostCache.mu.Lock()
	if len(privateHostCache.items) > 2048 {
		privateHostCache.items = make(map[string]privateHostEntry)
	}
	privateHostCache.items[host] = privateHostEntry{private: private, expiry: time.Now().Add(privateHostCacheTTL)}
	privateHostCache.mu.Unlock()
	return private
}

// isPrivateHostResolve performs the actual DNS lookup; kept separate from the
// cache wrapper so the resolution logic stays straightforward.
func isPrivateHostResolve(host string) bool {
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

// hashPassword returns the bcrypt hash of an access password, or "" when the
// password is empty (link stays open). Longer inputs are rejected up-front to
// keep bcrypt from choking on absurd payloads.
func hashPassword(password string) (string, error) {
	password = strings.TrimSpace(password)
	if password == "" {
		return "", nil
	}
	if len(password) > 72 {
		return "", errors.New("password too long (max 72 bytes)")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
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

// sameUint64Ptr reports whether two *uint64 point to equal values. nil is
// treated as a distinct value so that changing nil<->id is detected.
func sameUint64Ptr(a, b *uint64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
