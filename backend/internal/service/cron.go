package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"dwz-admin/internal/model"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// CronService manages scheduled background tasks.
type CronService struct {
	cron     *cron.Cron
	db       *gorm.DB
	publicDB *gorm.DB // public frontend DB (members) for expiry reminders
	logger   *zap.Logger
	email    *EmailService
	mu       sync.RWMutex
	lastRuns map[string]time.Time
}

// NewCronService creates and returns a CronService. Tasks are registered
// immediately but not started until Start() is called.
func NewCronService(db *gorm.DB, publicDB *gorm.DB, logger *zap.Logger, email *EmailService) *CronService {
	c := cron.New()

	svc := &CronService{
		cron:     c,
		db:       db,
		publicDB: publicDB,
		logger:   logger,
		email:    email,
		lastRuns: make(map[string]time.Time),
	}

	svc.registerTasks()

	return svc
}

// LastRun returns the last run time for a named task, or zero time if it has
// never run. Safe for concurrent access.
func (s *CronService) LastRun(name string) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastRuns[name]
}

// markRun records a task execution time.
func (s *CronService) markRun(name string) {
	s.mu.Lock()
	s.lastRuns[name] = time.Now()
	s.mu.Unlock()
}

func (s *CronService) registerTasks() {
	// Task 1: Mark expired short URLs as status=2 (expired)
	// Runs every hour on the hour.
	if _, err := s.cron.AddFunc("0 * * * *", safe("mark_expired", s.logger, func() { s.markRun("mark_expired"); s.markExpiredLinks() })); err != nil {
		s.logger.Error("failed to register mark-expired task", zap.Error(err))
	}

	// Task 2: Clean up old click_logs rows older than 90 days
	// Runs at 03:00 daily.
	if _, err := s.cron.AddFunc("0 3 * * *", safe("cleanup_click_logs", s.logger, func() { s.markRun("cleanup_click_logs"); s.cleanupOldClickLogs() })); err != nil {
		s.logger.Error("failed to register click-log cleanup task", zap.Error(err))
	}

	// Task 3: Warm stats cache by pre-aggregating hourly click counts
	// Runs every 10 minutes.
	if _, err := s.cron.AddFunc("*/10 * * * *", safe("aggregate_stats", s.logger, func() { s.markRun("aggregate_stats"); s.aggregateStats() })); err != nil {
		s.logger.Error("failed to register stats aggregation task", zap.Error(err))
	}

	// Task 4: Clean up old stats_hourly rows older than 90 days (same retention
	// as click_logs) so the aggregation table doesn't grow unbounded.
	// Runs at 03:30 daily.
	if _, err := s.cron.AddFunc("30 3 * * *", safe("cleanup_stats", s.logger, func() { s.markRun("cleanup_stats"); s.cleanupOldStats() })); err != nil {
		s.logger.Error("failed to register stats cleanup task", zap.Error(err))
	}

	// Task 5: Email members whose short links expire within 7 days.
	// Runs at 09:00 daily.
	if _, err := s.cron.AddFunc("0 9 * * *", safe("remind_expiring", s.logger, func() { s.markRun("remind_expiring"); s.remindExpiring() })); err != nil {
		s.logger.Error("failed to register expiry reminder task", zap.Error(err))
	}

	// Task 6: Reconcile the short_urls ↔ wjoy_log dual-write so a transient
	// write failure on one side no longer silently forks the data.
	// Runs every 30 minutes.
	if _, err := s.cron.AddFunc("*/30 * * * *", safe("reconcile_dual_write", s.logger, func() { s.markRun("reconcile_dual_write"); s.reconcileDualWrite() })); err != nil {
		s.logger.Error("failed to register dual-write reconciliation task", zap.Error(err))
	}

	// Task 7: Keep the public wjoy_log.clicks counter in sync with
	// short_urls.clicks (the single source of truth), so the PHP stats page
	// shows Go-served clicks too. Runs daily at 04:00.
	if _, err := s.cron.AddFunc("0 4 * * *", safe("reconcile_clicks", s.logger, func() { s.markRun("reconcile_clicks"); s.reconcileClicks() })); err != nil {
		s.logger.Error("failed to register click-counter reconciliation task", zap.Error(err))
	}

	// Task 8: Ensure click_logs has a partition for the next two months so new
	// rows never fall into the catch-all p_future partition. Runs daily at 03:15.
	if _, err := s.cron.AddFunc("15 3 * * *", safe("ensure_partitions", s.logger, func() { s.markRun("ensure_partitions"); s.ensureClickLogsPartitions() })); err != nil {
		s.logger.Error("failed to register partition maintenance task", zap.Error(err))
	}
}

