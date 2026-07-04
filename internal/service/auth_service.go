package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
	"github.com/raghna/fury-sms-gateway/internal/config"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
	jwtpkg "github.com/raghna/fury-sms-gateway/internal/pkg/jwt"
	"github.com/raghna/fury-sms-gateway/internal/pkg/password"
)

// AuthService handles authentication and authorization.
type AuthService struct {
	usersRepo       domain.UserRepository
	membersRepo     domain.TenantMemberRepository
	apiKeysRepo     domain.APIKeyRepository
	refreshTokenRepo domain.RefreshTokenRepository
	auditRepo       domain.AuditLogRepository
	eventBus        event.Bus
	cfg             *config.JWTConfig
	clock           domain.Clock
}

// NewAuthService creates a new authentication service.
func NewAuthService(
	usersRepo domain.UserRepository,
	membersRepo domain.TenantMemberRepository,
	apiKeysRepo domain.APIKeyRepository,
	refreshTokenRepo domain.RefreshTokenRepository,
	auditRepo domain.AuditLogRepository,
	eventBus event.Bus,
	cfg *config.JWTConfig,
	clock domain.Clock,
) *AuthService {
	return &AuthService{
		usersRepo:       usersRepo,
		membersRepo:     membersRepo,
		apiKeysRepo:     apiKeysRepo,
		refreshTokenRepo: refreshTokenRepo,
		auditRepo:       auditRepo,
		eventBus:        eventBus,
		cfg:             cfg,
		clock:           clock,
	}
}

// LoginRequest represents a login request.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse represents a login response.
type LoginResponse struct {
	User          *domain.User        `json:"user"`
	AccessToken   string              `json:"access_token"`
	RefreshToken  string              `json:"refresh_token"`
	ExpiresIn     int64               `json:"expires_in"`
	Tenants       []domain.TenantMember `json:"tenants"`
}

// Login authenticates a user by email and password.
func (s *AuthService) Login(ctx context.Context, req LoginRequest, requestID, ipAddress string) (*LoginResponse, error) {
	user, err := s.usersRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	if user.Status != domain.UserStatusActive {
		return nil, domain.ErrSuspended
	}

	if !password.Verify(req.Password, user.PasswordHash) {
		s.logAudit(ctx, nil, &user.ID, requestID, domain.AuditActionLogin, "login.failed", ipAddress)
		return nil, domain.ErrUnauthorized
	}

	// Get user's tenant memberships
	members, err := s.membersRepo.ListByUser(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("list memberships: %w", err)
	}

	// Get the primary tenant (first one)
	var activeTenantID, activeRole string
	if len(members) > 0 {
		activeTenantID = members[0].TenantID
		activeRole = string(members[0].Role)
	}

	accessToken, err := jwtpkg.GenerateAccessToken(
		s.cfg.Secret, user.ID, activeTenantID, activeRole, user.IsSuperAdmin,
		s.cfg.AccessDuration, s.cfg.Issuer, s.cfg.Audience,
	)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshTokenStr, jti, err := jwtpkg.GenerateRefreshToken(
		s.cfg.Secret, user.ID, activeTenantID,
		s.cfg.RefreshDuration, s.cfg.Issuer, s.cfg.Audience,
	)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	// Store refresh token
	refreshTokenHash := hashToken(refreshTokenStr, s.cfg.Secret)
	rt := &domain.RefreshToken{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		TenantID:  activeTenantID,
		TokenHash: refreshTokenHash,
		JTI:       jti,
		IPAddress: ipAddress,
		ExpiresAt: s.clock.Now().Add(s.cfg.RefreshDuration),
	}
	if err := s.refreshTokenRepo.Create(ctx, rt); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	// Update last login
	_ = s.usersRepo.UpdateLastLogin(ctx, user.ID)

	s.logAudit(ctx, &activeTenantID, &user.ID, requestID, domain.AuditActionLogin, "login.success", ipAddress)

	s.eventBus.Publish(event.Event{
		ID:    uuid.New().String(),
		Type:  event.EventUserLoggedIn,
		Payload: map[string]interface{}{
			"user_id":   user.ID,
			"tenant_id": activeTenantID,
		},
		Timestamp: s.clock.Now(),
	})

	return &LoginResponse{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshTokenStr,
		ExpiresIn:    int64(s.cfg.AccessDuration.Seconds()),
		Tenants:      members,
	}, nil
}

