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
	// Prepared is nil until PrepareStage runs — no heuristic invariants.
	if state.Prepared == nil {
		return nil, ErrNoPrepared
	}

	// Resolve sender from registry (domain.Sender, not a pipeline interface)
	sender, err := s.registry.Resolve(ctx, state.Decision.ConnectorID)
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %w", ErrConnectorUnavailable, state.Decision.ConnectorID, err)
	}

	// Dereference to copy: sender gets its own PreparedMessage, preventing mutation.
	// PipelineState.Prepared remains owned by the pipeline and is immutable.
	prepared := *state.Prepared // value copy (defensive)

	// Build domain-level SendRequest with the local copy.
	sendReq := domain.SendRequest{
		Message:   state.Message,
		Prepared:  &prepared,
	}

	// Call sender.Send with value-type SendRequest (connector cannot mutate it)
	domainResult, err := sender.Send(ctx, sendReq)
	if err != nil {
		return nil, fmt.Errorf("send stage: connector %q returned error: %w", state.Decision.ConnectorID, err)
	}

	// Defensive copy with deep copy of reference fields.
	// The shallow struct copy (dr := *domainResult) duplicates the slice header
	// but not the backing array — RawResponse would still share memory.
	// We deep-copy RawResponse to fully sever ownership from the sender.
	dr := *domainResult
	if dr.RawResponse != nil {
		dr.RawResponse = append([]byte(nil), dr.RawResponse...)
	}

	// Map domain.SendResult to pipeline.SendResult.
	// PipelineState.SendResult is immutable after this point — subsequent
	// stages read it but never modify it (same ownership boundary as Prepared).
	state.SendResult = &SendResult{
		Success:      true,
		ExternalID:   dr.ExternalID,
		Parts:        dr.Parts,
		ErrorCode:    dr.ProviderStatus,
		ErrorMessage: "",
	}

	return state, nil
}
