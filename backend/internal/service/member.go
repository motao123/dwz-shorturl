package service

import (
	"errors"

	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"

	"gorm.io/gorm"
)

var ErrMemberNotFound = errors.New("member not found")

type MemberService interface {
	List(page, perPage int, keyword string, status *int8) ([]model.Member, int64, error)
	UpdateStatus(id uint64, status int8) error
	ResetPassword(id uint64, newPassword string) error
	Delete(id uint64) error
}

type memberService struct {
	memberRepo repository.MemberRepo
}

func NewMemberService(memberRepo repository.MemberRepo) MemberService {
	return &memberService{memberRepo: memberRepo}
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

func (s *memberService) Delete(id uint64) error {
	if _, err := s.memberRepo.FindByID(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrMemberNotFound
		}
		return err
	}
	return s.memberRepo.Delete(id)
}