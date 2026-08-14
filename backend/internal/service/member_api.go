package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var ErrMemberLinkNotFound = errors.New("member link not found")
var ErrEmailNotVerified = errors.New("邮箱未验证")

// MemberLinkStat is the per-link click analytics for a member's own short link.
type MemberLinkStat struct {
	UID           string             `json:"uid"`
	Total         int64              `json:"total"`
	Trend         []MemberTrendPoint `json:"trend"`
	Referrers     []MemberReferrer   `json:"referrers"`
	ReferrerTypes []MemberDevice     `json:"referrer_types"`
	Devices       []MemberDevice     `json:"devices"`
	Browsers      []MemberDevice     `json:"browsers"`
	Countries     []MemberDevice     `json:"countries"`
}

type MemberTrendPoint struct {
	Date   string `json:"date"`
	Clicks int64  `json:"clicks"`
}

type MemberReferrer struct {
	Referrer string `json:"referrer"`
	Count    int64  `json:"count"`
}

type MemberDevice struct {
	Device string `json:"device"`
	Count  int64  `json:"count"`
}

type MemberApiService interface {
	ListLinks(memberID uint64, page, perPage int, keyword, status string) ([]model.ShortUrl, int64, error)
	CreateLink(memberID uint64, url, title, custom string, expireDays int, ip, password string) (*model.ShortUrl, error)
	BatchCreateLinks(memberID uint64, urls []string, ip string) ([]MemberBatchResult, error)
	DeleteLink(memberID, linkID uint64) error
	Me(memberID uint64) (*model.Member, error)
	GetLinkStats(memberID uint64, uid string) (*MemberLinkStat, error)
	UpdateLinkExpiry(memberID, linkID uint64, expireDays int) (*model.ShortUrl, error)
	UpdateLink(memberID, linkID uint64, longURL, title string, expireDays *int) (*model.ShortUrl, error)
	Summary(memberID uint64) (*MemberSummary, error)
	ExportLinks(memberID uint64) ([]byte, error)
	RenewExpiring(memberID uint64, expireDays int) (int64, error)
	FetchTitle(url string) (string, error)
	ImportLinks(memberID uint64, content string, ip string) ([]MemberBatchResult, error)
	RequestPasswordReset(email string) error
	ResetPassword(token, password string) error
	SendVerification(email string) error
	VerifyEmail(token string) error
}

// MemberSummary is the member console overview dashboard.
type MemberSummary struct {
	TotalLinks  int64 `json:"total_links"`
	TotalClicks int64 `json:"total_clicks"`
	MonthNew    int64 `json:"month_new"`
}

// MemberBatchResult is one row of a batch create result.
type MemberBatchResult struct {
	URL       string `json:"url"`
	UID       string `json:"uid"`
	ShortURL  string `json:"short_url"`
	Error     string `json:"error,omitempty"`
}

type memberApiService struct {
	shortUrlRepo repository.ShortUrlRepo
	wjoyLog      repository.WjoyLogRepo
	memberRepo   repository.MemberRepo
	db           *gorm.DB
	email        *EmailService
}

func NewMemberApiService(shortUrlRepo repository.ShortUrlRepo, wjoyLog repository.WjoyLogRepo, memberRepo repository.MemberRepo, db *gorm.DB, email *EmailService) MemberApiService {
	return &memberApiService{shortUrlRepo: shortUrlRepo, wjoyLog: wjoyLog, memberRepo: memberRepo, db: db, email: email}
}

func (s *memberApiService) ListLinks(memberID uint64, page, perPage int, keyword, status string) ([]model.ShortUrl, int64, error) {
	filters := repository.ShortUrlFilters{MemberID: &memberID, Keyword: strings.TrimSpace(keyword)}
	now := time.Now()

	switch status {
	case "active":
		one := int8(1)
		filters.Status = &one
	case "expired":
		// status=2 (cron) OR expire_at already passed but not yet marked.
		return s.listByStatus(memberID, page, perPage, keyword, "expired", now)
	case "expiring":
		return s.listByStatus(memberID, page, perPage, keyword, "expiring", now)
	case "disabled":
		zero := int8(0)
		filters.Status = &zero
	}
	return s.shortUrlRepo.List(page, perPage, filters)
}

