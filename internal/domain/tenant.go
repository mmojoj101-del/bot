package domain

// Tenant represents an organization/customer that uses the gateway.
type Tenant struct {
	BaseModel
	Name        string       `json:"name"`
	Slug        string       `json:"slug"`
	Status      TenantStatus `json:"status"`
	Settings    []byte       `json:"settings,omitempty"` // JSONB
	Balance     int64        `json:"balance"`            // In smallest currency unit
	CreatedBy   string       `json:"created_by,omitempty"`
	UpdatedBy   string       `json:"updated_by,omitempty"`
}

// CreateTenantInput represents the input for creating a new tenant.
type CreateTenantInput struct {
	Name     string        `json:"name" validate:"required"`
	Slug     string        `json:"slug" validate:"required"`
	Status   *TenantStatus `json:"status,omitempty"`
	Settings []byte        `json:"settings,omitempty"`
}

// UpdateTenantInput represents the input for updating a tenant.
type UpdateTenantInput struct {
	Name     *string        `json:"name,omitempty"`
	Slug     *string        `json:"slug,omitempty"`
	Status   *TenantStatus  `json:"status,omitempty"`
	Settings []byte         `json:"settings,omitempty"`
}
