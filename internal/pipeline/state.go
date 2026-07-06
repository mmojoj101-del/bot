package pipeline

import (
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/domain/events"
)

// PipelineState is the single object passed through all pipeline stages.
// It carries everything a stage needs and accumulates results.
type PipelineState struct {
	// Message is the canonical message being processed.
	Message *domain.Message

	// SendRequest is the prepared request for the connector (set by PrepareStage).
	SendRequest *SendRequest

	// Decision is the routing decision (set by RouteStage, never modified after).
	Decision *RoutingDecision

	// SendResult is the result from the connector (set by SendStage).
	SendResult *SendResult

	// Error captures a failure from any stage.
	Error error

	// Attempt is the current retry attempt (0 = first attempt).
	Attempt int

	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int

	// TraceID is the cross-lifecycle trace identifier.
	TraceID string

	// Metadata is a mutable map for stages to pass data downstream.
	// Namespace your keys (e.g., "prepare.encoding", "route.candidates").
	Metadata map[string]interface{}
}

// RoutingDecision is an immutable value object set once by the Routing Engine.
// No stage may modify it after creation.
type RoutingDecision struct {
	RouteID         string
	ConnectorID     string
	StrategyUsed    string   // static, round_robin, failover, weighted
	Priority        int
	Cost            int64    // thousandths of a cent, at selection time
	Reason          string   // why this route was chosen
	CapabilitiesUsed []string
}

// SendRequest carries the pipeline-local results of PrepareStage.
// It is deliberately minimal — only fields that cannot live in
// domain.Message (which is immutable in the pipeline) or that
// are pipeline-internal (Config for connector resolution).
// Everything else flows through domain.SendRequest.
type SendRequest struct {
	// Destination is the normalized phone number (E.164-like format).
	// The raw msg.Destination is preserved on domain.Message.
	Destination string

	// Encoding is the detected message content encoding ("GSM7" or "UCS2").
	Encoding string

	// Parts is the number of SMS parts after splitting.
	Parts int

	// Config is connector-specific configuration loaded by SendStage.
	// Populated from ConnectorRegistry during send; not set by PrepareStage.
	Config []byte
}

// SendResult is the output from a Connector.Send() call.
type SendResult struct {
	Success      bool
	ExternalID   string // provider-side message ID
	Parts        int
	ErrorCode    string
	ErrorMessage string
	Retryable    bool
	RequestsDLR  bool
	Status       domain.MessageStatus
}

// PipelineStateEvent converts a PipelineState into an EventEnvelope.
// This enables stages to emit events without knowing the event bus.
func PipelineStateEvent(state *PipelineState, eventType string, payload events.DomainEvent) events.EventEnvelope {
	return events.EventEnvelope{
		EventType:     eventType,
		TraceID:       state.TraceID,
		TenantID:      state.Message.TenantID,
		CorrelationID: state.Message.ID,
		// Payload is set by the caller with json.RawMessage
	}
}

// NewPipelineState creates a new PipelineState for a message.
func NewPipelineState(msg *domain.Message, traceID string) *PipelineState {
	return &PipelineState{
		Message:     msg,
		TraceID:     traceID,
		Attempt:     0,
		MaxRetries:  3,
		Metadata:    make(map[string]interface{}),
	}
}
