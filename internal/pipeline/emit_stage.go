package pipeline

import (
	"context"
	"fmt"

	"github.com/raghna/fury-sms-gateway/internal/domain/events"
)

// EmitStage publishes Events from PipelineState.
//
// It is deliberately "dumb":
//   - Does NOT create events (BuildEventsStage does that)
//   - Does NOT decide which events to publish
//   - Does NOT handle retry, metrics, or business logic
//   - Simply iterates Events and calls publisher.Publish()
//
// Reads:   PipelineState.Events
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

// Process publishes all Events in order.
func (s *EmitStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if s.publisher == nil {
		return nil, ErrEmitEmptyPublisher
	}

	for i := range state.Events {
		if err := s.publisher.Publish(ctx, state.Events[i]); err != nil {
			return nil, fmt.Errorf("emit stage: publish event %q: %w",
				state.Events[i].EventType, err)
		}
	}

	// Clear the events list — published events are final.
	state.Events = nil

	return state, nil
}
