package service

import (
	"sort"
	"time"

	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"
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

// LinkStatsResult is per-link click analytics for the admin console.
type LinkStatsResult struct {
	UID           string           `json:"uid"`
	Total         int64            `json:"total"`
	Trend         []TrendPoint     `json:"trend"`
	Referrers     []TrendPoint     `json:"referrers"`
	ReferrerTypes []TrendPoint     `json:"referrer_types"`
	Devices       []TrendPoint     `json:"devices"`
	Browsers      []TrendPoint     `json:"browsers"`
	Countries     []TrendPoint     `json:"countries"`
}

type StatsService interface {
	Overview() (*OverviewResult, error)
	Trend(granularity string, dateFrom, dateTo *time.Time) ([]TrendPoint, error)
	TopN(n int) ([]model.ShortUrl, error)
	Recent(n int) ([]model.ShortUrl, error)
	LinkStats(shortURLID uint64) (*LinkStatsResult, error)
	// Countries returns the top traffic-source countries across all links.
	Countries(limit int) ([]TrendPoint, error)
	// ReferrerTypes returns the global referrer-type breakdown.
	ReferrerTypes(limit int) ([]TrendPoint, error)
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

// LinkStats returns per-link click analytics (total, 7-day trend, top referrers).
func (s *statsService) LinkStats(shortURLID uint64) (*LinkStatsResult, error) {
	record, err := s.shortUrlRepo.FindByID(shortURLID)
	if err != nil {
		return nil, err
	}
	res := &LinkStatsResult{UID: record.UID}

	if err := s.db.Model(&model.ClickLog{}).Where("short_url_id = ?", record.ID).Count(&res.Total).Error; err != nil {
		return nil, err
	}

	since := time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	type row struct {
		Date   string
		Clicks int64
	}
	var trendRows []row
	if err := s.db.Model(&model.ClickLog{}).
		Select("DATE(created_at) as date, COUNT(*) as clicks").
		Where("short_url_id = ? AND created_at >= ?", record.ID, since).
		Group("date").
		Order("date ASC").
		Scan(&trendRows).Error; err != nil {
		return nil, err
	}
	for _, r := range trendRows {
		res.Trend = append(res.Trend, TrendPoint{Label: r.Date, Clicks: r.Clicks})
	}

	var refRows []row
	if err := s.db.Model(&model.ClickLog{}).
		Select("referer as date, COUNT(*) as clicks").
		Where("short_url_id = ?", record.ID).
		Group("referer").
		Order("clicks DESC").
		Limit(10).
		Scan(&refRows).Error; err != nil {
		return nil, err
	}
	for _, r := range refRows {
		res.Referrers = append(res.Referrers, TrendPoint{Label: r.Date, Clicks: r.Clicks})
	}

	// Referrer type breakdown (search / social / direct / other) from the
	// referer column, classified in memory.
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
	for t, count := range refTypes {
		res.ReferrerTypes = append(res.ReferrerTypes, TrendPoint{Label: t, Clicks: count})
	}
	sort.Slice(res.ReferrerTypes, func(i, j int) bool { return res.ReferrerTypes[i].Clicks > res.ReferrerTypes[j].Clicks })

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
		res.Devices = append(res.Devices, TrendPoint{Label: device, Clicks: count})
	}
	sort.Slice(res.Devices, func(i, j int) bool { return res.Devices[i].Clicks > res.Devices[j].Clicks })
	for browser, count := range browsers {
		res.Browsers = append(res.Browsers, TrendPoint{Label: browser, Clicks: count})
	}
	sort.Slice(res.Browsers, func(i, j int) bool { return res.Browsers[i].Clicks > res.Browsers[j].Clicks })

	// Country distribution (ISO alpha-2, from GeoIP resolution at click time).
	type countryRow struct {
		Country string
		Clicks  int64
	}
	var countryRows []countryRow
	if err := s.db.Model(&model.ClickLog{}).
		Select("country, COUNT(*) as clicks").
		Where("short_url_id = ? AND country <> ''", record.ID).
		Group("country").
		Order("clicks DESC").
		Limit(12).
		Scan(&countryRows).Error; err != nil {
		return nil, err
	}
	for _, r := range countryRows {
		res.Countries = append(res.Countries, TrendPoint{Label: r.Country, Clicks: r.Clicks})
	}

	return res, nil
}

// Countries returns the top traffic-source countries across all links, ranked
// by click count. A 30-day window keeps the aggregation bounded.
func (s *statsService) Countries(limit int) ([]TrendPoint, error) {
	if limit <= 0 || limit > 50 {
		limit = 12
	}
	since := time.Now().AddDate(0, 0, -30)
	type row struct {
		Country string
		Clicks  int64
	}
	var rows []row
	if err := s.db.Model(&model.ClickLog{}).
		Select("country, COUNT(*) as clicks").
		Where("created_at >= ? AND country <> ''", since).
		Group("country").
		Order("clicks DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]TrendPoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, TrendPoint{Label: r.Country, Clicks: r.Clicks})
	}
	return out, nil
}

// ReferrerTypes returns the global referrer-type breakdown across all links.
func (s *statsService) ReferrerTypes(limit int) ([]TrendPoint, error) {
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	since := time.Now().AddDate(0, 0, -30)
	var refs []string
	if err := s.db.Model(&model.ClickLog{}).
		Where("created_at >= ?", since).
		Limit(50000).
		Pluck("referer", &refs).Error; err != nil {
		return nil, err
	}
	types := map[string]int64{}
	for _, ref := range refs {
		types[pkg.ClassifyReferer(ref)]++
	}
	out := make([]TrendPoint, 0, len(types))
	for t, c := range types {
		out = append(out, TrendPoint{Label: t, Clicks: c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Clicks > out[j].Clicks })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
