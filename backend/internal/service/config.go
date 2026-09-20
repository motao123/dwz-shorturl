package service

import (
	"dwz-admin/internal/model"
	"dwz-admin/internal/repository"
)

type ConfigService interface {
	GetAll() ([]model.SystemConfig, error)
	BatchUpdate(configs []model.SystemConfig, updatedBy uint64) error
	// WithRuntimeConfig wires the runtime switch cache so a successful update
	// takes effect immediately instead of after the cache TTL.
	WithRuntimeConfig(rc *RuntimeConfig) ConfigService
}

type configService struct {
	repo    repository.ConfigRepo
	runtime *RuntimeConfig
}

func NewConfigService(repo repository.ConfigRepo) ConfigService {
	return &configService{repo: repo}
}

func (s *configService) WithRuntimeConfig(rc *RuntimeConfig) ConfigService {
	s.runtime = rc
	return s
}

func (s *configService) GetAll() ([]model.SystemConfig, error) {
	return s.repo.GetAll()
}

func (s *configService) BatchUpdate(configs []model.SystemConfig, updatedBy uint64) error {
	for i := range configs {
		configs[i].UpdatedBy = &updatedBy
	}
	if err := s.repo.BatchUpdate(configs); err != nil {
		return err
	}
	// Drop the cached switch values so the new setting is live on the next
	// request; otherwise the admin UI would say "已保存" while the old behaviour
	// persisted for up to a minute (#4 is precisely about the UI and the runtime
	// disagreeing).
	if s.runtime != nil {
		s.runtime.Invalidate()
	}
	return nil
}
