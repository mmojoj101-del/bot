package jwt

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims represents the JWT claims for the fury-sms-gateway.
type Claims struct {
	Sub          string `json:"sub"`
	TenantID     string `json:"tenant_id,omitempty"`
	Role         string `json:"role,omitempty"`
	IsSuperAdmin bool   `json:"is_super_admin"`
	jwt.RegisteredClaims
}

// TokenPair contains access and refresh tokens.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// GenerateAccessToken creates a new JWT access token.
func GenerateAccessToken(secret, userID, tenantID, role string, isSuperAdmin bool, duration time.Duration, issuer, audience string) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		Sub:          userID,
		TenantID:     tenantID,
		Role:         role,
		IsSuperAdmin: isSuperAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			Issuer:    issuer,
			Audience:  []string{audience},
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return signed, nil
}

// GenerateRefreshToken creates a new JWT refresh token with a longer expiry.
func GenerateRefreshToken(secret, userID, tenantID string, duration time.Duration, issuer, audience string) (string, string, error) {
	now := time.Now().UTC()
	jti := uuid.New().String()

	claims := jwt.RegisteredClaims{
		ID:        jti,
		Issuer:    issuer,
		Audience:  []string{audience},
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
		NotBefore: jwt.NewNumericDate(now),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// Add custom claims
	token.Header["typ"] = "refresh"

	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", "", fmt.Errorf("sign refresh token: %w", err)
	}
	return signed, jti, nil
}

// ValidateToken validates a JWT token and returns the claims.
func ValidateToken(tokenString, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// ValidateRefreshToken validates a refresh token (using standard claims only).
func ValidateRefreshToken(tokenString, secret string) (*jwt.RegisteredClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse refresh token: %w", err)
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid refresh token")
	}

	return claims, nil
}
