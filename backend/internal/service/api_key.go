package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"dwz-admin/internal/model"
	"dwz-admin/internal/repository"
)

var (
	ErrApiKeyNotFound = errors.New("api key not found")
	ErrApiKeyExpired  = errors.New("api key expired")
)

type ApiKeyCreateResult struct {
	Key       *model.ApiKey `json:"key"`
	PlainText string        `json:"plain_text"`
}

type ApiKeyService interface {
	Create(userID uint64, name string, permissions string, rateLimit int, expiresAt *time.Time) (*ApiKeyCreateResult, error)
	List(userID uint64) ([]model.ApiKey, error)
	Revoke(id uint64) error
	GetByID(id uint64) (*model.ApiKey, error)
}

type apiKeyService struct {
	repo repository.ApiKeyRepo
}

func NewApiKeyService(repo repository.ApiKeyRepo) ApiKeyService {
	return &apiKeyService{repo: repo}
}

func (s *apiKeyService) Create(userID uint64, name string, permissions string, rateLimit int, expiresAt *time.Time) (*ApiKeyCreateResult, error) {
	// Generate a random key: dwz_ prefix + 32 random hex chars
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, err
	}
	plainText := "dwz_" + hex.EncodeToString(randomBytes)

	// Hash the full key with SHA-256
	hash := sha256.Sum256([]byte(plainText))
	keyHash := hex.EncodeToString(hash[:])

	// Prefix is first 8 chars for display
	prefix := plainText[:8]

	if rateLimit <= 0 {
		rateLimit = 100
	}

	key := &model.ApiKey{
		UserID:      userID,
		Name:        name,
		KeyPrefix:   prefix,
		KeyHash:     keyHash,
		Permissions: permissions,
		RateLimit:   rateLimit,
		ExpiresAt:   expiresAt,
		Status:      1,
	}

	if err := s.repo.Create(key); err != nil {
		return nil, err
	}

	return &ApiKeyCreateResult{
		Key:       key,
		PlainText: plainText,
	}, nil
}

func (s *apiKeyService) List(userID uint64) ([]model.ApiKey, error) {
	return s.repo.ListByUser(userID)
}

func (s *apiKeyService) Revoke(id uint64) error {
	_, err := s.repo.FindByID(id)
	if err != nil {
		return ErrApiKeyNotFound
	}
	return s.repo.Revoke(id)
}

func (s *apiKeyService) GetByID(id uint64) (*model.ApiKey, error) {
	return s.repo.FindByID(id)
}
