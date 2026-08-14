package model

import (
	"time"

	"gorm.io/gorm"
)

// User represents an admin user account.
type User struct {
	ID           uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string         `gorm:"size:32;uniqueIndex:uk_username;not null" json:"username"`
	Email        string         `gorm:"size:128;uniqueIndex:uk_email;not null" json:"email"`
	PasswordHash string         `gorm:"size:255;not null" json:"-"`
	DisplayName  string         `gorm:"size:64" json:"display_name"`
	AvatarURL    string         `gorm:"size:512" json:"avatar_url"`
	Status       int8           `gorm:"default:1;not null" json:"status"`
	// TotpSecret is the base32 TOTP shared secret; non-empty = 2FA enabled.
	// Never serialised. TotpEnabled is computed for the UI.
	TotpSecret  string         `gorm:"size:64" json:"-"`
	TotpEnabled bool           `gorm:"-" json:"totp_enabled"`
	LastLoginAt *time.Time     `json:"last_login_at"`
	LastLoginIP string         `gorm:"size:45" json:"last_login_ip"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index:idx_deleted_at" json:"-"`
}

func (User) TableName() string { return "users" }

// Role represents a permission role.
type Role struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"size:32;uniqueIndex:uk_name;not null" json:"name"`
	DisplayName string    `gorm:"size:64;not null" json:"display_name"`
	Description string    `gorm:"size:255" json:"description"`
	IsSystem    int8      `gorm:"default:0;not null" json:"is_system"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Role) TableName() string { return "roles" }

// Permission represents an action on a resource.
type Permission struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	Resource    string `gorm:"size:64;not null;uniqueIndex:uk_resource_action" json:"resource"`
	Action      string `gorm:"size:32;not null;uniqueIndex:uk_resource_action" json:"action"`
	Description string `gorm:"size:255" json:"description"`
}

func (Permission) TableName() string { return "permissions" }

// RolePermission is the join table between roles and permissions.
type RolePermission struct {
	RoleID       uint64 `gorm:"primaryKey" json:"role_id"`
	PermissionID uint64 `gorm:"primaryKey" json:"permission_id"`
}

func (RolePermission) TableName() string { return "role_permissions" }

// UserRole is the join table between users and roles.
type UserRole struct {
	UserID uint64 `gorm:"primaryKey" json:"user_id"`
	RoleID uint64 `gorm:"primaryKey" json:"role_id"`
}

func (UserRole) TableName() string { return "user_roles" }

