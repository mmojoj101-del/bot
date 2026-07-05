package event

import (
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// Event represents a domain event.
type Event struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload"`
	Timestamp time.Time   `json:"timestamp"`
}

// Handler is a function that handles an event.
type Handler func(Event)

// Bus defines the interface for the event system.
type Bus interface {
	Publish(event Event)
	Subscribe(eventType string, handler Handler) func()
	Unsubscribe(eventType string, handler Handler)
	Close()
}

// Event types used in the system.
const (
	EventUserCreated        = "user.created"
	EventUserLoggedIn       = "user.logged_in"
	EventTenantCreated      = "tenant.created"
	EventTenantUpdated      = "tenant.updated"
	EventMemberAdded        = "tenant.member_added"
	EventMemberRemoved      = "tenant.member_removed"
	EventAPIKeyCreated      = "api_key.created"
	EventAPIKeyUsed         = "api_key.used"
	EventConnectorCreated   = "connector.created"
	EventConnectorUpdated   = "connector.updated"
	EventConnectorDeleted   = "connector.deleted"
	EventConnectorTested    = "connector.tested"
	EventRouteCreated       = "route.created"
	EventRouteUpdated       = "route.updated"
	EventRouteDeleted       = "route.deleted"
	EventAuditLogged        = "audit.logged"

	// Message events
	EventMessageAccepted  = "message.accepted"
	EventMessageQueued    = "message.queued"
	EventMessageSending   = "message.sending"
	EventMessageSent      = "message.sent"
	EventMessageDelivered = "message.delivered"
	EventMessageFailed    = "message.failed"
)

// MessageEventPayload carries common fields for message events with typed status.
type MessageEventPayload struct {
	MessageID   string              `json:"message_id"`
	TenantID    string              `json:"tenant_id"`
	ClientID    string              `json:"client_id"`
	Status      domain.MessageStatus `json:"status"`
	Source      string              `json:"source"`
	Destination string              `json:"destination"`
	ErrorCode   string              `json:"error_code,omitempty"`
}
