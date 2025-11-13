package ratelimit

import (
	"context"
	"fmt"

	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/composite"
	"github.com/ajiwo/ratelimit/utils/builderpool"
)

type Limiter = RateLimiter

// RateLimiter implements single or dual strategy rate limiting
type RateLimiter struct {
	config     Config
	strategy   strategies.Strategy
	basePrefix string // cached BaseKey + ":" for fast key construction
}

// New creates a new rate limiter with functional options
func New(opts ...Option) (*RateLimiter, error) {
	// Create default configuration
	config := Config{
		BaseKey: "default",
	}

	// Apply provided options
	for _, opt := range opts {
		if err := opt(&config); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	// Create the limiter with the final configuration
	return newRateLimiter(config)
}

// Allow checks if a request is allowed according to the configured strategies
func (r *RateLimiter) Allow(ctx context.Context, options AccessOptions) (bool, error) {
	dynamicKey, err := checkDynamicKey(options)
	if err != nil {
		return false, err
	}

	allowed, results, err := r.allowWithResult(ctx, dynamicKey)
	if err != nil {
		return false, err
	}

	// If result map is provided, populate it
	if options.Result != nil {
		*options.Result = results
	}

	return allowed, err
}

// Peek retrieves strategy results without consuming quota and returns an overall allowed boolean
func (r *RateLimiter) Peek(ctx context.Context, options AccessOptions) (bool, error) {
	dynamicKey, err := checkDynamicKey(options)
	if err != nil {
		return false, err
	}

	strategyConfig := r.buildStrategyConfig(dynamicKey)

	// Get stats from the strategy (composite or single)
	results, err := r.strategy.Peek(ctx, strategyConfig)
	if err != nil {
		return false, fmt.Errorf("failed to get stats: %w", err)
	}
	if options.Result != nil {
		*options.Result = results
	}
	// Determine overall allowed similarly to Allow
	allAllowed := true
	for _, res := range results {
		if !res.Allowed {
			allAllowed = false
			break
		}
	}
	return allAllowed, nil
}

// Reset resets the rate limit counters for all strategies (mainly for testing)
func (r *RateLimiter) Reset(ctx context.Context, options AccessOptions) error {
	dynamicKey, err := checkDynamicKey(options)
	if err != nil {
		return err
	}

	strategyConfig := r.buildStrategyConfig(dynamicKey)

	// Reset the strategy (composite or single)
	if err := r.strategy.Reset(ctx, strategyConfig); err != nil {
		return fmt.Errorf("failed to reset strategy: %w", err)
	}

	return nil
}

// Close cleans up resources used by the rate limiter
func (r *RateLimiter) Close() error {
	// Close the storage backend
	if r.config.Storage != nil {
		return r.config.Storage.Close()
	}
	return nil
}

// allowWithResult1 checks if a request is allowed and returns detailed results
func (r *RateLimiter) allowWithResult(ctx context.Context, dynamicKey string) (bool, strategies.Results, error) {
	strategyConfig := r.buildStrategyConfig(dynamicKey)

	// Use the strategy (composite or single)
	results, err := r.strategy.Allow(ctx, strategyConfig)
	if err != nil {
		return false, nil, fmt.Errorf("strategy check failed: %w", err)
	}

	// Determine if the request was allowed by checking if all results allow it
	allAllowed := true
	for _, result := range results {
		if !result.Allowed {
			allAllowed = false
			break
		}
	}

	return allAllowed, results, nil
}

// buildStrategyConfig builds the appropriate strategy config (composite or single)
func (r *RateLimiter) buildStrategyConfig(dynamicKey string) strategies.Config {
	// build dual strategy config
	if r.config.SecondaryConfig != nil {
		return composite.Config{
			BaseKey:   r.config.BaseKey,
			Primary:   r.config.PrimaryConfig,
			Secondary: r.config.SecondaryConfig,
		}.
			WithKey(dynamicKey).
			WithMaxRetries(r.config.maxRetries)
	}

	// build single strategy config
	sb := builderpool.Get()
	defer builderpool.Put(sb)

	if r.basePrefix == "" {
		sb.WriteString(r.config.BaseKey)
		sb.WriteString(":")
	} else {
		sb.WriteString(r.basePrefix)
	}

	sb.WriteString(dynamicKey)

	return r.config.PrimaryConfig.
		WithKey(sb.String()).
		WithMaxRetries(r.config.maxRetries)
}

// checkDynamicKey validates (if enabled) and returns the dynamic key
func checkDynamicKey(options AccessOptions) (string, error) {
	if options.Key != "" {
		if !options.SkipValidation {
			if err := validateKey(options.Key, "dynamic key"); err != nil {
				return "", err
			}
		}
		return options.Key, nil
	}
	return "default", nil
}

// newRateLimiter creates a new rate limiter
func newRateLimiter(config Config) (*RateLimiter, error) {
	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, err
	}

	limiter := &RateLimiter{
		config:     config,
		basePrefix: config.BaseKey + ":",
	}

	// Check if we have a dual-strategy configuration
	if config.SecondaryConfig != nil {
		// Use comp strategy for dual-strategy behavior
		comp, err := composite.New(config.Storage, config.PrimaryConfig, config.SecondaryConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create composite strategy: %w", err)
		}
		limiter.strategy = comp

		return limiter, nil
	}

	// Single strategy case
	primaryStrategyID := config.PrimaryConfig.ID()
	primaryStrategy, err := strategies.Create(primaryStrategyID, config.Storage)
	if err != nil {
		return nil, fmt.Errorf("failed to create primary strategy: %w", err)
	}
	limiter.strategy = primaryStrategy

	return limiter, nil
}
