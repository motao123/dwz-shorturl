package repository

import (
	"time"

	"dwz-admin/internal/model"

	"gorm.io/gorm"
)

// ViolationRepo reads/writes the public frontend `violation_reviews` table.
type ViolationRepo interface {
	List(page, perPage int, reviewed *int8, keyword string) ([]model.ViolationReview, int64, error)
	FindByID(id uint64) (*model.ViolationReview, error)
	Create(v *model.ViolationReview) error
	MarkReviewed(id uint64, note string) error
	Delete(id uint64) error
}

type violationRepo struct {
	db *gorm.DB
}

func NewViolationRepo(db *gorm.DB) ViolationRepo {
	return &violationRepo{db: db}
}

func (r *violationRepo) List(page, perPage int, reviewed *int8, keyword string) ([]model.ViolationReview, int64, error) {
	var rows []model.ViolationReview
	var total int64
	if r.db == nil {
		return rows, total, nil
	}

	query := r.db.Model(&model.ViolationReview{})
	if reviewed != nil {
		query = query.Where("reviewed = ?", *reviewed)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("url LIKE ? OR reason LIKE ?", like, like)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Offset(offset).Limit(perPage).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}

func (r *violationRepo) FindByID(id uint64) (*model.ViolationReview, error) {
	if r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var v model.ViolationReview
	err := r.db.First(&v, id).Error
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *violationRepo) Create(v *model.ViolationReview) error {
	if r.db == nil {
		return nil
	}
	return r.db.Create(v).Error
}

func (r *violationRepo) MarkReviewed(id uint64, note string) error {
	now := time.Now()
	return r.db.Model(&model.ViolationReview{}).Where("id = ?", id).Updates(map[string]interface{}{
		"reviewed":    1,
		"reviewed_at": now,
		"note":        note,
	}).Error
}

func (r *violationRepo) Delete(id uint64) error {
	return r.db.Delete(&model.ViolationReview{}, id).Error
}