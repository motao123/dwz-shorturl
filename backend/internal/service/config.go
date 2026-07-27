package service

import (
	"dwz-admin/internal/model"
	"dwz-admin/internal/repository"
)

type ConfigService interface {
	GetAll() ([]model.SystemConfig, error)
	BatchUpdate(configs []model.SystemConfig, updatedBy uint64) error
}

type configService struct {
	repo repository.ConfigRepo
}

func NewConfigService(repo repository.ConfigRepo) ConfigService {
	return &configService{repo: repo}
}

func (s *configService) GetAll() ([]model.SystemConfig, error) {
	return s.repo.GetAll()
}

func (s *configService) BatchUpdate(configs []model.SystemConfig, updatedBy uint64) error {
	for i := range configs {
		configs[i].UpdatedBy = &updatedBy
	}
	return s.repo.BatchUpdate(configs)
}
