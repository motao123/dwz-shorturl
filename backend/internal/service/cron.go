package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
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

	// partition maintenance observability (guarded by mu)
	partitionLastRunAt      time.Time
	partitionCreatedLastRun int
	partitionLastError      string
	partitionLastErrorAt    time.Time
	partitionFailures       int
	partitionAlert          PartitionAlertFunc
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

// Partition maintenance: coverage state, idempotent catch-up and alerting.
//
// click_logs is RANGE partitioned by month, and the catch-all p_future
// partition is split with REORGANIZE as the calendar advances. That task used
// to only write a log line, so a failing ALTER (DDL lock, full disk, revoked
// privilege, MySQL version differences) was invisible: month partitions ran
// out, every new row fell into p_future and the table silently degraded into
// one huge unpartitioned table, breaking partition pruning for both stats
// queries and the retention cleanup. The state below is what makes that
// observable and self-healing.

const (
	// partitionAheadMonths is how many months ahead of "now" must already have
	// a dedicated partition. This is the coverage target: if the newest monthly
	// partition is further back than now + partitionAheadMonths, new rows are
	// at risk of landing in p_future.
	partitionAheadMonths = 2
	// partitionAlertThreshold is the number of consecutive failures before the
	// monitor raises a hard (critical) alert.
	partitionAlertThreshold = 2
	// partitionMaxAheadMonths bounds the manual "catch up to N months" request.
	partitionMaxAheadMonths = 12
	// partitionSchemaMissing answers the "table does not exist" case from
	// information_schema: a single row with a NULL PARTITION_NAME.
	partitionSchemaMissing = "partition_schema_missing"
)

// partitionMismatch carries the "the table exists, but it is not the partitioned
// shape we expect" cases detected during inspection.
const (
	partitionMismatchNotPartitioned = "table_not_partitioned"
	partitionMismatchNoFuture       = "future_partition_missing"
)

// PartitionStatus describes how far click_logs monthly RANGE partitions
// currently reach, plus the last maintenance outcome. It is exposed through the
// monitor API so a silently-failing ALTER TABLE no longer hides behind a log
// line.
type PartitionStatus struct {
	Table string `json:"table"`
	// Available is false when the table is missing or not RANGE partitioned.
	Available bool `json:"available"`
	// Months is the sorted list of monthly partitions (pYYYYMM) that exist.
	Months []string `json:"months"`
	// EarliestMonth / LatestMonth bound the monthly partition range.
	EarliestMonth string `json:"earliest_month"`
	LatestMonth   string `json:"latest_month"`
	// RequiredMonth is the horizon that must exist (now + partitionAheadMonths).
	RequiredMonth string `json:"required_month"`
	// BehindMonths is how many months the coverage lags the horizon; 0 = healthy.
	BehindMonths int `json:"behind_months"`
	// PendingMonths lists the months the next reconcile would create.
	PendingMonths []string `json:"pending_months"`
	// FutureRows counts rows in p_future and FutureRatio is its share of the
	// table (4 decimals). Sustained growth means new rows are no longer landing
	// in a real monthly partition.
	FutureRows  int64   `json:"future_rows"`
	TotalRows   int64   `json:"total_rows"`
	FutureRatio float64 `json:"future_ratio"`
	// HasFuture is true when the p_future catch-all exists; REORGANIZE needs it.
	HasFuture bool `json:"has_future"`
	// Mismatch names a structural problem ("table_not_partitioned",
	// "future_partition_missing"), empty when the shape is as expected.
	Mismatch string `json:"mismatch,omitempty"`
	Healthy  bool   `json:"healthy"`
	// LastError is the most recent maintenance failure (empty when healthy).
	LastError string `json:"last_error,omitempty"`
	// LastErrorAt is when the last failure happened.
	LastErrorAt *time.Time `json:"last_error_at,omitempty"`
	// LastRunAt is the most recent successful/attempted maintenance run.
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
	// CreatedLastRun is how many partitions the last run created.
	CreatedLastRun int `json:"created_last_run"`
	// ConsecutiveFailures drives alerting: >= alertThreshold marks the task as
	// failing so the monitor flags it red instead of silently succeeding.
	ConsecutiveFailures int `json:"consecutive_failures"`
	// Failing is the alert signal (consecutive failures past the threshold or a
	// structural mismatch that blocks maintenance).
	Failing bool `json:"failing"`
	// AlertLevel is "ok", "warning" or "critical" for the monitor badge.
	AlertLevel string `json:"alert_level"`
	// Alerts lists every reason behind AlertLevel, human readable.
	Alerts []string `json:"alerts,omitempty"`
}

