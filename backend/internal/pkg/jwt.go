package pkg

import (
	"errors"
	"time"

	"dwz-admin/internal/config"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	UserID    uint64   `json:"user_id"`
	Username  string   `json:"username"`
	Roles     []string `json:"roles"`
	TokenType string   `json:"token_type,omitempty"`
	jwt.RegisteredClaims
}

// Token types prevent an access token from being replayed as a refresh token
// (token confusion). Old tokens minted before this field existed carry no type
// and are treated as access tokens.
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

var (
	ErrTokenExpired = errors.New("token expired")
	ErrTokenInvalid = errors.New("token invalid")
)

func GenerateTokens(userID uint64, username string, roles []string) (access, refresh string, err error) {
	cfg := config.Get()
	now := time.Now()

	accessClaims := Claims{
		UserID:    userID,
		Username:  username,
		Roles:     roles,
		TokenType: TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(cfg.JWT.AccessExpiry)),
			ID:        uuid.New().String(),
		},
	}

	refreshClaims := Claims{
		UserID:    userID,
		Username:  username,
		Roles:     roles,
		TokenType: TokenTypeRefresh,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(cfg.JWT.RefreshExpiry)),
			ID:        uuid.New().String(),
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	access, err = accessToken.SignedString([]byte(cfg.JWT.Secret))
	if err != nil {
		return "", "", err
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refresh, err = refreshToken.SignedString([]byte(cfg.JWT.Secret))
	if err != nil {
		return "", "", err
	}

	return access, refresh, nil
}

func ParseToken(tokenStr string) (*Claims, error) {
	cfg := config.Get()

	// Build the list of secrets to try: the current signing secret first, then
	// any previous secrets (kept valid during key rotation).
	secrets := []string{cfg.JWT.Secret}
	secrets = append(secrets, cfg.JWT.PreviousSecrets...)

	var lastErr error
	for _, secret := range secrets {
		token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, ErrTokenInvalid
			}
			return []byte(secret), nil
		})
		if err != nil {
			lastErr = err
			continue
		}
		claims, ok := token.Claims.(*Claims)
		if !ok || !token.Valid {
			lastErr = ErrTokenInvalid
			continue
		}
		return claims, nil
	}

	if errors.Is(lastErr, jwt.ErrTokenExpired) {
		return nil, ErrTokenExpired
	}
	return nil, ErrTokenInvalid
}
