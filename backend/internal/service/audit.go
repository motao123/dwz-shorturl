package service

import (
	"dwz-admin/internal/model"
	"dwz-admin/internal/repository"
)

type AuditService interface {
	Log(userID *uint64, action, resource, resourceID, detail, ip, ua string) error
	List(page, perPage int, filters repository.AuditFilters) ([]model.AuditLog, int64, error)
	GetByID(id uint64) (*model.AuditLog, error)
}

type auditService struct {
	repo repository.AuditRepo
}

func NewAuditService(repo repository.AuditRepo) AuditService {
	return &auditService{repo: repo}
}

func (s *auditService) Log(userID *uint64, action, resource, resourceID, detail, ip, ua string) error {
	log := &model.AuditLog{
		UserID:     userID,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Detail:     detail,
		IP:         ip,
		UserAgent:  ua,
	}
	return s.repo.Create(log)
}

func (s *auditService) List(page, perPage int, filters repository.AuditFilters) ([]model.AuditLog, int64, error) {
	return s.repo.List(page, perPage, filters)
}

func (s *auditService) GetByID(id uint64) (*model.AuditLog, error) {
	return s.repo.FindByID(id)
}