// listByStatus runs a custom query for statuses that can't be expressed with the
// simple status column filter (expired / expiring-soon).
func (s *memberApiService) listByStatus(memberID uint64, page, perPage int, keyword, status string, now time.Time) ([]model.ShortUrl, int64, error) {
	query := s.db.Model(&model.ShortUrl{}).Where("member_id = ? AND deleted_at IS NULL", memberID)
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		query = query.Where("(uid LIKE ? OR long_url LIKE ? OR title LIKE ?)", like, like, like)
	}
	switch status {
	case "expired":
		query = query.Where("status = 2 OR (expire_at IS NOT NULL AND expire_at < ?)", now)
	case "expiring":
		week := now.AddDate(0, 0, 7)
		query = query.Where("status = 1 AND expire_at IS NOT NULL AND expire_at >= ? AND expire_at <= ?", now, week)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.ShortUrl
	err := query.Order("created_at DESC").Offset((page - 1) * perPage).Limit(perPage).Find(&list).Error
	return list, total, err
}

func (s *memberApiService) CreateLink(memberID uint64, url, title, custom string, expireDays int, ip, password string) (*model.ShortUrl, error) {
	url = trimSpace(url)
	if err := validateURL(url); err != nil {
		return nil, err
	}
	custom = trimSpace(custom)
	if custom != "" && !isValidCustomCode(custom) {
		return nil, ErrCustomCodeFormat
	}
	hash := md5Hash(url)

	if existing, err := s.shortUrlRepo.FindByHash(hash); err == nil && existing != nil {
		if custom != "" && custom != existing.UID {
			return nil, errors.New("url already has a short link, cannot use different custom code")
		}
		return existing, nil
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
			uid = pkg.ShortURL(url)
		} else {
			salt := pkg.GenerateRandomCode(8)
			uid = pkg.ShortURL(url + "|" + salt)
		}
		record := model.ShortUrl{
			UID:          uid,
			LongURL:      url,
			Title:        strings.TrimSpace(title),
			URLHash:      hash,
			MemberID:     &memberID,
			Status:       1,
			ExpireAt:     expireAt,
			PasswordHash: passwordHash,
			Source:       "member",
			IP:           ip,
		}
		if err := s.shortUrlRepo.CreateWithDomainCount(&record); err == nil {
			if s.wjoyLog != nil {
				_ = s.wjoyLog.Create(record.UID, record.LongURL, record.URLHash, record.ExpireAt, record.PasswordHash)
			}
			return &record, nil
		} else if !isDuplicateError(err) {
			return nil, err
		}
		if existing, findErr := s.shortUrlRepo.FindByHash(hash); findErr == nil && existing != nil {
			return existing, nil
		}
		if custom != "" {
			return nil, ErrCustomCodeTaken
		}
	}
	return nil, ErrCodeCollision
}

// ExportLinks returns the member's links as a CSV document (all rows, not
// paginated) for download.
func (s *memberApiService) ExportLinks(memberID uint64) ([]byte, error) {
	filters := repository.ShortUrlFilters{MemberID: &memberID}
	urls, _, err := s.shortUrlRepo.List(1, 10000, filters)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.WriteString("uid,short_url,long_url,title,clicks,expire_at,created_at\n")
	for _, u := range urls {
		expireAt := ""
		if u.ExpireAt != nil {
			expireAt = u.ExpireAt.Format(time.RFC3339)
		}
		line := fmt.Sprintf("%s,\"%s\",\"%s\",\"%s\",%d,%s,%s\n",
			u.UID, PublicShortURL(u.UID), csvSafeCell(u.LongURL), csvSafeCell(u.Title), u.Clicks, expireAt, u.CreatedAt.Format(time.RFC3339))
		buf.WriteString(line)
	}
	return buf.Bytes(), nil
}

// Summary returns aggregate link counts for the member's console.
func (s *memberApiService) Summary(memberID uint64) (*MemberSummary, error) {
	res := &MemberSummary{}
	monthStart := time.Now().AddDate(0, 0, -30).Format("2006-01-02")

	var total int64
	if err := s.db.Model(&model.ShortUrl{}).Where("member_id = ? AND deleted_at IS NULL", memberID).Count(&total).Error; err != nil {
		return nil, err
	}
	res.TotalLinks = total

	var clicks int64
	if err := s.db.Model(&model.ShortUrl{}).
		Where("member_id = ? AND deleted_at IS NULL", memberID).
		Select("COALESCE(SUM(clicks), 0)").Scan(&clicks).Error; err != nil {
		return nil, err
	}
	res.TotalClicks = clicks

	var monthNew int64
	if err := s.db.Model(&model.ShortUrl{}).
		Where("member_id = ? AND deleted_at IS NULL AND created_at >= ?", memberID, monthStart).
		Count(&monthNew).Error; err != nil {
		return nil, err
	}
	res.MonthNew = monthNew

	return res, nil
}

