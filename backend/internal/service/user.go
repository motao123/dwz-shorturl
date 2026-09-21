package service

import (
	"context"
	"errors"

	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	ErrUsernameExists = errors.New("username already exists")
	ErrEmailExists    = errors.New("email already exists")
	// ErrPrivilegeEscalation is returned when the caller tries to act on an
	// account whose effective permissions meet or exceed their own.
	ErrPrivilegeEscalation = errors.New("无权操作权限不低于自身的账号")
	// ErrSuperAdminOnly is returned for actions reserved for super_admin.
	ErrSuperAdminOnly = errors.New("该操作仅限超级管理员")
	// ErrEmptyRoles is returned when a caller tries to strip every role from an
	// account through the ordinary assign-roles path. That path used to accept an
	// empty array (gin's binding:"required" does not reject an empty slice) and
	// silently DELETE all of the target's roles (#1).
	ErrEmptyRoles = errors.New("role_ids 不能为空；如需清空角色请使用专用的移除接口")
)

type UserService interface {
	Create(username, email, password, displayName string) (*model.User, error)
	Update(id uint64, email, displayName, avatarURL string, status *int8) (*model.User, error)
	Delete(id uint64) error
	GetByID(id uint64) (*model.User, error)
	List(page, perPage int, keyword string) ([]model.User, int64, error)
	AssignRoles(userID uint64, roleIDs []uint64) error
	ResetPassword(id uint64, newPassword string) error
	// TotpStatus reports whether the user has 2FA enabled (never returns the secret).
	TotpStatus(id uint64) (bool, error)
	// ProvisionTotp generates a new TOTP secret for enrollment (not yet saved).
	ProvisionTotp(id uint64) (secret, uri string, err error)
	// EnableTotp validates a code against a provisioned secret and persists it.
	EnableTotp(id uint64, code, secret string) error
	// DisableTotp clears the user's TOTP secret.
	DisableTotp(id uint64) error

	// --- caller-aware variants (privilege guards) -------------------------
	//
	// These take the acting administrator's identity so the service can refuse
	// operations that would let a lower-privileged caller take over a stronger
	// account. The guard lives in the service, not the handler, because it needs
	// both the caller's and the target's full role sets (#2/#3).
	AssignRolesAs(actorID, userID uint64, roleIDs []uint64) error
	ResetPasswordAs(actorID, userID uint64, newPassword string) error
	UpdateAs(actorID, userID uint64, email, displayName, avatarURL string, status *int8) (*model.User, error)
	DeleteAs(actorID, userID uint64) error
	EnableTotpAs(actorID, targetID uint64, code, secret string) error
	DisableTotpAs(actorID, targetID uint64) error
	// IsSuperAdmin reports whether the given user holds super_admin.
	IsSuperAdmin(userID uint64) (bool, error)
	// EffectivePermissionCount counts distinct permissions reachable by the user
	// through all of their roles; used to compare two accounts' strength.
	EffectivePermissionCount(userID uint64) (int64, error)
}

type userService struct {
	userRepo repository.UserRepo
	// revocation 作废被改动账号当前已签发的全部会话（#42）。nil 表示本部署没有
	// 吊销存储，此时行为与改动前一致。
	revocation Revoker
	logger     *zap.Logger
}

// NewUserService returns the concrete type so callers can chain optional wiring
// (WithSessionRevocation); it still satisfies UserService.
func NewUserService(userRepo repository.UserRepo) *userService {
	return &userService{userRepo: userRepo}
}

// WithSessionRevocation wires session revocation for the mutations that change
// what an account is allowed to be. A nil revoker leaves behaviour unchanged.
func (s *userService) WithSessionRevocation(revocation Revoker, logger *zap.Logger) *userService {
	s.revocation = revocation
	s.logger = logger
	return s
}

// revokeSessions 在 DB 变更已经落库之后作废会话。失败不回滚那次变更——禁用账号本身
// 才是要紧的一步——但必须记成 Error，否则"以为已经吊销"比没吊销更糟。
func (s *userService) revokeSessions(userID uint64, reason string) {
	if s.revocation == nil || userID == 0 {
		return
	}
	if err := s.revocation.RevokeUser(context.Background(), userID); err != nil {
		if s.logger != nil {
			s.logger.Error("failed to revoke sessions after an account change",
				zap.Uint64("user_id", userID), zap.String("reason", reason), zap.Error(err))
		}
		return
	}
	if s.logger != nil {
		s.logger.Info("sessions revoked after an account change",
			zap.Uint64("user_id", userID), zap.String("reason", reason))
	}
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

	statusChanged := status != nil && *status != user.Status

	if status != nil {
		user.Status = *status
	}

	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}

	// #42：禁用（或任何状态变更）必须立刻对在授 token 生效，而不是等它 2 小时自然过期。
	if statusChanged {
		s.revokeSessions(user.ID, "status_changed")
	}

	return user, nil
}

