package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims embedded in the admit token. The backend validates this to confirm
// the user actually passed through the queue rather than hitting the backend directly.
type AdmitClaims struct {
	UserID string `json:"uid"`
	jwt.RegisteredClaims
}

// Sign creates a JWT admit token. The backend uses the same secret (injected via
// K8s secrets / ExternalSecrets) to verify it.
func Sign(secret, userID string, ttlSeconds int) (string, error) {
	claims := AdmitClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(ttlSeconds) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "surge-queue-engine",
			Subject:   userID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// Verify validates an admit token and returns the claims.
func Verify(secret, tokenStr string) (*AdmitClaims, error) {
	claims := &AdmitClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
