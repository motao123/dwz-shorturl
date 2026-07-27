package service

import (
	"errors"

	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserDisabled       = errors.New("user account is disabled")
	ErrUserNotFound       = errors.New("user not found")
)

type LoginResult struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int64    `json:"expires_in"`
	User         UserInfo `json:"user"`
}

type UserInfo struct {
	ID       uint64   `json:"id"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Roles    []string `json:"roles"`
}

type AuthService interface {
	Login(username, password, ip string) (*LoginResult, error)
	Refresh(refreshToken string) (*LoginResult, error)
	GetMe(userID uint64) (*UserInfo, error)
}

type authService struct {
	userRepo repository.UserRepo
	roleRepo repository.RoleRepo
}

func NewAuthService(userRepo repository.UserRepo, roleRepo repository.RoleRepo) AuthService {
	return &authService{userRepo: userRepo, roleRepo: roleRepo}
}

func (s *authService) Login(username, password, ip string) (*LoginResult, error) {
	user, err := s.userRepo.FindByUsername(username)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if user.Status != 1 {
		return nil, ErrUserDisabled
	}

	if !pkg.CheckPassword(user.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}

	roles, err := s.userRepo.GetRoles(user.ID)
	if err != nil {
		return nil, err
	}

	roleNames := make([]string, 0, len(roles))
	for _, r := range roles {
		roleNames = append(roleNames, r.Name)
	}

	accessToken, refreshToken, err := pkg.GenerateTokens(user.ID, user.Username, roleNames)
	if err != nil {
		return nil, err
	}

	// Update last login
	_ = s.userRepo.UpdateLastLogin(user.ID, ip)

	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    7200,
		User: UserInfo{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			Roles:    roleNames,
		},
	}, nil
}

func (s *authService) Refresh(refreshToken string) (*LoginResult, error) {
	claims, err := pkg.ParseToken(refreshToken)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}

	user, err := s.userRepo.FindByID(claims.UserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	if user.Status != 1 {
		return nil, ErrUserDisabled
	}

	roles, err := s.userRepo.GetRoles(user.ID)
	if err != nil {
		return nil, err
	}

	roleNames := make([]string, 0, len(roles))
	for _, r := range roles {
		roleNames = append(roleNames, r.Name)
	}

	accessToken, newRefreshToken, err := pkg.GenerateTokens(user.ID, user.Username, roleNames)
	if err != nil {
		return nil, err
	}

	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    7200,
		User: UserInfo{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			Roles:    roleNames,
		},
	}, nil
}

func (s *authService) GetMe(userID uint64) (*UserInfo, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	roles, err := s.userRepo.GetRoles(user.ID)
	if err != nil {
		return nil, err
	}

	roleNames := make([]string, 0, len(roles))
	for _, r := range roles {
		roleNames = append(roleNames, r.Name)
	}

	return &UserInfo{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Roles:    roleNames,
	}, nil
}

// GetUserPermissions returns permission strings for a user (used by RBAC middleware).
func (s *authService) GetUserPermissions(userID uint64) ([]string, error) {
	perms, err := s.roleRepo.GetUserPermissions(userID)
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(perms))
	for _, p := range perms {
		result = append(result, p.Resource+"."+p.Action)
	}
	return result, nil
}

// Ensure authService implements a helper interface for permission loading.
type PermissionLoader interface {
	GetUserPermissions(userID uint64) ([]string, error)
}

var _ PermissionLoader = (*authService)(nil)

// NewPermissionFunc returns a function suitable for the LoadPermissions middleware.
func NewPermissionFunc(svc AuthService) func(uint64) ([]string, error) {
	if pl, ok := svc.(PermissionLoader); ok {
		return pl.GetUserPermissions
	}
	return func(uint64) ([]string, error) { return nil, nil }
}

// GetRolesForUser is a helper to get role names for a user.
func (s *authService) getRoleNames(user *model.User) ([]string, error) {
	roles, err := s.userRepo.GetRoles(user.ID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(roles))
	for _, r := range roles {
		names = append(names, r.Name)
	}
	return names, nil
}
