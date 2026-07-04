package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/google/uuid"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
)

const (
	apiKeyLength    = 48
	apiKeyPrefixLen = 12
	apiKeyCharset   = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

// APIKeyService handles API key management.
type APIKeyService struct {
	apiKeyRepo domain.APIKeyRepository
	auditRepo  domain.AuditLogRepository
	eventBus   event.Bus
	secret     string // server secret for HMAC
	clock      domain.Clock
}

// NewAPIKeyService creates a new API key service.
func NewAPIKeyService(
	apiKeyRepo domain.APIKeyRepository,
	auditRepo domain.AuditLogRepository,
	eventBus event.Bus,
	serverSecret string,
	clock domain.Clock,
) *APIKeyService {
	return &APIKeyService{
		apiKeyRepo: apiKeyRepo,
		auditRepo:  auditRepo,
		eventBus:   eventBus,
		secret:     serverSecret,
		clock:      clock,
	}
}

// CreateAPIKeyResult contains the created API key with its plaintext value.
type CreateAPIKeyResult struct {
	APIKey       *domain.APIKey `json:"api_key"`
	PlaintextKey string         `json:"plaintext_key"`
}

// Create creates a new API key and returns the plaintext key (shown only once).
func (s *APIKeyService) Create(ctx context.Context, input domain.CreateAPIKeyInput, createdBy, requestID, ipAddress string) (*CreateAPIKeyResult, error) {
	plaintextKey, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("generate api key: %w", err)
	}

	prefix := plaintextKey[:apiKeyPrefixLen]
	keyHash := hashAPIKey(plaintextKey, s.secret)

	apiKey, err := s.apiKeyRepo.Create(ctx, input, prefix, keyHash, createdBy)
	if err != nil {
		return nil, err
	}

	s.logAudit(ctx, &input.TenantID, &createdBy, requestID, domain.AuditActionCreate, "api_key.created", ipAddress)
	s.eventBus.Publish(event.Event{
		ID:    uuid.New().String(),
		Type:  event.EventAPIKeyCreated,
		Payload: map[string]interface{}{
			"api_key_id": apiKey.ID,
			"tenant_id":  input.TenantID,
		},
		Timestamp: s.clock.Now(),
	})

	return &CreateAPIKeyResult{
		APIKey:       apiKey,
		PlaintextKey: plaintextKey,
	}, nil
}

// GetByID retrieves an API key by ID (without the plaintext key).
func (s *APIKeyService) GetByID(ctx context.Context, id string) (*domain.APIKey, error) {
	return s.apiKeyRepo.GetByID(ctx, id)
}

// Update updates an API key.
func (s *APIKeyService) Update(ctx context.Context, id string, input domain.UpdateAPIKeyInput, updatedBy, requestID, ipAddress string) (*domain.APIKey, error) {
	current, err := s.apiKeyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	apiKey, err := s.apiKeyRepo.Update(ctx, id, input, updatedBy, current.Version)
	if err != nil {
		return nil, err
	}

	s.logAudit(ctx, &apiKey.TenantID, &updatedBy, requestID, domain.AuditActionUpdate, "api_key.updated", ipAddress)
	return apiKey, nil
}

// Delete soft-deletes an API key.
func (s *APIKeyService) Delete(ctx context.Context, id, deletedBy, requestID, ipAddress string) error {
	if err := s.apiKeyRepo.Delete(ctx, id); err != nil {
		return err
	}
	s.logAudit(ctx, nil, &deletedBy, requestID, domain.AuditActionDelete, "api_key.deleted", ipAddress)
	return nil
}

// ListByTenant lists API keys for a tenant.
func (s *APIKeyService) ListByTenant(ctx context.Context, tenantID string, page domain.Page) (domain.PageResult[domain.APIKey], error) {
	return s.apiKeyRepo.ListByTenant(ctx, tenantID, page)
}

func (s *APIKeyService) logAudit(ctx context.Context, tenantID, userID *string, requestID string, action domain.AuditAction, resource, ipAddress string) {
	log := &domain.AuditLog{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		UserID:    userID,
		RequestID: requestID,
		Action:    action,
		Resource:  resource,
		IPAddress: ipAddress,
	}
	_ = s.auditRepo.Create(ctx, log)
}

// generateAPIKey generates a cryptographically random API key.
func generateAPIKey() (string, error) {
	charsetLen := big.NewInt(int64(len(apiKeyCharset)))
	key := make([]byte, apiKeyLength)
	for i := range key {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", fmt.Errorf("generate random: %w", err)
		}
		key[i] = apiKeyCharset[n.Int64()]
	}
	return "fx_" + string(key), nil
}

// hashAPIKey creates an HMAC-SHA256 hash of an API key using the server secret.
func hashAPIKey(apiKey, serverSecret string) string {
	mac := hmac.New(sha256.New, []byte(serverSecret))
	mac.Write([]byte(apiKey))
	return hex.EncodeToString(mac.Sum(nil))
}
