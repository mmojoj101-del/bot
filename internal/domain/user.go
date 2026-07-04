package domain

import "time"

// User represents a platform user who can belong to multiple tenants.
type User struct {
	BaseModel
	Email             string     `json:"email"`
	PasswordHash      string     `json:"-"`
	Name              string     `json:"name"`
	Status            UserStatus `json:"status"`
	IsSuperAdmin      bool       `json:"is_super_admin"`
	LastLoginAt       *time.Time `json:"last_login_at,omitempty"`
	PasswordChangedAt time.Time  `json:"password_changed_at"`
}

// CreateUserInput represents the input for creating a new user.
type CreateUserInput struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=12"`
	Name     string `json:"name" validate:"required"`
}

// UpdateUserInput represents the input for updating a user.
type UpdateUserInput struct {
	Name   *string    `json:"name,omitempty"`
	Status *UserStatus `json:"status,omitempty"`
}
