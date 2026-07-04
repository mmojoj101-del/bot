package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
)

// ConnectorService handles connector management.
type ConnectorService struct {
	connectorRepo domain.ConnectorRepository
	auditRepo     domain.AuditLogRepository
	eventBus      event.Bus
	clock         domain.Clock
}

// NewConnectorService creates a new connector service.
func NewConnectorService(
	connectorRepo domain.ConnectorRepository,
	auditRepo domain.AuditLogRepository,
	eventBus event.Bus,
	clock domain.Clock,
) *ConnectorService {
	return &ConnectorService{
		connectorRepo: connectorRepo,
		auditRepo:     auditRepo,
		eventBus:      eventBus,
		clock:         clock,
	}
}

// Create creates a new connector.
func (s *ConnectorService) Create(ctx context.Context, input domain.CreateConnectorInput, createdBy, requestID, ipAddress string) (*domain.Connector, error) {
	connector, err := s.connectorRepo.Create(ctx, input, createdBy)
	if err != nil {
		return nil, err
	}

	s.logAudit(ctx, &connector.TenantID, &createdBy, requestID, domain.AuditActionCreate, fmt.Sprintf("connector.created:%s", connector.ID), ipAddress)
	s.eventBus.Publish(event.Event{
		ID:    uuid.New().String(),
		Type:  event.EventConnectorCreated,
		Payload: map[string]interface{}{
			"connector_id": connector.ID,
			"tenant_id":    connector.TenantID,
			"type":         connector.Type,
		},
		Timestamp: s.clock.Now(),
	})

	return connector, nil
}

// GetByID retrieves a connector by ID.
func (s *ConnectorService) GetByID(ctx context.Context, id string) (*domain.Connector, error) {
	return s.connectorRepo.GetByID(ctx, id)
}

// Update updates a connector.
func (s *ConnectorService) Update(ctx context.Context, id string, input domain.UpdateConnectorInput, updatedBy, requestID, ipAddress string) (*domain.Connector, error) {
	current, err := s.connectorRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	connector, err := s.connectorRepo.Update(ctx, id, input, updatedBy, current.Version)
	if err != nil {
		return nil, err
	}

	s.logAudit(ctx, &connector.TenantID, &updatedBy, requestID, domain.AuditActionUpdate, fmt.Sprintf("connector.updated:%s", connector.ID), ipAddress)
	s.eventBus.Publish(event.Event{
		ID:    uuid.New().String(),
		Type:  event.EventConnectorUpdated,
		Payload: map[string]interface{}{
			"connector_id": connector.ID,
			"tenant_id":    connector.TenantID,
		},
		Timestamp: s.clock.Now(),
	})

	return connector, nil
}

// Delete soft-deletes a connector.
func (s *ConnectorService) Delete(ctx context.Context, id, deletedBy, requestID, ipAddress string) error {
	current, err := s.connectorRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if err := s.connectorRepo.Delete(ctx, id); err != nil {
		return err
	}

	s.logAudit(ctx, &current.TenantID, &deletedBy, requestID, domain.AuditActionDelete, fmt.Sprintf("connector.deleted:%s", id), ipAddress)
	s.eventBus.Publish(event.Event{
		ID:    uuid.New().String(),
		Type:  event.EventConnectorDeleted,
		Payload: map[string]interface{}{
			"connector_id": id,
			"tenant_id":    current.TenantID,
		},
		Timestamp: s.clock.Now(),
	})

	return nil
}

// ListByTenant lists connectors for a tenant.
func (s *ConnectorService) ListByTenant(ctx context.Context, tenantID string, page domain.Page) (domain.PageResult[domain.Connector], error) {
	return s.connectorRepo.ListByTenant(ctx, tenantID, page)
}

// TestConnector tests a connector connection (stub for now).
func (s *ConnectorService) TestConnector(ctx context.Context, id, requestID, ipAddress string) error {
	connector, err := s.connectorRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// For now, just set status to testing and back to active
	// Real implementation will test the actual connection
	_, err = s.connectorRepo.Update(ctx, id, domain.UpdateConnectorInput{
		Status: &statusTesting,
	}, "", connector.Version)
	if err != nil {
		return err
	}

	s.logAudit(ctx, &connector.TenantID, nil, requestID, domain.AuditActionConnectorTested, fmt.Sprintf("connector.tested:%s", id), ipAddress)
	s.eventBus.Publish(event.Event{
		ID:    uuid.New().String(),
		Type:  event.EventConnectorTested,
		Payload: map[string]interface{}{
			"connector_id": id,
			"tenant_id":    connector.TenantID,
		},
		Timestamp: s.clock.Now(),
	})

	return nil
}

func (s *ConnectorService) logAudit(ctx context.Context, tenantID, userID *string, requestID string, action domain.AuditAction, resource, ipAddress string) {
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

var statusTesting = domain.ConnectorStatusTesting
