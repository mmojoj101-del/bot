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
	ConnectorTypeMock       ConnectorType = "mock"
)

// ConnectorStatus represents the operational status of a connector.
type ConnectorStatus string

const (
	ConnectorStatusActive   ConnectorStatus = "active"
	ConnectorStatusDisabled ConnectorStatus = "disabled"
	ConnectorStatusTesting  ConnectorStatus = "testing"
	ConnectorStatusError    ConnectorStatus = "error"
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
