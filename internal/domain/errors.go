package domain

import "errors"

var (
	// ErrNotFound is returned when a resource is not found.
	ErrNotFound = errors.New("resource not found")

	// ErrConflict is returned when there is a version conflict (optimistic locking).
	ErrConflict = errors.New("version conflict")

	// ErrDuplicate is returned when a duplicate resource is created.
	ErrDuplicate = errors.New("duplicate resource")

	// ErrUnauthorized is returned when authentication fails.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden is returned when the user lacks permission.
	ErrForbidden = errors.New("forbidden")

	// ErrInvalidInput is returned when input validation fails.
	ErrInvalidInput = errors.New("invalid input")

	// ErrExpired is returned when a token or resource has expired.
	ErrExpired = errors.New("expired")

	// ErrSuspended is returned when a tenant or user is suspended.
	ErrSuspended = errors.New("account suspended")

	// ErrRateLimited is returned when rate limit is exceeded.
	ErrRateLimited = errors.New("rate limit exceeded")

	// ErrConnectorTypeNotAllowed is returned when a connector type is not allowed.
	ErrConnectorTypeNotAllowed = errors.New("connector type not allowed in this environment")
)

// ValidationError represents a validation error with field-level details.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrors is a list of validation errors.
type ValidationErrors []ValidationError

func (v ValidationErrors) Error() string {
	if len(v) == 0 {
		return "validation failed"
	}
	return "validation failed: " + v[0].Field + " " + v[0].Message
}