// InspectClickLogsPartitions reports the current partition coverage without
// mutating anything. Used by the monitor endpoint and by tests. It never
// returns nil and never panics on a missing database: the read-only monitor
// path has to keep rendering even while the DB is unavailable.
func (s *CronService) InspectClickLogsPartitions() *PartitionStatus {
	return inspectPartitionStatus(s.db, s.logger, s.partitionSnapshot(), time.Now())
}

// partitionSnapshot is the maintenance bookkeeping recorded by notePartitionRun.
type partitionSnapshot struct {
	lastRunAt      time.Time
	createdLastRun int
	lastError      string
	lastErrorAt    time.Time
	failures       int
}

func (s *CronService) partitionSnapshot() partitionSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return partitionSnapshot{
		lastRunAt:      s.partitionLastRunAt,
		createdLastRun: s.partitionCreatedLastRun,
		lastError:      s.partitionLastError,
		lastErrorAt:    s.partitionLastErrorAt,
		failures:       s.partitionFailures,
	}
}

// inspectPartitionStatus is the pure, DB-shape-agnostic core of the inspection
// so it can be unit-tested: every branch is driven by the row set it is handed.
func inspectPartitionStatus(db *gorm.DB, logger *zap.Logger, snap partitionSnapshot, now time.Time) *PartitionStatus {
	st := &PartitionStatus{Table: "click_logs"}
	if logger == nil {
		logger = zap.NewNop()
	}

	// Report the last maintenance outcome first so it survives even when the
	// coverage query below cannot run (DB down / not configured).
	st.LastError = snap.lastError
	st.ConsecutiveFailures = snap.failures
	if !snap.lastErrorAt.IsZero() {
		t := snap.lastErrorAt
		st.LastErrorAt = &t
	}
	if !snap.lastRunAt.IsZero() {
		t := snap.lastRunAt
		st.LastRunAt = &t
	}
	st.CreatedLastRun = snap.createdLastRun

	if db == nil {
		// A nil *gorm.DB would panic on the query below; the read-only monitor
		// path must keep rendering while the DB is unavailable.
		st.LastError = orDefault(st.LastError, "database not configured")
		st.AlertLevel = "critical"
		st.Alerts = []string{"数据库未配置，无法检查 click_logs 分区"}
		st.Failing = true
		return st
	}

	var rows []partitionRow
	err := db.Raw(`SELECT PARTITION_NAME,
	                      PARTITION_METHOD,
	                      PARTITION_DESCRIPTION,
	                      TABLE_ROWS
	                 FROM information_schema.PARTITIONS
	                WHERE TABLE_SCHEMA = DATABASE()
	                  AND TABLE_NAME = 'click_logs'
	                ORDER BY PARTITION_ORDINAL_POSITION`).Scan(&rows).Error
	if err != nil {
		// Coverage is unknown, but the last known failure bookkeeping stays
		// visible so an operator still sees why maintenance broke.
		st.LastError = "partition check failed: " + err.Error()
		st.AlertLevel = "critical"
		st.Alerts = append(st.Alerts, "分区状态查询失败："+err.Error())
		st.Failing = true
		return st
	}
	if len(rows) == 0 {
		// information_schema knows nothing about click_logs: table missing.
		st.LastError = orDefault(st.LastError, "click_logs 表不存在")
		st.AlertLevel = "critical"
		st.Alerts = append(st.Alerts, "click_logs 表不存在，无法进行分区维护")
		st.Failing = true
		return st
	}

	st.Available = true
	for _, r := range rows {
		if r.PartitionMethod == nil || *r.PartitionMethod != "RANGE" {
			st.Available = false
		}
		if r.PartitionName == nil {
			continue
		}
		name := *r.PartitionName
		if isFuturePartition(name, r.PartitionDescription) {
			st.HasFuture = true
		}
		if t, ok := parseMonthPartition(name); ok {
			st.Months = append(st.Months, name)
			if st.EarliestMonth == "" || t.Before(parseMonthValue(st.EarliestMonth)) {
				st.EarliestMonth = name
			}
			if st.LatestMonth == "" || t.After(parseMonthValue(st.LatestMonth)) {
				st.LatestMonth = name
			}
		}
		if r.TableRows != nil {
			st.TotalRows += *r.TableRows
			if isFuturePartition(name, r.PartitionDescription) {
				st.FutureRows += *r.TableRows
			}
		}
	}
	sort.Strings(st.Months)

	required := monthStart(now.AddDate(0, partitionAheadMonths, 0))
	st.RequiredMonth = required.Format("200601")

	var latest time.Time
	if st.LatestMonth != "" {
		latest = parseMonthValue(st.LatestMonth)
	}
	st.PendingMonths = pendingPartitionMonths(latest, now, partitionAheadMonths)
	st.BehindMonths = len(st.PendingMonths)
	if latest.IsZero() {
		st.BehindMonths = monthsBetween(monthStart(now), required) + 1
	}

	if st.TotalRows > 0 {
		st.FutureRatio = float64(st.FutureRows) / float64(st.TotalRows)
	}

	// Structural problems have to be reported separately from "just behind":
	// maintenance cannot run at all without RANGE partitioning and p_future.
	if !st.Available {
		st.Mismatch = partitionMismatchNotPartitioned
	} else if !st.HasFuture {
		st.Mismatch = partitionMismatchNoFuture
	}

	// Alerting: repeated failures, structural mismatch, lagging coverage and
	// p_future overflow each contribute. Any alert is also logged so a
	// log-based rule can fire without polling the API.
	level := "ok"
	failing := false
	addAlert := func(critical bool, msg string) {
		st.Alerts = append(st.Alerts, msg)
		if critical && level != "critical" {
			level = "critical"
		} else if !critical && level == "ok" {
			level = "warning"
		}
	}
	if snap.failures >= partitionAlertThreshold {
		failing = true
		addAlert(true, fmt.Sprintf("分区维护任务连续失败 %d 次，新数据可能已写入 p_future", snap.failures))
	} else if snap.failures > 0 {
		addAlert(false, "分区维护任务最近一次执行失败，将在下个周期重试")
	}
	if st.Mismatch != "" {
		failing = true
		switch st.Mismatch {
		case partitionMismatchNotPartitioned:
			addAlert(true, "click_logs 未按 RANGE 分区，分区裁剪与自动扩展均失效")
		case partitionMismatchNoFuture:
			addAlert(true, "缺少 p_future 兜底分区，无法通过 REORGANIZE 新增月份分区")
		}
	}
	if st.BehindMonths > 0 {
		addAlert(st.Mismatch != "", fmt.Sprintf(
			"分区仅覆盖至 %s，落后目标月 %s 共 %d 个月（待建：%s）",
			orDefault(st.LatestMonth, "无月度分区"), st.RequiredMonth, st.BehindMonths, strings.Join(st.PendingMonths, ", ")))
	}
	if st.FutureRatio >= 0.1 && st.FutureRows > 0 {
		failing = true
		addAlert(true, fmt.Sprintf("p_future 已积累 %d 行（占比 %.2f%%），存在未按月分区的数据",
			st.FutureRows, st.FutureRatio*100))
	}
	if len(st.Alerts) == 0 {
		st.AlertLevel = "ok"
	} else {
		st.AlertLevel = level
	}
	st.Failing = failing
	st.Healthy = st.Available && st.HasFuture && st.BehindMonths == 0 && !st.Failing

	// Failure state is logged on inspection as well as on the write path, so an
	// operator tailing the logs sees the alert even between cron windows.
	for _, a := range st.Alerts {
		logger.Warn("click_logs partition alert", zap.String("table", st.Table), zap.String("level", st.AlertLevel), zap.String("detail", a))
	}
	return st
}