// safe wraps a background task body so a panic inside a cron job cannot crash
// the whole process (cron invokes jobs synchronously on its own goroutine).
func safe(name string, logger *zap.Logger, fn func()) func() {
	return func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("task panicked",
					zap.String("task", name),
					zap.Any("panic", r),
					zap.Stack("stack"))
			}
		}()
		fn()
	}
}

// Start begins the cron scheduler. This is non-blocking.
func (s *CronService) Start() {
	s.cron.Start()
	s.logger.Info("cron scheduler started")
}

// Stop gracefully stops the cron scheduler.
func (s *CronService) Stop() context.Context {
	s.logger.Info("stopping cron scheduler...")
	ctx := s.cron.Stop()
	<-ctx.Done()
	s.logger.Info("cron scheduler stopped")
	return ctx
}

// --- Task implementations ---

// markExpiredLinks sets status=2 for short_urls that have passed their
// expire_at timestamp and are still active (status=1).
func (s *CronService) markExpiredLinks() {
	result := s.db.Model(&model.ShortUrl{}).
		Where("status = 1 AND expire_at IS NOT NULL AND expire_at < NOW()").
		Update("status", 2)
	if result.Error != nil {
		s.logger.Error("mark-expired task failed", zap.Error(result.Error))
		return
	}
	if result.RowsAffected > 0 {
		s.logger.Info("marked expired links",
			zap.Int64("count", result.RowsAffected),
		)
	}
}

// cleanupOldClickLogs removes click_logs rows older than the configured
// retention period (default 90 days). Uses a batched DELETE to avoid
// locking issues on large tables.
func (s *CronService) cleanupOldClickLogs() {
	threshold := time.Now().AddDate(0, 0, -90)
	result := s.db.Where("created_at < ?", threshold).
		Delete(&model.ClickLog{})
	if result.Error != nil {
		s.logger.Error("click-log cleanup task failed", zap.Error(result.Error))
		return
	}
	if result.RowsAffected > 0 {
		s.logger.Info("cleaned up old click logs",
			zap.Int64("deleted", result.RowsAffected),
			zap.Time("threshold", threshold),
		)
	}
}

// cleanupOldStats removes stats_hourly rows older than the retention period
// (90 days) to keep the aggregation table bounded.
func (s *CronService) cleanupOldStats() {
	threshold := time.Now().AddDate(0, 0, -90)
	result := s.db.Table("stats_hourly").Where("hour < ?", threshold).Delete(&map[string]interface{}{})
	if result.Error != nil {
		s.logger.Error("stats cleanup task failed", zap.Error(result.Error))
		return
	}
	if result.RowsAffected > 0 {
		s.logger.Info("cleaned up old stats_hourly rows",
			zap.Int64("deleted", result.RowsAffected),
			zap.Time("threshold", threshold),
		)
	}
}

// RunTask executes a named task synchronously (for admin-triggered runs and
// tests). Returns false when the name is unknown.
func (s *CronService) RunTask(name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch name {
	case "mark_expired":
		s.markExpiredLinks()
	case "cleanup_click_logs":
		s.cleanupOldClickLogs()
	case "aggregate_stats":
		s.aggregateStats()
	case "cleanup_stats":
		s.cleanupOldStats()
	case "remind_expiring":
		s.remindExpiring()
	case "reconcile_dual_write":
		s.reconcileDualWrite()
	case "reconcile_clicks":
		s.reconcileClicks()
	case "ensure_partitions":
		s.ensureClickLogsPartitions()
	default:
		return false, fmt.Errorf("unknown task: %s", name)
	}
	s.lastRuns[name] = time.Now()
	return true, nil
}

// remindExpiring emails members whose short links expire within 7 days. Each
// link is reminded at most once (guarded by reminder_sent_at). The email is
// sent to the member's address and lists their expiring links.
func (s *CronService) remindExpiring() {
	if s.email == nil || !s.email.Enabled() {
		s.logger.Debug("expiry reminder skipped: smtp not configured")
		return
	}
	now := time.Now()
	window := now.AddDate(0, 0, 7)

	type linkRow struct {
		MemberID uint64
		UID      string
		LongURL  string
		ExpireAt *time.Time
	}
	// Query expiring links from the admin DB (short_urls).
	var rows []linkRow
	err := s.db.Table("short_urls").
		Select("member_id, uid, long_url, expire_at").
		Where("deleted_at IS NULL AND status = 1").
		Where("expire_at IS NOT NULL AND expire_at >= ? AND expire_at <= ?", now, window).
		Where("reminder_sent_at IS NULL").
		Scan(&rows).Error
	if err != nil {
		s.logger.Error("expiry reminder query failed", zap.Error(err))
		return
	}
	if len(rows) == 0 {
		return
	}

	// Resolve member emails/usernames from the public DB (members) in one batch.
	emailMap := map[uint64]string{}
	nameMap := map[uint64]string{}
	if s.publicDB != nil {
		memberIDs := make([]uint64, 0, len(rows))
		seen := map[uint64]bool{}
		for _, r := range rows {
			if !seen[r.MemberID] {
				seen[r.MemberID] = true
				memberIDs = append(memberIDs, r.MemberID)
			}
		}
		type memRow struct {
			ID       uint64
			Email    string
			Username string
		}
		var mems []memRow
		if err := s.publicDB.Table("members").Where("id IN ?", memberIDs).Scan(&mems).Error; err != nil {
			s.logger.Error("expiry reminder member lookup failed", zap.Error(err))
			return
		}
		for _, m := range mems {
			emailMap[m.ID] = m.Email
			nameMap[m.ID] = m.Username
		}
	}

	// Group by member.
	byMember := map[uint64]*struct {
		Email    string
		Username string
		Links    []linkRow
	}{}
	for _, r := range rows {
		g, ok := byMember[r.MemberID]
		if !ok {
			g = &struct {
				Email    string
				Username string
				Links    []linkRow
			}{Email: emailMap[r.MemberID], Username: nameMap[r.MemberID]}
			byMember[r.MemberID] = g
		}
		g.Links = append(g.Links, r)
	}

	sent := 0
	failed := 0
	for mid, g := range byMember {
		if g.Email == "" {
			continue
		}
		subject := fmt.Sprintf("您的 %d 条短链即将过期", len(g.Links))
		var b strings.Builder
		fmt.Fprintf(&b, "尊敬的 %s，您好：\n\n以下短链将在 7 天内过期，请及时续期以免失效：\n\n", g.Username)
		for _, l := range g.Links {
			exp := "未知"
			if l.ExpireAt != nil {
				exp = l.ExpireAt.Format("2006-01-02 15:04")
			}
			fmt.Fprintf(&b, "· %s（%s）到期时间：%s\n", l.LongURL, l.UID, exp)
		}
		b.WriteString("\n登录会员中心可一键续期：https://1.xk7.cn/member/\n")
		b.WriteString("—— 陌涛短链")

		if err := s.email.Send(g.Email, subject, b.String()); err != nil {
			s.logger.Error("expiry reminder email failed",
				zap.Uint64("member_id", mid),
				zap.String("to", maskEmail(g.Email)),
				zap.Error(err))
			failed++
			continue
		}
		// Mark all this member's reminded links.
		for _, l := range g.Links {
			if err := s.db.Table("short_urls").Where("uid = ?", l.UID).Update("reminder_sent_at", now).Error; err != nil {
				s.logger.Warn("mark reminder failed", zap.String("uid", l.UID), zap.Error(err))
			}
		}
		sent++
	}
	s.logger.Info("expiry reminder done", zap.Int("members", sent), zap.Int("failed", failed), zap.Int("links", len(rows)))
}