// Member is a public-facing registered user (lives in the public frontend DB,
// separate from the admin `users` table).
type Member struct {
	ID           uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string     `gorm:"size:32;uniqueIndex:uk_username;not null" json:"username"`
	Email        string     `gorm:"size:128;uniqueIndex:uk_email;not null" json:"email"`
	PasswordHash string     `gorm:"size:255;not null" json:"-"`
	Status       int8       `gorm:"default:1;not null" json:"status"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	LastLoginIP  string     `gorm:"size:45" json:"last_login_ip"`
	TokenVersion int        `gorm:"default:0;not null" json:"token_version"`
	EmailVerified int8      `gorm:"default:0;not null" json:"email_verified"`
	VerifyToken  string     `gorm:"size:64" json:"-"`
	VerifyExpiresAt *time.Time `json:"-"`
	ResetToken   string     `gorm:"size:64" json:"-"`
	ResetExpiresAt *time.Time `json:"-"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (Member) TableName() string { return "members" }

// ViolationReview records a blocked URL submission for manual review. Lives in
// the public frontend DB alongside members.
type ViolationReview struct {
	ID         uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	URL        string     `gorm:"type:text;not null" json:"url"`
	Reason     string     `gorm:"size:64;default:''" json:"reason"`
	IP         string     `gorm:"size:45" json:"ip"`
	Source     string     `gorm:"size:16;default:api" json:"source"`
	Reviewed   int8       `gorm:"default:0;not null" json:"reviewed"`
	ReviewedAt *time.Time `json:"reviewed_at"`
	Note       string     `gorm:"size:255" json:"note"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (ViolationReview) TableName() string { return "violation_reviews" }

// ShortUrl represents a shortened URL record.
type ShortUrl struct {
	ID         uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	UID        string         `gorm:"size:16;uniqueIndex:uk_uid;not null" json:"uid"`
	LongURL    string         `gorm:"type:text;not null" json:"long_url"`
	URLHash    string         `gorm:"size:32;uniqueIndex:uk_url_hash;not null" json:"url_hash"`
	Title      string         `gorm:"size:255" json:"title"`
	CategoryID *uint64        `json:"category_id"`
	DomainID   *uint64        `gorm:"index" json:"domain_id"`
	Clicks     uint32         `gorm:"default:0;not null" json:"clicks"`
	Status     int8           `gorm:"default:1;not null" json:"status"`
	ExpireAt   *time.Time     `json:"expire_at"`
	// PasswordHash is the bcrypt hash of the optional access password; never
	// serialised. HasPassword is computed for the UI (lock indicator).
	PasswordHash string         `gorm:"size:255" json:"-"`
	HasPassword  bool           `gorm:"-" json:"has_password"`
	CreatedBy    *uint64        `json:"created_by"`
	MemberID     *uint64        `gorm:"index" json:"member_id"`
	Source       string         `gorm:"size:16;default:web;not null" json:"source"`
	IP           string         `gorm:"size:45" json:"ip"`
	ReminderSentAt *time.Time   `json:"reminder_sent_at"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (ShortUrl) TableName() string { return "short_urls" }

// UrlCategory represents a grouping for short URLs.
type UrlCategory struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string         `gorm:"size:64;not null" json:"name"`
	Color     string         `gorm:"size:7" json:"color"`
	SortOrder int            `gorm:"default:0;not null" json:"sort_order"`
	CreatedBy uint64         `gorm:"not null" json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (UrlCategory) TableName() string { return "url_categories" }

// ClickLog represents a single click event on a short URL.
type ClickLog struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ShortUrlID uint64    `gorm:"not null" json:"short_url_id"`
	IP         string    `gorm:"size:45;not null" json:"ip"`
	UserAgent  string    `gorm:"size:512" json:"user_agent"`
	Referer    string    `gorm:"size:512" json:"referer"`
	Country    string    `gorm:"size:2" json:"country"`
	CreatedAt  time.Time `json:"created_at"`
}

func (ClickLog) TableName() string { return "click_logs" }

// AuditLog represents an audit trail entry.
type AuditLog struct {
	ID         uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     *uint64    `json:"user_id"`
	Action     string     `gorm:"size:64;not null" json:"action"`
	Resource   string     `gorm:"size:64" json:"resource"`
	ResourceID string     `gorm:"size:64" json:"resource_id"`
	Detail     string     `gorm:"type:json" json:"detail"`
	IP         string     `gorm:"size:45;not null" json:"ip"`
	UserAgent  string     `gorm:"size:255" json:"user_agent"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (AuditLog) TableName() string { return "audit_logs" }

// SystemConfig represents a key-value system configuration entry.
type SystemConfig struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ConfigKey   string    `gorm:"size:64;uniqueIndex:uk_key;not null" json:"config_key"`
	ConfigValue string    `gorm:"type:text;not null" json:"config_value"`
	ValueType   string    `gorm:"size:16;default:string;not null" json:"value_type"`
	Description string    `gorm:"size:255" json:"description"`
	IsPublic    int8      `gorm:"default:0;not null" json:"is_public"`
	UpdatedBy   *uint64   `json:"updated_by"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (SystemConfig) TableName() string { return "system_configs" }

// ApiKey represents an API key for external access.
type ApiKey struct {
	ID          uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint64         `gorm:"not null" json:"user_id"`
	Name        string         `gorm:"size:64;not null" json:"name"`
	KeyPrefix   string         `gorm:"size:8;not null" json:"key_prefix"`
	KeyHash     string         `gorm:"size:64;uniqueIndex:uk_key_hash;not null" json:"-"`
	Permissions string         `gorm:"type:json" json:"permissions"`
	RateLimit   int            `gorm:"default:100;not null" json:"rate_limit"`
	LastUsedAt  *time.Time     `json:"last_used_at"`
	ExpiresAt   *time.Time     `json:"expires_at"`
	Status      int8           `gorm:"default:1;not null" json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (ApiKey) TableName() string { return "api_keys" }

// WebhookSub is a webhook subscription for outbound event notifications.
type WebhookSub struct {
	ID        uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string     `gorm:"size:64;not null" json:"name"`
	URL       string     `gorm:"size:512;not null" json:"url"`
	Events    string     `gorm:"type:json;not null" json:"events"` // JSON array of event names
	Secret    string     `gorm:"size:64" json:"-"`
	Status    int8       `gorm:"default:1;not null" json:"status"`
	CreatedBy *uint64    `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (WebhookSub) TableName() string { return "webhooks" }

// WebhookDelivery records an outbound webhook delivery attempt.
type WebhookDelivery struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	WebhookID      uint64     `gorm:"not null" json:"webhook_id"`
	Event          string     `gorm:"size:32;not null" json:"event"`
	Payload        string     `gorm:"type:json;not null" json:"payload"`
	ResponseStatus int        `json:"response_status"`
	ResponseBody   string     `gorm:"type:text" json:"response_body"`
	Attempt        int        `gorm:"default:1;not null" json:"attempt"`
	Success        int8       `gorm:"default:0;not null" json:"success"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (WebhookDelivery) TableName() string { return "webhook_deliveries" }
