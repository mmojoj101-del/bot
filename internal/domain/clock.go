package domain

import "time"

// Clock provides time operations, enabling deterministic testing.
type Clock interface {
	Now() time.Time
}

// RealClock returns the actual current time.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }
