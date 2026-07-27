package repository

import (
	"time"

	"dwz-admin/internal/model"

	"gorm.io/gorm"
)

type ApiKeyRepo interface {
	Create(key *model.ApiKey) error
	FindByHash(hash string) (*model.ApiKey, error)
	FindByPrefix(prefix string) ([]model.ApiKey, error)
	FindByID(id uint64) (*model.ApiKey, error)
	ListByUser(userID uint64) ([]model.ApiKey, error)
	Revoke(id uint64) error
	UpdateLastUsed(id uint64) error
}

type apiKeyRepo struct {
	db *gorm.DB
}

func NewApiKeyRepo(db *gorm.DB) ApiKeyRepo {
	return &apiKeyRepo{db: db}
}

func (r *apiKeyRepo) Create(key *model.ApiKey) error {
	return r.db.Create(key).Error
}

func (r *apiKeyRepo) FindByHash(hash string) (*model.ApiKey, error) {
	var key model.ApiKey
	err := r.db.Where("key_hash = ? AND status = 1", hash).First(&key).Error
	if err != nil {
		return nil, err
	}
	return &key, nil
}

func (r *apiKeyRepo) FindByPrefix(prefix string) ([]model.ApiKey, error) {
	var keys []model.ApiKey
	err := r.db.Where("key_prefix = ?", prefix).Find(&keys).Error
	return keys, err
}

func (r *apiKeyRepo) FindByID(id uint64) (*model.ApiKey, error) {
	var key model.ApiKey
	err := r.db.First(&key, id).Error
	if err != nil {
		return nil, err
	}
	return &key, nil
}

func (r *apiKeyRepo) ListByUser(userID uint64) ([]model.ApiKey, error) {
	var keys []model.ApiKey
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&keys).Error
	return keys, err
}

func (r *apiKeyRepo) Revoke(id uint64) error {
	return r.db.Model(&model.ApiKey{}).Where("id = ?", id).Update("status", 0).Error
}

func (r *apiKeyRepo) UpdateLastUsed(id uint64) error {
	now := time.Now()
	return r.db.Model(&model.ApiKey{}).Where("id = ?", id).Update("last_used_at", now).Error
}
