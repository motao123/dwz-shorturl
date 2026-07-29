package repository

import (
	"time"

	"dwz-admin/internal/model"

	"gorm.io/gorm"
)

// ShortUrlFilters defines query filters for listing short URLs.
type ShortUrlFilters struct {
	Keyword    string
	Status     *int8
	CategoryID *uint64
	DomainID   *uint64
	CreatedBy  *uint64
	DateFrom   *time.Time
	DateTo     *time.Time
	Sort       string
	Order      string
}

type ShortUrlRepo interface {
	FindByUID(uid string) (*model.ShortUrl, error)
	FindByHash(hash string) (*model.ShortUrl, error)
	FindByID(id uint64) (*model.ShortUrl, error)
	CreateWithDomainCount(url *model.ShortUrl) error
	Update(url *model.ShortUrl) error
	UpdateWithDomainCount(url *model.ShortUrl, oldDomainID *uint64) error
	SoftDeleteWithDomainCount(url *model.ShortUrl) error
	BatchDeleteWithDomainCount(urls []model.ShortUrl) error
	List(page, perPage int, filters ShortUrlFilters) ([]model.ShortUrl, int64, error)
	Count() (int64, error)
	CountByStatus(status int8) (int64, error)
	CountToday() (int64, error)
	BatchCreate(urls []model.ShortUrl) error
	IncrementClicks(id uint64) error
	FindTopN(n int) ([]model.ShortUrl, error)
	FindRecent(n int) ([]model.ShortUrl, error)
}

type shortUrlRepo struct {
	db *gorm.DB
}

func NewShortUrlRepo(db *gorm.DB) ShortUrlRepo {
	return &shortUrlRepo{db: db}
}

func (r *shortUrlRepo) FindByUID(uid string) (*model.ShortUrl, error) {
	var url model.ShortUrl
	err := r.db.Where("uid = ?", uid).First(&url).Error
	if err != nil {
		return nil, err
	}
	return &url, nil
}

func (r *shortUrlRepo) FindByHash(hash string) (*model.ShortUrl, error) {
	var url model.ShortUrl
	err := r.db.Where("url_hash = ?", hash).First(&url).Error
	if err != nil {
		return nil, err
	}
	return &url, nil
}

func (r *shortUrlRepo) FindByID(id uint64) (*model.ShortUrl, error) {
	var url model.ShortUrl
	err := r.db.First(&url, id).Error
	if err != nil {
		return nil, err
	}
	return &url, nil
}

// CreateWithDomainCount atomically creates a short URL and increments the
// selected domain's derived link_count. A failed count update rolls back the
// short URL insert so both tables remain consistent.
func (r *shortUrlRepo) CreateWithDomainCount(url *model.ShortUrl) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(url).Error; err != nil {
			return err
		}
		if url.DomainID == nil {
			return nil
		}
		result := tx.Model(&model.Domain{}).
			Where("id = ?", *url.DomainID).
			UpdateColumn("link_count", gorm.Expr("link_count + 1"))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (r *shortUrlRepo) Update(url *model.ShortUrl) error {
	return r.db.Save(url).Error
}

// UpdateWithDomainCount atomically moves a short URL between domains and
// adjusts both derived counters. Passing equal old/new domain IDs only saves
// the record without touching counters.
func (r *shortUrlRepo) UpdateWithDomainCount(url *model.ShortUrl, oldDomainID *uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(url).Error; err != nil {
			return err
		}
		if sameUint64Ptr(oldDomainID, url.DomainID) {
			return nil
		}
		if oldDomainID != nil {
			result := tx.Model(&model.Domain{}).
				Where("id = ?", *oldDomainID).
				UpdateColumn("link_count", gorm.Expr("CASE WHEN link_count > 0 THEN link_count - 1 ELSE 0 END"))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}
		if url.DomainID != nil {
			result := tx.Model(&model.Domain{}).
				Where("id = ?", *url.DomainID).
				UpdateColumn("link_count", gorm.Expr("link_count + 1"))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}
		return nil
	})
}