// EnsurePartitionsAhead idempotently creates every missing click_logs month
// partition up to `months` ahead of now. It returns how many partitions were
// created and records the outcome for the monitor. Re-running is a no-op and
// every month is committed by its own ALTER, so a failure half-way still keeps
// the months that were already created.
func (s *CronService) EnsurePartitionsAhead(months int) (int, error) {
	months = normaliseAheadMonths(months)
	created, err := s.ensureClickLogsPartitionsAhead(months)
	s.notePartitionRun(created, err)
	if err != nil {
		s.logger.Error("partition creation failed", zap.Error(err))
		// Emit the alert immediately: waiting for the next cron window would
		// leave the failure invisible for up to 24h.
		if st := s.InspectClickLogsPartitions(); len(st.Alerts) > 0 {
			s.logger.Error("click_logs partition maintenance failed",
				zap.Strings("alerts", st.Alerts),
				zap.Int("consecutive_failures", st.ConsecutiveFailures),
				zap.Int("months_behind", st.BehindMonths),
				zap.Error(err))
		}
		return len(created), err
	}
	if len(created) > 0 {
		s.logger.Info("click_logs partitions created",
			zap.Int("count", len(created)),
			zap.Strings("months", created),
			zap.String("coverage_until", s.partitionCoverage()),
			zap.Int("ahead_months", months))
	}
	return len(created), nil
}

