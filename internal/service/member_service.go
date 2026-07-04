package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
)

// MemberService handles tenant member management.
type MemberService struct {
	memberRepo domain.TenantMemberRepository
	auditRepo  domain.AuditLogRepository
	eventBus   event.Bus
	clock      domain.Clock
}

// NewMemberService creates a new member service.
func NewMemberService(
	memberRepo domain.TenantMemberRepository,
	auditRepo domain.AuditLogRepository,
	eventBus event.Bus,
	clock domain.Clock,
) *MemberService {
	return &MemberService{
		memberRepo: memberRepo,
		auditRepo:  auditRepo,
		eventBus:   eventBus,
		clock:      clock,
	}
}

// Add adds a user to a tenant with a specific role.
func (s *MemberService) Add(ctx context.Context, input domain.AddMemberInput, addedBy, requestID, ipAddress string) (*domain.TenantMember, error) {
	member, err := s.memberRepo.Add(ctx, input)
	if err != nil {
		return nil, err
	}

	s.logAudit(ctx, &input.TenantID, &addedBy, requestID, domain.AuditActionCreate, "member.added", ipAddress)
	s.eventBus.Publish(event.Event{
		ID:    uuid.New().String(),
		Type:  event.EventMemberAdded,
		Payload: map[string]interface{}{
			"tenant_id": input.TenantID,
			"user_id":   input.UserID,
			"role":      input.Role,
		},
		Timestamp: s.clock.Now(),
	})

	return member, nil
}

// Get retrieves a member by tenant and user ID.
func (s *MemberService) Get(ctx context.Context, tenantID, userID string) (*domain.TenantMember, error) {
	return s.memberRepo.Get(ctx, tenantID, userID)
}

// UpdateRole updates a member's role.
func (s *MemberService) UpdateRole(ctx context.Context, tenantID, userID string, role domain.MemberRole, requestID, ipAddress string) error {
	if err := s.memberRepo.UpdateRole(ctx, tenantID, userID, role); err != nil {
		return err
	}
	s.logAudit(ctx, &tenantID, nil, requestID, domain.AuditActionUpdate, "member.role_updated", ipAddress)
	return nil
}

// Remove removes a user from a tenant.
func (s *MemberService) Remove(ctx context.Context, tenantID, userID, removedBy, requestID, ipAddress string) error {
	if err := s.memberRepo.Remove(ctx, tenantID, userID); err != nil {
		return err
	}
	s.logAudit(ctx, &tenantID, &removedBy, requestID, domain.AuditActionDelete, "member.removed", ipAddress)
	s.eventBus.Publish(event.Event{
		ID:    uuid.New().String(),
		Type:  event.EventMemberRemoved,
		Payload: map[string]interface{}{
			"tenant_id": tenantID,
			"user_id":   userID,
		},
		Timestamp: s.clock.Now(),
	})
	return nil
}

// ListByTenant lists members of a tenant.
func (s *MemberService) ListByTenant(ctx context.Context, tenantID string, page domain.Page) (domain.PageResult[domain.TenantMember], error) {
	return s.memberRepo.ListByTenant(ctx, tenantID, page)
}

// ListByUser lists tenants a user belongs to.
func (s *MemberService) ListByUser(ctx context.Context, userID string) ([]domain.TenantMember, error) {
	return s.memberRepo.ListByUser(ctx, userID)
}

func (s *MemberService) logAudit(ctx context.Context, tenantID, userID *string, requestID string, action domain.AuditAction, resource, ipAddress string) {
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
