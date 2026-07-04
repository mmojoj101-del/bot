package domain

// Route represents a routing rule that maps a prefix to a connector.
type Route struct {
	BaseModel
	TenantID    string        `json:"tenant_id"`
	Name        string        `json:"name"`
	Type        RouteType     `json:"type"`
	Strategy    RouteStrategy `json:"strategy"`
	Weight      int           `json:"weight"`
	Priority    int           `json:"priority"`
	Prefix      string        `json:"prefix"`
	ConnectorID string        `json:"connector_id"`
	Enabled     bool          `json:"enabled"`
	CreatedBy   string        `json:"created_by,omitempty"`
	UpdatedBy   string        `json:"updated_by,omitempty"`
}

// RouteStrategy represents the load balancing strategy for a route.
type RouteStrategy string

const (
	RouteStrategyStatic      RouteStrategy = "static"
	RouteStrategyRoundRobin  RouteStrategy = "round_robin"
	RouteStrategyFailover    RouteStrategy = "failover"
	RouteStrategyWeighted    RouteStrategy = "weighted"
)

// CreateRouteInput represents the input for creating a new route.
type CreateRouteInput struct {
	TenantID    string         `json:"-"`
	Name        string         `json:"name" validate:"required"`
	Type        RouteType      `json:"type" validate:"required"`
	Strategy    RouteStrategy  `json:"strategy"`
	Weight      int            `json:"weight"`
	Priority    int            `json:"priority"`
	Prefix      string         `json:"prefix" validate:"required"`
	ConnectorID string         `json:"connector_id" validate:"required"`
}

// UpdateRouteInput represents the input for updating a route.
type UpdateRouteInput struct {
	Name        *string         `json:"name,omitempty"`
	Type        *RouteType      `json:"type,omitempty"`
	Strategy    *RouteStrategy  `json:"strategy,omitempty"`
	Weight      *int            `json:"weight,omitempty"`
	Priority    *int            `json:"priority,omitempty"`
	Prefix      *string         `json:"prefix,omitempty"`
	ConnectorID *string         `json:"connector_id,omitempty"`
	Enabled     *bool           `json:"enabled,omitempty"`
}

// RouteFilter represents filtering options for listing routes.
type RouteFilter struct {
	TenantID    string
	Type        *RouteType
	Strategy    *RouteStrategy
	Prefix      string
	ConnectorID string
	Search      string
	Page        Page
}
