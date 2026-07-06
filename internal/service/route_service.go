package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
)

// RouteService handles route management.
type RouteService struct {
	routeRepo     domain.RouteRepository
	connectorRepo domain.ConnectorRepository
	auditRepo     domain.AuditLogRepository
	txManager     domain.TxManager
	eventBus      event.Bus
	clock         domain.Clock
}

// NewRouteService creates a new route service.
func NewRouteService(
	routeRepo domain.RouteRepository,
	connectorRepo domain.ConnectorRepository,
	auditRepo domain.AuditLogRepository,
	txManager domain.TxManager,
	eventBus event.Bus,
	clock domain.Clock,
) *RouteService {
	return &RouteService{
		routeRepo:     routeRepo,
		connectorRepo: connectorRepo,
		auditRepo:     auditRepo,
		txManager:     txManager,
		eventBus:      eventBus,
		clock:         clock,
	}
}

// Create creates a new route.
func (s *RouteService) Create(ctx context.Context, input domain.CreateRouteInput, createdBy, requestID, ipAddress string) (*domain.Route, error) {
	// Verify the connector exists — only if ConnectorID is provided
	if input.ConnectorID != "" {
		if _, err := s.connectorRepo.GetByID(ctx, input.ConnectorID); err != nil {
			return nil, fmt.Errorf("connector: %w", err)
		}
	}

	var route *domain.Route

	err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		route, err = s.routeRepo.Create(txCtx, input, createdBy)
		if err != nil {
			return err
		}

		return s.auditRepo.Create(txCtx, &domain.AuditLog{
			ID:        uuid.New().String(),
			TenantID:  &route.TenantID,
			UserID:    &createdBy,
			RequestID: requestID,
			Action:    domain.AuditActionCreate,
			Resource:  fmt.Sprintf("route.created:%s", route.ID),
			IPAddress: ipAddress,
		})
	})
	if err != nil {
		return nil, err
	}

	s.eventBus.Publish(event.Event{
		ID:   uuid.New().String(),
		Type: event.EventRouteCreated,
		Payload: map[string]interface{}{
			"route_id":     route.ID,
			"tenant_id":    route.TenantID,
			"prefix":       route.Prefix,
			"connector_id": route.ConnectorID,
		},
		Timestamp: s.clock.Now(),
	})

	return route, nil
}

// GetByID retrieves a route by ID.
func (s *RouteService) GetByID(ctx context.Context, id string) (*domain.Route, error) {
	return s.routeRepo.GetByID(ctx, id)
}

// Update updates a route.
func (s *RouteService) Update(ctx context.Context, id string, input domain.UpdateRouteInput, updatedBy, requestID, ipAddress string) (*domain.Route, error) {
	var route *domain.Route

	err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		current, err := s.routeRepo.GetByID(txCtx, id)
		if err != nil {
			return err
		}

		route, err = s.routeRepo.Update(txCtx, id, input, updatedBy, current.Version)
		if err != nil {
			return err
		}

		return s.auditRepo.Create(txCtx, &domain.AuditLog{
			ID:        uuid.New().String(),
			TenantID:  &route.TenantID,
			UserID:    &updatedBy,
			RequestID: requestID,
			Action:    domain.AuditActionUpdate,
			Resource:  fmt.Sprintf("route.updated:%s", route.ID),
			IPAddress: ipAddress,
		})
	})
	if err != nil {
		return nil, err
	}

	s.eventBus.Publish(event.Event{
		ID:   uuid.New().String(),
		Type: event.EventRouteUpdated,
		Payload: map[string]interface{}{
			"route_id":  route.ID,
			"tenant_id": route.TenantID,
		},
		Timestamp: s.clock.Now(),
	})

	return route, nil
}

// Delete soft-deletes a route.
func (s *RouteService) Delete(ctx context.Context, id, deletedBy, requestID, ipAddress string) error {
	var tenantID string

	err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		current, err := s.routeRepo.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		tenantID = current.TenantID

		if err := s.routeRepo.Delete(txCtx, id); err != nil {
			return err
		}

		return s.auditRepo.Create(txCtx, &domain.AuditLog{
			ID:        uuid.New().String(),
			TenantID:  &current.TenantID,
			UserID:    &deletedBy,
			RequestID: requestID,
			Action:    domain.AuditActionDelete,
			Resource:  fmt.Sprintf("route.deleted:%s", id),
			IPAddress: ipAddress,
		})
	})
	if err != nil {
		return err
	}

	s.eventBus.Publish(event.Event{
		ID:   uuid.New().String(),
		Type: event.EventRouteDeleted,
		Payload: map[string]interface{}{
			"route_id":  id,
			"tenant_id": tenantID,
		},
		Timestamp: s.clock.Now(),
	})

	return nil
}

// ListByTenant lists routes for a tenant with optional filtering.
func (s *RouteService) ListByTenant(ctx context.Context, filter domain.RouteFilter) (domain.PageResult[domain.Route], error) {
	return s.routeRepo.ListByTenant(ctx, filter)
}

// ListByTenantAndType lists routes for a tenant by type (for routing engine).
func (s *RouteService) ListByTenantAndType(ctx context.Context, tenantID string, routeType domain.RouteType) ([]domain.Route, error) {
	return s.routeRepo.ListByTenantAndType(ctx, tenantID, routeType)
}
