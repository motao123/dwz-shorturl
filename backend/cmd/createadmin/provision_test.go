package main

import (
	"errors"
	"testing"

	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// fakeUserRepo is an in-memory stand-in for repository.UserRepo, so the
// provisioning rules can be tested without a database.
type fakeUserRepo struct {
	users    map[string]*model.User
	roles    map[uint64][]uint64
	nextID   uint64
	lastHash string
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{users: map[string]*model.User{}, roles: map[uint64][]uint64{}, nextID: 1}
}

func (f *fakeUserRepo) FindByUsername(username string) (*model.User, error) {
	if u, ok := f.users[username]; ok {
		return u, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeUserRepo) FindByEmail(email string) (*model.User, error) {
	for _, u := range f.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeUserRepo) FindByID(id uint64) (*model.User, error) {
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeUserRepo) Create(user *model.User) error {
	user.ID = f.nextID
	f.nextID++
	f.users[user.Username] = user
	f.lastHash = user.PasswordHash
	return nil
}

func (f *fakeUserRepo) Update(user *model.User) error {
	f.users[user.Username] = user
	f.lastHash = user.PasswordHash
	return nil
}

func (f *fakeUserRepo) List(page, perPage int, keyword string) ([]model.User, int64, error) {
	return nil, 0, nil
}

func (f *fakeUserRepo) UpdateLastLogin(id uint64, ip string) error { return nil }

func (f *fakeUserRepo) UpdatePassword(id uint64, passwordHash string) error {
	for _, u := range f.users {
		if u.ID == id {
			u.PasswordHash = passwordHash
			f.lastHash = passwordHash
			return nil
		}
	}
	return errors.New("user not found")
}

func (f *fakeUserRepo) SoftDelete(id uint64) error { return nil }

func (f *fakeUserRepo) GetRoles(userID uint64) ([]model.Role, error) { return nil, nil }

func (f *fakeUserRepo) SetRoles(userID uint64, roleIDs []uint64) error {
	f.roles[userID] = roleIDs
	return nil
}

// fakeRoleRepo resolves the super_admin role without a database.
type fakeRoleRepo struct {
	superAdmin *model.Role
	missing    bool
}

func (f *fakeRoleRepo) FindAll() ([]model.Role, error) { return nil, nil }
func (f *fakeRoleRepo) FindByID(id uint64) (*model.Role, error) {
	return f.superAdmin, nil
}
func (f *fakeRoleRepo) FindByName(name string) (*model.Role, error) {
	if f.missing || name != "super_admin" {
		return nil, gorm.ErrRecordNotFound
	}
	return f.superAdmin, nil
}
func (f *fakeRoleRepo) Create(role *model.Role) error                                { return nil }
func (f *fakeRoleRepo) Update(role *model.Role) error                                { return nil }
func (f *fakeRoleRepo) Delete(id uint64) error                                       { return nil }
func (f *fakeRoleRepo) GetPermissions(roleID uint64) ([]model.Permission, error)     { return nil, nil }
func (f *fakeRoleRepo) SetPermissions(roleID uint64, permIDs []uint64) error         { return nil }
func (f *fakeRoleRepo) GetUserPermissions(userID uint64) ([]model.Permission, error) { return nil, nil }
func (f *fakeRoleRepo) FindAllPermissions() ([]model.Permission, error)              { return nil, nil }

func newTestProvisioner() (*provisioner, *fakeUserRepo) {
	users := newFakeUserRepo()
	roles := &fakeRoleRepo{superAdmin: &model.Role{ID: 1, Name: "super_admin"}}
	return newAdminProvisioner(users, roles), users
}

// A fresh database must get an account whose hash matches the supplied password,
// and that account must end up with super_admin.
func TestProvisionCreatesAdminWithGivenPassword(t *testing.T) {
	p, users := newTestProvisioner()

	action, err := p.provision("admin", "admin@example.com", "CorrectHorse123", "系统管理员", false)
	if err != nil {
		t.Fatalf("provision failed: %v", err)
	}
	if action != "创建" {
		t.Fatalf("expected 创建, got %q", action)
	}

	u := users.users["admin"]
	if u == nil {
		t.Fatal("admin user was not created")
	}
	if !pkg.CheckPassword(u.PasswordHash, "CorrectHorse123") {
		t.Fatal("stored hash does not match the supplied password")
	}
	if got := users.roles[u.ID]; len(got) != 1 || got[0] != 1 {
		t.Fatalf("super_admin role not granted, got %v", got)
	}
}

// Without -reset the existing password must be left untouched, so running the
// installer again on a live deployment cannot silently change someone's login.
func TestProvisionSkipsExistingWithoutReset(t *testing.T) {
	p, users := newTestProvisioner()
	if _, err := p.provision("admin", "admin@example.com", "OriginalPass1", "", false); err != nil {
		t.Fatal(err)
	}
	before := users.users["admin"].PasswordHash

	action, err := p.provision("admin", "admin@example.com", "DifferentPass2", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if action != "跳过（已存在，未改动密码）" {
		t.Fatalf("expected skip, got %q", action)
	}
	if users.users["admin"].PasswordHash != before {
		t.Fatal("password changed even though -reset was not set")
	}
	if !pkg.CheckPassword(before, "OriginalPass1") {
		t.Fatal("original password no longer verifies")
	}
}

// -reset is the supported recovery path when the administrator password is lost.
func TestProvisionResetsExistingWithReset(t *testing.T) {
	p, users := newTestProvisioner()
	if _, err := p.provision("admin", "admin@example.com", "OriginalPass1", "", false); err != nil {
		t.Fatal(err)
	}

	action, err := p.provision("admin", "admin@example.com", "RecoveredPass9", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if action != "重置" {
		t.Fatalf("expected 重置, got %q", action)
	}
	if !pkg.CheckPassword(users.users["admin"].PasswordHash, "RecoveredPass9") {
		t.Fatal("new password does not verify after reset")
	}
	if pkg.CheckPassword(users.users["admin"].PasswordHash, "OriginalPass1") {
		t.Fatal("old password still verifies after reset")
	}
}

// A missing super_admin role must be a hard error: reporting success would leave
// an administrator who cannot do anything.
func TestProvisionFailsWhenRoleSeedMissing(t *testing.T) {
	users := newFakeUserRepo()
	roles := &fakeRoleRepo{missing: true}
	p := newAdminProvisioner(users, roles)

	if _, err := p.provision("admin", "admin@example.com", "SomePassword1", "", false); err == nil {
		t.Fatal("expected an error when the super_admin role is missing")
	}
}

// every provisioning path must store a bcrypt hash, never the plaintext.
func TestProvisionNeverStoresPlaintext(t *testing.T) {
	p, users := newTestProvisioner()
	const plain = "PlaintextShouldNotAppear1"
	if _, err := p.provision("admin", "admin@example.com", plain, "", false); err != nil {
		t.Fatal(err)
	}
	u := users.users["admin"]
	if u.PasswordHash == plain {
		t.Fatal("password was stored in plaintext")
	}
	if _, err := bcryptCost(u.PasswordHash); err != nil {
		t.Fatalf("stored hash is not valid bcrypt: %v", err)
	}
}

// bcryptCost parses a stored hash and returns its cost, so the test can assert
// the value really is a bcrypt hash rather than some other encoding.
func bcryptCost(hash string) (int, error) {
	return bcrypt.Cost([]byte(hash))
}
