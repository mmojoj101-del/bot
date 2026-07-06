package pipeline

import (
	"context"
	"fmt"
)

// SendStage sends the prepared message through the chosen connector.
// It does NOT handle retry, backoff, circuit breakers, or metrics.
// Those concerns belong to separate stages or subscribers.
//
// Lifecycle:
//  1. Look up the connector by Decision.ConnectorID via ConnectorRegistry
//  2. Call connector.Send(ctx, SendRequest)
//  3. Store the result in PipelineState.SendResult
type SendStage struct {
	registry ConnectorRegistry
}

// NewSendStage creates a SendStage with the given ConnectorRegistry.
func NewSendStage(registry ConnectorRegistry) *SendStage {
	return &SendStage{registry: registry}
}

// Name returns the stage name for logging and metrics.
func (s *SendStage) Name() string {
	return "send"
}

// Process sends the prepared message through the routed connector.
// Returns an error if:
//   - Decision is nil (no route chosen)
//   - SendRequest is nil (not prepared)
//   - Connector not found in registry
//   - Connector.Send returns an error
func (s *SendStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state.Decision == nil {
		return nil, fmt.Errorf("send stage: no routing decision — cannot determine connector")
	}
	if state.SendRequest == nil {
		return nil, fmt.Errorf("send stage: no send request — message not prepared")
	}

	connector, err := s.registry.Get(ctx, state.Decision.ConnectorID)
	if err != nil {
		return nil, fmt.Errorf("send stage: connector %q not available: %w", state.Decision.ConnectorID, err)
	}

	result, err := connector.Send(ctx, state.SendRequest)
	if err != nil {
		return nil, fmt.Errorf("send stage: connector %q returned error: %w", state.Decision.ConnectorID, err)
	}

	state.SendResult = result
	return state, nil
}
