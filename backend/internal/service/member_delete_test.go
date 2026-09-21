package service

import (
	"errors"
	"testing"
	"time"

	"dwz-admin/internal/model"
	"dwz-admin/internal/repository"

	"gorm.io/gorm"
)

// --- stubs -----------------------------------------------------------------

type memberRepoStub struct {
	member      *model.Member
	findErr     error
	deleted     bool
	bumpedToken bool
	deleteErr   error
}

func (s *memberRepoStub) FindByID(uint64) (*model.Member, error) {
	if s.findErr != nil {
		return nil, s.findErr
	}
	return s.member, nil
}
func (s *memberRepoStub) FindByEmail(string) (*model.Member, error) {
	return nil, gorm.ErrRecordNotFound
}
func (s *memberRepoStub) FindByResetToken(string) (*model.Member, error) {
	return nil, gorm.ErrRecordNotFound
}
func (s *memberRepoStub) SetResetToken(uint64, string, *time.Time) error { return nil }
func (s *memberRepoStub) BumpTokenVersion(uint64) error {
	s.bumpedToken = true
	return nil
}
func (s *memberRepoStub) SetVerifyToken(uint64, string, *time.Time) error { return nil }
func (s *memberRepoStub) FindByVerifyToken(string) (*model.Member, error) {
	return nil, gorm.ErrRecordNotFound
}
func (s *memberRepoStub) MarkVerified(uint64) error { return nil }
func (s *memberRepoStub) List(int, int, string, *int8) ([]model.Member, int64, error) {
	return nil, 0, nil
}
func (s *memberRepoStub) UpdateStatus(uint64, int8) error    { return nil }
func (s *memberRepoStub) ResetPassword(uint64, string) error { return nil }
func (s *memberRepoStub) Delete(uint64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = true
	return nil
}

// purgeRepoStub records the cascade calls, in order, so the test can assert both
// that each step ran and that they ran in the intended sequence.
type purgeRepoStub struct {
	order        []string
	links        []repository.MemberLinkRef
	linksErr     error
	purgePubErr  error
	purgeAdmErr  error
	anonErr      error
	gotUIDs      []string
	gotIDs       []uint64
	gotPublicArg int
}

func (s *purgeRepoStub) MemberLinks(uint64) ([]repository.MemberLinkRef, error) {
	s.order = append(s.order, "links")
	if s.linksErr != nil {
		return nil, s.linksErr
	}
	return s.links, nil
}

func (s *purgeRepoStub) PurgePublicLinks(uids []string) (int64, error) {
	s.order = append(s.order, "purge_public")
	s.gotUIDs = uids
	if s.purgePubErr != nil {
		return 0, s.purgePubErr
	}
	return int64(len(uids)), nil
}

func (s *purgeRepoStub) PurgeAdminLinks(uint64) (int64, error) {
	s.order = append(s.order, "purge_admin")
	if s.purgeAdmErr != nil {
		return 0, s.purgeAdmErr
	}
	return 1, nil
}

func (s *purgeRepoStub) AnonymizeClickLogs(ids []uint64) (int64, error) {
	s.order = append(s.order, "anonymize")
	s.gotIDs = ids
	if s.anonErr != nil {
		return 0, s.anonErr
	}
	return int64(len(ids)), nil
}

func newDeleteFixture() (*memberRepoStub, *purgeRepoStub, *memberService) {
	mr := &memberRepoStub{member: &model.Member{ID: 5, Status: 1}}
	pr := &purgeRepoStub{links: []repository.MemberLinkRef{
		{ID: 11, UID: "aaa"},
		{ID: 12, UID: "bbb"},
	}}
	svc := NewMemberService(mr).WithPurge(pr, nil)
	return mr, pr, svc
}

// TestMemberDelete_CascadesToOwnedData is the regression test for #39: deleting
// a member must neutralise the links (both mirrors) and blank the visitor data,
// not just drop the member row.
func TestMemberDelete_CascadesToOwnedData(t *testing.T) {
	mr, pr, svc := newDeleteFixture()

	if err := svc.Delete(5); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	want := []string{"links", "purge_public", "purge_admin", "anonymize"}
	if len(pr.order) != len(want) {
		t.Fatalf("cascade steps = %v, want %v", pr.order, want)
	}
	for i := range want {
		if pr.order[i] != want[i] {
			t.Fatalf("cascade order = %v, want %v", pr.order, want)
		}
	}
	if len(pr.gotUIDs) != 2 || pr.gotUIDs[0] != "aaa" || pr.gotUIDs[1] != "bbb" {
		t.Errorf("public purge uids = %v, want [aaa bbb]", pr.gotUIDs)
	}
	if len(pr.gotIDs) != 2 || pr.gotIDs[0] != 11 || pr.gotIDs[1] != 12 {
		t.Errorf("anonymize ids = %v, want [11 12]", pr.gotIDs)
	}
	if !mr.bumpedToken {
		t.Error("token_version was not bumped; a live JWT would survive the delete")
	}
	if !mr.deleted {
		t.Error("member row was not deleted")
	}
}

// TestMemberDelete_PurgeFailureKeepsAccount makes the ordering guarantee
// explicit: if a purge step fails the member row must NOT be removed, otherwise
// the links become unreachable/unrevocable orphans.
func TestMemberDelete_PurgeFailureKeepsAccount(t *testing.T) {
	mr, pr, svc := newDeleteFixture()
	pr.purgePubErr = errors.New("public db down")

	if err := svc.Delete(5); err == nil {
		t.Fatal("Delete: want error when the public purge fails, got nil")
	}
	if mr.deleted {
		t.Error("member row was deleted even though the link purge failed (orphaned links)")
	}
}

// TestMemberDelete_NotFoundStillReturnsNotFound keeps the existing contract.
func TestMemberDelete_NotFoundStillReturnsNotFound(t *testing.T) {
	mr := &memberRepoStub{findErr: gorm.ErrRecordNotFound}
	svc := NewMemberService(mr).WithPurge(&purgeRepoStub{}, nil)

	if err := svc.Delete(99); !errors.Is(err, ErrMemberNotFound) {
		t.Fatalf("want ErrMemberNotFound, got %v", err)
	}
}

// TestMemberDelete_WithoutPurgeRepoStillDeletes documents the degraded mode: a
// deployment without the admin DB (nil purge repo) must still be able to delete
// the member instead of panicking.
func TestMemberDelete_WithoutPurgeRepoStillDeletes(t *testing.T) {
	mr := &memberRepoStub{member: &model.Member{ID: 1, Status: 1}}
	svc := NewMemberService(mr)

	if err := svc.Delete(1); err != nil {
		t.Fatalf("Delete without purge repo: %v", err)
	}
	if !mr.deleted {
		t.Error("member row was not deleted")
	}
}
