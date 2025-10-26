package strategies

import (
	"context"
	"time"
)

type Results = map[string]Result

// Result represents the result of a rate limiting check
type Result struct {
	Allowed   bool      // Whether the request is allowed
	Remaining int       // Remaining requests in the current window
	Reset     time.Time // When the current window resets
}

// Strategy defines the interface for rate limiting strategies
type Strategy interface {
	// Allow checks if a request is allowed and returns detailed statistics in a single call
	Allow(ctx context.Context, config StrategyConfig) (Results, error)

	// Peek inspects current rate limit status without consuming quota
	Peek(ctx context.Context, config StrategyConfig) (Results, error)

	// Reset resets the rate limit counter (mainly for testing)
	Reset(ctx context.Context, config StrategyConfig) error
}