func (s *userService) Delete(id uint64) error {
	if err := s.userRepo.SoftDelete(id); err != nil {
		return err
	}
	s.revokeSessions(id, "account_deleted")
	return nil
}

func (s *userService) GetByID(id uint64) (*model.User, error) {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, err
	}
	user.TotpEnabled = user.TotpSecret != ""
	if err := s.userRepo.LoadRoles(user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *userService) List(page, perPage int, keyword string) ([]model.User, int64, error) {
	users, total, err := s.userRepo.List(page, perPage, keyword)
	if err != nil {
		return nil, 0, err
	}
	ptrs := make([]*model.User, len(users))
	for i := range users {
		users[i].TotpEnabled = users[i].TotpSecret != ""
		ptrs[i] = &users[i]
	}
	// Roles must be part of the list response: the edit dialog seeds its role
	// picker from row.role_ids, and an empty picker is what erased roles (#1).
	if err := s.userRepo.LoadRoles(ptrs...); err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func (s *userService) AssignRoles(userID uint64, roleIDs []uint64) error {
	_, err := s.userRepo.FindByID(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		return err
	}
	if len(roleIDs) == 0 {
		return ErrEmptyRoles
	}
	if err := s.userRepo.SetRoles(userID, roleIDs); err != nil {
		return err
	}
	// #42：角色写在 claims 里，改角色后旧 token 仍带着旧权限，必须作废。
	s.revokeSessions(userID, "roles_changed")
	return nil
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
	if err := s.userRepo.Update(user); err != nil {
		return err
	}
	s.revokeSessions(user.ID, "password_reset")
	return nil
}

func (s *userService) TotpStatus(id uint64) (bool, error) {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return false, err
	}
	return user.TotpSecret != "", nil
}

func (s *userService) ProvisionTotp(id uint64) (string, string, error) {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return "", "", err
	}
	secret, err := pkg.GenerateTotpSecret()
	if err != nil {
		return "", "", err
	}
	uri := pkg.TotpSecretURI(secret, user.Username)
	return secret, uri, nil
}

func (s *userService) EnableTotp(id uint64, code, secret string) error {
	if !pkg.ValidateTotp(code, secret) {
		return errors.New("验证码无效，请重试")
	}
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return err
	}
	user.TotpSecret = secret
	return s.userRepo.Update(user)
}

func (s *userService) DisableTotp(id uint64) error {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return err
	}
	user.TotpSecret = ""
	return s.userRepo.Update(user)
}

// ---------------------------------------------------------------------------
// Privilege guards (#2/#3)
//
// Before these existed, any account holding the single permission
// `users:update` could take over an administrator:
//
//	POST /users/1/totp/enable {secret: S, code: TOTP(S)}   // bind a secret I control
//	PUT  /users/1/password   {password: "pwned"}          // then set the password
//
// and could self-escalate by creating a role with every permission and assigning
// it to itself. The rules below close both paths. They are deliberately simple
// and amount to one sentence: you may never act on an account that is at least
// as powerful as your own, and only a super_admin may hand out super_admin.
// ---------------------------------------------------------------------------

// isSuperAdminRoleName is the role name that unconditionally grants everything
// (see repository/role.go: the permission lookup short-circuits on it).
const isSuperAdminRoleName = "super_admin"

// IsSuperAdmin reports whether the user holds the super_admin role.
func (s *userService) IsSuperAdmin(userID uint64) (bool, error) {
	roles, err := s.userRepo.GetRoles(userID)
	if err != nil {
		return false, err
	}
	for _, r := range roles {
		if r.Name == isSuperAdminRoleName {
			return true, nil
		}
	}
	return false, nil
}

// EffectivePermissionCount counts the distinct permissions a user reaches via
// all of their roles. It is the comparable "strength" of an account.
func (s *userService) EffectivePermissionCount(userID uint64) (int64, error) {
	roles, err := s.userRepo.GetRoles(userID)
	if err != nil {
		return 0, err
	}
	// super_admin resolves to every permission by name, so it is always the
	// maximum; report a number larger than any real count.
	for _, r := range roles {
		if r.Name == isSuperAdminRoleName {
			return 1 << 30, nil
		}
	}
	seen := map[uint64]struct{}{}
	for _, r := range roles {
		perms, err := s.userRepo.GetPermissions(r.ID)
		if err != nil {
			return 0, err
		}
		for _, p := range perms {
			seen[p.ID] = struct{}{}
		}
	}
	return int64(len(seen)), nil
}

