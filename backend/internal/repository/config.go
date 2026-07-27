package repository

import (
	"dwz-admin/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ConfigRepo interface {
	GetAll() ([]model.SystemConfig, error)
	GetByKey(key string) (*model.SystemConfig, error)
	Upsert(cfg *model.SystemConfig) error
	BatchUpdate(configs []model.SystemConfig) error
}

type configRepo struct {
	db *gorm.DB
}

func NewConfigRepo(db *gorm.DB) ConfigRepo {
	return &configRepo{db: db}
}

func (r *configRepo) GetAll() ([]model.SystemConfig, error) {
	var configs []model.SystemConfig
	err := r.db.Order("config_key ASC").Find(&configs).Error
	return configs, err
}

func (r *configRepo) GetByKey(key string) (*model.SystemConfig, error) {
	var cfg model.SystemConfig
	err := r.db.Where("config_key = ?", key).First(&cfg).Error
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *configRepo) Upsert(cfg *model.SystemConfig) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "config_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"config_value", "value_type", "description", "is_public", "updated_by", "updated_at"}),
	}).Create(cfg).Error
}

func (r *configRepo) BatchUpdate(configs []model.SystemConfig) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		for i := range configs {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "config_key"}},
				DoUpdates: clause.AssignmentColumns([]string{"config_value", "value_type", "updated_by", "updated_at"}),
			}).Create(&configs[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
