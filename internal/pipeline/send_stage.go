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
//  1. Resolve the connector by Decision.ConnectorID via ConnectorRegistry
//  2. Build domain.SendRequest from PreparedMessage + PipelineState
//  3. Call connector.Send(ctx, domain.SendRequest)
//  4. Store the result in PipelineState.SendResult
//
// SendStage is protocol-agnostic: no if/switch on connector type.
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
	ErrNoRoute               = fmt.Errorf("send stage: no routing decision")
	ErrNoPrepared            = fmt.Errorf("send stage: no prepared message")
	ErrConnectorUnavailable  = fmt.Errorf("send stage: connector not available")
)

// Process sends the prepared message through the routed connector.
func (s *SendStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state.Decision == nil {
		return nil, ErrNoRoute
	}
	if state.Prepared == nil {
		return nil, ErrNoPrepared
	}

	// Resolve connector from registry.
	conn, err := s.registry.Get(state.Decision.ConnectorID)
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %w", ErrConnectorUnavailable, state.Decision.ConnectorID, err)
	}

	// Defensive copy: sender gets its own PreparedMessage copy.
	prepared := *state.Prepared

	sendReq := &domain.SendRequest{
		Message:  state.Message,
		Prepared: &prepared,
	}

	// Call the connector — no protocol-specific branches here.
	domainResult, err := conn.Send(ctx, sendReq)
	if err != nil {
		return nil, fmt.Errorf("send stage: connector %q returned error: %w", state.Decision.ConnectorID, err)
	}

	// Defensive copy: shallow copy of scalar fields only.
	dr := *domainResult

	// Map domain.SendResult → pipeline.SendResult.
	state.SendResult = &SendResult{
		Success:      true,
		ExternalID:   dr.ExternalID,
		Parts:        dr.Parts,
		ErrorCode:    dr.ProviderStatus,
		ErrorMessage: "",
		Acceptance:   dr.Acceptance,
	}

	return state, nil
}