// assertCanActOn is the single guard every caller-aware variant funnels through.
// It refuses when the target is stronger than (or equal in strength to) the
// actor, which also makes self-demotion/self-deletion via this path impossible.
func (s *userService) assertCanActOn(actorID, targetID uint64) error {
	if actorID == 0 {
		// No acting identity (internal caller / tests). Guards are enforced at the
		// HTTP boundary where an identity always exists; do not block background use.
		return nil
	}
	if actorID == targetID {
		return ErrPrivilegeEscalation
	}

	actorSuper, err := s.IsSuperAdmin(actorID)
	if err != nil {
		return err
	}
	targetSuper, err := s.IsSuperAdmin(targetID)
	if err != nil {
		return err
	}
	if targetSuper && !actorSuper {
		return ErrPrivilegeEscalation
	}
	if actorSuper {
		// A super_admin may act on anyone else (including other super_admins);
		// self-action is already excluded above.
		return nil
	}

	actorCount, err := s.EffectivePermissionCount(actorID)
	if err != nil {
		return err
	}
	targetCount, err := s.EffectivePermissionCount(targetID)
	if err != nil {
		return err
	}
	if targetCount >= actorCount {
		return ErrPrivilegeEscalation
	}
	return nil
}

// assertCanGrantRoles ensures the actor may hand out exactly this role set:
// only a super_admin may grant super_admin, and nobody may grant a role richer
// than what they already hold (otherwise "create a role with all permissions,
// assign it to myself" is a one-click escalation) (#3).
func (s *userService) assertCanGrantRoles(actorID uint64, roleIDs []uint64) error {
	if actorID == 0 {
		return nil
	}
	actorSuper, err := s.IsSuperAdmin(actorID)
	if err != nil {
		return err
	}
	actorCount, err := s.EffectivePermissionCount(actorID)
	if err != nil {
		return err
	}

	for _, rid := range roleIDs {
		role, err := s.userRepo.FindRoleByID(rid)
		if err != nil {
			return errors.New("角色不存在")
		}
		if role.Name == isSuperAdminRoleName && !actorSuper {
			return ErrSuperAdminOnly
		}
		if actorSuper {
			continue
		}
		if role.IsSystem == 1 {
			// Non-super actors may only assign roles strictly weaker than their own.
			continue
		}
		perms, err := s.userRepo.GetPermissions(rid)
		if err != nil {
			return err
		}
		if int64(len(perms)) >= actorCount {
			return ErrPrivilegeEscalation
		}
	}
	return nil
}

// AssignRolesAs assigns roles after checking both the target's strength and the
// grant set against the actor.
func (s *userService) AssignRolesAs(actorID, userID uint64, roleIDs []uint64) error {
	if err := s.assertCanActOn(actorID, userID); err != nil {
		return err
	}
	if err := s.assertCanGrantRoles(actorID, roleIDs); err != nil {
		return err
	}
	return s.AssignRoles(userID, roleIDs)
}

// ResetPasswordAs resets another account's password, refusing stronger targets.
func (s *userService) ResetPasswordAs(actorID, userID uint64, newPassword string) error {
	if err := s.assertCanActOn(actorID, userID); err != nil {
		return err
	}
	return s.ResetPassword(userID, newPassword)
}

// UpdateAs edits profile/status of another account, refusing stronger targets.
func (s *userService) UpdateAs(actorID, userID uint64, email, displayName, avatarURL string, status *int8) (*model.User, error) {
	if err := s.assertCanActOn(actorID, userID); err != nil {
		return nil, err
	}
	return s.Update(userID, email, displayName, avatarURL, status)
}

// DeleteAs deletes another account, refusing stronger targets.
func (s *userService) DeleteAs(actorID, userID uint64) error {
	if err := s.assertCanActOn(actorID, userID); err != nil {
		return err
	}
	return s.Delete(userID)
}

// EnableTotpAs binds a TOTP secret to another account. The secret arrives in the
// request body and was previously trusted verbatim, which let the caller bind a
// secret they control and then log in as the victim. With the strength guard, an
// attacker can now only do this to an account weaker than their own — i.e. one
// they could already take over — so the escalation is gone.
func (s *userService) EnableTotpAs(actorID, targetID uint64, code, secret string) error {
	if err := s.assertCanActOn(actorID, targetID); err != nil {
		return err
	}
	return s.EnableTotp(targetID, code, secret)
}

// DisableTotpAs removes 2FA from another account, refusing stronger targets.
func (s *userService) DisableTotpAs(actorID, targetID uint64) error {
	if err := s.assertCanActOn(actorID, targetID); err != nil {
		return err
	}
	return s.DisableTotp(targetID)
}
