package pipeline

import (
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// PipelineState carries message context through all pipeline stages.
// Each stage reads from relevant fields and populates exactly ONE output field.
// Nil pointers indicate "not yet set by that stage" — no heuristic invariants.
//
// Artifact chain (each stage → one new artifact, immutable thereafter):
//
//	Message ──→ PreparedMessage ──→ RoutingDecision ──→ SendResult ──→ DeliveryOutcome
//	  (input)    PrepareStage          RouteStage         SendStage       HandleResultStage
//	                                                                     ├─ PersistStage (writes)
//	                                                                     └─ EmitStage (publishes)
type PipelineState struct {
	// Message is the canonical domain message being processed. Immutable in the pipeline.
	Message *domain.Message

	// Prepared is the output of PrepareStage (nil = not yet prepared).
	// SendStage copies this before passing to the sender (safe copy pattern).
	Prepared *domain.PreparedMessage

	// Decision is the routing decision set by RouteStage (nil = undecided).
	// No stage modifies it after RouteStage sets it.
	Decision *RoutingDecision

	// SendResult is the result from the connector (set by SendStage, nil = not sent).
	// Immutable after SendStage — subsequent stages read but never modify it.
	SendResult *SendResult

	// DeliveryOutcome is the business decision after interpreting SendResult
	// (set by HandleResultStage). Contains the final or intermediary status
	// and retry policy — PersistStage and EmitStage read it, never modify it.
	DeliveryOutcome *DeliveryOutcome

	// TraceID is the cross-lifecycle trace identifier.
	TraceID string
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

// PreparedMessage is defined in the domain package and shared between
// PipelineState.Prepared (prepareStage output) and domain.SendRequest.Prepared
// (sender input). This alias avoids an import rename in pipeline code.
type PreparedMessage = domain.PreparedMessage

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

// DeliveryOutcome is the business decision produced by HandleResultStage.
// It interprets SendResult + Message context and decides the final or
// intermediary disposition of the message.
// PersistStage writes it, EmitStage publishes events from it — neither modifies it.
type DeliveryOutcome struct {
	// Status is the canonical message status after handling.
	Status domain.MessageStatus

	// Retry indicates whether the message should be retried.
	Retry bool

	// RetryAfter is the delay before the next retry attempt (zero = immediate).
	RetryAfter time.Duration

	// Reason is a human-readable explanation of the outcome.
	Reason string

	// Terminal is true if no further processing should happen for this message.
	// Terminal messages are persisted and emitted; non-terminal may trigger retry.
	Terminal bool
}

// NewPipelineState creates a new PipelineState for a message.
func NewPipelineState(msg *domain.Message, traceID string) *PipelineState {
	return &PipelineState{
		Message: msg,
		TraceID: traceID,
	}
}
