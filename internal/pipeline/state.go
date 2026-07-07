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
	// Acceptance tells the pipeline how to treat the response semantically.
	// Mapped from domain.SendResult.Acceptance so HandleResultStage doesn't
	// need to guess DLR semantics — the connector knows best.
	Acceptance   domain.AcceptanceKind
	Status       domain.MessageStatus
}

// FailureKind classifies the outcome at the business level.
// It is machine-readable — PersistStage, EmitStage, and metrics use it
// without parsing text. This is NOT a taxonomy of provider error codes;
// provider-specific details stay in SendResult.
//
// Business-level classification (not protocol-specific):
//   None      → success
//   Temporary → retry may help (provider overloaded, timeout, throttled)
//   Permanent → retry will not help (invalid destination, rejected, auth fail)
//   Internal  → system error (panic, DB, config, unexpected)
type FailureKind string

const (
	FailureNone      FailureKind = ""          // success
	FailureTemporary FailureKind = "temporary" // retryable — provider/throttle/timeout
	FailurePermanent FailureKind = "permanent" // non-retryable — rejected/invalid/auth
	FailureInternal  FailureKind = "internal"  // system error — bug/config/panic
)

// DeliveryOutcome is the business interpretation of a SendResult.
// It is produced by HandleResultStage — pure logic, no DB, no event bus.
// PersistStage and EmitStage read it; neither modifies it.
// Retry scheduling (backoff, jitter) is handled by a separate RetryPolicy/RetryDecorator.
//
// Create via NewDeliveryOutcome to ensure field validity.
type DeliveryOutcome struct {
	// Status is the canonical message status after handling.
	Status domain.MessageStatus

	// FailureKind classifies the cause. Empty = success.
	FailureKind FailureKind

	// Terminal is true when no further processing is expected on this message.
	// Most statuses have clear terminality (see IsTerminal). Sent is ambiguous:
	// it can be terminal (no DLR expected) or non-terminal (DLR pending).
	// HandleResultStage sets this based on AcceptanceKind.
	Terminal bool

	// Reason is a human-readable explanation (e.g., "provider returned 500").
	Reason string
}

// NewDeliveryOutcome creates a DeliveryOutcome with validated fields.
func NewDeliveryOutcome(status domain.MessageStatus, failureKind FailureKind, terminal bool, reason string) DeliveryOutcome {
	return DeliveryOutcome{
		Status:      status,
		FailureKind: failureKind,
		Terminal:    terminal,
		Reason:      reason,
	}
}

// IsTerminal returns true if the outcome is final. For most statuses it
// delegates to domain.IsTerminalStatus. Sent is ambiguous — the Terminal
// field distinguishes terminal (no DLR) from non-terminal (DLR pending).
func (d *DeliveryOutcome) IsTerminal() bool {
	if d.Status == domain.MessageStatusSent {
		return d.Terminal
	}
	return domain.IsTerminalStatus(d.Status)
}

// NewPipelineState creates a new PipelineState for a message.
func NewPipelineState(msg *domain.Message, traceID string) *PipelineState {
	return &PipelineState{
		Message: msg,
		TraceID: traceID,
	}
}
