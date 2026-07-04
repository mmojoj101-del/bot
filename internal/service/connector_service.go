package service

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
)

// ConnectorService handles connector management.
type ConnectorService struct {
	connectorRepo domain.ConnectorRepository
	auditRepo     domain.AuditLogRepository
	txManager     domain.TxManager
	eventBus      event.Bus
	clock         domain.Clock
	testers       map[domain.ConnectorType]domain.ConnectorTester
}

// NewConnectorService creates a new connector service.
func NewConnectorService(
	connectorRepo domain.ConnectorRepository,
	auditRepo domain.AuditLogRepository,
	txManager domain.TxManager,
	eventBus event.Bus,
	clock domain.Clock,
	testers ...domain.ConnectorTester,
) *ConnectorService {
	svc := &ConnectorService{
		connectorRepo: connectorRepo,
		auditRepo:     auditRepo,
		txManager:     txManager,
		eventBus:      eventBus,
		clock:         clock,
		testers:       make(map[domain.ConnectorType]domain.ConnectorTester),
	}
	for _, t := range testers {
		svc.testers[t.Type()] = t
	}
	return svc
}

// Create creates a new connector.
func (s *ConnectorService) Create(ctx context.Context, input domain.CreateConnectorInput, createdBy, requestID, ipAddress string) (*domain.Connector, error) {
	// Block mock type in production environments
	if input.Type == domain.ConnectorTypeMock && isProductionEnv() {
		return nil, domain.ErrConnectorTypeNotAllowed
	}

	var connector *domain.Connector

	err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		connector, err = s.connectorRepo.Create(txCtx, input, createdBy)
		if err != nil {
			return err
		}

		return s.auditRepo.Create(txCtx, &domain.AuditLog{
			ID:        uuid.New().String(),
			TenantID:  &connector.TenantID,
			UserID:    &createdBy,
			RequestID: requestID,
			Action:    domain.AuditActionCreate,
			Resource:  fmt.Sprintf("connector.created:%s", connector.ID),
			IPAddress: ipAddress,
		})
	})
	if err != nil {
		return nil, err
	}

	// Event after commit — safe to publish even if it fails (async-ready)
	s.eventBus.Publish(event.Event{
		ID:   uuid.New().String(),
		Type: event.EventConnectorCreated,
		Payload: map[string]interface{}{
			"connector_id": connector.ID,
			"tenant_id":    connector.TenantID,
			"type":         string(connector.Type),
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
	var connector *domain.Connector

	err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		current, err := s.connectorRepo.GetByID(txCtx, id)
		if err != nil {
			return err
		}

		connector, err = s.connectorRepo.Update(txCtx, id, input, updatedBy, current.Version)
		if err != nil {
			return err
		}

		return s.auditRepo.Create(txCtx, &domain.AuditLog{
			ID:        uuid.New().String(),
			TenantID:  &connector.TenantID,
			UserID:    &updatedBy,
			RequestID: requestID,
			Action:    domain.AuditActionUpdate,
			Resource:  fmt.Sprintf("connector.updated:%s", connector.ID),
			IPAddress: ipAddress,
		})
	})
	if err != nil {
		return nil, err
	}

	s.eventBus.Publish(event.Event{
		ID:   uuid.New().String(),
		Type: event.EventConnectorUpdated,
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
	var tenantID string

	err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		current, err := s.connectorRepo.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		tenantID = current.TenantID

		if err := s.connectorRepo.Delete(txCtx, id); err != nil {
			return err
		}

		return s.auditRepo.Create(txCtx, &domain.AuditLog{
			ID:        uuid.New().String(),
			TenantID:  &current.TenantID,
			UserID:    &deletedBy,
			RequestID: requestID,
			Action:    domain.AuditActionDelete,
			Resource:  fmt.Sprintf("connector.deleted:%s", id),
			IPAddress: ipAddress,
		})
	})
	if err != nil {
		return err
	}

	s.eventBus.Publish(event.Event{
		ID:   uuid.New().String(),
		Type: event.EventConnectorDeleted,
		Payload: map[string]interface{}{
			"connector_id": id,
			"tenant_id":    tenantID,
		},
		Timestamp: s.clock.Now(),
	})

	return nil
}

// ListByTenant lists connectors for a tenant with optional filtering.
func (s *ConnectorService) ListByTenant(ctx context.Context, filter domain.ConnectorFilter) (domain.PageResult[domain.Connector], error) {
	return s.connectorRepo.ListByTenant(ctx, filter)
}

// TestConnector tests a connector connection using the appropriate tester.
func (s *ConnectorService) TestConnector(ctx context.Context, id, requestID, ipAddress string) error {
	connector, err := s.connectorRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	tester, ok := s.testers[connector.Type]
	if !ok {
		// Stub: just toggle status to testing and back
		_, err = s.connectorRepo.Update(ctx, id, domain.UpdateConnectorInput{
			Status: &statusTesting,
		}, "", connector.Version)
		return err
	}

	// Set status to testing before running test
	_, err = s.connectorRepo.Update(ctx, id, domain.UpdateConnectorInput{
		Status: &statusTesting,
	}, "", connector.Version)
	if err != nil {
		return err
	}

	if err := tester.Test(ctx, connector); err != nil {
		errorStatus := domain.ConnectorStatusError
		_, _ = s.connectorRepo.Update(ctx, id, domain.UpdateConnectorInput{
			Status: &errorStatus,
		}, "", connector.Version+1)
		return err
	}

	activeStatus := domain.ConnectorStatusActive
	_, err = s.connectorRepo.Update(ctx, id, domain.UpdateConnectorInput{
		Status: &activeStatus,
	}, "", connector.Version+2)
	if err != nil {
		return err
	}

	s.logAudit(ctx, &connector.TenantID, nil, requestID, domain.AuditActionConnectorTested, fmt.Sprintf("connector.tested:%s", id), ipAddress)
	s.eventBus.Publish(event.Event{
		ID:   uuid.New().String(),
		Type: event.EventConnectorTested,
		Payload: map[string]interface{}{
			"connector_id": id,
			"tenant_id":    connector.TenantID,
		},
		Timestamp: s.clock.Now(),
	})

	return nil
}

func (s *ConnectorService) logAudit(ctx context.Context, tenantID, userID *string, requestID string, action domain.AuditAction, resource, ipAddress string) {
	_ = s.auditRepo.Create(ctx, &domain.AuditLog{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		UserID:    userID,
		RequestID: requestID,
		Action:    action,
		Resource:  resource,
		IPAddress: ipAddress,
	})
}

var statusTesting = domain.ConnectorStatusTesting

// isProductionEnv returns true if the application is running in production mode.
func isProductionEnv() bool {
	return os.Getenv("APP_ENV") == "production"
}