// RenewExpiring renews (extend by expireDays) all of the member's links that
// are already expired or expiring within 7 days. Returns the number renewed and
// keeps short_urls + wjoy_log in sync.
func (s *memberApiService) RenewExpiring(memberID uint64, expireDays int) (int64, error) {
	if expireDays <= 0 {
		return 0, errors.New("expire_days must be positive")
	}
	now := time.Now()
	week := now.AddDate(0, 0, 7)
	expireAt := now.Add(time.Duration(expireDays) * 24 * time.Hour)

	var list []model.ShortUrl
	err := s.db.Model(&model.ShortUrl{}).
		Where("member_id = ? AND deleted_at IS NULL", memberID).
		Where("(status = 2 OR (expire_at IS NOT NULL AND expire_at < ?)) OR (status = 1 AND expire_at IS NOT NULL AND expire_at <= ?)",
			now, week).
		Find(&list).Error
	if err != nil {
		return 0, err
	}

	renewed := int64(0)
	for i := range list {
		rec := &list[i]
		rec.ExpireAt = &expireAt
		rec.Status = 1
		rec.ReminderSentAt = nil // 续期后允许再次触发到期提醒
		if err := s.shortUrlRepo.Update(rec); err != nil {
			continue
		}
		if s.wjoyLog != nil {
			_ = s.wjoyLog.Update(rec.UID, rec.LongURL, rec.URLHash, rec.ExpireAt, nil)
			_ = s.wjoyLog.SetStatus(rec.UID, 1)
		}
		renewed++
	}
	return renewed, nil
}

// UpdateLink edits one of the member's own short links (target URL, title and
// optionally expiry). Ownership is enforced and the public wjoy_log row is kept
// in sync so the primary PHP path serves the updated target.
func (s *memberApiService) UpdateLink(memberID, linkID uint64, longURL, title string, expireDays *int) (*model.ShortUrl, error) {
	record, err := s.shortUrlRepo.FindByID(linkID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMemberLinkNotFound
		}
		return nil, err
	}
	if record.MemberID == nil || *record.MemberID != memberID {
		return nil, ErrMemberLinkNotFound
	}

	longURL = strings.TrimSpace(longURL)
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
			record.Status = 1 // re-enable if it was expired/disabled
		} else {
			record.ExpireAt = nil
		}
		record.ReminderSentAt = nil // 改有效期后允许再次触发到期提醒
	}

	if err := s.shortUrlRepo.Update(record); err != nil {
		return nil, err
	}
	if s.wjoyLog != nil {
		_ = s.wjoyLog.Update(record.UID, record.LongURL, record.URLHash, record.ExpireAt, nil)
		if expireDays != nil && *expireDays > 0 {
			_ = s.wjoyLog.SetStatus(record.UID, 1)
		}
	}
	return record, nil
}

// FetchTitle fetches the page <title> for a member-provided URL. The URL is
// validated (including SSRF private-host rejection) before fetching; best-effort
// with a short timeout so the caller UX stays snappy.
func (s *memberApiService) FetchTitle(rawURL string) (string, error) {
	if err := validateURL(rawURL); err != nil {
		return "", err
	}
	// P1-5: the dialer also refuses private IPs at connect time, closing the
	// DNS-rebinding window between validation and fetch.
	client := pkg.NewSafeHTTPClient(5 * time.Second)
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; dwz-shorturl-title/1.0)")
	req.Header.Set("Accept", "text/html")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	m := re.FindSubmatch(body)
	if len(m) < 2 {
		return "", errors.New("no title found")
	}
	title := strings.TrimSpace(string(m[1]))
	title = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(title, "")
	title = strings.Join(strings.Fields(title), " ")
	if title == "" || len([]rune(title)) > 64 {
		return "", errors.New("title unavailable")
	}
	return title, nil
}

// RequestPasswordReset generates a reset token for a member's email, stores it
// (30-min expiry) and emails a reset link. Always returns nil for unknown emails
// to avoid account enumeration.
func (s *memberApiService) RequestPasswordReset(email string) error {
	m, err := s.memberRepo.FindByEmail(strings.TrimSpace(email))
	if err != nil {
		// 统一行为：邮箱不存在也返回成功，避免枚举
		return nil
	}
	token := pkg.GenerateRandomCode(24)
	expiresAt := time.Now().Add(30 * time.Minute)
	if err := s.memberRepo.SetResetToken(m.ID, token, &expiresAt); err != nil {
		return err
	}
	if s.email == nil || !s.email.Enabled() {
		return errors.New("邮件服务未配置")
	}
	link := "https://1.xk7.cn/member/reset?token=" + token
	body := "您好，\n\n我们收到重置您短链账号密码的请求。请点击以下链接设置新密码（30 分钟内有效）：\n\n" + link + "\n\n如果不是您本人操作，请忽略此邮件。\n—— 陌涛短链"
	return s.email.Send(m.Email, "重置密码 - 陌涛短链", body)
}

