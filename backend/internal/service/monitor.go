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
}

type MonitorStatus struct {
	Uptime     string        `json:"uptime"`
	StartTime  time.Time     `json:"start_time"`
	Goroutines int           `json:"goroutines"`
	DB         *DBStatus     `json:"db"`
	Redis      *RedisStatus  `json:"redis"`
	Queue      *QueueStatus  `json:"queue"`
	Cron       []CronStatus  `json:"cron"`
}

type DBStatus struct {
	Healthy    bool   `json:"healthy"`
	OpenConns  int    `json:"open_conns"`
	InUse      int    `json:"in_use"`
	Idle       int    `json:"idle"`
	Error      string `json:"error,omitempty"`
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
}

type monitorService struct {
	db         *gorm.DB
	rdb        *redis.Client
	startTime  time.Time
	queue      ClickQueueStats
	cron       *CronService
	logger     *zap.Logger
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

func (s *monitorService) Status() (*MonitorStatus, error) {
	status := &MonitorStatus{
		StartTime:  s.startTime,
		Uptime:     time.Since(s.startTime).Round(time.Second).String(),
		Goroutines: runtime.NumGoroutine(),
	}

	// DB
	if s.db != nil {
		ds := &DBStatus{Healthy: true}
		sqlDB, err := s.db.DB()
		if err == nil && sqlDB != nil {
			stats := sqlDB.Stats()
			ds.OpenConns = stats.OpenConnections
			ds.InUse = stats.InUse
			ds.Idle = stats.Idle
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

	// Cron
	if s.cron != nil {
		status.Cron = []CronStatus{
			{Name: "mark_expired", LastRun: s.cron.LastRun("mark_expired")},
			{Name: "cleanup_click_logs", LastRun: s.cron.LastRun("cleanup_click_logs")},
			{Name: "aggregate_stats", LastRun: s.cron.LastRun("aggregate_stats")},
			{Name: "cleanup_stats", LastRun: s.cron.LastRun("cleanup_stats")},
			{Name: "remind_expiring", LastRun: s.cron.LastRun("remind_expiring")},
			{Name: "reconcile_dual_write", LastRun: s.cron.LastRun("reconcile_dual_write")},
			{Name: "reconcile_clicks", LastRun: s.cron.LastRun("reconcile_clicks")},
			{Name: "ensure_partitions", LastRun: s.cron.LastRun("ensure_partitions")},
		}
	}

	return status, nil
}