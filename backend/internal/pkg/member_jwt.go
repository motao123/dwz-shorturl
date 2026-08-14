package pkg

import (
	"errors"
	"time"

	"dwz-admin/internal/config"

	"github.com/golang-jwt/jwt/v5"
)

// MemberClaims are claims for a public registered member (frontend user).
type MemberClaims struct {
	MemberID     uint64 `json:"member_id"`
	Username     string `json:"username"`
	TokenVersion int    `json:"token_version"`
	jwt.RegisteredClaims
}

var ErrMemberTokenInvalid = errors.New("member token invalid")

// GenerateMemberToken issues a member JWT signed with the member_secret.
func GenerateMemberToken(memberID uint64, username string) (string, error) {
	cfg := config.Get()
	if cfg.JWT.MemberSecret == "" {
		return "", ErrMemberTokenInvalid
	}
	now := time.Now()
	claims := MemberClaims{
		MemberID: memberID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "member",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(cfg.JWT.MemberSecret))
}

// ParseMemberToken validates a member JWT and returns its claims.
func ParseMemberToken(tokenStr string) (*MemberClaims, error) {
	cfg := config.Get()
	if cfg.JWT.MemberSecret == "" {
		return nil, ErrMemberTokenInvalid
	}
	token, err := jwt.ParseWithClaims(tokenStr, &MemberClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrMemberTokenInvalid
		}
		return []byte(cfg.JWT.MemberSecret), nil
	})
	if err != nil {
		return nil, ErrMemberTokenInvalid
	}
	claims, ok := token.Claims.(*MemberClaims)
	if !ok || !token.Valid {
		return nil, ErrMemberTokenInvalid
	}
	return claims, nil
}