// RefreshToken refreshes an access token using a refresh token.
func (s *AuthService) RefreshToken(ctx context.Context, refreshTokenStr, requestID, ipAddress string) (*LoginResponse, error) {
	// Validate the refresh token JWT
	claims, err := jwtpkg.ValidateRefreshToken(refreshTokenStr, s.cfg.Secret)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	// Get the stored refresh token
	storedToken, err := s.refreshTokenRepo.GetByJTI(ctx, claims.ID)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	if storedToken.IsExpired() || storedToken.IsRevoked() {
		return nil, domain.ErrUnauthorized
	}

	// Verify the hash
	expectedHash := hashToken(refreshTokenStr, s.cfg.Secret)
	if storedToken.TokenHash != expectedHash {
		return nil, domain.ErrUnauthorized
	}

	// Revoke the old refresh token
	if err := s.refreshTokenRepo.Revoke(ctx, claims.ID); err != nil {
		return nil, fmt.Errorf("revoke old refresh token: %w", err)
	}

	// Get user
	user, err := s.usersRepo.GetByID(ctx, storedToken.UserID)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	if user.Status != domain.UserStatusActive {
		return nil, domain.ErrSuspended
	}

	// Get memberships
	members, err := s.membersRepo.ListByUser(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("list memberships: %w", err)
	}

	var activeRole string
	for _, m := range members {
		if m.TenantID == storedToken.TenantID {
			activeRole = string(m.Role)
			break
		}
	}

	// Generate new tokens
	accessToken, err := jwtpkg.GenerateAccessToken(
		s.cfg.Secret, user.ID, storedToken.TenantID, activeRole, user.IsSuperAdmin,
		s.cfg.AccessDuration, s.cfg.Issuer, s.cfg.Audience,
	)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	newRefreshTokenStr, newJTI, err := jwtpkg.GenerateRefreshToken(
		s.cfg.Secret, user.ID, storedToken.TenantID,
		s.cfg.RefreshDuration, s.cfg.Issuer, s.cfg.Audience,
	)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	// Store new refresh token
	newHash := hashToken(newRefreshTokenStr, s.cfg.Secret)
	rt := &domain.RefreshToken{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		TenantID:  storedToken.TenantID,
		TokenHash: newHash,
		JTI:       newJTI,
		IPAddress: ipAddress,
		ExpiresAt: s.clock.Now().Add(s.cfg.RefreshDuration),
	}
	if err := s.refreshTokenRepo.Create(ctx, rt); err != nil {
		return nil, fmt.Errorf("store new refresh token: %w", err)
	}

	s.logAudit(ctx, &storedToken.TenantID, &user.ID, requestID, domain.AuditActionLogin, "token.refreshed", ipAddress)

	return &LoginResponse{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: newRefreshTokenStr,
		ExpiresIn:    int64(s.cfg.AccessDuration.Seconds()),
		Tenants:      members,
	}, nil
}

// Logout revokes a refresh token.
func (s *AuthService) Logout(ctx context.Context, refreshTokenStr, requestID, ipAddress string) error {
	claims, err := jwtpkg.ValidateRefreshToken(refreshTokenStr, s.cfg.Secret)
	if err != nil {
		// Even if the token is expired/invalid, still try to clean up
		return nil
	}

	_ = s.refreshTokenRepo.Revoke(ctx, claims.ID)
	return nil
}

