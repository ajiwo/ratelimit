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

	// Reset removes the rate limit state, returning the strategy to its initial fresh state.
	//
	// "fresh state" means no stored state exists (empty string/"" in storage).
	// All strategies treat empty state as a fresh request with full quota available.
	// After Reset, the next Allow call will behave identically to a new user or
	// a request whose previous state has expired due to TTL.
	Reset(ctx context.Context, config StrategyConfig) error
}
