package service

import (
	"errors"

	"dwz-admin/internal/model"
	"dwz-admin/internal/repository"
)

var (
	ErrRoleNotFound     = errors.New("role not found")
	ErrSystemRoleDelete = errors.New("cannot delete system role")
)

type RoleService interface {
	Create(name, displayName, description string) (*model.Role, error)
	Update(id uint64, displayName, description string) (*model.Role, error)
	Delete(id uint64) error
	GetByID(id uint64) (*model.Role, error)
	GetAll() ([]model.Role, error)
	SetPermissions(roleID uint64, permIDs []uint64) error
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
	return s.roleRepo.FindAll()
}

func (s *roleService) SetPermissions(roleID uint64, permIDs []uint64) error {
	_, err := s.roleRepo.FindByID(roleID)
	if err != nil {
		return ErrRoleNotFound
	}
	return s.roleRepo.SetPermissions(roleID, permIDs)
}

func (s *roleService) GetPermissions(roleID uint64) ([]model.Permission, error) {
	return s.roleRepo.GetPermissions(roleID)
}

func (s *roleService) GetAllPermissions() ([]model.Permission, error) {
	return s.roleRepo.FindAllPermissions()
}
