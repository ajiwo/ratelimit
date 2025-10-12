package ratelimit

import (
	"context"
	"fmt"
	"strings"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/fixedwindow"
	"github.com/ajiwo/ratelimit/strategies/leakybucket"
	"github.com/ajiwo/ratelimit/strategies/tokenbucket"
)

type Limiter = RateLimiter

// RateLimiter implements single or dual strategy rate limiting
type RateLimiter struct {
	config            Config
	primaryStrategy   strategies.Strategy
	secondaryStrategy strategies.Strategy
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
		allowed, results, err := r.allowWithResult(opts...)
		if err != nil {
			return false, err
		}
		*accessOpts.result = results
		return allowed, nil
	}

	// No results requested, use simple version
	allowed, _, err := r.allowWithResult(opts...)
	return allowed, err
}

// GetStats returns detailed statistics for all configured strategies
func (r *RateLimiter) GetStats(opts ...AccessOption) (map[string]strategies.Result, error) {
	// Parse access options
	accessOpts, err := r.parseAccessOptions(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to parse access options: %w", err)
	}

	stats := make(map[string]strategies.Result)

	// Get stats from primary strategy
	primaryConfig, err := r.createPrimaryConfig(accessOpts.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create primary config: %w", err)
	}

	primaryResult, err := r.primaryStrategy.GetResult(accessOpts.ctx, primaryConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats from primary strategy: %w", err)
	}
	stats["primary"] = primaryResult

	// Get stats from secondary strategy if configured
	if r.secondaryStrategy != nil {
		secondaryConfig, err := r.createSecondaryConfig(accessOpts.key)
		if err != nil {
			return nil, fmt.Errorf("failed to create secondary config: %w", err)
		}

		secondaryResult, err := r.secondaryStrategy.GetResult(accessOpts.ctx, secondaryConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to get stats from secondary strategy: %w", err)
		}
		stats["secondary"] = secondaryResult
	}

	return stats, nil
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

	// Reset primary strategy
	primaryConfig, err := r.createPrimaryConfig(accessOpts.key)
	if err != nil {
		return fmt.Errorf("failed to create primary config: %w", err)
	}

	if err := r.primaryStrategy.Reset(accessOpts.ctx, primaryConfig); err != nil {
		return fmt.Errorf("failed to reset primary strategy: %w", err)
	}

	// Reset secondary strategy if configured
	if r.secondaryStrategy != nil {
		secondaryConfig, err := r.createSecondaryConfig(accessOpts.key)
		if err != nil {
			return fmt.Errorf("failed to create secondary config: %w", err)
		}

		if err := r.secondaryStrategy.Reset(accessOpts.ctx, secondaryConfig); err != nil {
			return fmt.Errorf("failed to reset secondary strategy: %w", err)
		}
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

// allowWithResult checks if a request is allowed and returns detailed results
func (r *RateLimiter) allowWithResult(opts ...AccessOption) (bool, map[string]strategies.Result, error) {
	// Parse access options
	accessOpts, err := r.parseAccessOptions(opts)
	if err != nil {
		return false, nil, fmt.Errorf("failed to parse access options: %w", err)
	}

	results := make(map[string]strategies.Result)

	// Handle dual strategy case
	if r.secondaryStrategy != nil {
		return r.handleDualStrategy(accessOpts, results)
	}

	// Handle single strategy case
	return r.handleSingleStrategy(accessOpts, results)
}

// handleDualStrategy handles dual strategy (primary + secondary)
func (r *RateLimiter) handleDualStrategy(accessOpts *accessOptions, results map[string]strategies.Result) (bool, map[string]strategies.Result, error) {
	// First check primary strategy using GetResult (no quota consumption)
	primaryConfig, err := r.createPrimaryConfig(accessOpts.key)
	if err != nil {
		return false, nil, fmt.Errorf("failed to create primary config: %w", err)
	}

	primaryResult, err := r.primaryStrategy.GetResult(accessOpts.ctx, primaryConfig)
	if err != nil {
		return false, nil, fmt.Errorf("primary strategy check failed: %w", err)
	}
	results["primary"] = primaryResult

	// If primary strategy doesn't allow, don't check secondary
	if !primaryResult.Allowed {
		return false, results, nil
	}

	// Primary allowed, now check secondary strategy using GetResult
	secondaryConfig, err := r.createSecondaryConfig(accessOpts.key)
	if err != nil {
		return false, nil, fmt.Errorf("failed to create secondary config: %w", err)
	}

	secondaryResult, err := r.secondaryStrategy.GetResult(accessOpts.ctx, secondaryConfig)
	if err != nil {
		return false, nil, fmt.Errorf("secondary strategy check failed: %w", err)
	}
	results["secondary"] = secondaryResult

	// If secondary strategy doesn't allow, don't consume from either strategy
	if !secondaryResult.Allowed {
		return false, results, nil
	}

	// Both strategies allow, now consume quota from both
	if _, err := r.primaryStrategy.Allow(accessOpts.ctx, primaryConfig); err != nil {
		return false, nil, fmt.Errorf("primary strategy quota consumption failed: %w", err)
	}

	if _, err := r.secondaryStrategy.Allow(accessOpts.ctx, secondaryConfig); err != nil {
		return false, nil, fmt.Errorf("secondary strategy quota consumption failed: %w", err)
	}

	return true, results, nil
}

// handleSingleStrategy handles single strategy case
func (r *RateLimiter) handleSingleStrategy(accessOpts *accessOptions, results map[string]strategies.Result) (bool, map[string]strategies.Result, error) {
	primaryConfig, err := r.createPrimaryConfig(accessOpts.key)
	if err != nil {
		return false, nil, fmt.Errorf("failed to create primary config: %w", err)
	}

	// Check if request is allowed using the primary strategy
	result, err := r.primaryStrategy.Allow(accessOpts.ctx, primaryConfig)
	if err != nil {
		return false, nil, fmt.Errorf("primary strategy check failed: %w", err)
	}

	results["primary"] = result
	return result.Allowed, results, nil
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
func (r *RateLimiter) createPrimaryConfig(dynamicKey string) (any, error) {
	var keyBuilder strings.Builder
	keyBuilder.Grow(len(r.config.BaseKey) + len(dynamicKey) + 1) // +1 for the colon
	keyBuilder.WriteString(r.config.BaseKey)
	keyBuilder.WriteByte(':')
	keyBuilder.WriteString(dynamicKey)
	key := keyBuilder.String()

	primaryConfig := r.config.PrimaryConfig
	switch primaryConfig.Type() {
	case strategies.StrategyFixedWindow:
		fixedConfig := primaryConfig.(strategies.FixedWindowConfig)
		return strategies.FixedWindowConfig{
			Key:    key,
			Limit:  fixedConfig.Limit,
			Window: fixedConfig.Window,
		}, nil

	case strategies.StrategyTokenBucket:
		tokenConfig := primaryConfig.(strategies.TokenBucketConfig)
		return strategies.TokenBucketConfig{
			Key:        key,
			BurstSize:  tokenConfig.BurstSize,
			RefillRate: tokenConfig.RefillRate,
		}, nil

	case strategies.StrategyLeakyBucket:
		leakyConfig := primaryConfig.(strategies.LeakyBucketConfig)
		return strategies.LeakyBucketConfig{
			Key:      key,
			Capacity: leakyConfig.Capacity,
			LeakRate: leakyConfig.LeakRate,
		}, nil

	default:
		return nil, fmt.Errorf("unknown primary strategy: %s", primaryConfig.Type())
	}
}

// createSecondaryConfig creates strategy-specific configuration for the secondary strategy
func (r *RateLimiter) createSecondaryConfig(dynamicKey string) (any, error) {
	if r.config.SecondaryConfig == nil {
		return nil, fmt.Errorf("secondary strategy not configured")
	}

	var keyBuilder strings.Builder
	keyBuilder.Grow(len(r.config.BaseKey) + len(dynamicKey) + 10) // +10 for ":secondary" suffix
	keyBuilder.WriteString(r.config.BaseKey)
	keyBuilder.WriteByte(':')
	keyBuilder.WriteString(dynamicKey)
	keyBuilder.WriteString(":secondary")
	key := keyBuilder.String()

	secondaryConfig := r.config.SecondaryConfig
	switch secondaryConfig.Type() {
	case strategies.StrategyTokenBucket:
		tokenConfig := secondaryConfig.(strategies.TokenBucketConfig)
		return strategies.TokenBucketConfig{
			Key:        key,
			BurstSize:  tokenConfig.BurstSize,
			RefillRate: tokenConfig.RefillRate,
		}, nil

	case strategies.StrategyLeakyBucket:
		leakyConfig := secondaryConfig.(strategies.LeakyBucketConfig)
		return strategies.LeakyBucketConfig{
			Key:      key,
			Capacity: leakyConfig.Capacity,
			LeakRate: leakyConfig.LeakRate,
		}, nil

	default:
		return nil, fmt.Errorf("unknown secondary strategy: %s", secondaryConfig.Type())
	}
}

// newRateLimiter creates a new rate limiter
func newRateLimiter(config Config) (*RateLimiter, error) {
	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, err
	}

	limiter := &RateLimiter{
		config: config,
	}

	// Create primary strategy
	primaryStrategyType := config.PrimaryConfig.Type()
	primaryStrategy, err := createStrategy(primaryStrategyType, config.Storage)
	if err != nil {
		return nil, fmt.Errorf("failed to create primary strategy: %w", err)
	}
	limiter.primaryStrategy = primaryStrategy

	// Create secondary strategy if configured
	if config.SecondaryConfig != nil {
		secondaryStrategyType := config.SecondaryConfig.Type()
		secondaryStrategy, err := createStrategy(secondaryStrategyType, config.Storage)
		if err != nil {
			return nil, fmt.Errorf("failed to create secondary strategy: %w", err)
		}
		limiter.secondaryStrategy = secondaryStrategy
	}

	return limiter, nil
}

// createStrategy creates a strategy instance based on the type
func createStrategy(strategyType strategies.StrategyType, storage backends.Backend) (strategies.Strategy, error) {
	switch strategyType {
	case strategies.StrategyFixedWindow:
		return fixedwindow.New(storage), nil
	case strategies.StrategyTokenBucket:
		return tokenbucket.New(storage), nil
	case strategies.StrategyLeakyBucket:
		return leakybucket.New(storage), nil
	default:
		return nil, fmt.Errorf("unknown strategy type: %s", strategyType)
	}
}
