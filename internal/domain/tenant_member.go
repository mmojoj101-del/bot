package domain

import "time"

// TenantMember represents a user's membership in a tenant with a specific role.
type TenantMember struct {
	BaseModel
	TenantID string     `json:"tenant_id"`
	UserID   string     `json:"user_id"`
	Role     MemberRole `json:"role"`
	JoinedAt time.Time  `json:"joined_at"`
}

// AddMemberInput represents the input for adding a member to a tenant.
type AddMemberInput struct {
	TenantID string     `json:"-"` // from context
	UserID   string     `json:"user_id" validate:"required"`
	Role     MemberRole `json:"role" validate:"required"`
}
