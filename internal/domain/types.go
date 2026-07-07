package domain

import "time"

// BaseModel contains common fields for all models.
type BaseModel struct {
	ID        string     `json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	Version   int        `json:"version"`
}

// MemberRole represents the role of a tenant member.
type MemberRole string

const (
	MemberRoleAdmin    MemberRole = "admin"
	MemberRoleOperator MemberRole = "operator"
	MemberRoleAPIUser  MemberRole = "api_user"
)

// TenantStatus represents the status of a tenant.
type TenantStatus string

const (
	TenantStatusActive    TenantStatus = "active"
	TenantStatusSuspended TenantStatus = "suspended"
	TenantStatusDisabled  TenantStatus = "disabled"
)

// UserStatus represents the status of a user.
type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusSuspended UserStatus = "suspended"
	UserStatusDisabled  UserStatus = "disabled"
)

// ConnectorType represents the type of a connector.
type ConnectorType string

const (
	ConnectorTypeSMPPClient ConnectorType = "smpp_client"
	ConnectorTypeSMPPServer ConnectorType = "smpp_server"
	ConnectorTypeHTTPClient ConnectorType = "http_client"
	ConnectorTypeHTTPServer ConnectorType = "http_server"
	ConnectorTypeSIPClient  ConnectorType = "sip_client"
	ConnectorTypeSIPServer  ConnectorType = "sip_server"
	ConnectorTypeMock       ConnectorType = "mock" // for development/testing only
)

// RouteType represents the type of a route.
type RouteType string

const (
	RouteTypeSMS RouteType = "sms"
	RouteTypeCall RouteType = "call"
)

// AuditAction represents the action performed in an audit log.
type AuditAction string

const (
	AuditActionCreate          AuditAction = "create"
	AuditActionUpdate          AuditAction = "update"
	AuditActionDelete          AuditAction = "delete"
	AuditActionLogin           AuditAction = "login"
	AuditActionLogout          AuditAction = "logout"
	AuditActionSwitchTenant    AuditAction = "switch_tenant"
	AuditActionAPIKeyAuth      AuditAction = "api_key_auth"
	AuditActionConnectorTested AuditAction = "connector_tested"
)

// MessageStatus represents the lifecycle status of a message.
type MessageStatus string

const (
	MessageStatusAccepted  MessageStatus = "accepted"
	MessageStatusQueued    MessageStatus = "queued"
	MessageStatusSending   MessageStatus = "sending"
	MessageStatusSent      MessageStatus = "sent"
	MessageStatusDelivered MessageStatus = "delivered"
	MessageStatusFailed    MessageStatus = "failed"
	MessageStatusRetrying  MessageStatus = "retrying"
)

// Direction represents the direction of a message.
type Direction string

const (
	DirectionOutbound Direction = "outbound"
	DirectionInbound  Direction = "inbound"
)

// Encoding represents the character encoding of a message.
type Encoding string

const (
	EncodingGSM7     Encoding = "gsm7"
	EncodingUCS2     Encoding = "ucs2"
	EncodingLatin1   Encoding = "latin1"
	EncodingASCII    Encoding = "ascii"
)

// MessagePriority represents the priority level of a message.
type MessagePriority string

const (
	MessagePriorityLow    MessagePriority = "low"
	MessagePriorityNormal MessagePriority = "normal"
	MessagePriorityHigh   MessagePriority = "high"
	MessagePriorityUrgent MessagePriority = "urgent"
)

// IsTerminalStatus returns true if the message status is final and no further
// processing should occur. Adding a new terminal status only requires adding
// it here — no DeliveryOutcome or pipeline changes needed.
func IsTerminalStatus(s MessageStatus) bool {
	switch s {
	case MessageStatusDelivered, MessageStatusFailed:
		return true
	default:
		return false
	}
}

// DLRStatus represents the delivery receipt status.
type DLRStatus string

const (
	DLRStatusPending    DLRStatus = "pending"
	DLRStatusDelivered  DLRStatus = "delivered"
	DLRStatusFailed     DLRStatus = "failed"
	DLRStatusExpired    DLRStatus = "expired"
	DLRStatusRejected   DLRStatus = "rejected"
	DLRStatusUnknown    DLRStatus = "unknown"
)
