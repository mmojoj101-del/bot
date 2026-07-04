package domain

import "time"

// RefreshToken represents a JWT refresh token stored for session management.
type RefreshToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	TenantID   string     `json:"tenant_id"`
	TokenHash  string     `json:"-"`
	JTI        string     `json:"jti"`
	DeviceName string     `json:"device_name,omitempty"`
	IPAddress  string     `json:"ip_address,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// IsExpired returns true if the refresh token has expired.
func (r *RefreshToken) IsExpired() bool {
	return time.Now().After(r.ExpiresAt)
}

// IsRevoked returns true if the refresh token has been revoked.
func (r *RefreshToken) IsRevoked() bool {
	return r.RevokedAt != nil
}
