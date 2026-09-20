package service

import (
	"errors"

	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

var ErrMemberNotFound = errors.New("member not found")

type MemberService interface {
	List(page, perPage int, keyword string, status *int8) ([]model.Member, int64, error)
	UpdateStatus(id uint64, status int8) error
	ResetPassword(id uint64, newPassword string) error
	// Delete removes the member and neutralises everything it owned (#39).
	Delete(id uint64) error
}

type memberService struct {
	memberRepo repository.MemberRepo
	purgeRepo repository.MemberPurgeRepo
	logger    *zap.Logger
}

func NewMemberService(memberRepo repository.MemberRepo) *memberService {
	return &memberService{memberRepo: memberRepo, logger: zap.NewNop()}
}

// WithPurge wires the cascade used by Delete. Kept as a fluent setter so the
// repository can stay optional: a deployment without the admin DB (or a test)
// still gets the previous behaviour instead of a nil-pointer panic.
func (s *memberService) WithPurge(purge repository.MemberPurgeRepo, logger *zap.Logger) *memberService {
	s.purgeRepo = purge
	if logger != nil {
		s.logger = logger
	}
	return s
}

func (s *memberService) List(page, perPage int, keyword string, status *int8) ([]model.Member, int64, error) {
	return s.memberRepo.List(page, perPage, keyword, status)
}

func (s *memberService) UpdateStatus(id uint64, status int8) error {
	if _, err := s.memberRepo.FindByID(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrMemberNotFound
		}
		return err
	}
	return s.memberRepo.UpdateStatus(id, status)
}

func (s *memberService) ResetPassword(id uint64, newPassword string) error {
	if _, err := s.memberRepo.FindByID(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrMemberNotFound
		}
		return err
	}
	hash, err := pkg.HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.memberRepo.ResetPassword(id, hash)
}

// Delete removes a member and everything the account owned (#39).
//
// Ordering matters. The short links are neutralised BEFORE the member row is
// removed: if the member delete succeeded first and the purge then failed, the
// links would be orphaned — no member row to find them by, so no future admin
// action could ever revoke them. Doing the purge first makes a partial failure
// safe: a retry re-runs both halves (they are idempotent) and the account, still
// present, remains visible/manageable in the admin list.
//
// token_version is bumped so any JWT already issued to the member is rejected
// immediately — without it the deleted account stays usable on the member API
// until the token expires (MemberAuth compares token_version against the row,
// so once the row is gone the mismatch is moot, but the bump also covers the
// window in which the purge fails and the row survives).
func (s *memberService) Delete(id uint64) error {
	if _, err := s.memberRepo.FindByID(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrMemberNotFound
		}
		return err
	}

	if s.purgeRepo != nil {
		links, err := s.purgeRepo.MemberLinks(id)
		if err != nil {
			return err
		}
		uids := make([]string, 0, len(links))
		ids := make([]uint64, 0, len(links))
		for _, l := range links {
			uids = append(uids, l.UID)
			ids = append(ids, l.ID)
		}

		// Stop the public redirect path (PHP do.php reads wjoy_log).
		if _, err := s.purgeRepo.PurgePublicLinks(uids); err != nil {
			return err
		}
		// Disable + detach the admin-side rows so the id cannot be inherited.
		if _, err := s.purgeRepo.PurgeAdminLinks(id); err != nil {
			return err
		}
		// Blank the personal data collected on those links.
		if _, err := s.purgeRepo.AnonymizeClickLogs(ids); err != nil {
			return err
		}
	}

	if err := s.memberRepo.BumpTokenVersion(id); err != nil {
		return err
	}
	return s.memberRepo.Delete(id)
}