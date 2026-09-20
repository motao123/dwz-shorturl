package service

import (
	"encoding/json"
	"strings"

	"dwz-admin/internal/model"
	"dwz-admin/internal/repository"

	"go.uber.org/zap"
)

type AuditService interface {
	Log(userID *uint64, action, resource, resourceID, detail, ip, ua string) error
	List(page, perPage int, filters repository.AuditFilters) ([]model.AuditLog, int64, error)
	GetByID(id uint64) (*model.AuditLog, error)
	// WithLogger attaches a logger so a failed audit write becomes visible instead
	// of being discarded by the caller's `_ =` (#24).
	WithLogger(logger *zap.Logger) AuditService
}

type auditService struct {
	repo   repository.AuditRepo
	logger *zap.Logger
}

func NewAuditService(repo repository.AuditRepo) AuditService {
	return &auditService{repo: repo}
}

func (s *auditService) WithLogger(logger *zap.Logger) AuditService {
	s.logger = logger
	return s
}

// auditDetail turns the caller's snapshot string into a column-safe value:
// empty becomes NULL (the column is typed json, and "" is not valid JSON — that
// combination is what used to lose whole audit rows), and anything that is not
// valid JSON is stored as a JSON string so the evidence survives instead of
// being dropped.
func auditDetail(detail string) *json.RawMessage {
	trimmed := strings.TrimSpace(detail)
	if trimmed == "" {
		return nil
	}
	if !json.Valid([]byte(trimmed)) {
		encoded, err := json.Marshal(trimmed)
		if err != nil {
			return nil
		}
		trimmed = string(encoded)
	}
	raw := json.RawMessage(trimmed)
	return &raw
}

func (s *auditService) Log(userID *uint64, action, resource, resourceID, detail, ip, ua string) error {
	log := &model.AuditLog{
		UserID:     userID,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Detail:     auditDetail(detail),
		IP:         ip,
		UserAgent:  ua,
	}
	err := s.repo.Create(log)
	if err != nil && s.logger != nil {
		// Losing the audit trail must not stay invisible: the action itself
		// already succeeded, so this is a log-only path.
		s.logger.Error("audit log write failed",
			zap.String("action", action),
			zap.String("resource", resource),
			zap.String("resource_id", resourceID),
			zap.Error(err))
	}
	return err
}

func (s *auditService) List(page, perPage int, filters repository.AuditFilters) ([]model.AuditLog, int64, error) {
	return s.repo.List(page, perPage, filters)
}

func (s *auditService) GetByID(id uint64) (*model.AuditLog, error) {
	return s.repo.FindByID(id)
}
