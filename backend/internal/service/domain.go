package service

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strings"
	"time"

	"dwz-admin/internal/config"
	"dwz-admin/internal/model"
	"dwz-admin/internal/repository"
)

var (
	ErrDomainInvalid    = errors.New("domain is invalid")
	ErrDomainExists     = errors.New("domain already exists")
	ErrDomainNotFound   = errors.New("domain not found")
	ErrNoDomainAvailable = errors.New("no available domain in the pool")
)

// DomainService handles domain pool management operations.
type DomainService interface {
	List(status *int8) ([]model.Domain, error)
	GetByID(id uint64) (*model.Domain, error)
	Create(domain, scheme, name, project string, priority int) (*model.Domain, error)
	Update(id uint64, domain, scheme, name, project string, status *int8, priority *int) (*model.Domain, error)
	Delete(id uint64) error
	CheckDomain(id uint64) error
	PickDomain() (*model.Domain, error)
	BuildShortURL(uid string, domainID *uint64) string
	IncrementLinkCount(id uint64) error
	DecrementLinkCount(id uint64) error
}

type domainService struct {
	repo repository.DomainRepo
}

func NewDomainService(repo repository.DomainRepo) DomainService {
	return &domainService{repo: repo}
}

func (s *domainService) List(status *int8) ([]model.Domain, error) {
	return s.repo.List(status)
}

func (s *domainService) GetByID(id uint64) (*model.Domain, error) {
	d, err := s.repo.FindByID(id)
	if err != nil {
		return nil, ErrDomainNotFound
	}
	return d, nil
}

func (s *domainService) Create(domain, scheme, name, project string, priority int) (*model.Domain, error) {
	domain = strings.TrimSpace(domain)
	if !isValidDomainName(domain) {
		return nil, ErrDomainInvalid
	}

	// Default scheme
	if scheme == "" {
		scheme = "https"
	}
	scheme = strings.ToLower(scheme)
	if scheme != "http" && scheme != "https" {
		return nil, ErrDomainInvalid
	}

	// Check for duplicates
	if existing, err := s.repo.FindByDomain(domain); err == nil && existing != nil {
		return nil, ErrDomainExists
	}

	if priority < 0 {
		priority = 100
	}

	d := &model.Domain{
		Domain:    domain,
		Scheme:    scheme,
		Name:      name,
		Project:   project,
		Status:    1,
		Priority:  priority,
		DNSStatus: "pending",
		SSLStatus: "pending",
		LinkCount: 0,
	}

	if err := s.repo.Create(d); err != nil {
		return nil, err
	}

	// Run DNS/SSL check asynchronously
	go func(id uint64) {
		_ = s.CheckDomain(id)
	}(d.ID)

	return d, nil
}

func (s *domainService) Update(id uint64, domain, scheme, name, project string, status *int8, priority *int) (*model.Domain, error) {
	d, err := s.repo.FindByID(id)
	if err != nil {
		return nil, ErrDomainNotFound
	}

	domain = strings.TrimSpace(domain)
	if domain != "" {
		if !isValidDomainName(domain) {
			return nil, ErrDomainInvalid
		}
		// Check duplicate against a different record
		if existing, err := s.repo.FindByDomain(domain); err == nil && existing != nil && existing.ID != id {
			return nil, ErrDomainExists
		}
		d.Domain = domain
	}

	if scheme != "" {
		scheme = strings.ToLower(scheme)
		if scheme != "http" && scheme != "https" {
			return nil, ErrDomainInvalid
		}
		d.Scheme = scheme
	}

	d.Name = name
	d.Project = project

	if status != nil {
		d.Status = *status
	}

	if priority != nil {
		if *priority < 0 {
			*priority = 100
		}
		d.Priority = *priority
	}

	if err := s.repo.Update(d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *domainService) Delete(id uint64) error {
	if _, err := s.repo.FindByID(id); err != nil {
		return ErrDomainNotFound
	}
	return s.repo.SoftDelete(id)
}

// CheckDomain performs a live DNS and SSL handshake check and persists the results.
func (s *domainService) CheckDomain(id uint64) error {
	d, err := s.repo.FindByID(id)
	if err != nil {
		return ErrDomainNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// DNS check
	var resolver net.Resolver
	addrs, lookupErr := resolver.LookupHost(ctx, d.Domain)
	if lookupErr != nil || len(addrs) == 0 {
		_ = s.repo.UpdateDNSStatus(id, "fail")
	} else {
		_ = s.repo.UpdateDNSStatus(id, "ok")
	}

	// SSL check (port 443)
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, dialErr := tls.DialWithDialer(dialer, "tcp", d.Domain+":443", &tls.Config{
		ServerName: d.Domain,
	})
	if dialErr != nil {
		_ = s.repo.UpdateSSLStatus(id, "fail")
	} else {
		_ = conn.Close()
		_ = s.repo.UpdateSSLStatus(id, "ok")
	}

	return nil
}

func (s *domainService) PickDomain() (*model.Domain, error) {
	d, err := s.repo.PickAvailable()
	if err != nil {
		return nil, ErrNoDomainAvailable
	}
	return d, nil
}

// BuildShortURL builds the full public short URL for a uid.
// If domainID is nil it uses the configured public base URL; otherwise it uses
// the scheme + domain from the matching pool entry, falling back to the base URL
// if the entry cannot be found.
func (s *domainService) BuildShortURL(uid string, domainID *uint64) string {
	if domainID != nil {
		if d, err := s.repo.FindByID(*domainID); err == nil && d != nil {
			scheme := strings.ToLower(d.Scheme)
			if scheme == "" {
				scheme = "https"
			}
			return scheme + "://" + strings.TrimRight(d.Domain, "/") + "/" + uid
		}
	}

	cfg := config.Get()
	base := strings.TrimRight(cfg.Public.BaseURL, "/")
	return base + "/" + uid
}

func (s *domainService) IncrementLinkCount(id uint64) error {
	return s.repo.IncrementLinkCount(id)
}

func (s *domainService) DecrementLinkCount(id uint64) error {
	return s.repo.DecrementLinkCount(id)
}

// isValidDomainName performs a lightweight format check on the host portion.
// It accepts a bare hostname (e.g. 1.xk7.cn) without scheme/path/port.
func isValidDomainName(domain string) bool {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" || len(domain) > 253 {
		return false
	}
	// Reject anything that looks like a URL (scheme) or contains path/port
	if strings.Contains(domain, "://") || strings.Contains(domain, "/") || strings.Contains(domain, ":") {
		return false
	}
	// Reject localhost-style / private suffixes used as identifiers
	if domain == "localhost" {
		return false
	}
	// Each label must be 1-63 chars of alnum / hyphen, not start or end with hyphen.
	for _, label := range strings.Split(domain, ".") {
		if len(label) < 1 || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
				return false
			}
		}
	}
	// Require at least one dot (a public-looking hostname)
	return strings.Contains(domain, ".")
}
