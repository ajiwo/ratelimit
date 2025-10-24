package ratelimit

import (
	"context"
	"fmt"
	"strings"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

type Limiter = RateLimiter

// RateLimiter implements single or dual strategy rate limiting
type RateLimiter struct {
	config          Config
	primaryStrategy strategies.Strategy
	basePrefix      string // cached BaseKey + ":" for fast key construction
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
func (r *RateLimiter) Allow(opts ...AccessOption) (bool, error) {
	// Parse access options to check if results are requested
	accessOpts, err := r.parseAccessOptions(opts)
	if err != nil {
		return false, fmt.Errorf("failed to parse access options: %w", err)
	}

	// Check if results are requested
	if accessOpts.result != nil {
		// Use allowWithResult and populate the results map
		allowed, results, err := r.allowWithResult(accessOpts)
		if err != nil {
			return false, err
		}
		*accessOpts.result = results
		return allowed, nil
	}

	// No results requested, use simple version
	allowed, _, err := r.allowWithResult(accessOpts)
	return allowed, err
}

// GetStats returns detailed statistics for all configured strategies
func (r *RateLimiter) GetStats(opts ...AccessOption) (map[string]strategies.Result, error) {
	// Parse access options
	accessOpts, err := r.parseAccessOptions(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to parse access options: %w", err)
	}

	strategyConfig, err := r.buildStrategyConfig(accessOpts)
	if err != nil {
		return nil, err
	}

	// Get stats from the strategy (composite or single)
	results, err := r.primaryStrategy.GetResult(accessOpts.ctx, strategyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	return results, nil
}

// GetBackend returns the storage backend used by this rate limiter
func (r *RateLimiter) GetBackend() backends.Backend {
	return r.config.Storage
}

// GetConfig returns the configuration used by this rate limiter
func (r *RateLimiter) GetConfig() Config {
	return r.config
}

// Reset resets the rate limit counters for all strategies (mainly for testing)
func (r *RateLimiter) Reset(opts ...AccessOption) error {
	// Parse access options
	accessOpts, err := r.parseAccessOptions(opts)
	if err != nil {
		return fmt.Errorf("failed to parse access options: %w", err)
	}

	strategyConfig, err := r.buildStrategyConfig(accessOpts)
	if err != nil {
		return err
	}

	// Reset the strategy (composite or single)
	if err := r.primaryStrategy.Reset(accessOpts.ctx, strategyConfig); err != nil {
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
func (r *RateLimiter) allowWithResult(accessOpts *accessOptions) (bool, map[string]strategies.Result, error) {
	strategyConfig, err := r.buildStrategyConfig(accessOpts)
	if err != nil {
		return false, nil, err
	}

	// Use the strategy (composite or single)
	results, err := r.primaryStrategy.Allow(accessOpts.ctx, strategyConfig)
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

// parseAccessOptions parses the provided access options and returns the configuration
func (r *RateLimiter) parseAccessOptions(opts []AccessOption) (*accessOptions, error) {
	result := &accessOptions{
		ctx: context.Background(), // Default context
		key: "default",            // Default key
	}

	for _, opt := range opts {
		if err := opt(result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// createPrimaryConfig creates strategy-specific configuration for the primary strategy
func (r *RateLimiter) createPrimaryConfig(dynamicKey string) (strategies.StrategyConfig, error) {
	prefix := r.basePrefix
	if prefix == "" {
		prefix = r.config.BaseKey + ":"
	}
	var keyBuilder strings.Builder
	keyBuilder.Grow(len(prefix) + len(dynamicKey))
	keyBuilder.WriteString(prefix)
	keyBuilder.WriteString(dynamicKey)
	key := keyBuilder.String()

	primaryConfig := r.config.PrimaryConfig
	return primaryConfig.WithKey(key), nil
}

// buildStrategyConfig builds the appropriate strategy config (composite or single)
func (r *RateLimiter) buildStrategyConfig(accessOpts *accessOptions) (strategies.StrategyConfig, error) {
	if r.config.SecondaryConfig != nil {
		return strategies.CompositeConfig{
			BaseKey:   r.config.BaseKey,
			Primary:   r.config.PrimaryConfig,
			Secondary: r.config.SecondaryConfig,
		}, nil
	}
	return r.createPrimaryConfig(accessOpts.key)
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
		// Use composite strategy for dual-strategy behavior
		compositeStrategy := strategies.NewComposite(config.Storage)
		limiter.primaryStrategy = compositeStrategy

		// Create and configure the individual strategies
		primaryStrategy, err := strategies.Create(config.PrimaryConfig.ID(), config.Storage)
		if err != nil {
			return nil, fmt.Errorf("failed to create primary strategy: %w", err)
		}

		secondaryStrategy, err := strategies.Create(config.SecondaryConfig.ID(), config.Storage)
		if err != nil {
			return nil, fmt.Errorf("failed to create secondary strategy: %w", err)
		}

		// Set the individual strategies on the composite
		if comp, ok := compositeStrategy.(interface {
			SetPrimaryStrategy(strategies.Strategy)
			SetSecondaryStrategy(strategies.Strategy)
		}); ok {
			comp.SetPrimaryStrategy(primaryStrategy)
			comp.SetSecondaryStrategy(secondaryStrategy)
		} else {
			return nil, fmt.Errorf("composite strategy type assertion failed")
		}

		return limiter, nil
	}

	// Single strategy case
	primaryStrategyID := config.PrimaryConfig.ID()
	primaryStrategy, err := strategies.Create(primaryStrategyID, config.Storage)
	if err != nil {
		return nil, fmt.Errorf("failed to create primary strategy: %w", err)
	}
	limiter.primaryStrategy = primaryStrategy

	return limiter, nil
}
