package repository

import (
	"time"

	"dwz-admin/internal/model"

	"gorm.io/gorm"
)

type UserRepo interface {
	FindByUsername(username string) (*model.User, error)
	FindByEmail(email string) (*model.User, error)
	FindByID(id uint64) (*model.User, error)
	Create(user *model.User) error
	Update(user *model.User) error
	List(page, perPage int, keyword string) ([]model.User, int64, error)
	UpdateLastLogin(id uint64, ip string) error
	UpdatePassword(id uint64, passwordHash string) error
	SoftDelete(id uint64) error
	GetRoles(userID uint64) ([]model.Role, error)
	SetRoles(userID uint64, roleIDs []uint64) error
	// LoadRoles fills RoleIDs/RoleNames on the given users from user_roles in a
	// single query (no N+1). Used by List/GetByID so the API returns the current
	// role assignment instead of an empty one (#1).
	LoadRoles(users ...*model.User) error
	// FindRoleByID resolves a role for privilege-grant checks.
	FindRoleByID(id uint64) (*model.Role, error)
	// GetPermissions lists a role's permissions (used to compare account strength).
	GetPermissions(roleID uint64) ([]model.Permission, error)
	// RemoveAllRoles strips every role from a user. Deliberately a separate method
	// from SetRoles: clearing a role set is a destructive act and the ordinary
	// assign path must not be able to do it by accident (#1).
	RemoveAllRoles(userID uint64) error
}

type userRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) UserRepo {
	return &userRepo{db: db}
}

func (r *userRepo) FindByUsername(username string) (*model.User, error) {
	var user model.User
	err := r.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepo) FindByEmail(email string) (*model.User, error) {
	var user model.User
	err := r.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepo) FindByID(id uint64) (*model.User, error) {
	var user model.User
	err := r.db.First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepo) Create(user *model.User) error {
	return r.db.Create(user).Error
}

func (r *userRepo) Update(user *model.User) error {
	return r.db.Save(user).Error
}

func (r *userRepo) List(page, perPage int, keyword string) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	query := r.db.Model(&model.User{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("username LIKE ? OR email LIKE ? OR display_name LIKE ?", like, like, like)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Offset(offset).Limit(perPage).Order("id DESC").Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (r *userRepo) UpdateLastLogin(id uint64, ip string) error {
	now := time.Now()
	return r.db.Model(&model.User{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_login_at": now,
		"last_login_ip": ip,
	}).Error
}

// UpdatePassword writes only the password hash, so a password reset cannot
// clobber concurrent edits to the rest of the row (Save would write every
// column it holds in memory).
func (r *userRepo) UpdatePassword(id uint64, passwordHash string) error {
	return r.db.Model(&model.User{}).Where("id = ?", id).
		Update("password_hash", passwordHash).Error
}

// SoftDelete soft-deletes a user and its role bindings so user_roles never
// keeps orphan rows pointing at removed accounts.
func (r *userRepo) SoftDelete(id uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&model.User{}, id).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", id).Delete(&model.UserRole{}).Error
	})
}

func (r *userRepo) GetRoles(userID uint64) ([]model.Role, error) {
	var roles []model.Role
	err := r.db.
		Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id = ?", userID).
		Find(&roles).Error
	if err != nil {
		return nil, err
	}
	return roles, nil
}

// LoadRoles batch-loads role assignments for the given users. A single query is
// used regardless of how many users are passed: List pages can hold 100 rows and
// the previous per-row lookup would have been an N+1.
func (r *userRepo) LoadRoles(users ...*model.User) error {
	if len(users) == 0 {
		return nil
	}
	ids := make([]uint64, 0, len(users))
	for _, u := range users {
		if u != nil {
			ids = append(ids, u.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	type row struct {
		UserID uint64
		ID     uint64
		Name   string
	}
	var rows []row
	if err := r.db.Table("user_roles").
		Select("user_roles.user_id AS user_id, roles.id AS id, roles.name AS name").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return err
	}

	byUser := map[uint64][]row{}
	for _, x := range rows {
		byUser[x.UserID] = append(byUser[x.UserID], x)
	}
	for _, u := range users {
		if u == nil {
			continue
		}
		rs := byUser[u.ID]
		u.RoleIDs = make([]uint64, 0, len(rs))
		u.RoleNames = make([]string, 0, len(rs))
		for _, x := range rs {
			u.RoleIDs = append(u.RoleIDs, x.ID)
			u.RoleNames = append(u.RoleNames, x.Name)
		}
	}
	return nil
}

func (r *userRepo) FindRoleByID(id uint64) (*model.Role, error) {
	var role model.Role
	if err := r.db.First(&role, id).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *userRepo) GetPermissions(roleID uint64) ([]model.Permission, error) {
	var perms []model.Permission
	err := r.db.
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", roleID).
		Find(&perms).Error
	return perms, err
}

func (r *userRepo) RemoveAllRoles(userID uint64) error {
	return r.db.Where("user_id = ?", userID).Delete(&model.UserRole{}).Error
}

func (r *userRepo) SetRoles(userID uint64, roleIDs []uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		if len(roleIDs) == 0 {
			return nil
		}
		var userRoles []model.UserRole
		for _, rid := range roleIDs {
			userRoles = append(userRoles, model.UserRole{UserID: userID, RoleID: rid})
		}
		return tx.Create(&userRoles).Error
	})
}