// partitionCoverage is a best-effort "newest monthly partition" label for logs.
func (s *CronService) partitionCoverage() string {
	if s.db == nil {
		return ""
	}
	var latest *string
	if err := s.db.Raw(
		`SELECT MAX(PARTITION_NAME) FROM information_schema.PARTITIONS
		  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'click_logs'
		    AND PARTITION_NAME REGEXP '^p[0-9]{6}$'`).Scan(&latest).Error; err != nil || latest == nil {
		return ""
	}
	return *latest
}

// ensureClickLogsPartitionsAhead performs the DDL. Every month boundary is the
// first day of the FOLLOWING month, matching the schema's
// `PARTITION pYYYYMM VALUES LESS THAN (TO_DAYS('<next month>-01'))` layout.
// Idempotency comes from reading the existing coverage first, so an already
// covered month is never re-created (a duplicate partition name is a hard MySQL
// error).
func (s *CronService) ensureClickLogsPartitionsAhead(ahead int) ([]string, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	st := inspectPartitionStatus(s.db, s.logger, partitionSnapshot{}, time.Now())
	if st.LastError != "" && !st.Available {
		return nil, fmt.Errorf("%s", st.LastError)
	}
	if st.Mismatch != "" {
		switch st.Mismatch {
		case partitionMismatchNotPartitioned:
			return nil, fmt.Errorf("click_logs is not RANGE partitioned; cannot extend monthly partitions")
		case partitionMismatchNoFuture:
			return nil, fmt.Errorf("click_logs has no p_future catch-all partition; REORGANIZE is not possible")
		}
	}

	pending := pendingPartitionMonths(parseMonthValue(st.LatestMonth), time.Now(), ahead)
	var created []string
	for _, name := range pending {
		pname := "p" + name
		month := parseMonthValue(name)
		// Boundary for partition pYYYYMM is the first day of the FOLLOWING month.
		boundary := month.AddDate(0, 1, 0).Format("2006-01-01")
		// Partition names come from pendingPartitionMonths (p + 6 digits) and
		// the boundary is a bound parameter, so this is not injectable even
		// though MySQL forbids placeholders in identifiers.
		if err := s.db.Exec(
			`ALTER TABLE click_logs REORGANIZE PARTITION p_future INTO (
				PARTITION `+pname+` VALUES LESS THAN (TO_DAYS(?)),
				PARTITION p_future VALUES LESS THAN MAXVALUE
			)`, boundary).Error; err != nil {
			return created, &partitionCreateError{month: pname, boundary: boundary, err: err}
		}
		created = append(created, pname)
	}
	return created, nil
}

