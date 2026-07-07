package domain

import (
	"context"
	"fmt"
	"time"
)

// Message represents an SMS message in the system.
type Message struct {
	BaseModel
	TenantID    string          `json:"tenant_id"`
	ConnectorID *string         `json:"connector_id,omitempty"`
	RouteID     *string         `json:"route_id,omitempty"`
	ClientID    string          `json:"client_id"` // API key ID or user ID
	Direction   Direction       `json:"direction"`
	Status      MessageStatus   `json:"status"`
	Source      string          `json:"source"`       // sender ID / originator
	Destination string          `json:"destination"`  // recipient number
	Text        string          `json:"text"`
	Encoding    Encoding        `json:"encoding"`
	Priority    MessagePriority `json:"priority"`
	Parts       int             `json:"parts"` // number of SMS parts (concatenation)
	DLRStatus   *DLRStatus      `json:"dlr_status,omitempty"`
	DLRURL      string          `json:"dlr_url,omitempty"`    // delivery receipt callback URL
	DLRID       string          `json:"dlr_id,omitempty"`     // external DLR ID
	ExternalID  string          `json:"external_id,omitempty"` // SMSC message ID
	ClientRef   string          `json:"client_ref,omitempty"`  // client-provided reference for idempotency
	RetryCount  int             `json:"retry_count"`
	MaxRetries  int             `json:"max_retries"`
	// Price/Cost stored as integer (thousandths of a cent = 1/100000 of a unit).
	// Example: 0.05000 USD = 5000, display by dividing by 100000.
	Price int64 `json:"price"`
	Cost  int64 `json:"cost"`
	SentAt        *time.Time     `json:"sent_at,omitempty"`
	DeliveredAt   *time.Time     `json:"delivered_at,omitempty"`
	FailedAt      *time.Time     `json:"failed_at,omitempty"`
	ErrorCode     string         `json:"error_code,omitempty"`
	ErrorMessage  string         `json:"error_message,omitempty"`
}

// CreateMessageInput represents the input for creating a new message.
type CreateMessageInput struct {
	TenantID    string          `json:"-"`
	ClientID    string          `json:"-"`
	Direction   Direction       `json:"direction"`
	Source      string          `json:"source" validate:"required"`
	Destination string          `json:"destination" validate:"required"`
	Text        string          `json:"text" validate:"required"`
	Encoding    Encoding        `json:"encoding,omitempty"`
	Priority    MessagePriority `json:"priority,omitempty"`
	DLRURL      string          `json:"dlr_url,omitempty"`
	ClientRef   string          `json:"client_ref,omitempty"`
	MaxRetries  int             `json:"max_retries,omitempty"`
}

// UpdateMessageInput represents the input for updating a message status.
type UpdateMessageInput struct {
	Status      *MessageStatus `json:"status,omitempty"`
	ConnectorID *string        `json:"connector_id,omitempty"`
	RouteID     *string        `json:"route_id,omitempty"`
	ExternalID  *string        `json:"external_id,omitempty"`
	DLRStatus   *DLRStatus     `json:"dlr_status,omitempty"`
	DLRID       *string        `json:"dlr_id,omitempty"`
	ErrorCode   *string        `json:"error_code,omitempty"`
	ErrorMessage *string       `json:"error_message,omitempty"`
	Parts       *int           `json:"parts,omitempty"`
	Price       *int64         `json:"price,omitempty"`
	Cost        *int64         `json:"cost,omitempty"`
	SentAt      *time.Time     `json:"sent_at,omitempty"`
	DeliveredAt *time.Time     `json:"delivered_at,omitempty"`
	FailedAt    *time.Time     `json:"failed_at,omitempty"`
}

// MessageFilter represents filtering options for listing messages.
type MessageFilter struct {
	TenantID    string
	Status      *MessageStatus
	Direction   *Direction
	ConnectorID string
	Source      string
	Destination string
	ClientRef   string
	ExternalID  string
	Search      string
	DateFrom    *time.Time
	DateTo      *time.Time
	Page        Page
}

