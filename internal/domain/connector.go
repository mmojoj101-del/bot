package domain

// Connector represents a protocol connector (SMPP, HTTP, SIP) configuration.
type Connector struct {
	BaseModel
	TenantID  string         `json:"tenant_id"`
	Type      ConnectorType  `json:"type"`
	Name      string         `json:"name"`
	Config    []byte         `json:"config"` // JSONB - protocol-specific config
	Enabled   bool           `json:"enabled"`
	CreatedBy string         `json:"created_by,omitempty"`
	UpdatedBy string         `json:"updated_by,omitempty"`
}

// CreateConnectorInput represents the input for creating a new connector.
type CreateConnectorInput struct {
	TenantID string        `json:"-"`
	Type     ConnectorType `json:"type" validate:"required"`
	Name     string        `json:"name" validate:"required"`
	Config   []byte        `json:"config"`
}

// UpdateConnectorInput represents the input for updating a connector.
type UpdateConnectorInput struct {
	Name    *string        `json:"name,omitempty"`
	Type    *ConnectorType `json:"type,omitempty"`
	Config  []byte         `json:"config,omitempty"`
	Enabled *bool          `json:"enabled,omitempty"`
}
