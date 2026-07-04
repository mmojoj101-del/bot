package domain

import "context"

// Connector represents a protocol connector (SMPP, HTTP, SIP) configuration.
type Connector struct {
	BaseModel
	TenantID  string           `json:"tenant_id"`
	Type      ConnectorType    `json:"type"`
	Name      string           `json:"name"`
	Status    ConnectorStatus  `json:"status"`
	Config    []byte           `json:"config"` // JSONB - protocol-specific config
	CreatedBy string           `json:"created_by,omitempty"`
	UpdatedBy string           `json:"updated_by,omitempty"`
}

// CreateConnectorInput represents the input for creating a new connector.
type CreateConnectorInput struct {
	TenantID string           `json:"-"`
	Type     ConnectorType    `json:"type" validate:"required"`
	Name     string           `json:"name" validate:"required"`
	Config   []byte           `json:"config"`
	Status   *ConnectorStatus `json:"status,omitempty"`
}

// UpdateConnectorInput represents the input for updating a connector.
type UpdateConnectorInput struct {
	Name    *string           `json:"name,omitempty"`
	Type    *ConnectorType    `json:"type,omitempty"`
	Status  *ConnectorStatus  `json:"status,omitempty"`
	Config  []byte            `json:"config,omitempty"`
}

// ConnectorStatus represents the operational status of a connector.
// The single source of truth — combines enablement and health.
//   active    → enabled and healthy
//   disabled  → administratively disabled
//   testing   → connection test in progress
//   error     → enabled but in an error state
type ConnectorStatus string

const (
	ConnectorStatusActive   ConnectorStatus = "active"
	ConnectorStatusDisabled ConnectorStatus = "disabled"
	ConnectorStatusTesting  ConnectorStatus = "testing"
	ConnectorStatusError    ConnectorStatus = "error"
)

// ConnectorFilter represents filtering options for listing connectors.
type ConnectorFilter struct {
	TenantID string
	Type     *ConnectorType
	Status   *ConnectorStatus
	Search   string
	Page     Page
}

// ConnectorTester tests a connector's connection.
type ConnectorTester interface {
	Test(ctx context.Context, connector *Connector) error
	Type() ConnectorType
}
