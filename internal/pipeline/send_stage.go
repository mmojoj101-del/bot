package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// SendStage sends the prepared message through the chosen connector.
// It does NOT handle retry, backoff, circuit breakers, or metrics.
// Those concerns belong to separate stages or subscribers.
//
// Lifecycle:
//  1. Resolve the sender by Decision.ConnectorID via ConnectorRegistry
//  2. Build domain.SendRequest from pipeline state (without mutating domain.Message)
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
	// ErrNoRoute is returned when SendStage has no routing decision.
	ErrNoRoute = fmt.Errorf("send stage: no routing decision")
	// ErrNoSendRequest is returned when SendStage has no prepared request.
	ErrNoSendRequest = fmt.Errorf("send stage: no prepared send request")
)

// Process sends the prepared message through the routed connector.
func (s *SendStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state.Decision == nil {
		return nil, ErrNoRoute
	}
	if state.SendRequest == nil {
		return nil, ErrNoSendRequest
	}

	// Resolve sender from registry (domain.Sender, not a pipeline-specific connector)
	sender, err := s.registry.Resolve(ctx, state.Decision.ConnectorID)
	if err != nil {
		return nil, fmt.Errorf("send stage: connector %q not available: %w", state.Decision.ConnectorID, err)
	}

	// Build domain-level SendRequest from pipeline state:
	//   - Message: canonical message (unchanged — Pipeline never mutates it)
	//   - Destination/Encoding/Parts: from PrepareStage (via pipeline.SendRequest)
	//   - Timeout: reasonable default (connector config may override)
	sendReq := domain.SendRequest{
		Message:     state.Message,
		Timeout:     30 * time.Second,
		Destination: state.SendRequest.Destination,
		Encoding:    state.SendRequest.Encoding,
		Parts:       state.SendRequest.Parts,
	}

	// Call sender.Send with value-type SendRequest (connector cannot mutate it)
	domainResult, err := sender.Send(ctx, sendReq)
	if err != nil {
		return nil, fmt.Errorf("send stage: connector %q returned error: %w", state.Decision.ConnectorID, err)
	}

	// Map domain.SendResult to pipeline.SendResult
	state.SendResult = &SendResult{
		Success:      domainResult.ExternalID != "",
		ExternalID:   domainResult.ExternalID,
		Parts:        domainResult.Parts,
		ErrorCode:    "",
		ErrorMessage: "",
	}

	return state, nil
}
