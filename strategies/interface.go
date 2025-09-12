package strategies

import (
	"context"
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

// Strategy defines the interface for rate limiting algorithms
type Strategy interface {
	// Allow checks if a request should be allowed based on the strategy.
	// The 'limit' parameter currently represents the primary rate limit (e.g., requests per minute).
	// In a future enhancement, this could be extended to take a struct or map
	// of multiple limits (e.g., per minute, per hour, per day).
	Allow(ctx context.Context, backend backends.Backend, key string, limit int64, window time.Duration) (*StrategyResult, error)

	// GetStatus returns the current status using the specific strategy
	GetStatus(ctx context.Context, backend backends.Backend, key string, limit int64, window time.Duration) (*StrategyStatus, error)

	// Reset clears the strategy-specific data for the given key
	Reset(ctx context.Context, backend backends.Backend, key string) error

	// Name returns the name of the strategy
	Name() string
}

// StrategyResult represents the result of a strategy check
type StrategyResult struct {
	// Allowed indicates whether the request should be allowed
	Allowed bool

	// Remaining is the number of requests remaining
	Remaining int64

	// ResetTime is when the limit will reset
	ResetTime time.Time

	// RetryAfter is the duration to wait before retrying
	RetryAfter time.Duration

	// TotalRequests is the total requests in the current window
	TotalRequests int64

	// WindowStart is when the current window started
	WindowStart time.Time
}

// StrategyStatus represents the current status from a strategy
type StrategyStatus struct {
	// Limit is the maximum number of requests allowed
	Limit int64

	// Remaining is the number of requests remaining
	Remaining int64

	// ResetTime is when the limit will reset
	ResetTime time.Time

	// WindowStart is when the current window started
	WindowStart time.Time

	// TotalRequests is the total requests in the current window
	TotalRequests int64

	// WindowDuration is the duration of the rate limit window
	WindowDuration time.Duration
}

// StrategyConfig holds configuration for strategy initialization
type StrategyConfig struct {
	// Type is the strategy type ("token_bucket", "fixed_window")
	Type string

	// TokenBucket specific configuration
	RefillRate   time.Duration // How often to refill tokens
	BucketSize   int64         // Maximum number of tokens in bucket
	RefillAmount int64         // Number of tokens to add per refill

	// FixedWindow specific configuration
	WindowDuration time.Duration // Duration of each fixed window
}
