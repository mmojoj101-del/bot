package pipeline

import (
	"context"
	"fmt"

	"github.com/raghna/fury-sms-gateway/internal/domain/events"
)

// EmitStage publishes PendingEvents from PipelineState.
//
// It is deliberately "dumb":
//   - Does NOT create events (HandleResultStage does that)
//   - Does NOT decide which events to publish
//   - Does NOT handle retry, metrics, or business logic
//   - Simply iterates PendingEvents and calls publisher.Publish()
//
// Reads:   PipelineState.PendingEvents
// Writes:  nothing (side effect: publisher.Publish)
// Produces: nothing (terminal stage)
type EmitStage struct {
	publisher events.DomainEventPublisher
}

// NewEmitStage creates an EmitStage with the given publisher.
func NewEmitStage(publisher events.DomainEventPublisher) *EmitStage {
	return &EmitStage{publisher: publisher}
}

// Name returns the stage name for logging and metrics.
func (s *EmitStage) Name() string {
	return "emit"
}

var ErrEmitEmptyPublisher = fmt.Errorf("emit stage: nil publisher")

// Process publishes all PendingEvents in order.
func (s *EmitStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if s.publisher == nil {
		return nil, ErrEmitEmptyPublisher
	}

	for i := range state.PendingEvents {
		if err := s.publisher.Publish(ctx, state.PendingEvents[i]); err != nil {
			return nil, fmt.Errorf("emit stage: publish event %q: %w",
				state.PendingEvents[i].EventType, err)
		}
	}

	// Clear the pending list — events are published.
	state.PendingEvents = nil

	return state, nil
}
