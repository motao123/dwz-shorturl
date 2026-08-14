package service

import (
	"errors"

	"dwz-admin/internal/model"
	"dwz-admin/internal/repository"

	"gorm.io/gorm"
)

var ErrViolationNotFound = errors.New("violation review not found")

type ViolationService interface {
	List(page, perPage int, reviewed *int8, keyword string) ([]model.ViolationReview, int64, error)
	MarkReviewed(id uint64, note string) error
	Delete(id uint64) error
}

type violationService struct {
	repo repository.ViolationRepo
}

func NewViolationService(repo repository.ViolationRepo) ViolationService {
	return &violationService{repo: repo}
}

func (s *violationService) List(page, perPage int, reviewed *int8, keyword string) ([]model.ViolationReview, int64, error) {
	return s.repo.List(page, perPage, reviewed, keyword)
}

func (s *violationService) MarkReviewed(id uint64, note string) error {
	if _, err := s.repo.FindByID(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrViolationNotFound
		}
		return err
	}
	return s.repo.MarkReviewed(id, note)
}

func (s *violationService) Delete(id uint64) error {
	if _, err := s.repo.FindByID(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrViolationNotFound
		}
		return err
	}
	return s.repo.Delete(id)
}