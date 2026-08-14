package service

import (
	"errors"
	"sync"
	"testing"

	"dwz-admin/internal/model"
	"dwz-admin/internal/repository"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// --- mock ShortUrlRepo ---

type mockShortRepo struct {
	mu       sync.Mutex
	records  map[uint64]*model.ShortUrl
	byHash   map[string]uint64
	byID     uint64
	domains  *mockDomainRepo
	createFn func(*model.ShortUrl) error // optional override
}

func newMockShortRepo() *mockShortRepo {
	return &mockShortRepo{records: map[uint64]*model.ShortUrl{}, byHash: map[string]uint64{}}
}

func (m *mockShortRepo) FindByUID(uid string) (*model.ShortUrl, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.records {
		if r.UID == uid {
			cp := *r
			return &cp, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *mockShortRepo) FindByHash(hash string) (*model.ShortUrl, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.byHash[hash]; ok {
		cp := *m.records[id]
		return &cp, nil
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *mockShortRepo) FindByHashIncludingDeleted(hash string) (*model.ShortUrl, error) {
	// Tests never exercise resurrection, so behave like FindByHash.
	return m.FindByHash(hash)
}
func (m *mockShortRepo) FindByID(id uint64) (*model.ShortUrl, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.records[id]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *mockShortRepo) FindByIDIncludingDeleted(id uint64) (*model.ShortUrl, error) {
	return m.FindByID(id)
}
func (m *mockShortRepo) RestoreWithDomainCount(url *model.ShortUrl) error {
	// Tests never exercise restore domain counting; clear the deleted flag.
	m.mu.Lock()
	defer m.mu.Unlock()
	url.DeletedAt = gorm.DeletedAt{}
	if r, ok := m.records[url.ID]; ok {
		r.DeletedAt = gorm.DeletedAt{}
	}
	return nil
}
func (m *mockShortRepo) Create(u *model.ShortUrl) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createFn != nil {
		return m.createFn(u)
	}
	m.byID++
	u.ID = m.byID
	m.records[u.ID] = u
	m.byHash[u.URLHash] = u.ID
	return nil
}
func (m *mockShortRepo) CreateWithDomainCount(u *model.ShortUrl) error {
	m.mu.Lock()
	if m.createFn != nil {
		err := m.createFn(u)
		m.mu.Unlock()
		return err
	}
	m.byID++
	u.ID = m.byID
	m.records[u.ID] = u
	m.byHash[u.URLHash] = u.ID
	m.mu.Unlock()
	if u.DomainID != nil && m.domains != nil {
		return m.domains.IncrementLinkCount(*u.DomainID)
	}
	return nil
}
func (m *mockShortRepo) Update(u *model.ShortUrl) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.records[u.ID]; !ok {
		return gorm.ErrRecordNotFound
	}
	m.records[u.ID] = u
	return nil
}
func (m *mockShortRepo) UpdateWithDomainCount(u *model.ShortUrl, oldDomainID *uint64) error {
	if err := m.Update(u); err != nil {
		return err
	}
	if m.domains != nil && !sameUint64Ptr(oldDomainID, u.DomainID) {
		if oldDomainID != nil {
			_ = m.domains.DecrementLinkCount(*oldDomainID)
		}
		if u.DomainID != nil {
			_ = m.domains.IncrementLinkCount(*u.DomainID)
		}
	}
	return nil
}
func (m *mockShortRepo) SoftDelete(id uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.records[id]; !ok {
		return gorm.ErrRecordNotFound
	}
	delete(m.records, id)
	return nil
}
func (m *mockShortRepo) SoftDeleteWithDomainCount(u *model.ShortUrl) error {
	if err := m.SoftDelete(u.ID); err != nil {
		return err
	}
	if u.DomainID != nil && m.domains != nil {
		return m.domains.DecrementLinkCount(*u.DomainID)
	}
	return nil
}
func (m *mockShortRepo) BatchDelete(ids []uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if r, ok := m.records[id]; ok {
			delete(m.byHash, r.URLHash)
			delete(m.records, id)
		}
	}
	return nil
}
func (m *mockShortRepo) BatchDeleteWithDomainCount(urls []model.ShortUrl) error {
	ids := make([]uint64, 0, len(urls))
	for _, u := range urls {
		ids = append(ids, u.ID)
	}
	if err := m.BatchDelete(ids); err != nil {
		return err
	}
	if m.domains != nil {
		for _, u := range urls {
			if u.DomainID != nil {
				_ = m.domains.DecrementLinkCount(*u.DomainID)
			}
		}
	}
	return nil
}
func (m *mockShortRepo) List(page, perPage int, f repository.ShortUrlFilters) ([]model.ShortUrl, int64, error) {
	return nil, 0, nil
}
func (m *mockShortRepo) Count() (int64, error)                    { return int64(len(m.records)), nil }
func (m *mockShortRepo) CountByStatus(int8) (int64, error)        { return 0, nil }
func (m *mockShortRepo) CountToday() (int64, error)               { return 0, nil }
func (m *mockShortRepo) BatchCreate(urls []model.ShortUrl) error  { return nil }
func (m *mockShortRepo) IncrementClicks(uint64) error             { return nil }
func (m *mockShortRepo) FindTopN(int) ([]model.ShortUrl, error)   { return nil, nil }
func (m *mockShortRepo) FindRecent(int) ([]model.ShortUrl, error) { return nil, nil }

// --- mock DomainRepo (only counting ops exercised here) ---

type mockDomainRepo struct {
	mu      sync.Mutex
	incrCnt int
	decrCnt int
	incrIDs []uint64
	decrIDs []uint64
}

func (m *mockDomainRepo) List(*int8) ([]model.Domain, error)     { return nil, nil }
func (m *mockDomainRepo) FindByID(id uint64) (*model.Domain, error) {
	// Tests exercise domain counting, so any requested domain is active.
	return &model.Domain{ID: id, Status: 1}, nil
}
func (m *mockDomainRepo) FindByDomain(string) (*model.Domain, error) {
	return nil, gorm.ErrRecordNotFound
}
func (m *mockDomainRepo) Create(*model.Domain) error { return nil }
func (m *mockDomainRepo) Update(*model.Domain) error { return nil }
func (m *mockDomainRepo) SoftDelete(uint64) error    { return nil }
func (m *mockDomainRepo) IncrementLinkCount(id uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.incrCnt++
	m.incrIDs = append(m.incrIDs, id)
	return nil
}
func (m *mockDomainRepo) DecrementLinkCount(id uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decrCnt++
	m.decrIDs = append(m.decrIDs, id)
	return nil
}
func (m *mockDomainRepo) UpdateDNSStatus(uint64, string) error  { return nil }
func (m *mockDomainRepo) UpdateSSLStatus(uint64, string) error  { return nil }
func (m *mockDomainRepo) PickAvailable() (*model.Domain, error) { return nil, gorm.ErrRecordNotFound }

// buildService wires a shortUrlService with mock repos and an unconnected redis.
func buildService(sr *mockShortRepo, dr *mockDomainRepo) ShortUrlService {
	sr.domains = dr
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"}) // never dialed in these tests
	return NewShortUrlService(sr, rdb, nil, nil, dr, nil)
}

// --- tests ---

// H-03: creating a short link with a domain_id must increment link_count exactly once.
func TestCreate_IncrementsDomainLinkCount(t *testing.T) {
	sr := newMockShortRepo()
	dr := &mockDomainRepo{}
	svc := buildService(sr, dr)

	domainID := uint64(7)
	rec, err := svc.Create("https://www.example.com", "", 0, &domainID, nil, "test", "127.0.0.1", "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if rec.DomainID == nil || *rec.DomainID != domainID {
		t.Fatalf("domain_id not preserved, got %v", rec.DomainID)
	}
	if dr.incrCnt != 1 {
		t.Fatalf("expected IncrementLinkCount called once, got %d", dr.incrCnt)
	}
	if len(dr.incrIDs) != 1 || dr.incrIDs[0] != domainID {
		t.Fatalf("expected increment on domain %d, got %v", domainID, dr.incrIDs)
	}
	if dr.decrCnt != 0 {
		t.Fatalf("expected no decrement on create, got %d", dr.decrCnt)
	}
}

// H-03: creating a short link without a domain_id must NOT touch link_count.
func TestCreate_NoDomain_NoCountChange(t *testing.T) {
	sr := newMockShortRepo()
	dr := &mockDomainRepo{}
	svc := buildService(sr, dr)

	if _, err := svc.Create("https://www.example.com", "", 0, nil, nil, "test", "127.0.0.1", ""); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if dr.incrCnt != 0 || dr.decrCnt != 0 {
		t.Fatalf("expected no count change, got incr=%d decr=%d", dr.incrCnt, dr.decrCnt)
	}
}

// H-03: re-creating the same URL (hash dedup) must NOT increment link_count a second time.
func TestCreate_Dedup_DoesNotDoubleIncrement(t *testing.T) {
	sr := newMockShortRepo()
	dr := &mockDomainRepo{}
	svc := buildService(sr, dr)

	domainID := uint64(7)
	if _, err := svc.Create("https://www.example.com", "", 0, &domainID, nil, "test", "127.0.0.1", ""); err != nil {
		t.Fatalf("first Create failed: %v", err)
	}
	// second create same URL -> dedup, should NOT increment again
	if _, err := svc.Create("https://www.example.com", "", 0, &domainID, nil, "test", "127.0.0.1", ""); err != nil {
		t.Fatalf("second Create failed: %v", err)
	}
	if dr.incrCnt != 1 {
		t.Fatalf("dedup must not double increment, got %d", dr.incrCnt)
	}
}

// H-03: deleting a short link with a domain_id must decrement link_count exactly once.
func TestDelete_DecrementsDomainLinkCount(t *testing.T) {
	sr := newMockShortRepo()
	dr := &mockDomainRepo{}
	svc := buildService(sr, dr)

	domainID := uint64(7)
	rec, err := svc.Create("https://www.example.com", "", 0, &domainID, nil, "test", "127.0.0.1", "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := svc.Delete(rec.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if dr.decrCnt != 1 {
		t.Fatalf("expected DecrementLinkCount once, got %d", dr.decrCnt)
	}
	if len(dr.decrIDs) != 1 || dr.decrIDs[0] != domainID {
		t.Fatalf("expected decrement on domain %d, got %v", domainID, dr.decrIDs)
	}
}

// H-03: batch delete must decrement by the number of links per domain.
func TestBatchDelete_DecrementsPerLink(t *testing.T) {
	sr := newMockShortRepo()
	dr := &mockDomainRepo{}
	svc := buildService(sr, dr)

	domainID := uint64(7)
	var ids []uint64
	// Inject three DISTINCT records under the same domain (distinct hashes).
	for _, u := range []string{"https://www.example.com", "https://www.example.org", "https://www.example.net"} {
		r := &model.ShortUrl{URLHash: md5Hash(u), LongURL: u, DomainID: &domainID, Status: 1}
		sr.mu.Lock()
		sr.byID++
		r.ID = sr.byID
		sr.records[r.ID] = r
		sr.byHash[r.URLHash] = r.ID
		sr.mu.Unlock()
		ids = append(ids, r.ID)
	}

	if err := svc.BatchDelete(ids); err != nil {
		t.Fatalf("BatchDelete failed: %v", err)
	}
	if dr.decrCnt != 3 {
		t.Fatalf("expected 3 decrements (3 links deleted), got %d", dr.decrCnt)
	}
}

// H-03: changing the domain on Update must decrement old and increment new.
func TestUpdate_DomainChange_AdjustsCounts(t *testing.T) {
	sr := newMockShortRepo()
	dr := &mockDomainRepo{}
	svc := buildService(sr, dr)

	oldDomain := uint64(1)
	newDomain := uint64(2)
	rec, err := svc.Create("https://www.example.com", "", 0, &oldDomain, nil, "test", "127.0.0.1", "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if dr.incrCnt != 1 {
		t.Fatalf("setup: expected 1 increment, got %d", dr.incrCnt)
	}

	if _, err := svc.Update(rec.ID, "", "", nil, nil, nil, &newDomain, nil); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if dr.incrCnt != 2 {
		t.Fatalf("expected 2 increments after domain change, got %d", dr.incrCnt)
	}
	if dr.decrCnt != 1 {
		t.Fatalf("expected 1 decrement after domain change, got %d", dr.decrCnt)
	}
}

// H-03: Update with the SAME domain_id must NOT adjust counts.
func TestUpdate_SameDomain_NoCountChange(t *testing.T) {
	sr := newMockShortRepo()
	dr := &mockDomainRepo{}
	svc := buildService(sr, dr)

	domainID := uint64(1)
	rec, err := svc.Create("https://www.example.com", "", 0, &domainID, nil, "test", "127.0.0.1", "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	beforeIncr, beforeDecr := dr.incrCnt, dr.decrCnt

	if _, err := svc.Update(rec.ID, "", "new title", nil, nil, nil, &domainID, nil); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if dr.incrCnt != beforeIncr || dr.decrCnt != beforeDecr {
		t.Fatalf("same domain must not change counts, got incr=%d decr=%d", dr.incrCnt, dr.decrCnt)
	}
}

// H-03: Delete failure (repo error) must NOT decrement.
func TestDelete_RepoFailure_NoDecrement(t *testing.T) {
	sr := newMockShortRepo()
	dr := &mockDomainRepo{}
	svc := buildService(sr, dr)

	domainID := uint64(1)
	rec, _ := svc.Create("https://www.example.com", "", 0, &domainID, nil, "test", "127.0.0.1", "")

	// Force SoftDelete to fail by deleting the record from the mock store first is not enough;
	// instead patch via a second delete attempt on an already-deleted id.
	_ = svc.Delete(rec.ID)
	dr.decrCnt = 0
	if err := svc.Delete(rec.ID); err == nil {
		t.Fatalf("expected error deleting non-existent record")
	}
	if dr.decrCnt != 0 {
		t.Fatalf("failed delete must not decrement, got %d", dr.decrCnt)
	}
}

func TestBatchCreate_PreservesDomainAndDeduplicates(t *testing.T) {
	sr := newMockShortRepo()
	dr := &mockDomainRepo{}
	svc := buildService(sr, dr)
	domainID := uint64(9)

	results, errs := svc.BatchCreate([]string{
		"https://www.example.com/a",
		"https://www.example.com/a",
		"https://www.example.com/b",
	}, &domainID, nil, "127.0.0.1")

	if len(errs) != 3 {
		t.Fatalf("expected one error slot per input, got %d", len(errs))
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("unexpected error at index %d: %v", i, err)
		}
	}
	if len(results) != 3 {
		t.Fatalf("expected one result per valid input, got %d", len(results))
	}
	if results[0].ID != results[1].ID {
		t.Fatalf("duplicate URL should return the existing record")
	}
	for i, result := range results {
		if result.DomainID == nil || *result.DomainID != domainID {
			t.Fatalf("result %d did not preserve domain_id", i)
		}
	}
	if dr.incrCnt != 2 {
		t.Fatalf("expected increments for 2 unique URLs, got %d", dr.incrCnt)
	}
}

func TestBatchCreate_ReportsErrorsByInputIndex(t *testing.T) {
	sr := newMockShortRepo()
	dr := &mockDomainRepo{}
	svc := buildService(sr, dr)

	results, errs := svc.BatchCreate([]string{
		"https://www.example.com/ok",
		"http://127.0.0.1/private",
		"",
	}, nil, nil, "127.0.0.1")

	if len(results) != 1 {
		t.Fatalf("expected 1 successful record, got %d", len(results))
	}
	if errs[0] != nil {
		t.Fatalf("expected first item to succeed: %v", errs[0])
	}
	if !errors.Is(errs[1], ErrSSRFBlocked) {
		t.Fatalf("expected SSRF error at input index 1, got %v", errs[1])
	}
	if errs[2] != nil {
		t.Fatalf("blank input is skipped and should not report an error, got %v", errs[2])
	}
}

// helper: ensure errors.Is still works for sentinel comparison if needed later
var _ = errors.Is
