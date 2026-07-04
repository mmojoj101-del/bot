package domain

// Route represents a routing rule that maps a prefix to a connector.
type Route struct {
	BaseModel
	TenantID    string    `json:"tenant_id"`
	Type        RouteType `json:"type"`
	Priority    int       `json:"priority"`
	Prefix      string    `json:"prefix"`
	ConnectorID string    `json:"connector_id"`
	Enabled     bool      `json:"enabled"`
	CreatedBy   string    `json:"created_by,omitempty"`
	UpdatedBy   string    `json:"updated_by,omitempty"`
}

// CreateRouteInput represents the input for creating a new route.
type CreateRouteInput struct {
	TenantID    string    `json:"-"`
	Type        RouteType `json:"type" validate:"required"`
	Priority    int       `json:"priority"`
	Prefix      string    `json:"prefix" validate:"required"`
	ConnectorID string    `json:"connector_id" validate:"required"`
}

// UpdateRouteInput represents the input for updating a route.
type UpdateRouteInput struct {
	Type        *RouteType `json:"type,omitempty"`
	Priority    *int       `json:"priority,omitempty"`
	Prefix      *string    `json:"prefix,omitempty"`
	ConnectorID *string    `json:"connector_id,omitempty"`
	Enabled     *bool      `json:"enabled,omitempty"`
}
