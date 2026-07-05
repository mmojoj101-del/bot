package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
)

// MessageService handles message lifecycle management.
type MessageService struct {
	messageRepo domain.MessageRepository
	auditRepo   domain.AuditLogRepository
	eventBus    event.Bus
	clock       domain.Clock
}

// NewMessageService creates a new message service.
func NewMessageService(
	messageRepo domain.MessageRepository,
	auditRepo domain.AuditLogRepository,
	eventBus event.Bus,
	clock domain.Clock,
) *MessageService {
	return &MessageService{
		messageRepo: messageRepo,
		auditRepo:   auditRepo,
		eventBus:    eventBus,
		clock:       clock,
	}
}

// Create creates a new outbound message (Outbox pattern: store first, dispatch later).
func (s *MessageService) Create(ctx context.Context, input domain.CreateMessageInput, createdBy, requestID, ipAddress string) (*domain.Message, error) {
	// Check idempotency if client_ref is provided
	if input.ClientRef != "" {
		existing, err := s.messageRepo.GetByClientRef(ctx, input.TenantID, input.ClientRef)
		if err == nil && existing != nil {
			return existing, nil
		}
	}

	input.Direction = domain.DirectionOutbound

	msg, err := s.messageRepo.Create(ctx, input, createdBy)
	if err != nil {
		return nil, err
	}

	s.eventBus.Publish(event.Event{
		ID:   uuid.New().String(),
		Type: event.EventMessageAccepted,
		Payload: event.MessageEventPayload{
			MessageID:   msg.ID,
			TenantID:    msg.TenantID,
			ClientID:    msg.ClientID,
			Status:      msg.Status,
			Source:      msg.Source,
			Destination: msg.Destination,
		},
		Timestamp: s.clock.Now(),
	})

	return msg, nil
}

// UpdateStatus updates a message's status with state machine validation.
func (s *MessageService) UpdateStatus(ctx context.Context, id string, input domain.UpdateMessageInput, requestID string) (*domain.Message, error) {
	current, err := s.messageRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Validate state transition if changing status
	if input.Status != nil && *input.Status != current.Status {
		if err := domain.ValidateTransition(current.Status, *input.Status); err != nil {
			return nil, fmt.Errorf("state machine: %w", err)
		}
	}

	msg, err := s.messageRepo.UpdateStatus(ctx, id, input, current.Version)
	if err != nil {
		return nil, err
	}

	// Publish event based on new status
	eventType := s.statusToEvent(msg.Status)
	if eventType != "" {
		s.eventBus.Publish(event.Event{
			ID:   uuid.New().String(),
			Type: eventType,
			Payload: event.MessageEventPayload{
				MessageID:   msg.ID,
				TenantID:    msg.TenantID,
				ClientID:    msg.ClientID,
				Status:      msg.Status,
				Source:      msg.Source,
				Destination: msg.Destination,
				ErrorCode:   safeStr(msg.ErrorCode),
			},
			Timestamp: s.clock.Now(),
		})
	}

	return msg, nil
}

// GetByID retrieves a message by ID.
func (s *MessageService) GetByID(ctx context.Context, id string) (*domain.Message, error) {
	return s.messageRepo.GetByID(ctx, id)
}

// GetByClientRef retrieves a message by client reference (idempotency).
func (s *MessageService) GetByClientRef(ctx context.Context, tenantID, clientRef string) (*domain.Message, error) {
	return s.messageRepo.GetByClientRef(ctx, tenantID, clientRef)
}

// List lists messages with filters.
func (s *MessageService) List(ctx context.Context, filter domain.MessageFilter) (domain.PageResult[domain.Message], error) {
	return s.messageRepo.List(ctx, filter)
}

// Count counts messages matching filters.
func (s *MessageService) Count(ctx context.Context, filter domain.MessageFilter) (int64, error) {
	return s.messageRepo.Count(ctx, filter)
}

// AppendDLR appends a delivery receipt log entry.
func (s *MessageService) AppendDLR(ctx context.Context, dlr *domain.DLRRecord) error {
	return s.messageRepo.AppendDLR(ctx, dlr)
}

// QueueMessage transitions a message from accepted → queued.
func (s *MessageService) QueueMessage(ctx context.Context, id string) (*domain.Message, error) {
	status := domain.MessageStatusQueued
	return s.UpdateStatus(ctx, id, domain.UpdateMessageInput{Status: &status}, "")
}

// SendMessage transitions a message from queued → sending.
func (s *MessageService) SendMessage(ctx context.Context, id string, connectorID, routeID string) (*domain.Message, error) {
	status := domain.MessageStatusSending
	return s.UpdateStatus(ctx, id, domain.UpdateMessageInput{
		Status:      &status,
		ConnectorID: &connectorID,
		RouteID:     &routeID,
	}, "")
}

// MarkSent transitions a message from sending → sent.
func (s *MessageService) MarkSent(ctx context.Context, id, externalID string, parts int, price, cost int64) (*domain.Message, error) {
	status := domain.MessageStatusSent
	now := s.clock.Now()
	return s.UpdateStatus(ctx, id, domain.UpdateMessageInput{
		Status:     &status,
		ExternalID: &externalID,
		Parts:      &parts,
		Price:      &price,
		Cost:       &cost,
		SentAt:     &now,
	}, "")
}

// MarkDelivered transitions a message from sent → delivered.
func (s *MessageService) MarkDelivered(ctx context.Context, id, dlrID string) (*domain.Message, error) {
	status := domain.MessageStatusDelivered
	dlrStatus := domain.DLRStatusDelivered
	now := s.clock.Now()
	return s.UpdateStatus(ctx, id, domain.UpdateMessageInput{
		Status:      &status,
		DLRStatus:   &dlrStatus,
		DLRID:       &dlrID,
		DeliveredAt: &now,
	}, "")
}

// MarkFailed transitions a message from any intermediate state → failed.
func (s *MessageService) MarkFailed(ctx context.Context, id, errorCode, errorMessage string) (*domain.Message, error) {
	status := domain.MessageStatusFailed
	dlrStatus := domain.DLRStatusFailed
	now := s.clock.Now()
	return s.UpdateStatus(ctx, id, domain.UpdateMessageInput{
		Status:       &status,
		DLRStatus:    &dlrStatus,
		ErrorCode:    &errorCode,
		ErrorMessage: &errorMessage,
		FailedAt:     &now,
	}, "")
}

// MarkRetrying transitions a message from sent → retrying.
func (s *MessageService) MarkRetrying(ctx context.Context, id, errorCode, errorMessage string) (*domain.Message, error) {
	status := domain.MessageStatusRetrying
	return s.UpdateStatus(ctx, id, domain.UpdateMessageInput{
		Status:       &status,
		ErrorCode:    &errorCode,
		ErrorMessage: &errorMessage,
	}, "")
}

// statusToEvent maps a message status to an event type.
func (s *MessageService) statusToEvent(status domain.MessageStatus) string {
	switch status {
	case domain.MessageStatusQueued:
		return event.EventMessageQueued
	case domain.MessageStatusSending:
		return event.EventMessageSending
	case domain.MessageStatusSent:
		return event.EventMessageSent
	case domain.MessageStatusDelivered:
		return event.EventMessageDelivered
	case domain.MessageStatusFailed:
		return event.EventMessageFailed
	default:
		return ""
	}
}

func safeStr(s string) string {
	if s == "" {
		return ""
	}
	return s
}
