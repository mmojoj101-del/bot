package pipeline

import (
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// PipelineState is the single object passed through all pipeline stages.
// It carries everything a stage needs and accumulates results.
// PipelineState carries message context through all pipeline stages.
// Each stage reads from relevant fields and populates its output field.
// Fields are deliberately typed — no map[string]interface{} junk drawer.
type PipelineState struct {
	// Message is the canonical domain message being processed. Immutable in the pipeline.
	Message *domain.Message

	// Prepared is the output of PrepareStage (normalized destination, encoding, parts).
	Prepared *domain.PreparedMessage

	// Decision is the routing decision (set by RouteStage, never modified after).
	Decision *RoutingDecision

	// SendResult is the result from the connector (set by SendStage).
	SendResult *SendResult

	// Attempt is the current retry attempt (0 = first attempt).
	// Managed by the retry decorator wrapping the pipeline, not by pipeline stages.
	Attempt int

	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int

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
		Message:    msg,
		TraceID:    traceID,
		Attempt:    0,
		MaxRetries: 3,
	}
}