// partitionCreateError carries the month whose ALTER failed, so partial
// progress can be reported precisely.
type partitionCreateError struct {
	month    string
	boundary string
	err      error
}

func (e *partitionCreateError) Error() string {
	return fmt.Sprintf("create partition %s (VALUES LESS THAN TO_DAYS('%s')): %v", e.month, e.boundary, e.err)
}

func (e *partitionCreateError) Unwrap() error { return e.err }

// ensureClickLogsPartitions keeps the monthly RANGE partitions of click_logs
// ahead of the calendar (the cron entry point) and surfaces the outcome through
// the monitor, not just a log line.
func (s *CronService) ensureClickLogsPartitions() {
	if _, err := s.EnsurePartitionsAhead(partitionAheadMonths); err != nil {
		// EnsurePartitionsAhead already logged the detailed alert.
		return
	}
}

// notePartitionRun records the outcome of a maintenance attempt. Consecutive
// failures are counted so the monitor can escalate instead of treating every
// failure as a fresh, equally-expected event.
func (s *CronService) notePartitionRun(created []string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.partitionLastRunAt = time.Now()
	s.partitionCreatedLastRun = len(created)
	if err != nil {
		s.partitionFailures++
		s.partitionLastError = err.Error()
		s.partitionLastErrorAt = time.Now()
		return
	}
	s.partitionFailures = 0
	s.partitionLastError = ""
	s.partitionLastErrorAt = time.Time{}
}

// --- pure partition helpers (unit-tested without a database) ---

// partitionRow mirrors the information_schema.PARTITIONS columns we read. The
// explicit column tags are required: gorm scans raw-SQL results by name, and the
// service queries these columns directly (no model/schema involved).
type partitionRow struct {
	PartitionName        *string `gorm:"column:PARTITION_NAME"`
	PartitionMethod      *string `gorm:"column:PARTITION_METHOD"`
	PartitionDescription *string `gorm:"column:PARTITION_DESCRIPTION"`
	TableRows            *int64  `gorm:"column:TABLE_ROWS"`
}

// monthStart floors a timestamp to the first day of its month at local midnight.
func monthStart(t time.Time) time.Time {
	t = t.In(time.Local)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.Local)
}

// parseMonth parses a "YYYYMM" (or "pYYYYMM") label into a month start. Invalid
// input yields the zero time.
func parseMonth(v string) time.Time {
	v = strings.TrimPrefix(v, "p")
	t, err := time.ParseInLocation("200601", v, time.Local)
	if err != nil {
		return time.Time{}
	}
	return monthStart(t)
}

// parseMonthValue is parseMonth with a "" → zero-time passthrough.
func parseMonthValue(v string) time.Time {
	return parseMonth(v)
}

// parseMonthPartition accepts only the pYYYYMM partition naming scheme. A
// partition called "p000000" is not a month and is rejected.
func parseMonthPartition(name string) (time.Time, bool) {
	if len(name) != 7 || name[0] != 'p' {
		return time.Time{}, false
	}
	for i := 1; i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			return time.Time{}, false
		}
	}
	if name == "p000000" {
		return time.Time{}, false
	}
	t := parseMonth(name[1:])
	if t.IsZero() {
		return time.Time{}, false
	}
	return t, true
}

// isFuturePartition recognises the catch-all partition by name or by its
// MAXVALUE upper bound (the schema names it p_future).
func isFuturePartition(name string, description *string) bool {
	if strings.EqualFold(name, "p_future") {
		return true
	}
	return description != nil && strings.EqualFold(strings.TrimSpace(*description), "MAXVALUE")
}

