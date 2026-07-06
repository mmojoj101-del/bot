package events

import (
	"encoding/json"
	"time"
)

// EventType constants — versioned for forward compatibility.
// Convention: <domain>.<action>.v<version>
const (
	// Message lifecycle events
	EventTypeMessageQueuedV1    = "message.queued.v1"
	EventTypeMessageClaimedV1   = "message.claimed.v1"
	EventTypeMessageSentV1      = "message.sent.v1"
	EventTypeMessageDeliveredV1 = "message.delivered.v1"
	EventTypeMessageFailedV1    = "message.failed.v1"
	EventTypeMessageRetryingV1  = "message.retrying.v1"
	EventTypeMessageExpiredV1   = "message.expired.v1"

	// Routing events
	EventTypeRouteSelectedV1 = "route.selected.v1"

	// Connector events
	EventTypeConnectorUnavailableV1 = "connector.unavailable.v1"
	EventTypeConnectorHealthyV1     = "connector.healthy.v1"

	// Infrastructure events
	EventTypeCircuitBreakerOpenedV1 = "infra.circuit-breaker.opened.v1"
	EventTypeCircuitBreakerClosedV1 = "infra.circuit-breaker.closed.v1"
)

// EventEnvelope is the standard wrapper for every Domain Event.
// Only Payload differs between event types; all metadata is in the envelope.
type EventEnvelope struct {
	EventID       string            `json:"event_id"`       // unique UUID
	EventType     string            `json:"event_type"`     // e.g. "message.sent.v1"
	Version       int               `json:"version"`        // extracted from event_type, e.g. 1
	OccurredAt    time.Time         `json:"occurred_at"`    // when the event happened
	TraceID       string            `json:"trace_id"`       // cross-lifecycle trace
	TenantID      string            `json:"tenant_id"`
	CorrelationID string            `json:"correlation_id"` // groups related events
	Payload       json.RawMessage   `json:"payload"`        // the actual event data
	Metadata      map[string]string `json:"metadata,omitempty"` // extensible key-value
}

// DomainEvent is the interface all event payloads implement.
// This enables type-safe handling in subscribers.
type DomainEvent interface {
	EventType() string
}

// --- V1 Payloads ---

// MessageQueuedV1Payload is the payload for message.queued.v1.
type MessageQueuedV1Payload struct {
	MessageID   string `json:"message_id"`
	TenantID    string `json:"tenant_id"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Parts       int    `json:"parts"`
	ClientRef   string `json:"client_ref,omitempty"`
}

func (p MessageQueuedV1Payload) EventType() string { return EventTypeMessageQueuedV1 }

// MessageClaimedV1Payload is the payload for message.claimed.v1.
type MessageClaimedV1Payload struct {
	MessageID string `json:"message_id"`
	TenantID  string `json:"tenant_id"`
	WorkerID  string `json:"worker_id"`
}

func (p MessageClaimedV1Payload) EventType() string { return EventTypeMessageClaimedV1 }

// MessageSentV1Payload is the payload for message.sent.v1.
type MessageSentV1Payload struct {
	MessageID   string `json:"message_id"`
	ExternalID  string `json:"external_id"`
	ConnectorID string `json:"connector_id"`
	Parts       int    `json:"parts"`
	Price       int64  `json:"price"` // thousandths of a cent
	Cost        int64  `json:"cost"`
}

func (p MessageSentV1Payload) EventType() string { return EventTypeMessageSentV1 }

// MessageDeliveredV1Payload is the payload for message.delivered.v1.
type MessageDeliveredV1Payload struct {
	MessageID  string `json:"message_id"`
	ExternalID string `json:"external_id"`
	DLRID      string `json:"dlr_id"`
	DoneAt     string `json:"done_at"` // RFC3339
}

func (p MessageDeliveredV1Payload) EventType() string { return EventTypeMessageDeliveredV1 }

// MessageFailedV1Payload is the payload for message.failed.v1.
type MessageFailedV1Payload struct {
	MessageID    string `json:"message_id"`
	ExternalID   string `json:"external_id,omitempty"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	Attempt      int    `json:"attempt"`
	Retryable    bool   `json:"retryable"`
}

func (p MessageFailedV1Payload) EventType() string { return EventTypeMessageFailedV1 }

// MessageRetryingV1Payload is the payload for message.retrying.v1.
type MessageRetryingV1Payload struct {
	MessageID   string `json:"message_id"`
	Attempt     int    `json:"attempt"`
	NextAttempt string `json:"next_attempt"` // RFC3339
}

func (p MessageRetryingV1Payload) EventType() string { return EventTypeMessageRetryingV1 }

// RouteSelectedV1Payload is the payload for route.selected.v1.
type RouteSelectedV1Payload struct {
	MessageID       string   `json:"message_id"`
	RouteID         string   `json:"route_id"`
	ConnectorID     string   `json:"connector_id"`
	Strategy        string   `json:"strategy"`
	Reason          string   `json:"reason"`
	CapabilitiesUsed []string `json:"capabilities_used"`
}

func (p RouteSelectedV1Payload) EventType() string { return EventTypeRouteSelectedV1 }
