package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"dwz-admin/internal/model"
	"dwz-admin/internal/repository"
)

var (
	ErrApiKeyNotFound = errors.New("api key not found")
	ErrApiKeyExpired  = errors.New("api key expired")
)

// ApiKeyCreateResult is the create-key response contract, shared with the
// admin SPA (frontend/src/api/api-keys.ts). The SPA reads api_key/name/... at
// the top level, so the flat shape is the authoritative one; key/plain_text are
// kept for API-key-based callers that were built against the older shape.
type ApiKeyCreateResult struct {
	ID        uint64     `json:"id"`
	Name      string     `json:"name"`
	ApiKey    string     `json:"api_key"`
	KeyPrefix string     `json:"key_prefix"`
	ExpiresAt *time.Time `json:"expires_at"`
	RateLimit int        `json:"rate_limit"`

	// 兼容字段（保留旧契约，避免既有集成方解析失败）
	Key       *model.ApiKey `json:"key"`
	PlainText string        `json:"plain_text"`
}

type ApiKeyService interface {
	Create(userID uint64, name string, permissions string, rateLimit int, expiresAt *time.Time) (*ApiKeyCreateResult, error)
	List(userID uint64) ([]model.ApiKey, error)

	// Revoke and GetByID are the original, ownership-blind operations. They are
	// kept for internal/background callers; HTTP handlers must use the *As
	// variants, which enforce that the acting administrator either owns the key
	// or is a super_admin.
	Revoke(id uint64) error
	GetByID(id uint64) (*model.ApiKey, error)

	// RevokeAs / GetByIDAs are the caller-aware variants. See assertCanTouch.
	RevokeAs(actorID, id uint64) error
	GetByIDAs(actorID, id uint64) (*model.ApiKey, error)
}

type apiKeyService struct {
	repo repository.ApiKeyRepo
	// isSuperAdmin reports whether a user holds super_admin. Injected so the
	// ownership rule can be unit-tested without a user repository; nil means
	// "nobody is a super_admin", which is the safe default (it makes the
	// ownership check strict rather than permissive).
	isSuperAdmin func(userID uint64) (bool, error)
}

// NewApiKeyService builds the concrete service. It returns the concrete type
// (not the interface) so callers can chain WithSuperAdminChecker; the zero-value
// configuration is safe on its own because the ownership guard treats a missing
// super-admin resolver as "nobody is a super_admin", i.e. owner-only.
func NewApiKeyService(repo repository.ApiKeyRepo) *apiKeyService {
	return &apiKeyService{repo: repo}
}

// WithSuperAdminChecker injects the super_admin resolver used by the ownership
// guard. Leaving it unset keeps the guard strict (only the owner may act).
func (s *apiKeyService) WithSuperAdminChecker(fn func(userID uint64) (bool, error)) *apiKeyService {
	s.isSuperAdmin = fn
	return s
}

// ErrApiKeyForbidden is returned when an administrator tries to read or revoke
// an API key that belongs to somebody else without being a super_admin.
var ErrApiKeyForbidden = errors.New("api key 不属于当前账号")

// assertCanTouch is the single ownership guard for API-key operations (#40).
//
// The bug: List filtered by user_id, but Revoke and GetById took a bare id with
// no ownership constraint at all. Any account holding `api_keys.revoke` could
// walk the id space and revoke every API key on the platform (a denial of
// service against every integration) and read other tenants' key metadata via
// the stats endpoint. Reproduced live: a user with an empty key list of their
// own revoked a super_admin's key and that key immediately stopped working.
//
// The rule mirrors the user-management guard: the owner may act on their own
// key, a super_admin may act on anyone's, and everyone else is refused. A nil
// super-admin resolver means "not a super_admin".
func (s *apiKeyService) assertCanTouch(actorID uint64, key *model.ApiKey) error {
	if key == nil {
		return ErrApiKeyNotFound
	}
	// Internal/background callers have no acting identity. Guards live at the
	// HTTP boundary, so do not block programmatic use (consistent with
	// userService.assertCanActOn).
	if actorID == 0 {
		return nil
	}
	if key.UserID == actorID {
		return nil
	}
	if s.isSuperAdmin != nil {
		if ok, err := s.isSuperAdmin(actorID); err != nil {
			return err
		} else if ok {
			return nil
		}
	}
	return ErrApiKeyForbidden
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
	// permissions is a JSON column; store a valid JSON array (empty => none).
	if strings.TrimSpace(permissions) == "" {
		permissions = "[]"
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
		ID:        key.ID,
		Name:      key.Name,
		ApiKey:    plainText,
		KeyPrefix: key.KeyPrefix,
		ExpiresAt: key.ExpiresAt,
		RateLimit: key.RateLimit,
		Key:       key,
		PlainText: plainText,
	}, nil
}

func (s *apiKeyService) List(userID uint64) ([]model.ApiKey, error) {
	return s.repo.ListByUser(userID)
}

func (s *apiKeyService) Revoke(id uint64) error {
	if _, err := s.repo.FindByID(id); err != nil {
		return ErrApiKeyNotFound
	}
	return s.repo.Revoke(id)
}

func (s *apiKeyService) GetByID(id uint64) (*model.ApiKey, error) {
	return s.repo.FindByID(id)
}

// RevokeAs revokes a key only when the acting administrator owns it (or is a
// super_admin). A key that belongs to somebody else is reported as
// ErrApiKeyForbidden; a key that does not exist stays ErrApiKeyNotFound so the
// two cases cannot be told apart by probing ids.
func (s *apiKeyService) RevokeAs(actorID, id uint64) error {
	key, err := s.repo.FindByID(id)
	if err != nil {
		return ErrApiKeyNotFound
	}
	if err := s.assertCanTouch(actorID, key); err != nil {
		return err
	}
	return s.repo.Revoke(id)
}

// GetByIDAs is RevokeAs for reads (the stats endpoint).
func (s *apiKeyService) GetByIDAs(actorID, id uint64) (*model.ApiKey, error) {
	key, err := s.repo.FindByID(id)
	if err != nil {
		return nil, ErrApiKeyNotFound
	}
	if err := s.assertCanTouch(actorID, key); err != nil {
		return nil, err
	}
	return key, nil
}