// monthsBetween counts whole months from a to b (0 when a is not before b).
func monthsBetween(a, b time.Time) int {
	a, b = monthStart(a), monthStart(b)
	diff := (b.Year()-a.Year())*12 + int(b.Month()) - int(a.Month())
	if diff < 0 {
		return 0
	}
	return diff
}

// pendingPartitionMonths returns the pYYYYMM names that must be created so
// coverage reaches now + ahead, ascending.
//
// When no month partition exists yet (initial setup on a table that was created
// without any, or a table whose monthly partitions were all reorganised away)
// the run starts at the current month and creates a gap-free range up to the
// target. That is deterministic and keeps the requirement "任意时刻手动触发均
// 可补齐缺失月份" satisfied: whatever the starting point, the end state is the
// same and re-running changes nothing.
//
// History is deliberately NOT back-filled: `REORGANIZE PARTITION p_future` can
// only split the rows that are actually stored in p_future, so a month that
// predates the first existing partition could only be created empty, and rows
// already sitting in an older month partition cannot be moved without a data
// rebuild (documented in docs/partition-maintenance.md).
func pendingPartitionMonths(latest time.Time, now time.Time, ahead int) []string {
	if ahead <= 0 {
		ahead = partitionAheadMonths
	}
	cur := monthStart(now)
	target := cur.AddDate(0, ahead, 0)

	next := latest.AddDate(0, 1, 0)
	if latest.IsZero() || next.Before(cur) {
		// No month partition yet, or coverage is in the past: start at the
		// current month so every month up to the target exists.
		next = cur
	}

	var out []string
	for !next.After(target) {
		out = append(out, next.Format("200601"))
		next = next.AddDate(0, 1, 0)
	}
	return out
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
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

// InspectClickLogsPartitionsContext is InspectClickLogsPartitions bound to a
// caller-supplied context, so the monitor endpoint cannot hang on a slow
// information_schema query.
func (s *CronService) InspectClickLogsPartitionsContext(ctx context.Context) *PartitionStatus {
	return inspectPartitionStatus(s.db.WithContext(ctx), s.logger, s.partitionSnapshot(), time.Now())
}

// partitionSnapshotValues exposes the maintenance bookkeeping as plain values
// (monitor cron table): last run, last error, consecutive failures.
func (s *CronService) partitionSnapshotValues() (time.Time, string, int) {
	snap := s.partitionSnapshot()
	return snap.lastRunAt, snap.lastError, snap.failures
}

func orTime(v, fallback time.Time) time.Time {
	if v.IsZero() {
		return fallback
	}
	return v
}

// PartitionReconcileResult describes what a catch-up attempt did (or would do,
// for a dry run). The created list is the interesting part for a partial
// failure: every month in it is already committed.
type PartitionReconcileResult struct {
	// Created lists the months (pYYYYMM) materialised by this run, ascending.
	Created []string `json:"created"`
	// Pending lists the months that still need to be created after this run.
	Pending []string `json:"pending"`
	// DryRun is true when no DDL was executed.
	DryRun bool `json:"dry_run"`
	// FailedMonth is the month whose ALTER failed, empty on success.
	FailedMonth string `json:"failed_month,omitempty"`
	// Error is the failure message, mirrored into the body for partial
	// successes so the UI can explain a non-200 response.
	Error string `json:"error,omitempty"`
	// Status is the post-run coverage snapshot.
	Status *PartitionStatus `json:"status,omitempty"`
	// Alerts / AlertLevel carry the post-run health signal.
	Alerts     []string `json:"alerts,omitempty"`
	AlertLevel string   `json:"alert_level,omitempty"`
}

// PreviewEnsurePartitions reports what EnsurePartitionsWithStatus would create
// without taking any DDL lock. Useful before running it against a large table.
func (s *CronService) PreviewEnsurePartitions(months int) (*PartitionReconcileResult, error) {
	months = normaliseAheadMonths(months)
	if s.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	st := s.InspectClickLogsPartitions()
	if st.LastError != "" && !st.Available {
		return nil, fmt.Errorf("%s", st.LastError)
	}
	pending := pendingPartitionMonths(parseMonthValue(st.LatestMonth), time.Now(), months)
	res := &PartitionReconcileResult{DryRun: true, Status: st, Alerts: st.Alerts, AlertLevel: st.AlertLevel}
	for _, m := range pending {
		res.Created = append(res.Created, "p"+m)
	}
	res.Pending = st.PendingMonths
	return res, nil
}

// EnsurePartitionsWithStatus runs the catch-up and reports the resulting
// coverage. It is the same operation as EnsurePartitionsAhead plus a status
// snapshot, so the manual entry point and the cron task cannot drift apart.
func (s *CronService) EnsurePartitionsWithStatus(months int) (*PartitionReconcileResult, error) {
	months = normaliseAheadMonths(months)
	created, err := s.ensureClickLogsPartitionsAhead(months)
	s.notePartitionRun(created, err)

	res := &PartitionReconcileResult{Created: created}
	if err != nil {
		res.Error = err.Error()
		res.FailedMonth = failedMonthFromError(err)
		s.logger.Error("partition creation failed", zap.Error(err))
	}

	// Always return the post-run snapshot: on failure it shows the progress
	// made before the error, and with it the remaining gap.
	st := s.InspectClickLogsPartitions()
	res.Status = st
	res.Pending = st.PendingMonths
	res.Alerts = st.Alerts
	res.AlertLevel = st.AlertLevel
	if len(res.Alerts) > 0 {
		// One consolidated log line, so the alert path does not depend on the
		// monitor page being open.
		s.firePartitionAlert(res, st)
		s.logger.Warn("click_logs partition maintenance finished with alerts",
			zap.Strings("alerts", res.Alerts),
			zap.Int("created", len(res.Created)),
			zap.String("failed_month", res.FailedMonth),
			zap.Int("months_behind", st.BehindMonths),
			zap.Int64("future_rows", st.FutureRows),
			zap.Float64("future_ratio", st.FutureRatio))
	}
	if err != nil {
		return res, err
	}
	if len(res.Created) > 0 {
		s.logger.Info("click_logs partitions created",
			zap.Int("count", len(res.Created)),
			zap.Strings("months", res.Created),
			zap.String("coverage_until", st.LatestMonth))
	}
	return res, nil
}

func normaliseAheadMonths(months int) int {
	if months <= 0 {
		return partitionAheadMonths
	}
	if months > partitionMaxAheadMonths {
		return partitionMaxAheadMonths
	}
	return months
}

// failedMonthFromError extracts the pYYYYMM partition name from a
// partitionCreateError, empty when the error carries no month information.
func failedMonthFromError(err error) string {
	var pce *partitionCreateError
	if errors.As(err, &pce) {
		return pce.month
	}
	return ""
}

// PartitionAlertFunc is invoked whenever a maintenance run finishes with
// concerns, so the alert can reach an outbound channel instead of only the
// server log.
type PartitionAlertFunc func(res *PartitionReconcileResult, st *PartitionStatus)

// SetPartitionAlertHook registers the alerting callback (nil disables it).
// main.go wires this to the webhook dispatcher, which means a failing ALTER
// notifies subscribers instead of degrading silently.
func (s *CronService) SetPartitionAlertHook(fn PartitionAlertFunc) {
	s.mu.Lock()
	s.partitionAlert = fn
	s.mu.Unlock()
}

// firePartitionAlert reports an unhealthy result to the registered hook. It is
// best-effort: alerting must never fail the maintenance path.
func (s *CronService) firePartitionAlert(res *PartitionReconcileResult, st *PartitionStatus) {
	if st == nil || len(st.Alerts) == 0 {
		return
	}
	s.mu.RLock()
	hook := s.partitionAlert
	s.mu.RUnlock()
	if hook == nil {
		return
	}
	hook(res, st)
}
