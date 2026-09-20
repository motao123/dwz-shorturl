package service

import (
	"errors"
	"testing"

	"dwz-admin/internal/model"
)

// guardRepo is a UserRepo that answers the role/permission questions the
// privilege guards ask. It is keyed by user id so a test can describe two
// accounts ("operator" vs "super_admin") and assert what one may do to the other.
type guardRepo struct {
	rolesOf map[uint64][]model.Role
	permsOf map[uint64][]model.Permission // keyed by role id
}

func (g *guardRepo) FindByUsername(string) (*model.User, error) { return nil, errors.New("nf") }
func (g *guardRepo) FindByEmail(string) (*model.User, error)    { return nil, errors.New("nf") }
func (g *guardRepo) FindByID(id uint64) (*model.User, error) {
	return &model.User{ID: id, Status: 1}, nil
}
func (g *guardRepo) Create(*model.User) error                           { return nil }
func (g *guardRepo) Update(*model.User) error                           { return nil }
func (g *guardRepo) List(int, int, string) ([]model.User, int64, error) { return nil, 0, nil }
func (g *guardRepo) UpdateLastLogin(uint64, string) error               { return nil }
func (g *guardRepo) UpdatePassword(uint64, string) error                { return nil }
func (g *guardRepo) SoftDelete(uint64) error                            { return nil }
func (g *guardRepo) GetRoles(userID uint64) ([]model.Role, error)       { return g.rolesOf[userID], nil }
func (g *guardRepo) SetRoles(uint64, []uint64) error                    { return nil }
func (g *guardRepo) RemoveAllRoles(uint64) error                        { return nil }
func (g *guardRepo) LoadRoles(users ...*model.User) error               { return nil }
func (g *guardRepo) GetPermissions(roleID uint64) ([]model.Permission, error) {
	return g.permsOf[roleID], nil
}
func (g *guardRepo) FindRoleByID(id uint64) (*model.Role, error) {
	for _, rs := range g.rolesOf {
		for _, r := range rs {
			if r.ID == id {
				return &r, nil
			}
		}
	}
	return nil, errors.New("not found")
}

func permSet(ids ...uint64) []model.Permission {
	out := make([]model.Permission, 0, len(ids))
	for _, id := range ids {
		out = append(out, model.Permission{ID: id})
	}
	return out
}

// TestGuard_LowerPrivilegeCannotActOnSuperAdmin is the regression test for the
// P0 escalation chain: a holder of the single `users:update` permission used
// `POST /users/1/totp/enable` (with a secret of their choosing) followed by
// `PUT /users/1/password` to take over the super administrator.
func TestGuard_LowerPrivilegeCannotActOnSuperAdmin(t *testing.T) {
	repo := &guardRepo{
		rolesOf: map[uint64][]model.Role{
			1: {{ID: 1, Name: "super_admin"}},
			2: {{ID: 2, Name: "admin"}},
		},
		permsOf: map[uint64][]model.Permission{
			2: permSet(1, 2, 3), // fewer permissions than super_admin
		},
	}
	svc := NewUserService(repo)

	cases := map[string]func() error{
		"enable totp":    func() error { return svc.EnableTotpAs(2, 1, "123456", "ATTACKERSECRET") },
		"disable totp":   func() error { return svc.DisableTotpAs(2, 1) },
		"reset password": func() error { return svc.ResetPasswordAs(2, 1, "pwned-password") },
		"delete":         func() error { return svc.DeleteAs(2, 1) },
		"assign roles":   func() error { return svc.AssignRolesAs(2, 1, []uint64{1}) },
		"update": func() error {
			_, err := svc.UpdateAs(2, 1, "", "", "", nil)
			return err
		},
	}
	for name, fn := range cases {
		if err := fn(); !errors.Is(err, ErrPrivilegeEscalation) {
			t.Errorf("%s: expected ErrPrivilegeEscalation, got %v", name, err)
		}
	}
}

// TestGuard_SelfActionRejected covers the "edit my own account to grant myself
// roles" path, which is the cheap half of the escalation chain.
func TestGuard_SelfActionRejected(t *testing.T) {
	repo := &guardRepo{rolesOf: map[uint64][]model.Role{2: {{ID: 2, Name: "admin"}}}}
	svc := NewUserService(repo)

	if err := svc.AssignRolesAs(2, 2, []uint64{1}); !errors.Is(err, ErrPrivilegeEscalation) {
		t.Errorf("self assign roles: expected ErrPrivilegeEscalation, got %v", err)
	}
	if err := svc.ResetPasswordAs(2, 2, "newpass123"); !errors.Is(err, ErrPrivilegeEscalation) {
		t.Errorf("self reset password: expected ErrPrivilegeEscalation, got %v", err)
	}
}

// TestGuard_NonSuperCannotGrantSuperAdmin is the second escalation entry point:
// create a role, grant it everything, assign it to yourself.
func TestGuard_NonSuperCannotGrantSuperAdmin(t *testing.T) {
	repo := &guardRepo{
		rolesOf: map[uint64][]model.Role{
			1: {{ID: 1, Name: "super_admin"}},
			3: {{ID: 5, Name: "operator"}},
		},
		permsOf: map[uint64][]model.Permission{5: permSet(1, 2)},
	}
	svc := NewUserService(repo)

	err := svc.AssignRolesAs(3, 4, []uint64{1})
	if !errors.Is(err, ErrPrivilegeEscalation) && !errors.Is(err, ErrSuperAdminOnly) {
		t.Errorf("expected a refusal, got %v", err)
	}
}

// TestGuard_SuperAdminMayActOnLowerAccount keeps the guard from breaking normal
// administration: a super_admin must still be able to manage everyone else.
func TestGuard_SuperAdminMayActOnLowerAccount(t *testing.T) {
	repo := &guardRepo{
		rolesOf: map[uint64][]model.Role{
			1: {{ID: 1, Name: "super_admin"}},
			2: {{ID: 2, Name: "viewer"}},
		},
		permsOf: map[uint64][]model.Permission{2: permSet(1)},
	}
	svc := NewUserService(repo)

	if err := svc.ResetPasswordAs(1, 2, "newpass123"); err != nil {
		t.Errorf("super_admin resetting a weaker account should succeed, got %v", err)
	}
	if err := svc.DisableTotpAs(1, 2); err != nil {
		t.Errorf("super_admin disabling 2FA on a weaker account should succeed, got %v", err)
	}
}

// TestAssignRoles_EmptySetRejected is the direct regression for #1: gin's
// binding:"required" does not reject an empty slice, so `{"role_ids":[]}`
// reached the repository, which DELETEd every role and inserted nothing.
func TestAssignRoles_EmptySetRejected(t *testing.T) {
	repo := &guardRepo{rolesOf: map[uint64][]model.Role{4: {{ID: 2, Name: "admin"}}}}
	svc := NewUserService(repo)

	if err := svc.AssignRoles(4, []uint64{}); !errors.Is(err, ErrEmptyRoles) {
		t.Fatalf("expected ErrEmptyRoles for an empty role set, got %v", err)
	}
	if err := svc.AssignRoles(4, nil); !errors.Is(err, ErrEmptyRoles) {
		t.Fatalf("expected ErrEmptyRoles for a nil role set, got %v", err)
	}
}
