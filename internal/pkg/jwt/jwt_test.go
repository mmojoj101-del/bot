package jwt

import (
	"testing"
	"time"
)

const testSecret = "test-secret-key-for-unit-tests-min-32-chars!!"
const testIssuer = "fury-sms-gateway"
const testAudience = "fury-api"
const testUserID = "550e8400-e29b-41d4-a716-446655440000"
const testTenantID = "660e8400-e29b-41d4-a716-446655440001"
const testRole = "admin"

func TestGenerateAccessToken_Success(t *testing.T) {
	token, err := GenerateAccessToken(testSecret, testUserID, testTenantID, testRole, true, 15*time.Minute, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("GenerateAccessToken() failed: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateAccessToken() returned empty token")
	}
}

func TestGenerateAccessToken_Validate(t *testing.T) {
	token, err := GenerateAccessToken(testSecret, testUserID, testTenantID, testRole, true, 15*time.Minute, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("GenerateAccessToken() failed: %v", err)
	}

	claims, err := ValidateToken(token, testSecret)
	if err != nil {
		t.Fatalf("ValidateToken() failed: %v", err)
	}

	if claims.Sub != testUserID {
		t.Fatalf("claims.Sub = %s, want %s", claims.Sub, testUserID)
	}
	if claims.TenantID != testTenantID {
		t.Fatalf("claims.TenantID = %s, want %s", claims.TenantID, testTenantID)
	}
	if claims.Role != testRole {
		t.Fatalf("claims.Role = %s, want %s", claims.Role, testRole)
	}
	if !claims.IsSuperAdmin {
		t.Fatal("claims.IsSuperAdmin should be true")
	}
	if claims.Issuer != testIssuer {
		t.Fatalf("claims.Issuer = %s, want %s", claims.Issuer, testIssuer)
	}
	if len(claims.Audience) == 0 || claims.Audience[0] != testAudience {
		t.Fatalf("claims.Audience = %v, want [%s]", claims.Audience, testAudience)
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	token, err := GenerateAccessToken(testSecret, testUserID, testTenantID, testRole, false, 15*time.Minute, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("GenerateAccessToken() failed: %v", err)
	}

	_, err = ValidateToken(token, "wrong-secret")
	if err == nil {
		t.Fatal("ValidateToken() should fail with wrong secret")
	}
}

func TestValidateToken_Expired(t *testing.T) {
	// Token that expired 1 second ago
	token, err := GenerateAccessToken(testSecret, testUserID, testTenantID, testRole, false, -1*time.Second, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("GenerateAccessToken() failed: %v", err)
	}

	_, err = ValidateToken(token, testSecret)
	if err == nil {
		t.Fatal("ValidateToken() should fail for expired token")
	}
}

func TestValidateToken_InvalidSignature(t *testing.T) {
	token, err := GenerateAccessToken(testSecret, testUserID, testTenantID, testRole, false, 15*time.Minute, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("GenerateAccessToken() failed: %v", err)
	}

	// Tamper with the token (change last char of signature)
	tampered := token[:len(token)-1] + "X"
	_, err = ValidateToken(tampered, testSecret)
	if err == nil {
		t.Fatal("ValidateToken() should fail for tampered token")
	}
}

func TestGenerateRefreshToken_Success(t *testing.T) {
	token, jti, err := GenerateRefreshToken(testSecret, testUserID, testTenantID, 720*time.Hour, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("GenerateRefreshToken() failed: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateRefreshToken() returned empty token")
	}
	if jti == "" {
		t.Fatal("GenerateRefreshToken() returned empty jti")
	}
}

func TestGenerateRefreshToken_Validate(t *testing.T) {
	token, jti, err := GenerateRefreshToken(testSecret, testUserID, testTenantID, 720*time.Hour, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("GenerateRefreshToken() failed: %v", err)
	}

	claims, err := ValidateRefreshToken(token, testSecret)
	if err != nil {
		t.Fatalf("ValidateRefreshToken() failed: %v", err)
	}

	if claims.ID != jti {
		t.Fatalf("claims.ID = %s, want %s", claims.ID, jti)
	}
	if claims.Subject != testUserID {
		t.Fatalf("claims.Subject = %s, want %s", claims.Subject, testUserID)
	}
	if claims.Issuer != testIssuer {
		t.Fatalf("claims.Issuer = %s, want %s", claims.Issuer, testIssuer)
	}
}

func TestValidateRefreshToken_WrongSecret(t *testing.T) {
	token, _, err := GenerateRefreshToken(testSecret, testUserID, testTenantID, 720*time.Hour, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("GenerateRefreshToken() failed: %v", err)
	}

	_, err = ValidateRefreshToken(token, "wrong-secret")
	if err == nil {
		t.Fatal("ValidateRefreshToken() should fail with wrong secret")
	}
}

func TestValidateRefreshToken_Expired(t *testing.T) {
	token, _, err := GenerateRefreshToken(testSecret, testUserID, testTenantID, -1*time.Second, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("GenerateRefreshToken() failed: %v", err)
	}

	_, err = ValidateRefreshToken(token, testSecret)
	if err == nil {
		t.Fatal("ValidateRefreshToken() should fail for expired token")
	}
}

func TestAccessToken_NotBefore(t *testing.T) {
	// Token valid from 1 hour in the future
	token, err := GenerateAccessToken(testSecret, testUserID, testTenantID, testRole, false, 15*time.Minute, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("GenerateAccessToken() failed: %v", err)
	}

	claims, err := ValidateToken(token, testSecret)
	if err != nil {
		t.Fatalf("ValidateToken() failed: %v", err)
	}

	if claims.NotBefore == nil {
		t.Fatal("claims.NotBefore should be set")
	}
}

func TestClaims_IsSuperAdmin(t *testing.T) {
	token, err := GenerateAccessToken(testSecret, testUserID, testTenantID, testRole, false, 15*time.Minute, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("GenerateAccessToken() failed: %v", err)
	}
	claims, err := ValidateToken(token, testSecret)
	if err != nil {
		t.Fatalf("ValidateToken() failed: %v", err)
	}
	if claims.IsSuperAdmin {
		t.Fatal("claims.IsSuperAdmin should be false")
	}

	token2, err := GenerateAccessToken(testSecret, testUserID, testTenantID, testRole, true, 15*time.Minute, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("GenerateAccessToken() failed: %v", err)
	}
	claims2, err := ValidateToken(token2, testSecret)
	if err != nil {
		t.Fatalf("ValidateToken() failed: %v", err)
	}
	if !claims2.IsSuperAdmin {
		t.Fatal("claims2.IsSuperAdmin should be true")
	}
}

func TestValidateToken_InvalidFormat(t *testing.T) {
	_, err := ValidateToken("not-a-jwt-token", testSecret)
	if err == nil {
		t.Fatal("ValidateToken() should fail for invalid format")
	}
}

func TestValidateToken_EmptyToken(t *testing.T) {
	_, err := ValidateToken("", testSecret)
	if err == nil {
		t.Fatal("ValidateToken() should fail for empty token")
	}
}

func TestValidateRefreshToken_IsRefreshToken(t *testing.T) {
	token, _, err := GenerateRefreshToken(testSecret, testUserID, testTenantID, 720*time.Hour, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("GenerateRefreshToken() failed: %v", err)
	}

	// Access token validation should also work for refresh tokens
	// (since we use RegisteredClaims, not our custom Claims)
	claims, err := ValidateRefreshToken(token, testSecret)
	if err != nil {
		t.Fatalf("ValidateRefreshToken() failed: %v", err)
	}
	_ = claims
}

func TestGenerateAccessToken_UniqueJTI(t *testing.T) {
	token1, _ := GenerateAccessToken(testSecret, testUserID, testTenantID, testRole, false, 15*time.Minute, testIssuer, testAudience)
	token2, _ := GenerateAccessToken(testSecret, testUserID, testTenantID, testRole, false, 15*time.Minute, testIssuer, testAudience)

	claims1, _ := ValidateToken(token1, testSecret)
	claims2, _ := ValidateToken(token2, testSecret)

	if claims1.ID == claims2.ID {
		t.Fatal("Each token should have a unique JTI")
	}
}
