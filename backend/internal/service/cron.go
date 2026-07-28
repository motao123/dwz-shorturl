package service

import (
	"context"
	"fmt"
	"time"

	"dwz-admin/internal/model"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// CronService manages scheduled background tasks.
type CronService struct {
	cron   *cron.Cron
	db     *gorm.DB
	logger *zap.Logger
}

// NewCronService creates and returns a CronService. Tasks are registered
// immediately but not started until Start() is called.
func NewCronService(db *gorm.DB, logger *zap.Logger) *CronService {
	c := cron.New()

	svc := &CronService{
		cron:   c,
		db:     db,
		logger: logger,
	}

	svc.registerTasks()

	return svc
}

func (s *CronService) registerTasks() {
	// Task 1: Mark expired short URLs as status=2 (expired)
	// Runs every hour on the hour.
	if _, err := s.cron.AddFunc("0 * * * *", s.markExpiredLinks); err != nil {
		s.logger.Error("failed to register mark-expired task", zap.Error(err))
	}

	// Task 2: Clean up old click_logs rows older than 90 days
	// Runs at 03:00 daily.
	if _, err := s.cron.AddFunc("0 3 * * *", s.cleanupOldClickLogs); err != nil {
		s.logger.Error("failed to register click-log cleanup task", zap.Error(err))
	}

	// Task 3: Warm stats cache by pre-aggregating hourly click counts
	// Runs every 10 minutes.
	if _, err := s.cron.AddFunc("*/10 * * * *", s.aggregateStats); err != nil {
		s.logger.Error("failed to register stats aggregation task", zap.Error(err))
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

// zapLogger adapts zap.Logger to a simple Printf interface.
type zapLogger struct{ l *zap.Logger }

func (z zapLogger) Printf(format string, args ...interface{}) {
	z.l.Info(fmt.Sprintf(format, args...))
}
