package strategies

import (
	"context"
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

// Strategy defines the interface for rate limiting strategies
type Strategy interface {
	// Allow checks if a request is allowed based on the strategy
	//
	// Deprecated: Use AllowWithResult instead. Allow will be removed in a future release.
	Allow(ctx context.Context, config any) (bool, error)

	// AllowWithResult checks if a request is allowed and returns detailed statistics in a single call
	AllowWithResult(ctx context.Context, config any) (Result, error)

	// GetResult returns detailed statistics for the current state
	GetResult(ctx context.Context, config any) (Result, error)

	// Reset resets the rate limit counter (mainly for testing)
	Reset(ctx context.Context, config any) error
}
