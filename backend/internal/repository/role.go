package repository

import (
	"dwz-admin/internal/model"

	"gorm.io/gorm"
)

type RoleRepo interface {
	FindAll() ([]model.Role, error)
	FindByID(id uint64) (*model.Role, error)
	FindByName(name string) (*model.Role, error)
	Create(role *model.Role) error
	Update(role *model.Role) error
	Delete(id uint64) error
	GetPermissions(roleID uint64) ([]model.Permission, error)
	SetPermissions(roleID uint64, permIDs []uint64) error
	GetUserPermissions(userID uint64) ([]model.Permission, error)
	FindAllPermissions() ([]model.Permission, error)
}

type roleRepo struct {
	db *gorm.DB
}

func NewRoleRepo(db *gorm.DB) RoleRepo {
	return &roleRepo{db: db}
}

func (r *roleRepo) FindAll() ([]model.Role, error) {
	var roles []model.Role
	err := r.db.Order("id ASC").Find(&roles).Error
	return roles, err
}

func (r *roleRepo) FindByID(id uint64) (*model.Role, error) {
	var role model.Role
	err := r.db.First(&role, id).Error
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepo) FindByName(name string) (*model.Role, error) {
	var role model.Role
	err := r.db.Where("name = ?", name).First(&role).Error
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepo) Create(role *model.Role) error {
	return r.db.Create(role).Error
}

func (r *roleRepo) Update(role *model.Role) error {
	return r.db.Save(role).Error
}

func (r *roleRepo) Delete(id uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", id).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", id).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Role{}, id).Error
	})
}

func (r *roleRepo) GetPermissions(roleID uint64) ([]model.Permission, error) {
	var perms []model.Permission
	err := r.db.
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", roleID).
		Find(&perms).Error
	return perms, err
}

func (r *roleRepo) SetPermissions(roleID uint64, permIDs []uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", roleID).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}
		if len(permIDs) == 0 {
			return nil
		}
		var rps []model.RolePermission
		for _, pid := range permIDs {
			rps = append(rps, model.RolePermission{RoleID: roleID, PermissionID: pid})
		}
		return tx.Create(&rps).Error
	})
}

func (r *roleRepo) GetUserPermissions(userID uint64) ([]model.Permission, error) {
	var perms []model.Permission
	err := r.db.
		Joins("JOIN user_roles ON user_roles.role_id = role_permissions.role_id").
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("user_roles.user_id = ?", userID).
		Distinct().
		Find(&perms).Error
	return perms, err
}

func (r *roleRepo) FindAllPermissions() ([]model.Permission, error) {
	var perms []model.Permission
	err := r.db.Order("resource, action").Find(&perms).Error
	return perms, err
}
