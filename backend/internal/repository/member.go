package repository

import (
	"time"

	"dwz-admin/internal/model"

	"gorm.io/gorm"
)

// MemberRepo reads/writes the public frontend `members` table (a separate DB
// from the admin `dwz_admin` database).
type MemberRepo interface {
	FindByID(id uint64) (*model.Member, error)
	FindByEmail(email string) (*model.Member, error)
	FindByResetToken(token string) (*model.Member, error)
	SetResetToken(id uint64, token string, expiresAt *time.Time) error
	BumpTokenVersion(id uint64) error
	SetVerifyToken(id uint64, token string, expiresAt *time.Time) error
	FindByVerifyToken(token string) (*model.Member, error)
	MarkVerified(id uint64) error
	List(page, perPage int, keyword string, status *int8) ([]model.Member, int64, error)
	UpdateStatus(id uint64, status int8) error
	ResetPassword(id uint64, hash string) error
	Delete(id uint64) error
}

type memberRepo struct {
	db *gorm.DB
}

func NewMemberRepo(db *gorm.DB) MemberRepo {
	return &memberRepo{db: db}
}

func (r *memberRepo) FindByID(id uint64) (*model.Member, error) {
	if r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var m model.Member
	err := r.db.First(&m, id).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *memberRepo) FindByEmail(email string) (*model.Member, error) {
	if r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var m model.Member
	err := r.db.Where("email = ?", email).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *memberRepo) FindByResetToken(token string) (*model.Member, error) {
	if r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var m model.Member
	err := r.db.Where("reset_token = ?", token).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *memberRepo) SetResetToken(id uint64, token string, expiresAt *time.Time) error {
	if r.db == nil {
		return nil
	}
	return r.db.Model(&model.Member{}).Where("id = ?", id).
		Updates(map[string]interface{}{"reset_token": token, "reset_expires_at": expiresAt}).Error
}

func (r *memberRepo) BumpTokenVersion(id uint64) error {
	if r.db == nil {
		return nil
	}
	return r.db.Model(&model.Member{}).Where("id = ?", id).
		UpdateColumn("token_version", gorm.Expr("token_version + 1")).Error
}

func (r *memberRepo) SetVerifyToken(id uint64, token string, expiresAt *time.Time) error {
	if r.db == nil {
		return nil
	}
	return r.db.Model(&model.Member{}).Where("id = ?", id).
		Updates(map[string]interface{}{"verify_token": token, "verify_expires_at": expiresAt}).Error
}

func (r *memberRepo) FindByVerifyToken(token string) (*model.Member, error) {
	if r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var m model.Member
	err := r.db.Where("verify_token = ?", token).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *memberRepo) MarkVerified(id uint64) error {
	if r.db == nil {
		return nil
	}
	return r.db.Model(&model.Member{}).Where("id = ?", id).
		Updates(map[string]interface{}{"email_verified": 1, "verify_token": "", "verify_expires_at": nil}).Error
}

func (r *memberRepo) List(page, perPage int, keyword string, status *int8) ([]model.Member, int64, error) {
	var members []model.Member
	var total int64
	if r.db == nil {
		return members, total, nil
	}

	query := r.db.Model(&model.Member{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("username LIKE ? OR email LIKE ?", like, like)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Offset(offset).Limit(perPage).Order("id DESC").Find(&members).Error; err != nil {
		return nil, 0, err
	}

	return members, total, nil
}

func (r *memberRepo) UpdateStatus(id uint64, status int8) error {
	return r.db.Model(&model.Member{}).Where("id = ?", id).Update("status", status).Error
}

func (r *memberRepo) ResetPassword(id uint64, hash string) error {
	return r.db.Model(&model.Member{}).Where("id = ?", id).Update("password_hash", hash).Error
}

func (r *memberRepo) Delete(id uint64) error {
	return r.db.Delete(&model.Member{}, id).Error
}