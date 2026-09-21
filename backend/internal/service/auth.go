package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"
)

// User-facing auth errors. Returned verbatim to the browser (the login handler
// forwards err.Error() into the JSON envelope), so they are in Chinese to match
// the admin UI — they used to be English while every surrounding message was
// Chinese.
var (
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrUserDisabled       = errors.New("账号已被禁用")
	ErrUserNotFound       = errors.New("用户不存在")
	// ErrAccountLocked is returned while a username is in its lockout window.
	ErrAccountLocked = errors.New("账号尝试次数过多已临时锁定，请稍后再试")
)

// Account-level lockout policy for the admin login endpoint.
//
// The per-IP limiter in the router only slows an attacker who keeps one IP:
// rotating across a handful of addresses lets them try max-1 times from each and
// never trip it. Locking on the *username* is what actually bounds a brute-force
// run against a known account. The frontend member login has had this since the
// beginning (see includes/auth.php member_login), so the highest-privilege entry
// point was the one left unprotected.
const (
	adminLoginMaxFailures = 8
	adminLoginLockWindow  = 15 * time.Minute
	adminLoginKeyPrefix   = "admin-login-fail:"
)

type LoginResult struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int64    `json:"expires_in"`
	User         UserInfo `json:"user"`
}

type UserInfo struct {
	ID          uint64   `json:"id"`
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	AvatarUrl   string   `json:"avatar_url"`
	Status      int8     `json:"status"`
	LastLoginAt *string  `json:"last_login_at"`
	LastLoginIP *string  `json:"last_login_ip"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

type AuthService interface {
	Login(username, password, totpCode, ip string) (*LoginResult, error)
	Refresh(refreshToken string) (*LoginResult, error)
	GetMe(userID uint64) (*UserInfo, error)
}

type authService struct {
	userRepo repository.UserRepo
	roleRepo repository.RoleRepo
	// limiter backs account-level failure counting. Nil disables lockout so
	// unit tests and a Redis-less deployment keep working.
	limiter *pkg.RateLimiter
	// sessionCheck reports whether a token is still usable (logout blacklist +
	// per-user revocation watermark). Nil is treated as "nothing revoked".
	sessionCheck pkg.SessionChecker
}

// NewAuthService returns the concrete service so callers can chain options
// (WithLoginLimiter); it still satisfies AuthService.
func NewAuthService(userRepo repository.UserRepo, roleRepo repository.RoleRepo) *authService {
	return &authService{userRepo: userRepo, roleRepo: roleRepo}
}

// WithLoginLimiter enables account-level lockout on the service. Passing nil
// leaves the login path unchanged.
// WithSessionCheck wires the revocation check used by Refresh, so a logged-out
// refresh token cannot be exchanged for a fresh access token (#7), and so a
// disabled / role-changed / password-reset account cannot mint a new session
// from a refresh token minted before that change (#42). A nil check keeps the
// service usable in tests and without Redis.
func (s *authService) WithSessionCheck(fn pkg.SessionChecker) *authService {
	s.sessionCheck = fn
	return s
}

func (s *authService) WithLoginLimiter(l *pkg.RateLimiter) *authService {
	s.limiter = l
	return s
}

// loginFailKey namespaces the failure counter by lower-cased username so
// "Admin" and "admin" cannot be used to get two independent budgets.
func loginFailKey(username string) string {
	return adminLoginKeyPrefix + strings.ToLower(strings.TrimSpace(username))
}

func (s *authService) Login(username, password, totpCode, ip string) (*LoginResult, error) {
	ctx := context.Background()

	// Account-level lockout, checked before the password is compared so a locked
	// account costs no bcrypt work (and cannot be probed for timing).
	if s.locked(ctx, username) {
		return nil, ErrAccountLocked
	}

	user, err := s.userRepo.FindByUsername(username)
	if err != nil {
		// Unknown username still burns a count so the endpoint cannot be used to
		// enumerate which usernames exist via lockout behaviour alone.
		s.noteFailure(ctx, username)
		return nil, ErrInvalidCredentials
	}

	if user.Status != 1 {
		return nil, ErrUserDisabled
	}

	if !pkg.CheckPassword(user.PasswordHash, password) {
		s.noteFailure(ctx, username)
		return nil, ErrInvalidCredentials
	}

	// 2FA: accounts with an enrolled TOTP secret must present a valid code.
	if user.TotpSecret != "" {
		if !pkg.ValidateTotp(totpCode, user.TotpSecret) {
			// A wrong TOTP is a failed login: count it, otherwise a valid
			// password alone would let an attacker brute-force the 6 digits.
			s.noteFailure(ctx, username)
			// 区分「未提供/错误」两种：未提供时前端展示验证码输入框。
			if totpCode == "" {
				return nil, pkg.ErrTotpRequired
			}
			return nil, errors.New("两步验证码不正确")
		}
	}

	// Successful login clears the accumulated failures for this account.
	s.resetFailures(ctx, username)

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
		User:         buildUserInfo(user, roleNames),
	}, nil
}

// locked reports whether the username has exhausted its failure budget.
//
// It reads the current failure count without consuming budget (a plain Allow
// would make every attempt — successful or not — eat into the allowance).
// Fail-open on limiter errors: a Redis outage must not lock every admin out.
func (s *authService) locked(ctx context.Context, username string) bool {
	if s.limiter == nil {
		return false
	}
	n, err := s.limiter.Count(ctx, loginFailKey(username))
	if err != nil {
		return false
	}
	return n >= adminLoginMaxFailures
}

// noteFailure records one failed attempt for the username.
func (s *authService) noteFailure(ctx context.Context, username string) {
	if s.limiter == nil {
		return
	}
	_, _ = s.limiter.Allow(ctx, loginFailKey(username), adminLoginMaxFailures, adminLoginLockWindow)
}

// resetFailures clears the failure counter after a successful login.
func (s *authService) resetFailures(ctx context.Context, username string) {
	if s.limiter == nil {
		return
	}
	_ = s.limiter.Reset(ctx, loginFailKey(username))
}

func (s *authService) Refresh(refreshToken string) (*LoginResult, error) {
	claims, err := pkg.ParseToken(refreshToken)
	if err != nil {
		return nil, errors.New("refresh_token 无效或已过期")
	}
	// P1-4: only refresh-type tokens may be exchanged for a new pair; an access
	// token replayed here would otherwise escalate its own lifetime.
	if claims.TokenType != pkg.TokenTypeRefresh {
		return nil, errors.New("refresh_token 无效或已过期")
	}
	// #7: honour logout for refresh tokens too. Without this, blacklisting the
	// access jti at logout still let the (unrevoked) refresh token mint a new
	// access token for the rest of its 7-day lifetime.
	// #42: the same call also carries the per-user watermark, so a refresh token
	// minted before a disable/role-change/password-reset is refused as well.
	// #38: an unavailable revocation store is treated as revoked here — a session
	// that cannot be checked must not be used to mint a longer-lived credential.
	if s.sessionCheck != nil {
		var issuedAt time.Time
		if claims.IssuedAt != nil {
			issuedAt = claims.IssuedAt.Time
		}
		if s.sessionCheck(claims.UserID, claims.ID, issuedAt) != pkg.SessionActive {
			return nil, errors.New("refresh_token 无效或已过期")
		}
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
		User:         buildUserInfo(user, roleNames),
	}, nil
}

// buildUserInfo assembles the full user info payload for login/refresh
// responses (mirrors GetMe so the client always sees status/display fields).
func buildUserInfo(user *model.User, roleNames []string) UserInfo {
	var lastLoginAt *string
	if user.LastLoginAt != nil {
		s := user.LastLoginAt.Format("2006-01-02T15:04:05Z")
		lastLoginAt = &s
	}
	var lastLoginIP *string
	if user.LastLoginIP != "" {
		lastLoginIP = &user.LastLoginIP
	}
	return UserInfo{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		AvatarUrl:   user.AvatarURL,
		Status:      user.Status,
		LastLoginAt: lastLoginAt,
		LastLoginIP: lastLoginIP,
		Roles:       roleNames,
	}
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

	// Load permissions
	perms, err := s.GetUserPermissions(userID)
	if err != nil {
		perms = []string{}
	}

	var lastLoginAt *string
	if user.LastLoginAt != nil {
		s := user.LastLoginAt.Format("2006-01-02T15:04:05Z")
		lastLoginAt = &s
	}
	var lastLoginIP *string
	if user.LastLoginIP != "" {
		lastLoginIP = &user.LastLoginIP
	}

	return &UserInfo{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		AvatarUrl:   user.AvatarURL,
		Status:      user.Status,
		LastLoginAt: lastLoginAt,
		LastLoginIP: lastLoginIP,
		Roles:       roleNames,
		Permissions: perms,
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
