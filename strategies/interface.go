package strategies

import (
	"context"
	"sync"
	"time"
)

// RateLimitConfig contains basic configuration for rate limiting
// Each strategy may extend this with strategy-specific fields
type RateLimitConfig struct {
	Key   string
	Limit int
}

// FixedWindowConfig extends RateLimitConfig for fixed window strategy
type FixedWindowConfig struct {
	RateLimitConfig
	Window time.Duration
}

// TokenBucketConfig extends RateLimitConfig for token bucket strategy
type TokenBucketConfig struct {
	RateLimitConfig
	BurstSize  int     // Maximum tokens the bucket can hold
	RefillRate float64 // Tokens to add per second
}

// LeakyBucketConfig extends RateLimitConfig for leaky bucket strategy
type LeakyBucketConfig struct {
	RateLimitConfig
	Capacity int     // Maximum requests the bucket can hold
	LeakRate float64 // Requests to process per second
}

// Result represents the result of a rate limiting check
type Result struct {
	Allowed   bool      // Whether the request is allowed
	Remaining int       // Remaining requests in the current window
	Reset     time.Time // When the current window resets
}

// LockInfo tracks information about a lock for cleanup purposes
type LockInfo struct {
	mutex    *sync.Mutex
	mu       sync.Mutex // protects lastUsed
	lastUsed time.Time
}

// Strategy defines the interface for rate limiting strategies
type Strategy interface {
	// Allow checks if a request is allowed based on the strategy
	Allow(ctx context.Context, config any) (bool, error)

	// GetResult returns detailed statistics for the current state
	GetResult(ctx context.Context, config any) (Result, error)

	// Reset resets the rate limit counter (mainly for testing)
	Reset(ctx context.Context, config any) error

	// Cleanup removes stale locks and data
	Cleanup(maxAge time.Duration)
}
