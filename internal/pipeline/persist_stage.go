package pipeline

import (
	"context"
	"fmt"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// PersistStage writes the pipeline results to the database.
//
// It is deliberately "dumb" — it copies DeliveryOutcome fields verbatim
// into a single UPDATE. No business logic, no event publishing,
// no retry decisions, no timestamp policy.
//
// Reads:   DeliveryOutcome (Status, ExternalID, ConnectorID, RouteID,
//                            Parts, ErrorCode, ErrorMessage, timestamps)
//          Message (ID, Version, Price, Cost)
// Writes:  MessageRepository.UpdateStatus
// Produces: nothing (terminal stage)
type PersistStage struct {
	repo domain.MessageRepository
}

// NewPersistStage creates a PersistStage with the given repository.
func NewPersistStage(repo domain.MessageRepository) *PersistStage {
	return &PersistStage{repo: repo}
}

// Name returns the stage name for logging and metrics.
func (s *PersistStage) Name() string {
	return "persist"
}

var (
	ErrPersistNoMessage         = fmt.Errorf("persist stage: no message")
	ErrPersistNoDeliveryOutcome = fmt.Errorf("persist stage: no delivery outcome")
	ErrPersistUpdateFailed      = fmt.Errorf("persist stage: update failed")
)

// Process copies DeliveryOutcome fields to the database.
func (s *PersistStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state.Message == nil {
		return nil, ErrPersistNoMessage
	}
	if state.DeliveryOutcome == nil {
		return nil, ErrPersistNoDeliveryOutcome
	}

	msg := state.Message
	delivery := state.DeliveryOutcome

	// Build UpdateMessageInput — direct copy from DeliveryOutcome.
	status := string(delivery.Status)
	input := domain.UpdateMessageInput{
		Status:      (*domain.MessageStatus)(&status),
		ExternalID:  strPtr(delivery.ExternalID),
		ConnectorID: strPtr(delivery.ConnectorID),
		RouteID:     strPtr(delivery.RouteID),
		Parts:       intPtr(delivery.Parts),
		ErrorCode:   strPtr(delivery.ErrorCode),
		ErrorMessage: strPtr(delivery.ErrorMessage),

		// Timestamps — copied directly from DeliveryOutcome.
		SentAt:      delivery.SentAt,
		DeliveredAt: delivery.DeliveredAt,
		FailedAt:    delivery.FailedAt,
	}

	// Price/Cost from domain message — if available.
	if msg.Price > 0 {
		input.Price = &msg.Price
	}
	if msg.Cost > 0 {
		input.Cost = &msg.Cost
	}

	_, err := s.repo.UpdateStatus(ctx, msg.ID, input, msg.Version)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrPersistUpdateFailed, msg.ID, err)
	}

	return state, nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtr(i int) *int {
	return &i
}
