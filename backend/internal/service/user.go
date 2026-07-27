package service

import (
	"errors"

	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"

	"gorm.io/gorm"
)

var (
	ErrUsernameExists = errors.New("username already exists")
	ErrEmailExists    = errors.New("email already exists")
)

type UserService interface {
	Create(username, email, password, displayName string) (*model.User, error)
	Update(id uint64, email, displayName, avatarURL string, status *int8) (*model.User, error)
	Delete(id uint64) error
	GetByID(id uint64) (*model.User, error)
	List(page, perPage int, keyword string) ([]model.User, int64, error)
	AssignRoles(userID uint64, roleIDs []uint64) error
	ResetPassword(id uint64, newPassword string) error
}

type userService struct {
	userRepo repository.UserRepo
}

func NewUserService(userRepo repository.UserRepo) UserService {
	return &userService{userRepo: userRepo}
}

func (s *userService) Create(username, email, password, displayName string) (*model.User, error) {
	// Check username uniqueness
	existing, err := s.userRepo.FindByUsername(username)
	if err == nil && existing != nil {
		return nil, ErrUsernameExists
	}

	// Check email uniqueness
	existing, err = s.userRepo.FindByEmail(email)
	if err == nil && existing != nil {
		return nil, ErrEmailExists
	}

	hash, err := pkg.HashPassword(password)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username:     username,
		Email:        email,
		PasswordHash: hash,
		DisplayName:  displayName,
		Status:       1,
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *userService) Update(id uint64, email, displayName, avatarURL string, status *int8) (*model.User, error) {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, err
	}

	if email != "" && email != user.Email {
		existing, err := s.userRepo.FindByEmail(email)
		if err == nil && existing != nil && existing.ID != id {
			return nil, ErrEmailExists
		}
		user.Email = email
	}

	if displayName != "" {
		user.DisplayName = displayName
	}

	if avatarURL != "" {
		user.AvatarURL = avatarURL
	}

	if status != nil {
		user.Status = *status
	}

	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *userService) Delete(id uint64) error {
	return s.userRepo.SoftDelete(id)
}

func (s *userService) GetByID(id uint64) (*model.User, error) {
	return s.userRepo.FindByID(id)
}

func (s *userService) List(page, perPage int, keyword string) ([]model.User, int64, error) {
	return s.userRepo.List(page, perPage, keyword)
}

func (s *userService) AssignRoles(userID uint64, roleIDs []uint64) error {
	_, err := s.userRepo.FindByID(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		return err
	}
	return s.userRepo.SetRoles(userID, roleIDs)
}

func (s *userService) ResetPassword(id uint64, newPassword string) error {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return err
	}

	hash, err := pkg.HashPassword(newPassword)
	if err != nil {
		return err
	}

	user.PasswordHash = hash
	return s.userRepo.Update(user)
}
