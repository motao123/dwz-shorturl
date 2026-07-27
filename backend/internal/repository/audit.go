package repository

import (
	"time"

	"dwz-admin/internal/model"

	"gorm.io/gorm"
)

// AuditFilters defines query filters for listing audit logs.
type AuditFilters struct {
	UserID   *uint64
	Action   string
	DateFrom *time.Time
	DateTo   *time.Time
}

type AuditRepo interface {
	Create(log *model.AuditLog) error
	List(page, perPage int, filters AuditFilters) ([]model.AuditLog, int64, error)
	FindByID(id uint64) (*model.AuditLog, error)
}

type auditRepo struct {
	db *gorm.DB
}

func NewAuditRepo(db *gorm.DB) AuditRepo {
	return &auditRepo{db: db}
}

func (r *auditRepo) Create(log *model.AuditLog) error {
	return r.db.Create(log).Error
}

func (r *auditRepo) List(page, perPage int, filters AuditFilters) ([]model.AuditLog, int64, error) {
	var logs []model.AuditLog
	var total int64

	query := r.db.Model(&model.AuditLog{})

	if filters.UserID != nil {
		query = query.Where("user_id = ?", *filters.UserID)
	}
	if filters.Action != "" {
		query = query.Where("action LIKE ?", "%"+filters.Action+"%")
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

	offset := (page - 1) * perPage
	if err := query.Offset(offset).Limit(perPage).Order("created_at DESC").Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

func (r *auditRepo) FindByID(id uint64) (*model.AuditLog, error) {
	var log model.AuditLog
	err := r.db.First(&log, id).Error
	if err != nil {
		return nil, err
	}
	return &log, nil
}