// DLRRecord represents a delivery receipt log entry.
type DLRRecord struct {
	BaseModel
	MessageID     string    `json:"message_id"`
	TenantID      string    `json:"tenant_id"`
	Status        DLRStatus `json:"status"`
	ExternalID    string    `json:"external_id,omitempty"`
	ConnectorName string    `json:"connector_name,omitempty"`
	RemoteIP      string    `json:"remote_ip,omitempty"`
	Headers       []byte    `json:"headers,omitempty"`  // JSONB - DLR HTTP headers
	RawPayload    []byte    `json:"raw_payload,omitempty"` // JSONB - full DLR payload
	ErrorCode     string    `json:"error_code,omitempty"`
	Description   string    `json:"description,omitempty"`
	CreatedAt     time.Time `json:"received_at"` // when the DLR was received
}

// OutboxEvent represents an event waiting to be published (Outbox pattern).
type OutboxEvent struct {
	BaseModel
	EventType string      `json:"event_type"`
	TenantID  string      `json:"tenant_id,omitempty"`
	Payload   []byte      `json:"payload"` // JSONB - serialized event payload
	Status    string      `json:"status"`  // pending, published, failed
	Attempts  int         `json:"attempts"`
	LastError string      `json:"last_error,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
}

// PreparedMessage carries the results of message preparation.
// Created by PrepareStage (pipeline), consumed by Sender implementations.
// This shared struct ensures preparation fields are defined once and
// never drift between the pipeline and sender contracts.
type PreparedMessage struct {
	// Destination is the normalized phone number (E.164-like format).
	// The raw msg.Destination is preserved on domain.Message.
	Destination string

	// Encoding is the detected message content encoding ("GSM7" or "UCS2").
	Encoding string

	// Parts is the number of SMS parts after splitting.
	Parts int
}

// SendRequest carries all context needed for a connector to send a message.
type SendRequest struct {
	Message   *Message
	Connector *Connector
	Timeout   time.Duration

	// Prepared carries the normalization result from PrepareStage.
	// The sender SHOULD prefer Prepared.Destination over Message.Destination
	// (which holds the original raw input).
	Prepared *PreparedMessage
}

// AcceptanceKind tells the pipeline what the connector knows about the send outcome.
// This is set by the connector (not guessed by HandleResultStage) because
// only the connector understands provider-specific response semantics.
type AcceptanceKind string

const (
	// AcceptanceFinal means the provider accepted the message and no further
	// updates (DLR) are expected. The pipeline considers this delivered.
	AcceptanceFinal AcceptanceKind = "final"

	// AcceptancePendingDLR means the provider accepted but will send a
	// delivery receipt later. The pipeline considers this sent (pending DLR).
	AcceptancePendingDLR AcceptanceKind = "pending_dlr"

	// AcceptanceRejected means the provider rejected the message outright.
	// The pipeline may retry or fail depending on Retryable + retry budget.
	AcceptanceRejected AcceptanceKind = "rejected"
)

// SendResult carries the outcome of a send attempt.
type SendResult struct {
	ExternalID     string         `json:"external_id"`
	Parts          int            `json:"parts"`
	Price          int64          `json:"price"`
	Cost           int64          `json:"cost"`
	RawResponse    []byte         `json:"raw_response,omitempty"`
	ProviderStatus string         `json:"provider_status,omitempty"`
	Acceptance     AcceptanceKind `json:"acceptance,omitempty"`
}

// Sender defines the interface for sending messages through a connector.
type Sender interface {
	Type() ConnectorType
	Send(ctx context.Context, req SendRequest) (*SendResult, error)
}

// RetryPolicy defines the backoff strategy for message retries.
type RetryPolicy interface {
	MaxRetries() int
	NextDelay(attempt int) time.Duration
}

// DLRMapper maps provider-specific delivery status to the internal DLRStatus.
type DLRMapper interface {
	MapProviderStatus(providerStatus string) (DLRStatus, MessageStatus)
	ConnectorType() ConnectorType
}

// MetricsRecorder records operational metrics from workers and senders.
type MetricsRecorder interface {
	RecordMessageSent(tenantID, connectorID string, parts int, latency time.Duration)
	RecordMessageFailed(tenantID, connectorID, errorCode string)
	RecordDLRReceived(tenantID, status string)
	RecordRetry(tenantID string, attempt int)
}

// ============================================================
// Phase 2.5 Design Notes (production hardening)
// ============================================================
//
// IMPLEMENTED in Phase 2.4:
//   ✅ FOR UPDATE SKIP LOCKED in QueueRepository.ClaimQueued()
//   ✅ Sender interface with SendRequest/SendResult
//   ✅ QueueRepository (Claim/AckSent/AckFailed/ScheduleRetry)
//   ✅ RetryEngine with exponential backoff + jitter
//   ✅ OutboxWorker (batch publish)
//   ✅ DLRHandler with idempotent version check
//   ✅ Concurrent worker integration tests
//   ✅ Singleton http.Client with connection pooling
//   ✅ Template caching (sync.Map)
//   ✅ Retry-After header detection (ProviderRetryRequiredError)
//
// DESIGNED for Phase 2.5:
//   📝 Graceful shutdown — drain in-flight messages before stopping workers
//   📝 Dead Letter Queue — after MaxRetries exceeded, move to dlq table
//   📝 Rate Limiter — per-connector, token bucket or sliding window
//   📝 Circuit Breaker — trip when provider returns 5xx repeatedly
//   📝 LISTEN/NOTIFY or Redis Stream — replace polling with push
//   📝 SMPP Sender — implement domain.Sender for SMPP protocol
//   📝 Prometheus MetricsRecorder — replace NoopMetricsRecorder
//   📝 Integration tests with real PostgreSQL + Docker
//   📝 Load test: 100k messages with 10 concurrent workers

// ============================================================
// Architectural Separation: Message Queue vs Outbox Events
// ============================================================
//
// Two independent queues serve different purposes:
//
// 1. MESSAGE QUEUE (messages table with status = 'queued'):
//    - Messages waiting to be SENT through a connector.
//    - Worker pulls via FOR UPDATE SKIP LOCKED, calls Sender.Send().
//    - After send: status → 'sent'/'failed', writes to outbox_events.
//    - NOT processed through outbox_events.
//
// 2. OUTBOX EVENTS (outbox_events table):
//    - Domain events published AFTER an action completes.
//    - Examples: MessageSent, MessageDelivered, MessageFailed.
//    - A separate Outbox Worker reads and publishes to eventBus
//      so in-process subscribers (analytics, audit, billing) react.
//    - NOT used to trigger sending; purely event propagation.
//
// Flow:
//   Worker → SELECT FROM messages WHERE status='queued' SKIP LOCKED
//         → Sender.Send()
//         → UPDATE messages SET status='sent'
//         → INSERT INTO outbox_events (event_type='message.sent')
//         → Outbox Worker → eventBus.Publish() → subscribers
//
// ============================================================
// Message State Machine
// ============================================================

// validTransitions defines the allowed status transitions.
var validTransitions = map[MessageStatus][]MessageStatus{
	MessageStatusAccepted:  {MessageStatusQueued, MessageStatusFailed},
	MessageStatusQueued:    {MessageStatusSending, MessageStatusFailed},
	MessageStatusSending:   {MessageStatusSent, MessageStatusFailed},
	MessageStatusSent:      {MessageStatusDelivered, MessageStatusFailed, MessageStatusRetrying},
	MessageStatusRetrying:  {MessageStatusSending, MessageStatusFailed},
	MessageStatusDelivered: {}, // terminal state
	MessageStatusFailed:    {}, // terminal state
}

// ValidateTransition checks if a status transition is allowed.
func ValidateTransition(current, next MessageStatus) error {
	allowed, ok := validTransitions[current]
	if !ok {
		return fmt.Errorf("unknown current status: %s", current)
	}
	for _, s := range allowed {
		if s == next {
			return nil
		}
	}
	return fmt.Errorf("invalid transition: %s -> %s", current, next)
}

// MaxRetriesDefault is the default maximum retry count.
const MaxRetriesDefault = 3

// MoneyScale is the scaling factor for monetary values.
// Stored as int64 (thousandths of a cent), divide by MoneyScale to get the unit value.
// Example: 1.50000 USD = 150000 stored, 150000 / 100000 = 1.5
const MoneyScale int64 = 100000
