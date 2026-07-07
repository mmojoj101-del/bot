package pipeline

import (
	"context"
	"fmt"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// SendStage sends the prepared message through the chosen connector.
// It does NOT handle retry, backoff, circuit breakers, or metrics.
// Those concerns belong to separate stages or subscribers.
//
// Lifecycle:
//  1. Resolve the sender by Decision.ConnectorID via ConnectorRegistry
//  2. Build domain.SendRequest from PreparedMessage + PipelineState
//  3. Call sender.Send(ctx, domain.SendRequest)
//  4. Store the result in PipelineState.SendResult
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

var (
	ErrNoRoute         = fmt.Errorf("send stage: no routing decision")
	ErrNoPrepared      = fmt.Errorf("send stage: no prepared message")
	ErrConnectorUnavailable = fmt.Errorf("send stage: connector not available")
)

// Process sends the prepared message through the routed connector.
func (s *SendStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state.Decision == nil {
		return nil, ErrNoRoute
	}
	// Prepared is a value type; zero value means PrepareStage hasn't run.
	if state.Prepared.Destination == "" && state.Prepared.Encoding == "" {
		return nil, ErrNoPrepared
	}

	// Resolve sender from registry (domain.Sender, not a pipeline interface)
	sender, err := s.registry.Resolve(ctx, state.Decision.ConnectorID)
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %w", ErrConnectorUnavailable, state.Decision.ConnectorID, err)
	}

	// Copy Prepared to a local variable so the sender gets its own copy.
	// This prevents the sender from mutating PipelineState.Prepared.
	prepared := state.Prepared // value copy

	// Build domain-level SendRequest:
	//   - Message: canonical domain message (unchanged — pipeline never mutates it)
	//   - Prepared: local copy (sender cannot see or modify PipelineState's copy)
	sendReq := domain.SendRequest{
		Message:   state.Message,
		Prepared:  &prepared,
	}

	// Call sender.Send with value-type SendRequest (connector cannot mutate it)
	domainResult, err := sender.Send(ctx, sendReq)
	if err != nil {
		return nil, fmt.Errorf("send stage: connector %q returned error: %w", state.Decision.ConnectorID, err)
	}

	// Map domain.SendResult to pipeline.SendResult.
	// Success = the provider accepted the message (no transport/protocol error).
	// Domain-level details (external_id, provider_status) are in the result.
	state.SendResult = &SendResult{
		Success:      true,
		ExternalID:   domainResult.ExternalID,
		Parts:        domainResult.Parts,
		ErrorCode:    domainResult.ProviderStatus,
		ErrorMessage: "",
	}

	return state, nil
}
