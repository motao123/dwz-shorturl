package service

import (
	"errors"

	"dwz-admin/internal/model"
	"dwz-admin/internal/repository"
)

var (
	ErrRoleNotFound     = errors.New("role not found")
	ErrSystemRoleDelete = errors.New("cannot delete system role")
	// ErrSystemRoleEdit refuses permission edits on built-in roles. Without it a
	// non-super administrator could rewrite `super_admin` itself (or grant a
	// stronger set to its own role), which is a one-step self-escalation (#3).
	ErrSystemRoleEdit = errors.New("系统内置角色的权限不可修改")
	// ErrPermissionEscalation refuses granting permissions the actor lacks.
	ErrPermissionEscalation = errors.New("不能授予自身不具备的权限")
)

type RoleService interface {
	Create(name, displayName, description string) (*model.Role, error)
	Update(id uint64, displayName, description string) (*model.Role, error)
	Delete(id uint64) error
	GetByID(id uint64) (*model.Role, error)
	GetAll() ([]model.Role, error)
	SetPermissions(roleID uint64, permIDs []uint64) error
	// SetPermissionsAs is SetPermissions guarded by the acting administrator's
	// own permission set (#3).
	SetPermissionsAs(actorID, roleID uint64, permIDs []uint64) error
	GetPermissions(roleID uint64) ([]model.Permission, error)
	GetAllPermissions() ([]model.Permission, error)
}

type roleService struct {
	roleRepo repository.RoleRepo
}

func NewRoleService(roleRepo repository.RoleRepo) RoleService {
	return &roleService{roleRepo: roleRepo}
}

func (s *roleService) Create(name, displayName, description string) (*model.Role, error) {
	existing, err := s.roleRepo.FindByName(name)
	if err == nil && existing != nil {
		return nil, errors.New("role name already exists")
	}

	role := &model.Role{
		Name:        name,
		DisplayName: displayName,
		Description: description,
		IsSystem:    0,
	}

	if err := s.roleRepo.Create(role); err != nil {
		return nil, err
	}

	return role, nil
}

func (s *roleService) Update(id uint64, displayName, description string) (*model.Role, error) {
	role, err := s.roleRepo.FindByID(id)
	if err != nil {
		return nil, ErrRoleNotFound
	}

	if displayName != "" {
		role.DisplayName = displayName
	}
	role.Description = description

	if err := s.roleRepo.Update(role); err != nil {
		return nil, err
	}

	return role, nil
}

func (s *roleService) Delete(id uint64) error {
	role, err := s.roleRepo.FindByID(id)
	if err != nil {
		return ErrRoleNotFound
	}

	if role.IsSystem == 1 {
		return ErrSystemRoleDelete
	}

	return s.roleRepo.Delete(id)
}

func (s *roleService) GetByID(id uint64) (*model.Role, error) {
	return s.roleRepo.FindByID(id)
}

func (s *roleService) GetAll() ([]model.Role, error) {
	roles, err := s.roleRepo.FindAll()
	if err != nil {
		return nil, err
	}
	for i := range roles {
		perms, pErr := s.roleRepo.GetPermissions(roles[i].ID)
		if pErr != nil {
			return nil, pErr
		}
		codes := make([]string, 0, len(perms))
		for _, p := range perms {
			codes = append(codes, p.Resource+"."+p.Action)
		}
		roles[i].Permissions = codes
	}
	return roles, nil
}

func (s *roleService) SetPermissions(roleID uint64, permIDs []uint64) error {
	_, err := s.roleRepo.FindByID(roleID)
	if err != nil {
		return ErrRoleNotFound
	}
	return s.roleRepo.SetPermissions(roleID, permIDs)
}

// SetPermissionsAs enforces two rules before writing a role's permissions:
//
//  1. built-in (is_system) roles are immutable — otherwise a non-super admin
//     could add every permission to `super_admin`, or strip it, or grant itself
//     a stronger set through the `admin` role it holds;
//  2. the granted set must be a subset of the actor's own effective permissions.
//
// actorID == 0 means an internal/CLI caller; guards are lifted then, matching
// the convention used elsewhere in this package.
func (s *roleService) SetPermissionsAs(actorID, roleID uint64, permIDs []uint64) error {
	if actorID == 0 {
		return s.SetPermissions(roleID, permIDs)
	}

	role, err := s.roleRepo.FindByID(roleID)
	if err != nil {
		return ErrRoleNotFound
	}
	// A super_admin may edit any role; everyone else is confined to non-system
	// roles they could already manage.
	actorPerms, err := s.roleRepo.GetUserPermissions(actorID)
	if err != nil {
		return err
	}
	// super_admin short-circuits the permission lookup in repository/role.go, so
	// detect it by name rather than by permission count.
	actorIsSuper := false
	roles, err := s.roleRepo.RolesOfUser(actorID)
	if err != nil {
		return err
	}
	for _, r := range roles {
		if r.Name == "super_admin" {
			actorIsSuper = true
			break
		}
	}

	if role.IsSystem == 1 && !actorIsSuper {
		return ErrSystemRoleEdit
	}
	if !actorIsSuper {
		owned := map[uint64]struct{}{}
		for _, p := range actorPerms {
			owned[p.ID] = struct{}{}
		}
		for _, pid := range permIDs {
			if _, ok := owned[pid]; !ok {
				return ErrPermissionEscalation
			}
		}
	}
	return s.roleRepo.SetPermissions(roleID, permIDs)
}

func (s *roleService) GetPermissions(roleID uint64) ([]model.Permission, error) {
	return s.roleRepo.GetPermissions(roleID)
}

func (s *roleService) GetAllPermissions() ([]model.Permission, error) {
	return s.roleRepo.FindAllPermissions()
}