// SoftDeleteWithDomainCount atomically soft-deletes one short URL and
// decrements its domain counter.
func (r *shortUrlRepo) SoftDeleteWithDomainCount(url *model.ShortUrl) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Delete(&model.ShortUrl{}, url.ID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if url.DomainID == nil {
			return nil
		}
		result = tx.Model(&model.Domain{}).
			Where("id = ?", *url.DomainID).
			UpdateColumn("link_count", gorm.Expr("CASE WHEN link_count > 0 THEN link_count - 1 ELSE 0 END"))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// BatchDeleteWithDomainCount atomically soft-deletes a batch and decrements
// each affected domain by the exact number of deleted links.
func (r *shortUrlRepo) BatchDeleteWithDomainCount(urls []model.ShortUrl) error {
	if len(urls) == 0 {
		return nil
	}
	ids := make([]uint64, 0, len(urls))
	domainCounts := make(map[uint64]int)
	for _, url := range urls {
		ids = append(ids, url.ID)
		if url.DomainID != nil {
			domainCounts[*url.DomainID]++
		}
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Delete(&model.ShortUrl{}, ids)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(ids)) {
			return gorm.ErrRecordNotFound
		}
		for domainID, count := range domainCounts {
			result = tx.Model(&model.Domain{}).
				Where("id = ?", domainID).
				UpdateColumn("link_count", gorm.Expr("CASE WHEN link_count >= ? THEN link_count - ? ELSE 0 END", count, count))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}
		return nil
	})
}

func (r *shortUrlRepo) List(page, perPage int, filters ShortUrlFilters) ([]model.ShortUrl, int64, error) {
	var urls []model.ShortUrl
	var total int64

	query := r.db.Model(&model.ShortUrl{})

	if filters.Keyword != "" {
		like := "%" + filters.Keyword + "%"
		query = query.Where("uid LIKE ? OR long_url LIKE ? OR title LIKE ?", like, like, like)
	}
	if filters.Status != nil {
		query = query.Where("status = ?", *filters.Status)
	}
	if filters.CategoryID != nil {
		query = query.Where("category_id = ?", *filters.CategoryID)
	}
	if filters.DomainID != nil {
		query = query.Where("domain_id = ?", *filters.DomainID)
	}
	if filters.CreatedBy != nil {
		query = query.Where("created_by = ?", *filters.CreatedBy)
	}
	if filters.DateFrom != nil {
		query = query.Where("created_at >= ?", *filters.DateFrom)
	}
	if filters.DateTo != nil {
		query = query.Where("created_at <= ?", *filters.DateTo)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Sorting
	sortField := "created_at"
	if filters.Sort != "" {
		allowed := map[string]bool{"created_at": true, "clicks": true, "uid": true, "updated_at": true}
		if allowed[filters.Sort] {
			sortField = filters.Sort
		}
	}
	orderDir := "DESC"
	if filters.Order == "asc" {
		orderDir = "ASC"
	}

	offset := (page - 1) * perPage
	if err := query.Offset(offset).Limit(perPage).Order(sortField + " " + orderDir).Find(&urls).Error; err != nil {
		return nil, 0, err
	}

	return urls, total, nil
}

func sameUint64Ptr(a, b *uint64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (r *shortUrlRepo) Count() (int64, error) {
	var count int64
	err := r.db.Model(&model.ShortUrl{}).Count(&count).Error
	return count, err
}

func (r *shortUrlRepo) CountByStatus(status int8) (int64, error) {
	var count int64
	err := r.db.Model(&model.ShortUrl{}).Where("status = ?", status).Count(&count).Error
	return count, err
}

func (r *shortUrlRepo) CountToday() (int64, error) {
	var count int64
	today := time.Now().Format("2006-01-02")
	err := r.db.Model(&model.ShortUrl{}).Where("DATE(created_at) = ?", today).Count(&count).Error
	return count, err
}

func (r *shortUrlRepo) BatchCreate(urls []model.ShortUrl) error {
	return r.db.Create(&urls).Error
}

func (r *shortUrlRepo) IncrementClicks(id uint64) error {
	return r.db.Model(&model.ShortUrl{}).Where("id = ?", id).
		UpdateColumn("clicks", gorm.Expr("clicks + 1")).Error
}

func (r *shortUrlRepo) FindTopN(n int) ([]model.ShortUrl, error) {
	var urls []model.ShortUrl
	err := r.db.Order("clicks DESC").Limit(n).Find(&urls).Error
	return urls, err
}

func (r *shortUrlRepo) FindRecent(n int) ([]model.ShortUrl, error) {
	var urls []model.ShortUrl
	err := r.db.Order("created_at DESC").Limit(n).Find(&urls).Error
	return urls, err
}
