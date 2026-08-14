package repository

import (
	"dwz-admin/internal/model"

	"gorm.io/gorm"
)

// WebhookDeliveryFilters defines query filters for webhook delivery logs.
type WebhookDeliveryFilters struct {
	WebhookID *uint64
	Event     string
	Success   *int8
}

// WebhookRepo manages webhook subscriptions and delivery logs.
type WebhookRepo interface {
	ListByStatus(status int8) ([]model.WebhookSub, error)
	FindByID(id uint64) (*model.WebhookSub, error)
	Create(sub *model.WebhookSub) error
	Delete(id uint64) error
	CreateDelivery(d *model.WebhookDelivery) error
	FindDeliveryByID(id uint64) (*model.WebhookDelivery, error)
	ListDeliveries(page, perPage int, filters WebhookDeliveryFilters) ([]model.WebhookDelivery, int64, error)
}

type webhookRepo struct {
	db *gorm.DB
}

func NewWebhookRepo(db *gorm.DB) WebhookRepo {
	return &webhookRepo{db: db}
}

func (r *webhookRepo) ListByStatus(status int8) ([]model.WebhookSub, error) {
	var subs []model.WebhookSub
	err := r.db.Where("status = ?", status).Find(&subs).Error
	return subs, err
}

func (r *webhookRepo) FindByID(id uint64) (*model.WebhookSub, error) {
	var sub model.WebhookSub
	err := r.db.First(&sub, id).Error
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (r *webhookRepo) Create(sub *model.WebhookSub) error {
	return r.db.Create(sub).Error
}

func (r *webhookRepo) Delete(id uint64) error {
	return r.db.Delete(&model.WebhookSub{}, id).Error
}

func (r *webhookRepo) CreateDelivery(d *model.WebhookDelivery) error {
	return r.db.Create(d).Error
}

func (r *webhookRepo) FindDeliveryByID(id uint64) (*model.WebhookDelivery, error) {
	var d model.WebhookDelivery
	err := r.db.First(&d, id).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *webhookRepo) ListDeliveries(page, perPage int, filters WebhookDeliveryFilters) ([]model.WebhookDelivery, int64, error) {
	var list []model.WebhookDelivery
	var total int64

	query := r.db.Model(&model.WebhookDelivery{})
	if filters.WebhookID != nil {
		query = query.Where("webhook_id = ?", *filters.WebhookID)
	}
	if filters.Event != "" {
		query = query.Where("event = ?", filters.Event)
	}
	if filters.Success != nil {
		query = query.Where("success = ?", *filters.Success)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Offset(offset).Limit(perPage).Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}