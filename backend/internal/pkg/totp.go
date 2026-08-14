package pkg

import (
	"errors"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// TOTP helpers for admin 2FA (RFC 6238, 30s window, 6 digits).

// GenerateTotpSecret creates a new base32 TOTP secret for enrollment.
func GenerateTotpSecret() (string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "dwz-shorturl",
		AccountName: "admin",
	})
	if err != nil {
		return "", err
	}
	return key.Secret(), nil
}

// TotpSecretURI builds the otpauth:// provisioning URI for QR enrollment.
func TotpSecretURI(secret, account string) string {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "dwz-shorturl",
		AccountName: account,
		Secret:      []byte(secret),
	})
	if err != nil {
		return ""
	}
	return key.URL()
}

// ValidateTotp verifies a 6-digit code against a base32 secret with a
// 1-window skew allowance (tolerates +/- 30s clock drift).
func ValidateTotp(code, secret string) bool {
	if secret == "" || len(code) < 6 {
		return false
	}
	ok, err := totp.ValidateCustom(code, secret, time.Now().UTC(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}

// ErrTotpRequired is returned when a 2FA-enabled account logs in without a
// valid TOTP code.
var ErrTotpRequired = errors.New("totp code required")
