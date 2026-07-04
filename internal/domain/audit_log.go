package domain

import "time"

// AuditLog represents an audit trail entry for tracking actions in the system.
type AuditLog struct {
	ID         string      `json:"id"`
	TenantID   *string     `json:"tenant_id,omitempty"`
	UserID     *string     `json:"user_id,omitempty"`
	RequestID  string      `json:"request_id"`
	Action     AuditAction `json:"action"`
	Resource   string      `json:"resource"`
	Metadata   []byte      `json:"metadata,omitempty"` // JSONB
	IPAddress  string      `json:"ip_address,omitempty"`
	UserAgent  string      `json:"user_agent,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
}
