package service

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"dwz-admin/internal/config"
	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"
)

// TestMain initialises the config singleton so token generation (which reads
// jwt.secret) works. config.Init bails out when the file is missing, leaving
// the singleton nil, so a minimal real file is written to a temp dir.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "dwz-auth-test")
	if err != nil {
		panic(err)
	}
	path := dir + "/config.yaml"
	body := "jwt:\n  secret: test-secret-for-unit-tests-only\n  access_expiry: 2h\n  refresh_expiry: 168h\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		panic(err)
	}
	if err := config.Init(path); err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// fakeUserRepo is the minimum UserRepo surface the login path touches.
type fakeUserRepo struct {
	user  *model.User
	roles []model.Role
}

func (f *fakeUserRepo) FindByUsername(string) (*model.User, error) {
	if f.user == nil {
		return nil, errors.New("not found")
	}
	return f.user, nil
}
func (f *fakeUserRepo) FindByEmail(string) (*model.User, error) { return nil, errors.New("not found") }
func (f *fakeUserRepo) FindByID(uint64) (*model.User, error)     { return f.user, nil }
func (f *fakeUserRepo) Create(*model.User) error                 { return nil }
func (f *fakeUserRepo) Update(*model.User) error                 { return nil }
func (f *fakeUserRepo) List(int, int, string) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (f *fakeUserRepo) UpdateLastLogin(uint64, string) error    { return nil }
func (f *fakeUserRepo) UpdatePassword(uint64, string) error     { return nil }
func (f *fakeUserRepo) SoftDelete(uint64) error                 { return nil }
func (f *fakeUserRepo) GetRoles(uint64) ([]model.Role, error)   { return f.roles, nil }
func (f *fakeUserRepo) SetRoles(uint64, []uint64) error         { return nil }
func (f *fakeUserRepo) FindRoleByID(uint64) (*model.Role, error) {
	return nil, errors.New("not found")
}
func (f *fakeUserRepo) LoadRoles(users ...*model.User) error { return nil }
func (f *fakeUserRepo) GetPermissions(uint64) ([]model.Permission, error) { return nil, nil }
func (f *fakeUserRepo) RemoveAllRoles(uint64) error         { return nil }

type fakeRoleRepo struct{}

func (fakeRoleRepo) FindByName(string) (*model.Role, error) { return nil, errors.New("not found") }
func (fakeRoleRepo) FindByID(uint64) (*model.Role, error)   { return nil, errors.New("not found") }
func (fakeRoleRepo) FindAll() ([]model.Role, error)         { return nil, nil }
func (fakeRoleRepo) Create(*model.Role) error               { return nil }
func (fakeRoleRepo) Update(*model.Role) error               { return nil }
func (fakeRoleRepo) Delete(uint64) error                    { return nil }
func (fakeRoleRepo) GetPermissions(uint64) ([]model.Permission, error) {
	return nil, nil
}
func (fakeRoleRepo) SetPermissions(uint64, []uint64) error { return nil }
func (fakeRoleRepo) GetUserPermissions(uint64) ([]model.Permission, error) {
	return nil, nil
}
func (fakeRoleRepo) FindAllPermissions() ([]model.Permission, error) { return nil, nil }
func (fakeRoleRepo) RolesOfUser(uint64) ([]model.Role, error)        { return nil, nil }

// newLockoutLimiter builds a limiter over a fake counter so the lockout logic
// can be exercised without Redis.
func newLockoutLimiter() *pkg.RateLimiter {
	return pkg.NewRateLimiterWithCounter(newFakeCounterForTest())
}

func TestLogin_LocksAfterMaxFailures(t *testing.T) {
	hash, _ := pkg.HashPassword("correct-horse-battery")
	repo := &fakeUserRepo{
		user:  &model.User{ID: 1, Username: "admin", PasswordHash: hash, Status: 1},
		roles: []model.Role{{Name: "super_admin"}},
	}
	svc := NewAuthService(repo, fakeRoleRepo{}).WithLoginLimiter(newLockoutLimiter())

	// Exhaust the budget with wrong passwords.
	for i := 0; i < adminLoginMaxFailures; i++ {
		if _, err := svc.Login("admin", "wrong", "", "1.2.3.4"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: want ErrInvalidCredentials, got %v", i+1, err)
		}
	}

	// Even the correct password is now rejected: the account is locked, and the
	// lock must not be bypassable by getting the password right.
	if _, err := svc.Login("admin", "correct-horse-battery", "", "1.2.3.4"); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("want ErrAccountLocked after %d failures, got %v", adminLoginMaxFailures, err)
	}
}

