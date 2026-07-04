package event

import "time"

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
	EventRouteCreated       = "route.created"
	EventRouteUpdated       = "route.updated"
	EventAuditLogged        = "audit.logged"
)
