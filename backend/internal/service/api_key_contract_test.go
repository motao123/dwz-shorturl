package service

import (
	"encoding/json"
	"testing"
	"time"

	"dwz-admin/internal/model"
)

// stubApiKeyRepo 只实现 Create，其余方法在契约测试里用不到。
type stubApiKeyRepo struct {
	last *model.ApiKey
}

func (s *stubApiKeyRepo) Create(key *model.ApiKey) error {
	key.ID = 42
	s.last = key
	return nil
}
func (s *stubApiKeyRepo) FindByHash(string) (*model.ApiKey, error)    { return nil, nil }
func (s *stubApiKeyRepo) FindByPrefix(string) ([]model.ApiKey, error) { return nil, nil }
func (s *stubApiKeyRepo) FindByID(uint64) (*model.ApiKey, error)      { return nil, nil }
func (s *stubApiKeyRepo) ListByUser(uint64) ([]model.ApiKey, error)   { return nil, nil }
func (s *stubApiKeyRepo) Revoke(uint64) error                         { return nil }
func (s *stubApiKeyRepo) UpdateLastUsed(uint64) error                 { return nil }

// 管理台前端读取的是扁平结构 res.api_key / res.name。
// 这个测试锁死该契约，防止字段名再次漂移导致用户拿不到明文密钥。
func TestApiKeyCreateJSONContract(t *testing.T) {
	svc := NewApiKeyService(&stubApiKeyRepo{})
	res, err := svc.Create(7, "我的密钥", "", 0, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// 前端契约字段必须存在且非空
	if v, _ := got["api_key"].(string); v == "" {
		t.Error(`response missing top-level "api_key" (the admin SPA reads res.api_key)`)
	}
	if v, _ := got["name"].(string); v != "我的密钥" {
		t.Errorf(`response "name" = %v, want 我的密钥`, got["name"])
	}
	if v, _ := got["key_prefix"].(string); v == "" {
		t.Error(`response missing "key_prefix"`)
	}
	if _, ok := got["id"]; !ok {
		t.Error(`response missing "id"`)
	}

	// 兼容字段仍需保留，避免既有 API 集成方解析失败
	if _, ok := got["plain_text"]; !ok {
		t.Error(`response missing legacy "plain_text" field`)
	}
	if _, ok := got["key"]; !ok {
		t.Error(`response missing legacy "key" field`)
	}

	// api_key 与 plain_text 必须一致，且前缀对齐
	if got["api_key"] != got["plain_text"] {
		t.Errorf("api_key (%v) and plain_text (%v) diverged", got["api_key"], got["plain_text"])
	}
	if s := got["api_key"].(string); len(s) < 8 || s[:8] != got["key_prefix"].(string) {
		t.Errorf("key_prefix (%v) is not a prefix of api_key", got["key_prefix"])
	}
}

// 默认配额与到期时间也要在响应里，供前端展示。
func TestApiKeyCreateDefaults(t *testing.T) {
	svc := NewApiKeyService(&stubApiKeyRepo{})
	res, err := svc.Create(1, "k", "", 0, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.RateLimit != 100 {
		t.Errorf("default rate limit = %d, want 100", res.RateLimit)
	}
	if res.ExpiresAt != nil {
		t.Errorf("expires_at = %v, want nil", res.ExpiresAt)
	}
	exp := time.Now().Add(time.Hour)
	res, err = svc.Create(1, "k", "", 5, &exp)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.RateLimit != 5 || res.ExpiresAt == nil {
		t.Errorf("explicit rate limit/expiry not echoed: %+v", res)
	}
}
