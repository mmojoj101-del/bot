package domain

import (
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
	MessageID   string    `json:"message_id"`
	TenantID    string    `json:"tenant_id"`
	Status      DLRStatus `json:"status"`
	ExternalID  string    `json:"external_id,omitempty"`
	ErrorCode   string    `json:"error_code,omitempty"`
	Description string    `json:"description,omitempty"`
	RawResponse string    `json:"raw_response,omitempty"` // full DLR payload for debugging
}

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
