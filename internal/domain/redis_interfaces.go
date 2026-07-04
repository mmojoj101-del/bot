package domain

import "context"

// SessionStore manages user sessions in Redis.
type SessionStore interface {
	Set(ctx context.Context, key string, value interface{}, ttl int) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// RateLimiter provides rate limiting functionality.
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window int) (bool, error)
	Remaining(ctx context.Context, key string, limit int, window int) (int, error)
	Reset(ctx context.Context, key string) error
}

// DistributedLock provides distributed locking via Redis.
type DistributedLock interface {
	Acquire(ctx context.Context, key string, ttl int) (bool, error)
	Release(ctx context.Context, key string) error
}

// Cache provides general-purpose caching.
type Cache interface {
	Set(ctx context.Context, key string, value interface{}, ttl int) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
	Flush(ctx context.Context) error
}
