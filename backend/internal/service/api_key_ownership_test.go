package service

import (
	"errors"
	"testing"

	"dwz-admin/internal/model"
)

// ownerStubRepo is an API-key repo whose single row can be owned by any user id,
// so the ownership guard can be exercised without a database.
type ownerStubRepo struct {
	key       *model.ApiKey
	revokedID uint64
	revokeErr error
}

func (r *ownerStubRepo) Create(*model.ApiKey) error { return nil }
func (r *ownerStubRepo) FindByHash(string) (*model.ApiKey, error) {
	return nil, errors.New("not used")
}
func (r *ownerStubRepo) FindByPrefix(string) ([]model.ApiKey, error) { return nil, nil }
func (r *ownerStubRepo) FindByID(id uint64) (*model.ApiKey, error) {
	if r.key == nil || r.key.ID != id {
		return nil, errors.New("record not found")
	}
	return r.key, nil
}
func (r *ownerStubRepo) ListByUser(uint64) ([]model.ApiKey, error) { return nil, nil }
func (r *ownerStubRepo) Revoke(id uint64) error {
	r.revokedID = id
	return r.revokeErr
}
func (r *ownerStubRepo) UpdateLastUsed(uint64) error { return nil }

func newOwnerSvc(repo *ownerStubRepo, superAdmins ...uint64) *apiKeyService {
	svc := NewApiKeyService(repo)
	if len(superAdmins) > 0 {
		set := map[uint64]bool{}
		for _, id := range superAdmins {
			set[id] = true
		}
		svc = svc.WithSuperAdminChecker(func(uid uint64) (bool, error) {
			return set[uid], nil
		})
	}
	return svc
}

// TestApiKeyRevoke_OwnerAllowed 钉住正常路径：本人可以吊销自己的密钥。
func TestApiKeyRevoke_OwnerAllowed(t *testing.T) {
	repo := &ownerStubRepo{key: &model.ApiKey{ID: 5, UserID: 7}}
	svc := newOwnerSvc(repo)
	if err := svc.RevokeAs(7, 5); err != nil {
		t.Fatalf("密钥归属者应可吊销，got %v", err)
	}
	if repo.revokedID != 5 {
		t.Fatalf("预期吊销 id=5，实际 %d", repo.revokedID)
	}
}

// TestApiKeyRevoke_OtherUserForbidden 是本轮 #40 的核心回归保护。
// 反向验证：把 handler 里的 RevokeAs 改回 Revoke（去掉归属校验），
// 这个用例会变成 nil 错误 —— 正是漏洞复现时的真实行为。
func TestApiKeyRevoke_OtherUserForbidden(t *testing.T) {
	repo := &ownerStubRepo{key: &model.ApiKey{ID: 5, UserID: 1}}
	svc := newOwnerSvc(repo) // 没有任何超管
	err := svc.RevokeAs(2, 5)
	if !errors.Is(err, ErrApiKeyForbidden) {
		t.Fatalf("他人密钥应返回 ErrApiKeyForbidden，got %v", err)
	}
	if repo.revokedID != 0 {
		t.Fatalf("越权请求不得真正吊销，revokedID=%d", repo.revokedID)
	}
}

// TestApiKeyRevoke_SuperAdminAllowed 超管是显式例外，应可管理任何人的密钥。
func TestApiKeyRevoke_SuperAdminAllowed(t *testing.T) {
	repo := &ownerStubRepo{key: &model.ApiKey{ID: 5, UserID: 1}}
	svc := newOwnerSvc(repo, 99) // 99 是超管
	if err := svc.RevokeAs(99, 5); err != nil {
		t.Fatalf("超管应可吊销他人密钥，got %v", err)
	}
	if repo.revokedID != 5 {
		t.Fatalf("预期吊销 id=5，实际 %d", repo.revokedID)
	}
}

// TestApiKeyRevoke_NonSuperAdminCannotPretend 非超管不能靠"自称"绕过。
func TestApiKeyRevoke_NonSuperAdminCannotPretend(t *testing.T) {
	repo := &ownerStubRepo{key: &model.ApiKey{ID: 5, UserID: 1}}
	svc := newOwnerSvc(repo, 99) // 只有 99 是超管
	if err := svc.RevokeAs(2, 5); !errors.Is(err, ErrApiKeyForbidden) {
		t.Fatalf("非超管应被拒，got %v", err)
	}
}

// TestApiKeyRevoke_MissingKeyNotFound 不存在的 id 保持 404 语义，
// 且与"存在但不属于你"可区分（后者是 403）。
func TestApiKeyRevoke_MissingKeyNotFound(t *testing.T) {
	repo := &ownerStubRepo{key: &model.ApiKey{ID: 5, UserID: 1}}
	svc := newOwnerSvc(repo)
	if err := svc.RevokeAs(1, 999); !errors.Is(err, ErrApiKeyNotFound) {
		t.Fatalf("不存在的密钥应返回 ErrApiKeyNotFound，got %v", err)
	}
}

// TestApiKeyRevoke_NoIdentitySkipsGuard 内部调用方（actorID=0）不受归属约束，
// 与 userService.assertCanActOn 的既有约定一致（守卫放在 HTTP 边界）。
func TestApiKeyRevoke_NoIdentitySkipsGuard(t *testing.T) {
	repo := &ownerStubRepo{key: &model.ApiKey{ID: 5, UserID: 1}}
	svc := newOwnerSvc(repo)
	if err := svc.RevokeAs(0, 5); err != nil {
		t.Fatalf("无身份的内部调用应放行，got %v", err)
	}
}

// TestApiKeyGetByIDAs_OwnershipEnforced 读接口（stats）与写接口同一套归属规则。
func TestApiKeyGetByIDAs_OwnershipEnforced(t *testing.T) {
	repo := &ownerStubRepo{key: &model.ApiKey{ID: 5, UserID: 1, Name: "prod-integration"}}
	svc := newOwnerSvc(repo)

	// 归属者可以读
	if _, err := svc.GetByIDAs(1, 5); err != nil {
		t.Fatalf("归属者应可读取，got %v", err)
	}
	// 他人不可读
	if _, err := svc.GetByIDAs(2, 5); !errors.Is(err, ErrApiKeyForbidden) {
		t.Fatalf("他人应被拒，got %v", err)
	}
	// 超管可读
	superSvc := newOwnerSvc(repo, 99)
	if _, err := superSvc.GetByIDAs(99, 5); err != nil {
		t.Fatalf("超管应可读取，got %v", err)
	}
}

// TestApiKeyGuard_NilSuperAdminResolverIsStrict 未注入超管解析器时必须从严：
// 只允许归属者，而不是"没人可验证，那就放行"。
func TestApiKeyGuard_NilSuperAdminResolverIsStrict(t *testing.T) {
	repo := &ownerStubRepo{key: &model.ApiKey{ID: 5, UserID: 1}}
	svc := NewApiKeyService(repo) // 未注入 isSuperAdmin
	if err := svc.RevokeAs(2, 5); !errors.Is(err, ErrApiKeyForbidden) {
		t.Fatalf("未注入超管解析器时应从严拒绝，got %v", err)
	}
}
