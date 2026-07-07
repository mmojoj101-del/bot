package pipeline

import (
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/domain/events"
)

// PipelineState carries message context through all pipeline stages.
// Each stage reads from relevant fields and populates exactly ONE output field.
// Nil pointers indicate "not yet set by that stage" — no heuristic invariants.
//
// Artifact chain (each stage → one new artifact, immutable thereafter):
//
//	Message ──→ PreparedMessage ──→ RoutingDecision ──→ SendResult ──→ DeliveryOutcome ──→ DomainEvents
//	  (input)    PrepareStage          RouteStage         SendStage       HandleResultStage   BuildEventsStage
//	                                                                     ├─ PersistStage (copies DeliveryOutcome to DB)
//	                                                                     ├─ BuildEventsStage (builds events from DeliveryOutcome + Message)
//	                                                                     ├─ EmitStage (publishes DomainEvents)
//	                                                                     └─ RetryDecorator (schedules retry)
type PipelineState struct {
	// Message is the canonical domain message being processed. Immutable in the pipeline.
	Message *domain.Message

	// Prepared is the output of PrepareStage (nil = not yet prepared).
	// SendStage copies this before passing to the sender (safe copy pattern).
	Prepared *domain.PreparedMessage

	// Decision is the routing decision set by RouteStage (nil = undecided).
	// HandleResultStage reads it to copy ConnectorID/RouteID to DeliveryOutcome.
	// After HandleResultStage, no one reads Decision directly.
	Decision *RoutingDecision

	// SendResult is the result from the connector (set by SendStage, nil = not sent).
	// HandleResultStage is the LAST consumer of SendResult.
	// After HandleResultStage maps it to DeliveryOutcome, SendResult is ignored.
	SendResult *SendResult

	// DeliveryOutcome is the business interpretation of SendResult.
	// Produced by HandleResultStage — self-contained, carries everything
	// needed by subsequent stages (PersistStage, BuildEventsStage, RetryDecorator).
	DeliveryOutcome *DeliveryOutcome

	// DomainEvents is an ordered list of domain events produced by BuildEventsStage.
	// EmitStage publishes them (and nothing else).
	DomainEvents []events.EventEnvelope

	// TraceID is the cross-lifecycle trace identifier.
	TraceID string
}

// RoutingDecision is defined in the domain package.
// It is an alias here so PipelineState.Decision reads naturally
// without importing the domain package in every file.
type RoutingDecision = domain.RoutingDecision

// PreparedMessage is defined in the domain package and shared between
// PipelineState.Prepared (prepareStage output) and domain.SendRequest.Prepared
// (sender input). This alias avoids an import rename in pipeline code.
type PreparedMessage = domain.PreparedMessage

// SendResult is the output from a Connector.Send() call.
// HandleResultStage is the LAST consumer. After it maps SendResult to
// DeliveryOutcome, no subsequent stage reads SendResult.
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
	Acceptance domain.AcceptanceKind
	Status     domain.MessageStatus
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
// Produced by HandleResultStage — pure logic, no DB, no event bus.
// It is SELF-CONTAINED: everything downstream needs is in this struct.
//
// After HandleResultStage, no stage reads SendResult or Decision directly.
// PersistStage copies DeliveryOutcome fields to DB.
// BuildEventsStage builds domain events from DeliveryOutcome + Message.
//
// Timestamps are set by HandleResultStage based on Status semantics:
//
//	Sent/Delivered → SentAt = now
//	Delivered      → DeliveredAt = now
//	Failed         → FailedAt = now, SentAt = now (retroactive if never sent)
//
// Terminality is derived via IsTerminal().
// Sent is ambiguous: AwaitingDLR=true → non-terminal, false → terminal.
type DeliveryOutcome struct {
	// Status is the canonical message status after handling.
	Status domain.MessageStatus

	// FailureKind classifies the cause. Empty = success.
	FailureKind FailureKind

	// Reason is a human-readable explanation (e.g., "provider returned 500").
	Reason string

	// AwaitingDLR is true only when Status=Sent and DLR is expected.
	// This is the only case where Sent is non-terminal.
	AwaitingDLR bool

	// Retryable is true if the connector indicated this error was retryable.
	// Copied from SendResult so BuildEventsStage doesn't need SendResult.
	Retryable bool

	// --- Artifacts from SendResult — carried forward from SendResult/Decision ---
	// These are populated by HandleResultStage so downstream stages
	// (PersistStage, BuildEventsStage) don't need to read SendResult or Decision.
	ExternalID   string `json:"external_id"`    // provider-side message ID
	ConnectorID  string `json:"connector_id"`   // from RoutingDecision
	RouteID      string `json:"route_id"`       // from RoutingDecision
	Parts        int    `json:"parts"`          // message part count
	ErrorCode    string `json:"error_code"`     // provider error code
	ErrorMessage string `json:"error_message"`  // provider error message

	// --- Timestamps ---
	SentAt      *time.Time
	DeliveredAt *time.Time
	FailedAt    *time.Time
}

// NewDeliveryOutcome creates a DeliveryOutcome with validated fields.
func NewDeliveryOutcome(status domain.MessageStatus, failureKind FailureKind, awaitingDLR bool, reason string) DeliveryOutcome {
	return DeliveryOutcome{
		Status:      status,
		FailureKind: failureKind,
		AwaitingDLR: awaitingDLR,
		Reason:      reason,
	}
}

// IsTerminal returns true if the outcome is final.
// For most statuses it delegates to domain.IsTerminalStatus.
// Sent is ambiguous — AwaitingDLR distinguishes terminal (no DLR)
// from non-terminal (DLR expected).
func (d *DeliveryOutcome) IsTerminal() bool {
	if d.Status == domain.MessageStatusSent {
		return !d.AwaitingDLR
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