// SwitchTenant switches the active tenant for a user.
func (s *AuthService) SwitchTenant(ctx context.Context, userID, targetTenantID, requestID, ipAddress string) (*LoginResponse, error) {
	// Verify the user is a member of the target tenant
	member, err := s.membersRepo.Get(ctx, targetTenantID, userID)
	if err != nil {
		return nil, domain.ErrForbidden
	}

	user, err := s.usersRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	members, err := s.membersRepo.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list memberships: %w", err)
	}

	accessToken, err := jwtpkg.GenerateAccessToken(
		s.cfg.Secret, user.ID, targetTenantID, string(member.Role), user.IsSuperAdmin,
		s.cfg.AccessDuration, s.cfg.Issuer, s.cfg.Audience,
	)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshTokenStr, jti, err := jwtpkg.GenerateRefreshToken(
		s.cfg.Secret, user.ID, targetTenantID,
		s.cfg.RefreshDuration, s.cfg.Issuer, s.cfg.Audience,
	)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	refreshTokenHash := hashToken(refreshTokenStr, s.cfg.Secret)
	rt := &domain.RefreshToken{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		TenantID:  targetTenantID,
		TokenHash: refreshTokenHash,
		JTI:       jti,
		IPAddress: ipAddress,
		ExpiresAt: s.clock.Now().Add(s.cfg.RefreshDuration),
	}
	if err := s.refreshTokenRepo.Create(ctx, rt); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	s.logAudit(ctx, &targetTenantID, &userID, requestID, domain.AuditActionSwitchTenant, "tenant.switched", ipAddress)

	return &LoginResponse{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshTokenStr,
		ExpiresIn:    int64(s.cfg.AccessDuration.Seconds()),
		Tenants:      members,
	}, nil
}

// ValidateAPIKey validates an API key and returns the associated tenant ID and key.
func (s *AuthService) ValidateAPIKey(ctx context.Context, key string, requestID, ipAddress string) (*domain.APIKey, error) {
	if len(key) < 12 {
		return nil, domain.ErrUnauthorized
	}

	prefix := key[:12]
	apiKey, err := s.apiKeysRepo.GetByPrefix(ctx, prefix)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	if !apiKey.Enabled {
		return nil, domain.ErrForbidden
	}

	if apiKey.ExpiresAt != nil && s.clock.Now().After(*apiKey.ExpiresAt) {
		return nil, domain.ErrExpired
	}

	// Verify the full key using HMAC-SHA256
	expectedHash := hashAPIKey(key, s.cfg.Secret)
	if apiKey.KeyHash != expectedHash {
		return nil, domain.ErrUnauthorized
	}

	// Update last used
	_ = s.apiKeysRepo.UpdateLastUsed(ctx, apiKey.ID)

	s.logAudit(ctx, &apiKey.TenantID, nil, requestID, domain.AuditActionAPIKeyAuth, "api_key.used", ipAddress)
	s.eventBus.Publish(event.Event{
		ID:    uuid.New().String(),
		Type:  event.EventAPIKeyUsed,
		Payload: map[string]interface{}{
			"api_key_id": apiKey.ID,
			"tenant_id":  apiKey.TenantID,
		},
		Timestamp: s.clock.Now(),
	})

	return apiKey, nil
}

// hashToken creates an HMAC-SHA256 hash of a token.
func hashToken(token, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

// logAudit logs an audit entry.// GetUser retrieves a user by ID (for the /me endpoint).
func (s *AuthService) GetUser(ctx context.Context, userID string) (*domain.User, error) {
	return s.usersRepo.GetByID(ctx, userID)
}

func (s *AuthService) logAudit(ctx context.Context, tenantID, userID *string, requestID string, action domain.AuditAction, resource, ipAddress string) {
	log := &domain.AuditLog{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		UserID:    userID,
		RequestID: requestID,
		Action:    domain.AuditAction(action),
		Resource:  resource,
		IPAddress: ipAddress,
	}
	_ = s.auditRepo.Create(ctx, log)
}

