package backends

import (
	"context"
	"time"
)

// Backend defines the interface for rate limit data storage
type Backend interface {
	// Get retrieves rate limit data for the given key
	Get(ctx context.Context, key string) (*BackendData, error)

	// Set stores rate limit data for the given key with optional TTL
	Set(ctx context.Context, key string, data *BackendData, ttl time.Duration) error

	// Increment atomically checks if incrementing would exceed the limit
	// If not, increments and returns the new count and true
	// If it would exceed, returns current count and false
	// This is used for rate limiting where we only want to increment if allowed
	Increment(ctx context.Context, key string, window time.Duration, limit int64) (count int64, incremented bool, err error)

	// ConsumeToken atomically checks if a token is available and consumes it
	// Handles token refill calculations and bucket capacity management
	// Returns: currentTokens, tokenConsumed, totalRequests, error
	ConsumeToken(ctx context.Context, key string, bucketSize int64, refillRate time.Duration, refillAmount int64, window time.Duration) (tokens float64, consumed bool, requests int64, err error)

	// Delete removes rate limit data for the given key
	Delete(ctx context.Context, key string) error

	// Close releases any resources held by the backend
	Close() error
}

// BackendData represents the data stored by the backend
type BackendData struct {
	// Count is the current request count
	Count int64

	// WindowStart is when the current window started
	WindowStart time.Time

	// LastRequest is when the last request was made
	LastRequest time.Time

	// Tokens is the current number of tokens (for token bucket strategy)
	Tokens float64

	// LastRefill is when tokens were last refilled (for token bucket strategy)
	LastRefill time.Time

	// Metadata stores additional strategy-specific data
	Metadata map[string]any
}

// BackendConfig holds configuration for backend initialization
type BackendConfig struct {
	// Type is the backend type ("memory", "redis")
	Type string

	// Redis configuration
	RedisAddress  string
	RedisPassword string
	RedisDB       int
	RedisPoolSize int

	// Memory configuration
	CleanupInterval time.Duration
	MaxKeys         int
}
