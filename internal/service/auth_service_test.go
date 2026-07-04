package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/raghna/fury-sms-gateway/internal/config"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
	pwdpkg "github.com/raghna/fury-sms-gateway/internal/pkg/password"
)

// ============================================================
// Mocks
// ============================================================

type mockClock struct{ now time.Time }

func (m *mockClock) Now() time.Time { return m.now }

type mockUserRepo struct {
	mu     sync.Mutex
	users  map[string]*domain.User
	emails map[string]*domain.User
	err    error
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		users:  make(map[string]*domain.User),
		emails: make(map[string]*domain.User),
	}
}

func (r *mockUserRepo) Create(ctx context.Context, input domain.CreateUserInput, passwordHash string) (*domain.User, error) {
	if r.err != nil {
		return nil, r.err
	}
	u := &domain.User{
		BaseModel: domain.BaseModel{
			ID:        uuid.New().String(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Version:   1,
		},
		Email:        input.Email,
		PasswordHash: passwordHash,
		Name:         input.Name,
		Status:       domain.UserStatusActive,
	}
	r.mu.Lock()
	r.users[u.ID] = u
	r.emails[u.Email] = u
	r.mu.Unlock()
	return u, nil
}

func (r *mockUserRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	if r.err != nil {
		return nil, r.err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return u, nil
}

func (r *mockUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	if r.err != nil {
		return nil, r.err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.emails[email]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return u, nil
}

func (r *mockUserRepo) Update(ctx context.Context, id string, input domain.UpdateUserInput, version int) (*domain.User, error) {
	return nil, nil
}
func (r *mockUserRepo) Delete(ctx context.Context, id string) error { return nil }
func (r *mockUserRepo) List(ctx context.Context, page domain.Page) (domain.PageResult[domain.User], error) {
	return domain.PageResult[domain.User]{}, nil
}
func (r *mockUserRepo) UpdateLastLogin(ctx context.Context, id string) error { return nil }
func (r *mockUserRepo) UpdatePassword(ctx context.Context, id string, passwordHash string, version int) error {
	return nil
}
func (r *mockUserRepo) Count(ctx context.Context) (int64, error) { return 0, nil }

type mockMemberRepo struct {
	mu      sync.Mutex
	members map[string]*domain.TenantMember
	err     error
}

func newMockMemberRepo() *mockMemberRepo {
	return &mockMemberRepo{members: make(map[string]*domain.TenantMember)}
}

func (r *mockMemberRepo) Add(ctx context.Context, input domain.AddMemberInput) (*domain.TenantMember, error) {
	if r.err != nil {
		return nil, r.err
	}
	m := &domain.TenantMember{
		BaseModel: domain.BaseModel{ID: uuid.New().String(), CreatedAt: time.Now(), UpdatedAt: time.Now(), Version: 1},
		TenantID:  input.TenantID,
		UserID:    input.UserID,
		Role:      input.Role,
		JoinedAt:  time.Now(),
	}
	r.mu.Lock()
	r.members[input.TenantID+"-"+input.UserID] = m
	r.mu.Unlock()
	return m, nil
}

func (r *mockMemberRepo) Get(ctx context.Context, tenantID, userID string) (*domain.TenantMember, error) {
	if r.err != nil {
		return nil, r.err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.members[tenantID+"-"+userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return m, nil
}

func (r *mockMemberRepo) UpdateRole(ctx context.Context, tenantID, userID string, role domain.MemberRole) error {
	return nil
}
func (r *mockMemberRepo) Remove(ctx context.Context, tenantID, userID string) error { return nil }
func (r *mockMemberRepo) ListByTenant(ctx context.Context, tenantID string, page domain.Page) (domain.PageResult[domain.TenantMember], error) {
	return domain.PageResult[domain.TenantMember]{}, nil
}
func (r *mockMemberRepo) ListByUser(ctx context.Context, userID string) ([]domain.TenantMember, error) {
	return []domain.TenantMember{}, nil
}
func (r *mockMemberRepo) CountByTenant(ctx context.Context, tenantID string) (int64, error) { return 0, nil }

type mockAPIKeyRepo struct {
	mu   sync.Mutex
	keys map[string]*domain.APIKey
	err  error
}

func newMockAPIKeyRepo() *mockAPIKeyRepo {
	return &mockAPIKeyRepo{keys: make(map[string]*domain.APIKey)}
}

func (r *mockAPIKeyRepo) Create(ctx context.Context, input domain.CreateAPIKeyInput, keyPrefix, keyHash string, createdBy string) (*domain.APIKey, error) {
	return nil, nil
}
func (r *mockAPIKeyRepo) GetByID(ctx context.Context, id string) (*domain.APIKey, error) { return nil, nil }
func (r *mockAPIKeyRepo) GetByPrefix(ctx context.Context, prefix string) (*domain.APIKey, error) {
	if r.err != nil {
		return nil, r.err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, k := range r.keys {
		if k.KeyPrefix == prefix {
			return k, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *mockAPIKeyRepo) Update(ctx context.Context, id string, input domain.UpdateAPIKeyInput, updatedBy string, version int) (*domain.APIKey, error) {
	return nil, nil
}
func (r *mockAPIKeyRepo) Delete(ctx context.Context, id string) error { return nil }
func (r *mockAPIKeyRepo) ListByTenant(ctx context.Context, tenantID string, page domain.Page) (domain.PageResult[domain.APIKey], error) {
	return domain.PageResult[domain.APIKey]{}, nil
}
func (r *mockAPIKeyRepo) UpdateLastUsed(ctx context.Context, id string) error { return nil }
func (r *mockAPIKeyRepo) CountByTenant(ctx context.Context, tenantID string) (int64, error) { return 0, nil }

type mockRefreshTokenRepo struct {
	mu     sync.Mutex
	tokens map[string]*domain.RefreshToken
	err    error
}

func newMockRefreshTokenRepo() *mockRefreshTokenRepo {
	return &mockRefreshTokenRepo{tokens: make(map[string]*domain.RefreshToken)}
}

func (r *mockRefreshTokenRepo) Create(ctx context.Context, token *domain.RefreshToken) error {
	if r.err != nil {
		return r.err
	}
	r.mu.Lock()
	r.tokens[token.JTI] = token
	r.mu.Unlock()
	return nil
}
func (r *mockRefreshTokenRepo) GetByJTI(ctx context.Context, jti string) (*domain.RefreshToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tokens[jti]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return t, nil
}
func (r *mockRefreshTokenRepo) Revoke(ctx context.Context, jti string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.tokens[jti]; ok {
		now := time.Now()
		t.RevokedAt = &now
	}
	return nil
}
func (r *mockRefreshTokenRepo) RevokeAllByUser(ctx context.Context, userID string) error { return nil }
func (r *mockRefreshTokenRepo) ListByUser(ctx context.Context, userID string) ([]domain.RefreshToken, error) {
	return nil, nil
}
func (r *mockRefreshTokenRepo) DeleteExpired(ctx context.Context) (int64, error) { return 0, nil }

type mockAuditRepo struct{ err error }

func (r *mockAuditRepo) Create(ctx context.Context, log *domain.AuditLog) error { return r.err }
func (r *mockAuditRepo) GetByID(ctx context.Context, id string) (*domain.AuditLog, error) {
	return nil, nil
}
func (r *mockAuditRepo) ListByTenant(ctx context.Context, tenantID string, page domain.CursorPage) (domain.CursorPageResult[domain.AuditLog], error) {
	return domain.CursorPageResult[domain.AuditLog]{}, nil
}
func (r *mockAuditRepo) ListByUser(ctx context.Context, userID string, page domain.CursorPage) (domain.CursorPageResult[domain.AuditLog], error) {
	return domain.CursorPageResult[domain.AuditLog]{}, nil
}

// ============================================================
// Tests
// ============================================================

const testSecret = "test-secret-key-for-auth-service-tests"

func setupAuthService() (*AuthService, *mockUserRepo, *mockMemberRepo, *mockRefreshTokenRepo, *mockAPIKeyRepo, *mockAuditRepo, *event.MemoryBus) {
	userRepo := newMockUserRepo()
	memberRepo := newMockMemberRepo()
	apiKeyRepo := newMockAPIKeyRepo()
	refreshRepo := newMockRefreshTokenRepo()
	auditRepo := &mockAuditRepo{}
	bus := event.NewMemoryBus()
	clock := &mockClock{now: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)}

	cfg := &config.JWTConfig{
		Secret:          testSecret,
		AccessDuration:  15 * time.Minute,
		RefreshDuration: 720 * time.Hour,
		Issuer:          "fury-sms-gateway",
		Audience:        "fury-api",
	}

	svc := NewAuthService(userRepo, memberRepo, apiKeyRepo, refreshRepo, auditRepo, bus, cfg, clock)
	return svc, userRepo, memberRepo, refreshRepo, apiKeyRepo, auditRepo, bus
}

func createTestUser(repo *mockUserRepo, email, pwd string) *domain.User {
	hash, _ := pwdpkg.Hash(pwd)
	user, _ := repo.Create(context.Background(), domain.CreateUserInput{
		Email:    email,
		Password: pwd,
		Name:     "Test User",
	}, hash)
	return user
}

func TestLogin_Success(t *testing.T) {
	svc, userRepo, _, _, _, _, _ := setupAuthService()
	createTestUser(userRepo, "test@example.com", "ValidPassword123!")

	resp, err := svc.Login(context.Background(), LoginRequest{
		Email:    "test@example.com",
		Password: "ValidPassword123!",
	}, uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Login() failed: %v", err)
	}
	if resp.AccessToken == "" {
		t.Fatal("access token should not be empty")
	}
	if resp.RefreshToken == "" {
		t.Fatal("refresh token should not be empty")
	}
	if resp.User == nil {
		t.Fatal("user should not be nil")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	svc, userRepo, _, _, _, _, _ := setupAuthService()
	createTestUser(userRepo, "test@example.com", "ValidPassword123!")

	_, err := svc.Login(context.Background(), LoginRequest{
		Email:    "test@example.com",
		Password: "WrongPassword123!",
	}, uuid.New().String(), "127.0.0.1")
	if err != domain.ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	svc, _, _, _, _, _, _ := setupAuthService()

	_, err := svc.Login(context.Background(), LoginRequest{
		Email:    "nonexistent@example.com",
		Password: "ValidPassword123!",
	}, uuid.New().String(), "127.0.0.1")
	if err != domain.ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestLogin_UserDisabled(t *testing.T) {
	svc, userRepo, _, _, _, _, _ := setupAuthService()
	user := createTestUser(userRepo, "test@example.com", "ValidPassword123!")
	user.Status = domain.UserStatusDisabled

	_, err := svc.Login(context.Background(), LoginRequest{
		Email:    "test@example.com",
		Password: "ValidPassword123!",
	}, uuid.New().String(), "127.0.0.1")
	if err != domain.ErrSuspended {
		t.Fatalf("expected ErrSuspended, got %v", err)
	}
}

func TestLogin_UserSuspended(t *testing.T) {
	svc, userRepo, _, _, _, _, _ := setupAuthService()
	user := createTestUser(userRepo, "test@example.com", "ValidPassword123!")
	user.Status = domain.UserStatusSuspended

	_, err := svc.Login(context.Background(), LoginRequest{
		Email:    "test@example.com",
		Password: "ValidPassword123!",
	}, uuid.New().String(), "127.0.0.1")
	if err != domain.ErrSuspended {
		t.Fatalf("expected ErrSuspended, got %v", err)
	}
}

func TestLogin_EmptyEmail(t *testing.T) {
	svc, _, _, _, _, _, _ := setupAuthService()

	_, err := svc.Login(context.Background(), LoginRequest{
		Email:    "",
		Password: "ValidPassword123!",
	}, uuid.New().String(), "127.0.0.1")
	if err != domain.ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestRefreshToken_Success(t *testing.T) {
	svc, userRepo, _, _, _, _, _ := setupAuthService()
	createTestUser(userRepo, "test@example.com", "ValidPassword123!")

	// First login
	loginResp, err := svc.Login(context.Background(), LoginRequest{
		Email:    "test@example.com",
		Password: "ValidPassword123!",
	}, uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Login() failed: %v", err)
	}

	// Refresh
	resp, err := svc.RefreshToken(context.Background(), loginResp.RefreshToken, uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("RefreshToken() failed: %v", err)
	}
	if resp.AccessToken == "" {
		t.Fatal("new access token should not be empty")
	}
	if resp.RefreshToken == loginResp.RefreshToken {
		t.Fatal("refresh token should be rotated")
	}
}

func TestRefreshToken_ExpiredToken(t *testing.T) {
	svc, userRepo, _, refreshRepo, _, _, _ := setupAuthService()
	user := createTestUser(userRepo, "test@example.com", "ValidPassword123!")

	// Manually create an expired refresh token
	clock := &mockClock{now: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)}
	_ = refreshRepo.Create(context.Background(), &domain.RefreshToken{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		TenantID:  "",
		TokenHash: "hash",
		JTI:       "expired-jti",
		ExpiresAt: clock.Now().Add(-1 * time.Hour),
	})

	_, err := svc.RefreshToken(context.Background(), "invalid-refresh-token", uuid.New().String(), "127.0.0.1")
	if err != domain.ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestRefreshToken_RevokedToken(t *testing.T) {
	svc, userRepo, _, _, _, _, _ := setupAuthService()
	createTestUser(userRepo, "test@example.com", "ValidPassword123!")

	loginResp, err := svc.Login(context.Background(), LoginRequest{
		Email:    "test@example.com",
		Password: "ValidPassword123!",
	}, uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Login() failed: %v", err)
	}

	// Logout (revokes the token)
	_ = svc.Logout(context.Background(), loginResp.RefreshToken, uuid.New().String(), "127.0.0.1")

	// Try to refresh with revoked token
	_, err = svc.RefreshToken(context.Background(), loginResp.RefreshToken, uuid.New().String(), "127.0.0.1")
	if err != domain.ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized for revoked token, got %v", err)
	}
}

func TestLogout_Success(t *testing.T) {
	svc, userRepo, _, _, _, _, _ := setupAuthService()
	createTestUser(userRepo, "test@example.com", "ValidPassword123!")

	loginResp, err := svc.Login(context.Background(), LoginRequest{
		Email:    "test@example.com",
		Password: "ValidPassword123!",
	}, uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Login() failed: %v", err)
	}

	err = svc.Logout(context.Background(), loginResp.RefreshToken, uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Logout() failed: %v", err)
	}
}

func TestLogout_InvalidToken(t *testing.T) {
	svc, _, _, _, _, _, _ := setupAuthService()

	// Should not return error even with invalid token
	err := svc.Logout(context.Background(), "invalid-token", uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Logout() should not error on invalid token: %v", err)
	}
}

func TestSwitchTenant_Unauthorized(t *testing.T) {
	svc, userRepo, _, _, _, _, _ := setupAuthService()
	user := createTestUser(userRepo, "test@example.com", "ValidPassword123!")

	// User is not a member of the target tenant
	_, err := svc.SwitchTenant(context.Background(), user.ID, "nonexistent-tenant", uuid.New().String(), "127.0.0.1")
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestGetUser_Success(t *testing.T) {
	svc, userRepo, _, _, _, _, _ := setupAuthService()
	user := createTestUser(userRepo, "test@example.com", "ValidPassword123!")

	fetched, err := svc.GetUser(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetUser() failed: %v", err)
	}
	if fetched.Email != user.Email {
		t.Fatalf("email = %s, want %s", fetched.Email, user.Email)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	svc, _, _, _, _, _, _ := setupAuthService()

	_, err := svc.GetUser(context.Background(), "nonexistent-id")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
