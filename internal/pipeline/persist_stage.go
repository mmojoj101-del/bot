package pipeline

import (
	"context"
	"fmt"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// PersistStage writes the pipeline results to the database.
//
// It is deliberately "dumb" — it reads the finalized artifacts
// (SendResult + DeliveryOutcome + Decision) and copies their values
// into a single UPDATE. No business logic, no event publishing,
// no retry decisions, no timestamp policy.
//
// Reads:   SendResult (transport metadata: ExternalID, Parts, ErrorCode)
//          DeliveryOutcome (Status, timestamps — copied verbatim)
//          Decision (ConnectorID, RouteID)
//          Message (ID, Version)
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
	ErrPersistNoSendResult      = fmt.Errorf("persist stage: no send result")
	ErrPersistNoDeliveryOutcome = fmt.Errorf("persist stage: no delivery outcome")
	ErrPersistNoDecision        = fmt.Errorf("persist stage: no routing decision")
	ErrPersistUpdateFailed      = fmt.Errorf("persist stage: update failed")
)

// Process writes artifacts to the database.
// Timestamps are read from DeliveryOutcome — no status-aware logic here.
func (s *PersistStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state.Message == nil {
		return nil, ErrPersistNoMessage
	}
	if state.SendResult == nil {
		return nil, ErrPersistNoSendResult
	}
	if state.DeliveryOutcome == nil {
		return nil, ErrPersistNoDeliveryOutcome
	}
	if state.Decision == nil {
		return nil, ErrPersistNoDecision
	}

	msg := state.Message
	sr := state.SendResult
	decision := state.Decision
	delivery := state.DeliveryOutcome

	// Build UpdateMessageInput — direct copy from state artifacts.
	status := string(delivery.Status)
	input := domain.UpdateMessageInput{
		Status:      (*domain.MessageStatus)(&status),
		ConnectorID: &decision.ConnectorID,
		RouteID:     &decision.RouteID,
		Parts:       &sr.Parts,

		// Timestamps — copied directly from DeliveryOutcome.
		// HandleResultStage determines when each timestamp should be set.
		SentAt:      delivery.SentAt,
		DeliveredAt: delivery.DeliveredAt,
		FailedAt:    delivery.FailedAt,
	}

	// ExternalID from provider (SendResult)
	if sr.ExternalID != "" {
		input.ExternalID = &sr.ExternalID
	}

	// Error details (provider status / error code)
	if sr.ErrorCode != "" {
		ec := sr.ErrorCode
		input.ErrorCode = &ec
	}
	if sr.ErrorMessage != "" {
		em := sr.ErrorMessage
		input.ErrorMessage = &em
	}

	// Price/Cost from domain result — if available.
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
