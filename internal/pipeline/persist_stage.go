package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// PersistStage writes the pipeline results to the database.
//
// It is deliberately "dumb" — it reads the finalized artifacts
// (SendResult + DeliveryOutcome + Decision) and translates them
// into a single UPDATE. No business logic, no event publishing,
// no retry decisions.
//
// Reads:   SendResult (transport metadata: ExternalID, Parts, ErrorCode)
//          DeliveryOutcome (business decision: Status, AwaitingDLR)
//          Decision (ConnectorID, RouteID)
//          Message (ID, Version, timestamps for first-write detection)
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
	ErrPersistNoMessage        = fmt.Errorf("persist stage: no message")
	ErrPersistNoSendResult     = fmt.Errorf("persist stage: no send result")
	ErrPersistNoDeliveryOutcome = fmt.Errorf("persist stage: no delivery outcome")
	ErrPersistNoDecision       = fmt.Errorf("persist stage: no routing decision")
	ErrPersistUpdateFailed     = fmt.Errorf("persist stage: update failed")
)

// Process writes SendResult + DeliveryOutcome to the database.
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

	now := time.Now().UTC()

	// Build UpdateMessageInput from SendResult + DeliveryOutcome + Decision.
	status := string(delivery.Status)
	input := domain.UpdateMessageInput{
		Status:      (*domain.MessageStatus)(&status),
		ConnectorID: &decision.ConnectorID,
		RouteID:     &decision.RouteID,
		Parts:       &sr.Parts,
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

	// Timestamps — set only on first transition to the respective status.
	// COALESCE in the SQL handles nil = keep existing.
	if delivery.Status == domain.MessageStatusSent || delivery.Status == domain.MessageStatusDelivered {
		if msg.SentAt == nil {
			input.SentAt = &now
		}
	}
	if delivery.Status == domain.MessageStatusDelivered {
		if msg.DeliveredAt == nil {
			input.DeliveredAt = &now
		}
	}
	if delivery.Status == domain.MessageStatusFailed {
		if msg.FailedAt == nil {
			input.FailedAt = &now
		}
		// If pipeline failed before recording a send timestamp, record it retroactively.
		if msg.SentAt == nil {
			input.SentAt = &now
		}
	}

	// Price/Cost from domain result — if available.
	// These come from the connector's SendResult but aren't mapped
	// to pipeline.SendResult yet. Use message defaults for now.
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
