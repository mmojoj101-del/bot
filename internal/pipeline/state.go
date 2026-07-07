package pipeline

import (
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

	// DeliveryOutcome is the business interpretation of SendResult
	// (set by HandleResultStage). PersistStage and EmitStage read it
	// but never modify it. Retry scheduling is external (RetryPolicy).
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

// FailureKind classifies the cause of a non-success outcome.
// It is machine-readable — PersistStage, EmitStage, and metrics use it without parsing text.
type FailureKind string

const (
	FailureNone      FailureKind = ""           // success — no failure
	FailureProvider  FailureKind = "provider"   // provider returned an error code
	FailureTransport FailureKind = "transport"  // transport-level failure (connection, timeout)
	FailureRejected  FailureKind = "rejected"   // message rejected (invalid destination, auth)
	FailureInternal  FailureKind = "internal"   // internal pipeline or system error
)

// DeliveryOutcome is the business interpretation of a SendResult.
// It is produced by HandleResultStage — pure logic, no DB, no event bus.
// PersistStage and EmitStage read it; neither modifies it.
// Retry scheduling (backoff, jitter) is handled by a separate RetryPolicy/RetryDecorator.
type DeliveryOutcome struct {
	// Status is the canonical message status after handling.
	Status domain.MessageStatus

	// FailureKind classifies the cause. Empty = success.
	FailureKind FailureKind

	// Reason is a human-readable explanation (e.g., "provider returned 500").
	Reason string
}

// IsTerminal returns true if the outcome is final and no further processing
// should occur for this message. Derived from Status — no redundant field.
func (d *DeliveryOutcome) IsTerminal() bool {
	switch d.Status {
	case domain.MessageStatusDelivered, domain.MessageStatusFailed:
		return true
	default:
		return false
	}
}

// NewPipelineState creates a new PipelineState for a message.
func NewPipelineState(msg *domain.Message, traceID string) *PipelineState {
	return &PipelineState{
		Message: msg,
		TraceID: traceID,
	}
}
