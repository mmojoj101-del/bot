package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
)

// TenantService handles tenant management.
type TenantService struct {
	tenantRepo domain.TenantRepository
	memberRepo domain.TenantMemberRepository
	auditRepo  domain.AuditLogRepository
	eventBus   event.Bus
	clock      domain.Clock
}

// NewTenantService creates a new tenant service.
func NewTenantService(
	tenantRepo domain.TenantRepository,
	memberRepo domain.TenantMemberRepository,
	auditRepo domain.AuditLogRepository,
	eventBus event.Bus,
	clock domain.Clock,
) *TenantService {
	return &TenantService{
		tenantRepo: tenantRepo,
		memberRepo: memberRepo,
		auditRepo:  auditRepo,
		eventBus:   eventBus,
		clock:      clock,
	}
}

// Create creates a new tenant and adds the creator as an admin member.
func (s *TenantService) Create(ctx context.Context, input domain.CreateTenantInput, createdBy, requestID, ipAddress string) (*domain.Tenant, error) {
	tenant, err := s.tenantRepo.Create(ctx, input, createdBy)
	if err != nil {
		return nil, err
	}

	// Add creator as admin member
	_, err = s.memberRepo.Add(ctx, domain.AddMemberInput{
		TenantID: tenant.ID,
		UserID:   createdBy,
		Role:     domain.MemberRoleAdmin,
	})
	if err != nil {
		return nil, fmt.Errorf("add creator as admin: %w", err)
	}

	// Audit log
	s.logAudit(ctx, &tenant.ID, &createdBy, requestID, domain.AuditActionCreate, "tenant.created", ipAddress)

	s.eventBus.Publish(event.Event{
		ID:    uuid.New().String(),
		Type:  event.EventTenantCreated,
		Payload: map[string]interface{}{
			"tenant_id": tenant.ID,
			"created_by": createdBy,
		},
		Timestamp: s.clock.Now(),
	})

	return tenant, nil
}

// GetByID retrieves a tenant by ID.
func (s *TenantService) GetByID(ctx context.Context, id string) (*domain.Tenant, error) {
	return s.tenantRepo.GetByID(ctx, id)
}

// Update updates a tenant.
func (s *TenantService) Update(ctx context.Context, id string, input domain.UpdateTenantInput, updatedBy, requestID, ipAddress string) (*domain.Tenant, error) {
	// Get current tenant for version
	current, err := s.tenantRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	tenant, err := s.tenantRepo.Update(ctx, id, input, updatedBy, current.Version)
	if err != nil {
		return nil, err
	}

	s.logAudit(ctx, &id, &updatedBy, requestID, domain.AuditActionUpdate, "tenant.updated", ipAddress)
	s.eventBus.Publish(event.Event{
		ID:    uuid.New().String(),
		Type:  event.EventTenantUpdated,
		Payload: map[string]interface{}{"tenant_id": id},
		Timestamp: s.clock.Now(),
	})

	return tenant, nil
}

// Delete soft-deletes a tenant.
func (s *TenantService) Delete(ctx context.Context, id, deletedBy, requestID, ipAddress string) error {
	if err := s.tenantRepo.Delete(ctx, id); err != nil {
		return err
	}
	s.logAudit(ctx, &id, &deletedBy, requestID, domain.AuditActionDelete, "tenant.deleted", ipAddress)
	return nil
}

// List returns a paginated list of tenants.
func (s *TenantService) List(ctx context.Context, page domain.Page) (domain.PageResult[domain.Tenant], error) {
	return s.tenantRepo.List(ctx, page)
}

func (s *TenantService) logAudit(ctx context.Context, tenantID, userID *string, requestID string, action domain.AuditAction, resource, ipAddress string) {
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