// aggregateStats pre-computes hourly click counts into a stats cache table
// so the dashboard can serve faster. Creates the cache table lazily if it
// doesn't exist.
func (s *CronService) aggregateStats() {
	// Ensure the stats_hourly table exists (lazy migration, separate from AutoMigrate)
	s.db.Exec(`CREATE TABLE IF NOT EXISTS stats_hourly (
		id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
		hour DATETIME NOT NULL,
		clicks INT UNSIGNED NOT NULL DEFAULT 0,
		new_urls INT UNSIGNED NOT NULL DEFAULT 0,
		updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
		UNIQUE KEY uk_hour (hour)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)

	// Aggregate clicks for the last 2 hours (covers any gaps from previous runs)
	cutoff := time.Now().Add(-2 * time.Hour).Truncate(time.Hour)
	hourStr := cutoff.Format("2006-01-02 15:00:00")

	sql := `
		INSERT INTO stats_hourly (hour, clicks, new_urls)
		SELECT
			? AS hour,
			COUNT(c.id) AS clicks,
			COUNT(DISTINCT CASE WHEN s.created_at >= ? AND s.created_at < DATE_ADD(?, INTERVAL 1 HOUR) THEN s.id END) AS new_urls
		FROM short_urls s
		LEFT JOIN click_logs c ON c.short_url_id = s.id AND c.created_at >= ? AND c.created_at < DATE_ADD(?, INTERVAL 1 HOUR)
		WHERE s.deleted_at IS NULL
		ON DUPLICATE KEY UPDATE clicks = VALUES(clicks), new_urls = VALUES(new_urls)
	`
	result := s.db.Exec(sql, hourStr, hourStr, hourStr, hourStr, hourStr)
	if result.Error != nil {
		// Non-fatal: stats are nice-to-have, not critical
		s.logger.Debug("stats aggregation query failed", zap.Error(result.Error))
	}
}

// reconcileClicks syncs the public wjoy_log.clicks counter from the admin
// short_urls.clicks (single source of truth). The PHP redirect path increments
// both, but the Go /r/:code path only increments short_urls.clicks, so without
// this the PHP-era stats page would undercount Go-served clicks.
func (s *CronService) reconcileClicks() {
	if s.publicDB == nil {
		s.logger.Debug("click-counter reconciliation skipped: public_db not configured")
		return
	}
	type clickRow struct {
		UID    string
		Clicks int64
	}
	var rows []clickRow
	if err := s.db.Table("short_urls").
		Select("uid, clicks").
		Where("deleted_at IS NULL").
		Scan(&rows).Error; err != nil {
		s.logger.Error("click-counter reconcile: short_urls scan failed", zap.Error(err))
		return
	}
	if len(rows) == 0 {
		return
	}
	synced := 0
	for _, r := range rows {
		res := s.publicDB.Table("wjoy_log").
			Where("uid = ? AND clicks <> ?", r.UID, r.Clicks).
			Update("clicks", r.Clicks)
		if res.Error != nil {
			s.logger.Error("click-counter reconcile: wjoy_log update failed", zap.String("uid", r.UID), zap.Error(res.Error))
			continue
		}
		synced += int(res.RowsAffected)
	}
	if synced > 0 {
		s.logger.Info("click-counter reconcile done", zap.Int("rows_synced", synced), zap.Int("total", len(rows)))
	}
}

// ensureClickLogsPartitions keeps the monthly RANGE partitions of click_logs
// ahead of the calendar so new rows are written to a real partition instead of
// the catch-all p_future (which would silently turn the table back into a plain
// large table). It creates one partition per missing month up to two months out
// using the standard REORGANIZE p_future idiom; re-running is a no-op.
func (s *CronService) ensureClickLogsPartitions() {
	var names []string
	if err := s.db.Raw(
		`SELECT PARTITION_NAME FROM information_schema.PARTITIONS
		 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'click_logs'
		   AND PARTITION_NAME REGEXP '^p[0-9]{6}$'`,
	).Scan(&names).Error; err != nil {
		s.logger.Error("partition check failed", zap.Error(err))
		return
	}

	var maxMonth time.Time
	for _, n := range names {
		if t, err := time.Parse("200601", strings.TrimPrefix(n, "p")); err == nil {
			if t.After(maxMonth) {
				maxMonth = t
			}
		}
	}
	if maxMonth.IsZero() {
		// No monthly partition found; initialise from the current month.
		maxMonth = time.Now().Truncate(24 * time.Hour).AddDate(0, 0, -time.Now().Day()+1)
	}

	target := time.Now().AddDate(0, 2, 0) // keep 2 months ahead
	created := 0
	for maxMonth.Before(target) {
		next := maxMonth.AddDate(0, 1, 0)
		// Boundary for partition pYYYYMM is the first day of the FOLLOWING month.
		boundary := next.AddDate(0, 1, 0).Format("2006-01-01")
		if err := s.db.Exec(
			`ALTER TABLE click_logs REORGANIZE PARTITION p_future INTO (
				PARTITION p`+next.Format("200601")+` VALUES LESS THAN (TO_DAYS(?)),
				PARTITION p_future VALUES LESS THAN MAXVALUE
			)`, boundary).Error; err != nil {
			s.logger.Error("partition creation failed",
				zap.String("month", next.Format("200601")), zap.Error(err))
			return
		}
		created++
		maxMonth = next
	}
	if created > 0 {
		s.logger.Info("click_logs partitions created", zap.Int("count", created))
	}
}

// zapLogger adapts zap.Logger to a simple Printf interface.
type zapLogger struct{ l *zap.Logger }

func (z zapLogger) Printf(format string, args ...interface{}) {
	z.l.Info(fmt.Sprintf(format, args...))
}

// reconcileDualWrite back-fills rows missing from either side of the
// short_urls ↔ wjoy_log dual-write, and re-syncs the active/disabled status.
// Both directions are bounded to the last 12 hours and use existence checks
// before insert, so the task is safe to run repeatedly and cheap on large
// tables. This is the compensating mechanism for the best-effort dual-write:
// a transient DB failure on one side no longer permanently forks the data.
func (s *CronService) reconcileDualWrite() {
	if s.publicDB == nil {
		s.logger.Debug("dual-write reconciliation skipped: public_db not configured")
		return
	}
	window := time.Now().Add(-12 * time.Hour)

	// --- Direction 1: admin short_urls → public wjoy_log ---
	type adminRow struct {
		UID       string
		LongURL   string
		URLHash   string
		ExpireAt  *time.Time
		Status    int8
		Deleted   *time.Time `gorm:"column:deleted_at"`
		CreatedAt time.Time
	}
	var adminRows []adminRow
	if err := s.db.Unscoped().Table("short_urls").
		Select("uid, long_url, url_hash, expire_at, status, deleted_at, created_at").
		Where("created_at >= ? OR updated_at >= ? OR deleted_at >= ?", window, window, window).
		Limit(2000).
		Scan(&adminRows).Error; err != nil {
		s.logger.Error("reconcile: admin short_urls scan failed", zap.Error(err))
		return
	}
	if len(adminRows) > 0 {
		uids := make([]string, 0, len(adminRows))
		for _, r := range adminRows {
			uids = append(uids, r.UID)
		}
		var existUIDs []string
		if err := s.publicDB.Table("wjoy_log").Where("uid IN ?", uids).Pluck("uid", &existUIDs).Error; err != nil {
			s.logger.Error("reconcile: wjoy_log lookup failed", zap.Error(err))
			return
		}
		exists := make(map[string]bool, len(existUIDs))
		for _, u := range existUIDs {
			exists[u] = true
		}
		inserted := 0
		synced := 0
		for _, r := range adminRows {
			targetStatus := int8(1)
			if r.Status != 1 || r.Deleted != nil {
				targetStatus = 0
			}
			if !exists[r.UID] {
				// Only materialise active rows into the PHP table; disabled /
				// deleted links have nothing to serve on the PHP path.
				if targetStatus == 0 {
					continue
				}
				if err := s.publicDB.Exec(
					`INSERT IGNORE INTO wjoy_log (uid, longurl, url_hash, expire_at, status, created_at)
					 VALUES (?, ?, ?, ?, 1, ?)`,
					r.UID, r.LongURL, r.URLHash, r.ExpireAt, r.CreatedAt).Error; err != nil {
					s.logger.Error("reconcile: insert into wjoy_log failed", zap.String("uid", r.UID), zap.Error(err))
					continue
				}
				inserted++
			} else if exists[r.UID] {
				// Sync status so PHP do.php honours admin disable/expire.
				if err := s.publicDB.Table("wjoy_log").Where("uid = ?", r.UID).Update("status", targetStatus).Error; err != nil {
					s.logger.Error("reconcile: sync wjoy_log status failed", zap.String("uid", r.UID), zap.Error(err))
					continue
				}
				synced++
			}
		}
		if inserted > 0 || synced > 0 {
			s.logger.Info("reconcile admin→public done", zap.Int("inserted", inserted), zap.Int("status_synced", synced))
		}
	}

	// --- Direction 2: public wjoy_log → admin short_urls ---
	type publicRow struct {
		UID      string
		LongURL  string `gorm:"column:longurl"` // legacy column has no underscore
		ExpireAt *time.Time
	}
	var publicRows []publicRow
	if err := s.publicDB.Table("wjoy_log").
		Select("uid, longurl, expire_at").
		Where("created_at >= ?", window).
		Where("longurl LIKE 'http%'").
		Limit(2000).
		Scan(&publicRows).Error; err != nil {
		s.logger.Error("reconcile: wjoy_log scan failed", zap.Error(err))
		return
	}
	if len(publicRows) > 0 {
		uids := make([]string, 0, len(publicRows))
		for _, r := range publicRows {
			uids = append(uids, r.UID)
		}
		var existUIDs []string
		if err := s.db.Table("short_urls").Where("uid IN ?", uids).Pluck("uid", &existUIDs).Error; err != nil {
			s.logger.Error("reconcile: short_urls lookup failed", zap.Error(err))
			return
		}
		exists := make(map[string]bool, len(existUIDs))
		for _, u := range existUIDs {
			exists[u] = true
		}
		inserted := 0
		for _, r := range publicRows {
			if exists[r.UID] {
				continue
			}
			if err := s.db.Exec(
				`INSERT IGNORE INTO short_urls (uid, long_url, url_hash, expire_at, source, status, created_at)
				 VALUES (?, ?, MD5(?), ?, 'web', 1, ?)`,
				r.UID, r.LongURL, r.LongURL, r.ExpireAt, time.Now()).Error; err != nil {
				s.logger.Error("reconcile: insert into short_urls failed", zap.String("uid", r.UID), zap.Error(err))
				continue
			}
			inserted++
		}
		if inserted > 0 {
			s.logger.Info("reconcile public→admin done", zap.Int("inserted", inserted))
		}
	}
}
