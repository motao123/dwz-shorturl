package model

import (
	"time"

	"gorm.io/gorm"
)

// Domain represents an entry in the domain pool used to build short URLs.
type Domain struct {
	ID         uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Domain     string         `gorm:"size:128;uniqueIndex:uk_domain;not null" json:"domain"`
	Scheme     string         `gorm:"size:8;default:https" json:"scheme"`
	Name       string         `gorm:"size:64" json:"name"`
	Project    string         `gorm:"size:64" json:"project"`
	Status     int8           `gorm:"default:1;not null" json:"status"`
	Priority   int            `gorm:"default:100;not null" json:"priority"`
	DNSStatus  string         `gorm:"size:16;default:pending" json:"dns_status"`
	SSLStatus  string         `gorm:"size:16;default:pending" json:"ssl_status"`
	LinkCount  uint32         `gorm:"default:0;not null" json:"link_count"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Domain) TableName() string { return "domains" }
