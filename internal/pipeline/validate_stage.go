package pipeline

import (
	"context"
	"errors"
	"fmt"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

var (
	// ErrNilMessage is returned when PipelineState.Message is nil.
	ErrNilMessage = errors.New("message is nil")

	// ErrMissingTenantID is returned when the message has no tenant.
	ErrMissingTenantID = errors.New("message missing tenant_id")

	// ErrMissingDestination is returned when the message has no destination address.
	ErrMissingDestination = errors.New("message missing destination")

	// ErrMissingContent is returned when the message has no text content.
	ErrMissingContent = errors.New("message missing text content")

	// ErrInvalidStatus is returned when the message status does not allow processing.
	ErrInvalidStatus = errors.New("message status does not allow processing")
)

// ValidateStage checks that PipelineState and the enclosed message are valid
// for further processing. It performs pure business validation only:
//   - No network calls
//   - No database queries
//   - No routing or normalization
//   - No deduplication or rate limiting
//
// Returns a domain-level error for each violation.
type ValidateStage struct{}

// NewValidateStage creates a new ValidateStage.
func NewValidateStage() *ValidateStage {
	return &ValidateStage{}
}

// Name returns the stage name for logging and metrics.
func (s *ValidateStage) Name() string {
	return "validate"
}

// Process validates the PipelineState and returns an appropriate error
// if the state or message is not suitable for pipeline execution.
func (s *ValidateStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if state == nil {
		return nil, ErrNilMessage
	}

	msg := state.Message
	if msg == nil {
		return nil, ErrNilMessage
	}

	if msg.TenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is empty", ErrMissingTenantID)
	}

	if msg.Destination == "" {
		return nil, fmt.Errorf("%w: destination is required", ErrMissingDestination)
	}

	if msg.Text == "" {
		return nil, fmt.Errorf("%w: text is required for SMS messages", ErrMissingContent)
	}

	// Check that the message status allows processing.
	// Only "queued" messages should enter the pipeline.
	if msg.Status != domain.MessageStatusQueued {
		return nil, fmt.Errorf("%w: current status is %q, expected queued", ErrInvalidStatus, msg.Status)
	}

	return state, nil
}