// ResetPassword validates a reset token and sets a new password. Also bumps
// token_version to invalidate all previously issued JWTs.
func (s *memberApiService) ResetPassword(token, password string) error {
	if len(password) < 8 || !regexp.MustCompile(`[A-Za-z]`).MatchString(password) || !regexp.MustCompile(`[0-9]`).MatchString(password) {
		return errors.New("密码需 8-64 位且同时包含字母和数字")
	}
	m, err := s.memberRepo.FindByResetToken(token)
	if err != nil {
		return errors.New("重置链接无效或已过期")
	}
	if m.ResetExpiresAt == nil || m.ResetExpiresAt.Before(time.Now()) {
		return errors.New("重置链接已过期，请重新申请")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.memberRepo.ResetPassword(m.ID, string(hash)); err != nil {
		return err
	}
	_ = s.memberRepo.SetResetToken(m.ID, "", nil) // 使用后清除令牌
	// 使旧 JWT 全部失效
	_ = s.memberRepo.BumpTokenVersion(m.ID)
	return nil
}

// SendVerification generates a verification token for a member's email, stores
// it (24h expiry) and emails a verification link. Unknown emails are silent.
func (s *memberApiService) SendVerification(email string) error {
	m, err := s.memberRepo.FindByEmail(strings.TrimSpace(email))
	if err != nil {
		return nil
	}
	if m.EmailVerified == 1 {
		return errors.New("该邮箱已验证")
	}
	token := pkg.GenerateRandomCode(24)
	expiresAt := time.Now().Add(24 * time.Hour)
	if err := s.memberRepo.SetVerifyToken(m.ID, token, &expiresAt); err != nil {
		return err
	}
	if s.email == nil || !s.email.Enabled() {
		return errors.New("邮件服务未配置")
	}
	link := "https://1.xk7.cn/member/verify?token=" + token
	body := "您好，\n\n感谢注册短链账号。请点击以下链接验证您的邮箱（24 小时内有效）：\n\n" + link + "\n\n如果这不是您的操作，请忽略此邮件。\n—— 陌涛短链"
	return s.email.Send(m.Email, "验证邮箱 - 陌涛短链", body)
}

// VerifyEmail marks a member's email as verified using their token.
func (s *memberApiService) VerifyEmail(token string) error {
	m, err := s.memberRepo.FindByVerifyToken(token)
	if err != nil {
		return errors.New("验证链接无效或已过期")
	}
	if m.VerifyExpiresAt == nil || m.VerifyExpiresAt.Before(time.Now()) {
		return errors.New("验证链接已过期，请重新申请")
	}
	return s.memberRepo.MarkVerified(m.ID)
}

// ImportLinks imports links from CSV text. Each line: url,title,custom,expire_days
// (expire_days and the rest optional). Mirrors the admin CSV import columns.
func (s *memberApiService) ImportLinks(memberID uint64, content string, ip string) ([]MemberBatchResult, error) {
	results := make([]MemberBatchResult, 0, 16)
	if len(content) > 210000 {
		return nil, errors.New("import content too large")
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		url := strings.TrimSpace(parts[0])
		if url == "" {
			continue
		}
		title := ""
		if len(parts) > 1 {
			title = strings.Trim(strings.TrimSpace(parts[1]), `"`)
		}
		custom := ""
		if len(parts) > 2 {
			custom = strings.Trim(strings.TrimSpace(parts[2]), `"`)
		}
		expireDays := 0
		if len(parts) > 3 {
			expireDays, _ = strconv.Atoi(strings.TrimSpace(parts[3]))
		}
		row := MemberBatchResult{URL: url}
		record, err := s.CreateLink(memberID, url, title, custom, expireDays, ip, "")
		if err != nil {
			row.Error = err.Error()
		} else {
			row.UID = record.UID
			row.ShortURL = PublicShortURL(record.UID)
		}
		results = append(results, row)
		if len(results) >= 100 {
			break
		}
	}
	if len(results) == 0 {
		return nil, errors.New("no valid rows")
	}
	return results, nil
}

// BatchCreateLinks creates multiple short links for a member, one per line.
// Each result row carries the short URL or the error for that URL.
func (s *memberApiService) BatchCreateLinks(memberID uint64, urls []string, ip string) ([]MemberBatchResult, error) {
	// 批量创建需邮箱已验证（防垃圾账号批量刷链）
	if m, err := s.memberRepo.FindByID(memberID); err == nil && m.EmailVerified != 1 {
		return nil, ErrEmailNotVerified
	}
	results := make([]MemberBatchResult, 0, len(urls))
	for _, raw := range urls {
		u := strings.TrimSpace(raw)
		if u == "" {
			continue
		}
		row := MemberBatchResult{URL: u}
		record, err := s.CreateLink(memberID, u, "", "", 0, ip, "")
		if err != nil {
			row.Error = err.Error()
		} else {
			row.UID = record.UID
			row.ShortURL = PublicShortURL(record.UID)
		}
		results = append(results, row)
	}
	return results, nil
}

func (s *memberApiService) DeleteLink(memberID, linkID uint64) error {
	record, err := s.shortUrlRepo.FindByID(linkID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrMemberLinkNotFound
		}
		return err
	}
	if record.MemberID == nil || *record.MemberID != memberID {
		return ErrMemberLinkNotFound
	}
	if err := s.shortUrlRepo.SoftDeleteWithDomainCount(record); err != nil {
		return err
	}
	// Disable the public wjoy_log row so the primary PHP path stops serving it.
	if s.wjoyLog != nil {
		_ = s.wjoyLog.SetStatus(record.UID, 0)
	}
	return nil
}

