package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// AuditService handles audit log operations.
type AuditService struct {
	auditRepo domain.AuditLogRepository
}

// NewAuditService creates a new audit service.
func NewAuditService(auditRepo domain.AuditLogRepository) *AuditService {
	return &AuditService{auditRepo: auditRepo}
}

// ListByTenant lists audit logs for a tenant using cursor-based pagination.
func (s *AuditService) ListByTenant(ctx context.Context, tenantID string, page domain.CursorPage) (domain.CursorPageResult[domain.AuditLog], error) {
	return s.auditRepo.ListByTenant(ctx, tenantID, page)
}

// ListByUser lists audit logs for a user using cursor-based pagination.
func (s *AuditService) ListByUser(ctx context.Context, userID string, page domain.CursorPage) (domain.CursorPageResult[domain.AuditLog], error) {
	return s.auditRepo.ListByUser(ctx, userID, page)
}

// Write writes an audit log entry.
func (s *AuditService) Write(ctx context.Context, tenantID, userID *string, requestID string, action domain.AuditAction, resource, ipAddress string) {
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
