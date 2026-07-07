package pipeline

import (
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// PipelineState carries message context through all pipeline stages.
// Each stage reads from relevant fields and populates its output field.
// Fields are deliberately typed — no map[string]any, no Error duplication.
// Prepared is a value type (copy on assignment) to prevent mutation.
type PipelineState struct {
	// Message is the canonical domain message being processed. Immutable in the pipeline.
	Message *domain.Message

	// Prepared is the output of PrepareStage (normalized destination, encoding, parts).
	// Value type — any stage that reads it gets a copy.
	Prepared domain.PreparedMessage

	// Decision is the routing decision set by RouteStage (pointer = nil when undecided).
	// After RouteStage: treat as immutable — do not modify its fields.
	Decision *RoutingDecision

	// SendResult is the result from the connector (set by SendStage).
	SendResult *SendResult

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



// NewPipelineState creates a new PipelineState for a message.
func NewPipelineState(msg *domain.Message, traceID string) *PipelineState {
	return &PipelineState{
		Message: msg,
		TraceID: traceID,
	}
}
