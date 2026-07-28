package repository

import (
	"dwz-admin/internal/model"

	"gorm.io/gorm"
)

// DomainRepo defines persistence operations for the domain pool.
type DomainRepo interface {
	List(status *int8) ([]model.Domain, error)
	FindByID(id uint64) (*model.Domain, error)
	FindByDomain(domain string) (*model.Domain, error)
	Create(d *model.Domain) error
	Update(d *model.Domain) error
	SoftDelete(id uint64) error
	IncrementLinkCount(id uint64) error
	DecrementLinkCount(id uint64) error
	UpdateDNSStatus(id uint64, status string) error
	UpdateSSLStatus(id uint64, status string) error
	PickAvailable() (*model.Domain, error)
}

type domainRepo struct {
	db *gorm.DB
}

func NewDomainRepo(db *gorm.DB) DomainRepo {
	return &domainRepo{db: db}
}

func (r *domainRepo) List(status *int8) ([]model.Domain, error) {
	var domains []model.Domain
	query := r.db.Order("priority ASC, id ASC")
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	if err := query.Find(&domains).Error; err != nil {
		return nil, err
	}
	return domains, nil
}

func (r *domainRepo) FindByID(id uint64) (*model.Domain, error) {
	var d model.Domain
	if err := r.db.First(&d, id).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *domainRepo) FindByDomain(domain string) (*model.Domain, error) {
	var d model.Domain
	if err := r.db.Where("domain = ?", domain).First(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *domainRepo) Create(d *model.Domain) error {
	return r.db.Create(d).Error
}

func (r *domainRepo) Update(d *model.Domain) error {
	return r.db.Save(d).Error
}

func (r *domainRepo) SoftDelete(id uint64) error {
	return r.db.Delete(&model.Domain{}, id).Error
}

func (r *domainRepo) IncrementLinkCount(id uint64) error {
	return r.db.Model(&model.Domain{}).Where("id = ?", id).
		UpdateColumn("link_count", gorm.Expr("link_count + 1")).Error
}

func (r *domainRepo) DecrementLinkCount(id uint64) error {
	return r.db.Model(&model.Domain{}).Where("id = ?", id).
		UpdateColumn("link_count", gorm.Expr("CASE WHEN link_count > 0 THEN link_count - 1 ELSE 0 END")).Error
}

func (r *domainRepo) UpdateDNSStatus(id uint64, status string) error {
	return r.db.Model(&model.Domain{}).Where("id = ?", id).
		UpdateColumn("dns_status", status).Error
}

func (r *domainRepo) UpdateSSLStatus(id uint64, status string) error {
	return r.db.Model(&model.Domain{}).Where("id = ?", id).
		UpdateColumn("ssl_status", status).Error
}

// PickAvailable selects the best available domain: lowest link_count first,
// then lowest priority, among non-deleted active domains.
func (r *domainRepo) PickAvailable() (*model.Domain, error) {
	var d model.Domain
	err := r.db.
		Where("status = ?", 1).
		Order("link_count ASC, priority ASC, id ASC").
		First(&d).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}