func TestLogin_SuccessResetsFailureCount(t *testing.T) {
	hash, _ := pkg.HashPassword("correct-horse-battery")
	repo := &fakeUserRepo{
		user:  &model.User{ID: 1, Username: "admin", PasswordHash: hash, Status: 1},
		roles: []model.Role{{Name: "super_admin"}},
	}
	svc := NewAuthService(repo, fakeRoleRepo{}).WithLoginLimiter(newLockoutLimiter())

	// Just under the limit, then succeed: the counter must clear.
	for i := 0; i < adminLoginMaxFailures-1; i++ {
		_, _ = svc.Login("admin", "wrong", "", "1.2.3.4")
	}
	if _, err := svc.Login("admin", "correct-horse-battery", "", "1.2.3.4"); err != nil {
		t.Fatalf("valid login within budget should succeed, got %v", err)
	}
	// Budget is fresh again, so a full new round of failures is still allowed
	// before locking.
	for i := 0; i < adminLoginMaxFailures; i++ {
		if _, err := svc.Login("admin", "wrong", "", "1.2.3.4"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("round 2 attempt %d: want ErrInvalidCredentials, got %v", i+1, err)
		}
	}
	if _, err := svc.Login("admin", "correct-horse-battery", "", "1.2.3.4"); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("want locked after a full second round, got %v", err)
	}
}

func TestLogin_LockoutKeyIsCaseInsensitive(t *testing.T) {
	hash, _ := pkg.HashPassword("pw")
	repo := &fakeUserRepo{
		user:  &model.User{ID: 1, Username: "Admin", PasswordHash: hash, Status: 1},
		roles: []model.Role{{Name: "super_admin"}},
	}
	svc := NewAuthService(repo, fakeRoleRepo{}).WithLoginLimiter(newLockoutLimiter())

	for i := 0; i < adminLoginMaxFailures; i++ {
		_, _ = svc.Login("Admin", "wrong", "", "1.2.3.4")
	}
	// A different casing must share the same budget, not get a fresh one.
	if _, err := svc.Login("admin", "pw", "", "1.2.3.4"); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("casing must not create a second budget, got %v", err)
	}
}

func TestLogin_NoLimiterKeepsLegacyBehaviour(t *testing.T) {
	hash, _ := pkg.HashPassword("pw")
	repo := &fakeUserRepo{
		user:  &model.User{ID: 1, Username: "admin", PasswordHash: hash, Status: 1},
		roles: []model.Role{{Name: "super_admin"}},
	}
	svc := NewAuthService(repo, fakeRoleRepo{}) // no limiter wired

	for i := 0; i < adminLoginMaxFailures*2; i++ {
		if _, err := svc.Login("admin", "wrong", "", "1.2.3.4"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("without a limiter every failure is just bad credentials, got %v", err)
		}
	}
	if _, err := svc.Login("admin", "pw", "", "1.2.3.4"); err != nil {
		t.Fatalf("login must still work without a limiter, got %v", err)
	}
}

// countingCounter exposes only what the limiter needs, mirroring the Redis path.
func newFakeCounterForTest() pkg.Counter { return &memCounter{counts: map[string]int64{}} }

type memCounter struct {
	counts map[string]int64
}

func (m *memCounter) IncrBy(_ context.Context, key string, n int64) (int64, error) {
	m.counts[key] += n
	return m.counts[key], nil
}
func (m *memCounter) Expire(_ context.Context, key string, _ time.Duration) (bool, error) {
	return true, nil
}
func (m *memCounter) Delete(_ context.Context, key string) error {
	delete(m.counts, key)
	return nil
}
func (m *memCounter) Get(_ context.Context, key string) (int64, error) {
	return m.counts[key], nil
}
