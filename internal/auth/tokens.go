package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/refa-optical/backend/internal/models"
	"time"
)

type Claims struct {
	UserID string `json:"uid"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func IssueAccess(user models.User, secret string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{UserID: user.ID, Email: user.Email, Role: user.Role, RegisteredClaims: jwt.RegisteredClaims{Subject: user.ID, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(ttl)), Issuer: "refa-optical-api"}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
func ParseAccess(token, secret string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	}, jwt.WithIssuer("refa-optical-api"))
	if err != nil || !parsed.Valid {
		return nil, errors.New("invalid access token")
	}
	return claims, nil
}
func NewRefreshToken() (string, string, error) {
	var raw [48]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	sum := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(sum[:]), nil
}
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