// UpdateLinkExpiry changes the expiry of one of the member's own short links.
// expireDays <= 0 clears the expiry (permanent). Ownership is enforced.
func (s *memberApiService) UpdateLinkExpiry(memberID, linkID uint64, expireDays int) (*model.ShortUrl, error) {
	record, err := s.shortUrlRepo.FindByID(linkID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMemberLinkNotFound
		}
		return nil, err
	}
	if record.MemberID == nil || *record.MemberID != memberID {
		return nil, ErrMemberLinkNotFound
	}

	var expireAt *time.Time
	if expireDays > 0 {
		exp := time.Now().Add(time.Duration(expireDays) * 24 * time.Hour)
		expireAt = &exp
		// Re-enable a link that was marked expired/disabled by the cron or a delete.
		record.Status = 1
		record.ReminderSentAt = nil // 续期后允许再次触发到期提醒
	}
	record.ExpireAt = expireAt

	if err := s.shortUrlRepo.Update(record); err != nil {
		return nil, err
	}
	if s.wjoyLog != nil {
		_ = s.wjoyLog.UpdateExpiry(record.UID, expireAt)
		if expireDays > 0 {
			_ = s.wjoyLog.SetStatus(record.UID, 1)
		}
	}
	return record, nil
}

func (s *memberApiService) Me(memberID uint64) (*model.Member, error) {
	m, err := s.memberRepo.FindByID(memberID)
	if err != nil {
		return nil, ErrMemberNotFound
	}
	return m, nil
}

