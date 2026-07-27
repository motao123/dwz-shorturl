package service

import (
	"time"

	"dwz-admin/internal/model"
	"dwz-admin/internal/repository"

	"gorm.io/gorm"
)

type OverviewResult struct {
	TotalUrls   int64   `json:"total_urls"`
	TotalClicks int64   `json:"total_clicks"`
	TodayNew    int64   `json:"today_new"`
	TodayClicks int64   `json:"today_clicks"`
	ActiveRate  float64 `json:"active_rate"`
}

type TrendPoint struct {
	Label  string `json:"label"`
	Clicks int64  `json:"clicks"`
}

type StatsService interface {
	Overview() (*OverviewResult, error)
	Trend(granularity string, dateFrom, dateTo *time.Time) ([]TrendPoint, error)
	TopN(n int) ([]model.ShortUrl, error)
	Recent(n int) ([]model.ShortUrl, error)
}

type statsService struct {
	shortUrlRepo repository.ShortUrlRepo
	db           *gorm.DB
}

func NewStatsService(shortUrlRepo repository.ShortUrlRepo, db *gorm.DB) StatsService {
	return &statsService{shortUrlRepo: shortUrlRepo, db: db}
}

func (s *statsService) Overview() (*OverviewResult, error) {
	totalUrls, err := s.shortUrlRepo.Count()
	if err != nil {
		return nil, err
	}

	todayNew, err := s.shortUrlRepo.CountToday()
	if err != nil {
		return nil, err
	}

	// Total clicks across all short URLs
	var totalClicks int64
	err = s.db.Model(&model.ShortUrl{}).Select("COALESCE(SUM(clicks), 0)").Scan(&totalClicks).Error
	if err != nil {
		return nil, err
	}

	// Count today's clicks from click_logs
	var todayClicks int64
	today := time.Now().Format("2006-01-02")
	err = s.db.Model(&model.ClickLog{}).
		Where("DATE(created_at) = ?", today).
		Count(&todayClicks).Error
	if err != nil {
		return nil, err
	}

	// Active rate: percentage of URLs with clicks > 0
	var activeCount int64
	err = s.db.Model(&model.ShortUrl{}).Where("clicks > 0").Count(&activeCount).Error
	if err != nil {
		return nil, err
	}

	var activeRate float64
	if totalUrls > 0 {
		activeRate = float64(activeCount) / float64(totalUrls) * 100
	}

	return &OverviewResult{
		TotalUrls:   totalUrls,
		TotalClicks: totalClicks,
		TodayNew:    todayNew,
		TodayClicks: todayClicks,
		ActiveRate:  activeRate,
	}, nil
}

func (s *statsService) Trend(granularity string, dateFrom, dateTo *time.Time) ([]TrendPoint, error) {
	results := make([]TrendPoint, 0)

	query := s.db.Model(&model.ClickLog{})

	if dateFrom != nil {
		query = query.Where("created_at >= ?", *dateFrom)
	}
	if dateTo != nil {
		query = query.Where("created_at <= ?", *dateTo)
	}

	var format string
	switch granularity {
	case "hour":
		format = "%Y-%m-%d %H:00"
	case "month":
		format = "%Y-%m"
	default:
		format = "%Y-%m-%d"
	}

	type row struct {
		Date   string
		Clicks int64
	}
	var rows []row

	err := query.
		Select("DATE_FORMAT(created_at, ?) as date, COUNT(*) as clicks", format).
		Group("date").
		Order("date ASC").
		Scan(&rows).Error
	if err != nil {
		return results, err
	}

	for _, r := range rows {
		results = append(results, TrendPoint{Label: r.Date, Clicks: r.Clicks})
	}

	return results, nil
}

func (s *statsService) TopN(n int) ([]model.ShortUrl, error) {
	if n <= 0 || n > 100 {
		n = 10
	}
	return s.shortUrlRepo.FindTopN(n)
}

func (s *statsService) Recent(n int) ([]model.ShortUrl, error) {
	if n <= 0 || n > 100 {
		n = 20
	}
	return s.shortUrlRepo.FindRecent(n)
}
