package domain

import "time"

// APIKey represents an API key used for programmatic access to the gateway.
type APIKey struct {
	BaseModel
	TenantID    string     `json:"tenant_id"`
	Name        string     `json:"name"`
	KeyPrefix   string     `json:"key_prefix"`
	KeyHash     string     `json:"-"`
	Permissions []byte     `json:"permissions,omitempty"` // JSONB
	IPWhitelist []byte     `json:"ip_whitelist,omitempty"` // JSONB
	RateLimits  []byte     `json:"rate_limits,omitempty"` // JSONB
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Enabled     bool       `json:"enabled"`
	CreatedBy   string     `json:"created_by,omitempty"`
	UpdatedBy   string     `json:"updated_by,omitempty"`
}

// CreateAPIKeyInput represents the input for creating a new API key.
type CreateAPIKeyInput struct {
	TenantID    string   `json:"-"`
	Name        string   `json:"name" validate:"required"`
	Permissions []string `json:"permissions,omitempty"`
	IPWhitelist []string `json:"ip_whitelist,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// UpdateAPIKeyInput represents the input for updating an API key.
type UpdateAPIKeyInput struct {
	Name        *string   `json:"name,omitempty"`
	Permissions []string  `json:"permissions,omitempty"`
	IPWhitelist []string  `json:"ip_whitelist,omitempty"`
	Enabled     *bool     `json:"enabled,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}