// GetLinkStats returns click analytics for one of the member's own short links.
// Ownership is enforced: the link must belong to memberID.
func (s *memberApiService) GetLinkStats(memberID uint64, uid string) (*MemberLinkStat, error) {
	record, err := s.shortUrlRepo.FindByUID(uid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMemberLinkNotFound
		}
		return nil, err
	}
	if record.MemberID == nil || *record.MemberID != memberID {
		return nil, ErrMemberLinkNotFound
	}

	stat := &MemberLinkStat{UID: uid}
	if s.db == nil {
		return stat, nil
	}

	if err := s.db.Model(&model.ClickLog{}).Where("short_url_id = ?", record.ID).Count(&stat.Total).Error; err != nil {
		return nil, err
	}

	// Referrer type breakdown (search / social / direct / other).
	var refs []string
	if err := s.db.Model(&model.ClickLog{}).
		Where("short_url_id = ?", record.ID).
		Pluck("referer", &refs).Error; err != nil {
		return nil, err
	}
	refTypes := map[string]int64{}
	for _, ref := range refs {
		refTypes[pkg.ClassifyReferer(ref)]++
	}
	for t, c := range refTypes {
		stat.ReferrerTypes = append(stat.ReferrerTypes, MemberDevice{Device: t, Count: c})
	}
	sort.Slice(stat.ReferrerTypes, func(i, j int) bool { return stat.ReferrerTypes[i].Count > stat.ReferrerTypes[j].Count })

	// Trend over the last 7 days.
	since := time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	type trendRow struct {
		Date   string
		Clicks int64
	}
	var trendRows []trendRow
	if err := s.db.Model(&model.ClickLog{}).
		Select("DATE(created_at) as date, COUNT(*) as clicks").
		Where("short_url_id = ? AND created_at >= ?", record.ID, since).
		Group("date").
		Order("date ASC").
		Scan(&trendRows).Error; err != nil {
		return nil, err
	}
	for _, r := range trendRows {
		stat.Trend = append(stat.Trend, MemberTrendPoint{Date: r.Date, Clicks: r.Clicks})
	}

	// Top referrers.
	type refRow struct {
		Referrer string
		Count    int64
	}
	var refRows []refRow
	if err := s.db.Model(&model.ClickLog{}).
		Select("referer, COUNT(*) as count").
		Where("short_url_id = ?", record.ID).
		Group("referer").
		Order("count DESC").
		Limit(10).
		Scan(&refRows).Error; err != nil {
		return nil, err
	}
	for _, r := range refRows {
		stat.Referrers = append(stat.Referrers, MemberReferrer{Referrer: r.Referrer, Count: r.Count})
	}

	// Device breakdown from user_agent (TEXT column, Pluck into []string).
	var uas []string
	if err := s.db.Model(&model.ClickLog{}).
		Where("short_url_id = ?", record.ID).
		Pluck("user_agent", &uas).Error; err != nil {
		return nil, err
	}
	devices := map[string]int64{}
	browsers := map[string]int64{}
	for _, ua := range uas {
		devices[classifyDevice(ua)]++
		browsers[classifyBrowser(ua)]++
	}
	for device, count := range devices {
		stat.Devices = append(stat.Devices, MemberDevice{Device: device, Count: count})
	}
	sort.Slice(stat.Devices, func(i, j int) bool { return stat.Devices[i].Count > stat.Devices[j].Count })
	for browser, count := range browsers {
		stat.Browsers = append(stat.Browsers, MemberDevice{Device: browser, Count: count})
	}
	sort.Slice(stat.Browsers, func(i, j int) bool { return stat.Browsers[i].Count > stat.Browsers[j].Count })

	// Country distribution (ISO alpha-2, GeoIP resolution at click time).
	type countryRow struct {
		Country string
		Count   int64
	}
	var countryRows []countryRow
	if err := s.db.Model(&model.ClickLog{}).
		Select("country, COUNT(*) as count").
		Where("short_url_id = ? AND country <> ''", record.ID).
		Group("country").
		Order("count DESC").
		Limit(12).
		Scan(&countryRows).Error; err != nil {
		return nil, err
	}
	for _, r := range countryRows {
		stat.Countries = append(stat.Countries, MemberDevice{Device: r.Country, Count: r.Count})
	}

	return stat, nil
}

// classifyDevice buckets a user-agent string into a coarse device category.
func classifyDevice(ua string) string {
	ua = strings.ToLower(ua)
	switch {
	case strings.Contains(ua, "ipad") || strings.Contains(ua, "tablet"):
		return "平板"
	case strings.Contains(ua, "mobile") || strings.Contains(ua, "android") || strings.Contains(ua, "iphone"):
		return "手机"
	default:
		return "桌面"
	}
}

// classifyBrowser buckets a user-agent string into a browser brand. WeChat is
// common for Chinese users, so it gets its own bucket.
func classifyBrowser(ua string) string {
	u := strings.ToLower(ua)
	switch {
	case strings.Contains(u, "micromessenger") || strings.Contains(u, "wechat"):
		return "微信"
	case strings.Contains(u, "edg/"):
		return "Edge"
	case strings.Contains(u, "firefox") && !strings.Contains(u, "seamonkey"):
		return "Firefox"
	case strings.Contains(u, "opr/") || strings.Contains(u, "opera"):
		return "Opera"
	case strings.Contains(u, "chrome") && !strings.Contains(u, "edg/"):
		return "Chrome"
	case strings.Contains(u, "safari") && !strings.Contains(u, "chrome"):
		return "Safari"
	case strings.Contains(u, "ucbrowser"):
		return "UC"
	case strings.Contains(u, "qqbrowser"):
		return "QQ 浏览器"
	default:
		return "其他"
	}
}

func trimSpace(s string) string {
	return strings.TrimSpace(s)
}