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
	SoftDelete(id uint64) error
	GetRoles(userID uint64) ([]model.Role, error)
	SetRoles(userID uint64, roleIDs []uint64) error
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

func (r *userRepo) SoftDelete(id uint64) error {
	return r.db.Delete(&model.User{}, id).Error
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
