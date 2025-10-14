package ratelimit

import (
	"context"
	"fmt"
	"maps"
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

	primaryResults, err := r.primaryStrategy.GetResult(accessOpts.ctx, primaryConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats from primary strategy: %w", err)
	}
	// Merge strategy results into our stats map
	for key, result := range primaryResults {
		stats["primary_"+key] = result
	}

	// Get stats from secondary strategy if configured
	if r.secondaryStrategy != nil {
		secondaryConfig, err := r.createSecondaryConfig(accessOpts.key)
		if err != nil {
			return nil, fmt.Errorf("failed to create secondary config: %w", err)
		}

		secondaryResults, err := r.secondaryStrategy.GetResult(accessOpts.ctx, secondaryConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to get stats from secondary strategy: %w", err)
		}
		// Merge strategy results into our stats map
		for key, result := range secondaryResults {
			stats["secondary_"+key] = result
		}
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

	primaryResults, err := r.primaryStrategy.GetResult(accessOpts.ctx, primaryConfig)
	if err != nil {
		return false, nil, fmt.Errorf("primary strategy check failed: %w", err)
	}

	// Check if all tiers allow the request
	allAllowed := true
	for tierName, result := range primaryResults {
		// Merge all strategy results
		results["primary_"+tierName] = result
		if !result.Allowed {
			allAllowed = false
		}
	}

	// If any tier doesn't allow, don't check secondary
	if !allAllowed {
		return false, results, nil
	}

	// Primary allowed, now check secondary strategy using GetResult
	secondaryConfig, err := r.createSecondaryConfig(accessOpts.key)
	if err != nil {
		return false, nil, fmt.Errorf("failed to create secondary config: %w", err)
	}

	secondaryResults, err := r.secondaryStrategy.GetResult(accessOpts.ctx, secondaryConfig)
	if err != nil {
		return false, nil, fmt.Errorf("secondary strategy check failed: %w", err)
	}

	// Check if all secondary tiers allow the request
	secondaryAllAllowed := true
	for tierName, result := range secondaryResults {
		// Merge all strategy results
		results["secondary_"+tierName] = result
		if !result.Allowed {
			secondaryAllAllowed = false
		}
	}

	// If any secondary tier doesn't allow, don't consume from either strategy
	if !secondaryAllAllowed {
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
	strategyResults, err := r.primaryStrategy.Allow(accessOpts.ctx, primaryConfig)
	if err != nil {
		return false, nil, fmt.Errorf("primary strategy check failed: %w", err)
	}

	// Check if all tiers allow the request
	allAllowed := true
	for tierName, result := range strategyResults {
		// Merge all strategy results
		results["primary_"+tierName] = result
		if !result.Allowed {
			allAllowed = false
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
	var keyBuilder strings.Builder
	keyBuilder.Grow(len(r.config.BaseKey) + len(dynamicKey) + 1) // +1 for the colon
	keyBuilder.WriteString(r.config.BaseKey)
	keyBuilder.WriteByte(':')
	keyBuilder.WriteString(dynamicKey)
	key := keyBuilder.String()

	primaryConfig := r.config.PrimaryConfig
	switch primaryConfig.Name() {
	case "fixed_window":
		fixedConfig := primaryConfig.(fixedwindow.Config)
		// Create new tiers map with updated key
		newTiers := make(map[string]fixedwindow.Tier)
		maps.Copy(newTiers, fixedConfig.Tiers)
		return fixedwindow.Config{
			Key:   key,
			Tiers: newTiers,
		}, nil

	case "token_bucket":
		tokenConfig := primaryConfig.(tokenbucket.Config)
		return tokenbucket.Config{
			Key:        key,
			BurstSize:  tokenConfig.BurstSize,
			RefillRate: tokenConfig.RefillRate,
		}, nil

	case "leaky_bucket":
		leakyConfig := primaryConfig.(leakybucket.Config)
		return leakybucket.Config{
			Key:      key,
			Capacity: leakyConfig.Capacity,
			LeakRate: leakyConfig.LeakRate,
		}, nil

	default:
		return nil, fmt.Errorf("unknown primary strategy: %s", primaryConfig.Name())
	}
}

// createSecondaryConfig creates strategy-specific configuration for the secondary strategy
func (r *RateLimiter) createSecondaryConfig(dynamicKey string) (strategies.StrategyConfig, error) {
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
	switch secondaryConfig.Name() {
	case "token_bucket":
		tokenConfig := secondaryConfig.(tokenbucket.Config)
		return tokenbucket.Config{
			Key:        key,
			BurstSize:  tokenConfig.BurstSize,
			RefillRate: tokenConfig.RefillRate,
		}, nil

	case "leaky_bucket":
		leakyConfig := secondaryConfig.(leakybucket.Config)
		return leakybucket.Config{
			Key:      key,
			Capacity: leakyConfig.Capacity,
			LeakRate: leakyConfig.LeakRate,
		}, nil

	default:
		return nil, fmt.Errorf("unknown secondary strategy: %s", secondaryConfig.Name())
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
	primaryStrategyName := config.PrimaryConfig.Name()
	primaryStrategy, err := createStrategy(primaryStrategyName, config.Storage)
	if err != nil {
		return nil, fmt.Errorf("failed to create primary strategy: %w", err)
	}
	limiter.primaryStrategy = primaryStrategy

	// Create secondary strategy if configured
	if config.SecondaryConfig != nil {
		secondaryStrategyName := config.SecondaryConfig.Name()
		secondaryStrategy, err := createStrategy(secondaryStrategyName, config.Storage)
		if err != nil {
			return nil, fmt.Errorf("failed to create secondary strategy: %w", err)
		}
		limiter.secondaryStrategy = secondaryStrategy
	}

	return limiter, nil
}

// createStrategy creates a strategy instance based on the name
func createStrategy(strategyName string, storage backends.Backend) (strategies.Strategy, error) {
	switch strategyName {
	case "fixed_window":
		return fixedwindow.New(storage), nil
	case "token_bucket":
		return tokenbucket.New(storage), nil
	case "leaky_bucket":
		return leakybucket.New(storage), nil
	default:
		return nil, fmt.Errorf("unknown strategy: %s", strategyName)
	}
}
