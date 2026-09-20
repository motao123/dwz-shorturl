package service

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MonitorService aggregates system health and background task status for a
// monitoring page.
type MonitorService interface {
	Status() (*MonitorStatus, error)
	RunTask(name string) (bool, error)
	// EnsurePartitionsWithStatus idempotently creates missing click_logs monthly
	// partitions up to `months` ahead of now and returns the post-run coverage
	// snapshot plus the alert state (partial progress survives a mid-way
	// failure, so the caller can report exactly how far it got).
	EnsurePartitionsWithStatus(months int) (*PartitionReconcileResult, error)
	// PreviewEnsurePartitions lists the months a catch-up would create, without
	// taking the DDL lock (dry run).
	PreviewEnsurePartitions(months int) (*PartitionReconcileResult, error)
	// PartitionStatus reports the click_logs partition coverage (read-only).
	PartitionStatus() (*PartitionStatus, error)
}

type MonitorStatus struct {
	Uptime     string       `json:"uptime"`
	StartTime  time.Time    `json:"start_time"`
	Goroutines int          `json:"goroutines"`
	DB         *DBStatus    `json:"db"`
	Redis      *RedisStatus `json:"redis"`
	Queue      *QueueStatus `json:"queue"`
	Cron       []CronStatus `json:"cron"`
	// Partitions reports click_logs monthly partition coverage so an operator
	// can see at a glance whether the maintenance task is keeping up.
	Partitions *PartitionStatus `json:"partitions"`
}

type DBStatus struct {
	Healthy   bool   `json:"healthy"`
	OpenConns int    `json:"open_conns"`
	InUse     int    `json:"in_use"`
	Idle      int    `json:"idle"`
	Error     string `json:"error,omitempty"`
}

type RedisStatus struct {
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

type QueueStatus struct {
	Pending int `json:"pending"`
}

type CronStatus struct {
	Name    string    `json:"name"`
	LastRun time.Time `json:"last_run"`
	// Failed is true when the task's most recent run errored. Only the
	// partition task tracks its outcome today (a failed ALTER TABLE must not
	// look like a healthy task).
	Failed bool `json:"failed,omitempty"`
	// Error carries the last failure message, when known.
	Error string `json:"error,omitempty"`
	// ConsecutiveFailures counts failures since the last success; the monitor
	// turns this red past the alert threshold.
	ConsecutiveFailures int `json:"consecutive_failures,omitempty"`
}

type monitorService struct {
	db        *gorm.DB
	rdb       *redis.Client
	startTime time.Time
	queue     ClickQueueStats
	cron      *CronService
	logger    *zap.Logger
}

// ClickQueueStats is the minimal interface the monitor needs from the click
// queue (keeps monitor decoupled from the concrete handler type).
type ClickQueueStats interface {
	PendingCount() int
}

func NewMonitorService(db *gorm.DB, rdb *redis.Client, queue ClickQueueStats, cron *CronService, logger *zap.Logger) MonitorService {
	return &monitorService{
		db:        db,
		rdb:       rdb,
		startTime: time.Now(),
		queue:     queue,
		cron:      cron,
		logger:    logger,
	}
}

func (s *monitorService) RunTask(name string) (bool, error) {
	if s.cron == nil {
		return false, fmt.Errorf("cron not available")
	}
	ok, err := s.cron.RunTask(name)
	if err != nil {
		return false, err
	}
	return ok, nil
}

// EnsurePartitionsWithStatus lets an operator fix a lagging partition horizon on
// demand instead of waiting for the next daily cron window, and returns the
// resulting coverage so the UI can show exactly what happened (including partial
// progress when an ALTER fails half-way). It is idempotent: months that already
// exist are never re-created.
func (s *monitorService) EnsurePartitionsWithStatus(months int) (*PartitionReconcileResult, error) {
	if s.cron == nil {
		return nil, fmt.Errorf("cron not available")
	}
	return s.cron.EnsurePartitionsWithStatus(months)
}

// PreviewEnsurePartitions reports what a catch-up would do, without DDL.
func (s *monitorService) PreviewEnsurePartitions(months int) (*PartitionReconcileResult, error) {
	if s.cron == nil {
		return nil, fmt.Errorf("cron not available")
	}
	return s.cron.PreviewEnsurePartitions(months)
}

// PartitionStatus reports the click_logs partition coverage without DDL.
func (s *monitorService) PartitionStatus() (*PartitionStatus, error) {
	if s.cron == nil {
		return nil, fmt.Errorf("cron not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.cron.InspectClickLogsPartitionsContext(ctx), nil
}

func (s *monitorService) Status() (*MonitorStatus, error) {
	status := &MonitorStatus{
		StartTime:  s.startTime,
		Uptime:     time.Since(s.startTime).Round(time.Second).String(),
		Goroutines: runtime.NumGoroutine(),
	}

	// DB
	//
	// #29: this used to report Healthy unconditionally, with no Ping and no code
	// path that could ever set Error. The monitor page therefore showed a green
	// database while /metrics in the same process correctly reported
	// dwz_db_up = 0 — two views of one fact that disagreed. Ping it.
	if s.db != nil {
		ds := &DBStatus{Healthy: true}
		sqlDB, err := s.db.DB()
		if err != nil || sqlDB == nil {
			ds.Healthy = false
			if err != nil {
				ds.Error = err.Error()
			} else {
				ds.Error = "database handle unavailable"
			}
		} else {
			stats := sqlDB.Stats()
			ds.OpenConns = stats.OpenConnections
			ds.InUse = stats.InUse
			ds.Idle = stats.Idle

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if pingErr := sqlDB.PingContext(ctx); pingErr != nil {
				ds.Healthy = false
				ds.Error = pingErr.Error()
			}
			cancel()
		}
		status.DB = ds
	}

	// Redis
	if s.rdb != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		rs := &RedisStatus{Healthy: true}
		if err := s.rdb.Ping(ctx).Err(); err != nil {
			rs.Healthy = false
			rs.Error = err.Error()
		}
		status.Redis = rs
	}

	// Queue
	if s.queue != nil {
		status.Queue = &QueueStatus{Pending: s.queue.PendingCount()}
	}

	// Click-log partition coverage / maintenance health.
	if s.cron != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		status.Partitions = s.cron.InspectClickLogsPartitionsContext(ctx)
		cancel()
	}

	// Cron
	if s.cron != nil {
		lastRun, lastErr, failures := s.cron.partitionSnapshotValues()
		status.Cron = []CronStatus{
			{Name: "mark_expired", LastRun: s.cron.LastRun("mark_expired")},
			{Name: "cleanup_click_logs", LastRun: s.cron.LastRun("cleanup_click_logs")},
			{Name: "aggregate_stats", LastRun: s.cron.LastRun("aggregate_stats")},
			{Name: "cleanup_stats", LastRun: s.cron.LastRun("cleanup_stats")},
			{Name: "remind_expiring", LastRun: s.cron.LastRun("remind_expiring")},
			{Name: "reconcile_dual_write", LastRun: s.cron.LastRun("reconcile_dual_write")},
			{Name: "reconcile_clicks", LastRun: s.cron.LastRun("reconcile_clicks")},
			{
				Name:                "ensure_partitions",
				LastRun:             orTime(lastRun, s.cron.LastRun("ensure_partitions")),
				Failed:              lastErr != "",
				Error:               lastErr,
				ConsecutiveFailures: failures,
			},
		}
	}

	return status, nil
}
