package repository

import (
	"time"

	"gorm.io/gorm"
)

// WjoyLogRepo writes to the public PHP-facing `wjoy_log` table (in the public
// DB). This keeps short links created through the Go backend redirectable via
// the PHP do.php path that reads wjoy_log.
type WjoyLogRepo interface {
	Create(uid, longURL, hash string, expireAt *time.Time, passwordHash string) error
	UpdateExpiry(uid string, expireAt *time.Time) error
	Update(uid, longURL, hash string, expireAt *time.Time, passwordHash *string) error
	SetStatus(uid string, status int8) error
}

type wjoyLogRepo struct {
	db *gorm.DB
}

func NewWjoyLogRepo(db *gorm.DB) WjoyLogRepo {
	return &wjoyLogRepo{db: db}
}

func (r *wjoyLogRepo) Create(uid, longURL, hash string, expireAt *time.Time, passwordHash string) error {
	if r.db == nil {
		return nil
	}
	row := map[string]interface{}{
		"uid":           uid,
		"longurl":       longURL,
		"url_hash":      hash,
		"expire_at":     expireAt,
		"password_hash": nullableString(passwordHash),
	}
	return r.db.Table("wjoy_log").Create(row).Error
}

func (r *wjoyLogRepo) UpdateExpiry(uid string, expireAt *time.Time) error {
	if r.db == nil {
		return nil
	}
	return r.db.Table("wjoy_log").Where("uid = ?", uid).Update("expire_at", expireAt).Error
}

// Update syncs the public wjoy_log row after an admin edit. longURL may be empty
// to only touch expire_at; expireAt nil clears the expiry; passwordHash nil
// leaves the password untouched, "" clears it, any other value sets it.
func (r *wjoyLogRepo) Update(uid, longURL, hash string, expireAt *time.Time, passwordHash *string) error {
	if r.db == nil {
		return nil
	}
	updates := map[string]interface{}{"expire_at": expireAt}
	if longURL != "" {
		updates["longurl"] = longURL
		updates["url_hash"] = hash
	}
	if passwordHash != nil {
		updates["password_hash"] = nullableString(*passwordHash)
	}
	return r.db.Table("wjoy_log").Where("uid = ?", uid).Updates(updates).Error
}

// SetStatus enables/disables a wjoy_log row so the PHP do.php path stops serving
// soft-deleted links. 1 = active, 0 = disabled.
func (r *wjoyLogRepo) SetStatus(uid string, status int8) error {
	if r.db == nil {
		return nil
	}
	return r.db.Table("wjoy_log").Where("uid = ?", uid).Update("status", status).Error
}

// nullableString converts an empty string to nil so empty values are stored as
// NULL (open link) rather than as an empty/whitespace password hash.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}