package ratelimit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

const (
	// MaxTiers is the maximum number of tiers allowed in a multi-tier configuration
	MaxTiers = 12

	// MinInterval is the minimum allowed interval for any tier
	MinInterval = 5 * time.Second

	// DefaultCleanupInterval is the default interval for cleaning up stale locks
	DefaultCleanupInterval = 10 * time.Minute
)

// TierConfig defines a single tier in multi-tier rate limiting
type TierConfig struct {
	Interval time.Duration // Time window (1 minute, 1 hour, 1 day, etc.)
	Limit    int           // Number of requests allowed in this interval
}

// StrategyType defines the rate limiting strategy to use
type StrategyType string

type Limiter = MultiTierLimiter

const (
	StrategyFixedWindow StrategyType = "fixed_window"
	StrategyTokenBucket StrategyType = "token_bucket"
	StrategyLeakyBucket StrategyType = "leaky_bucket"
)

// MultiTierConfig defines the configuration for multi-tier rate limiting
type MultiTierConfig struct {
	// Base configuration
	BaseKey  string           // Base key for rate limiting (e.g., "user:123")
	Storage  backends.Backend // Storage backend to use
	Strategy StrategyType     // Rate limiting strategy
	Tiers    []TierConfig     // Multiple tiers (minute, hour, day, etc.)

	// Strategy-specific configurations (only used by the chosen strategy)
	BurstSize  int     // For token bucket
	RefillRate float64 // For token bucket
	Capacity   int     // For leaky bucket
	LeakRate   float64 // For leaky bucket

	// Cleanup configuration
	CleanupInterval time.Duration // Interval for cleaning up stale locks (0 to disable)
}

// Result represents the result of a rate limiting strategy check
type Result struct {
	Allowed     bool              // Whether the request is allowed
	Results     map[string]Result // Individual tier results
	DeniedTiers []string          // Names of tiers that denied the request
}

// TierResult represents the result for a single tier
type TierResult struct {
	Allowed   bool      // Whether this tier allowed the request
	Remaining int       // Remaining requests in this tier
	Reset     time.Time // When this tier resets
	Total     int       // Total limit for this tier
	Used      int       // Number of requests used in this tier
}

// MultiTierLimiter implements multi-tier rate limiting
type MultiTierLimiter struct {
	config MultiTierConfig
	// Create separate strategy instances for each tier
	strategies map[string]strategies.Strategy

	// Cleanup ticker for managing stale locks
	cleanupTicker *time.Ticker
	cleanupStop   chan bool
}

// newMultiTierLimiter creates a new multi-tier rate limiter
func newMultiTierLimiter(config MultiTierConfig) (*MultiTierLimiter, error) {
	// Validate configuration
	if err := validateMultiTierConfig(config); err != nil {
		return nil, err
	}

	limiter := &MultiTierLimiter{
		config:     config,
		strategies: make(map[string]strategies.Strategy),
	}

	// Create a strategy for each tier
	for _, tier := range config.Tiers {
		tierName := getTierName(tier.Interval)

		strategy, err := createStrategy(config.Strategy, config.Storage)
		if err != nil {
			return nil, fmt.Errorf("failed to create strategy for tier %s: %w", tierName, err)
		}

		limiter.strategies[tierName] = strategy
	}

	// Start cleanup ticker if cleanup is enabled
	if config.CleanupInterval > 0 {
		limiter.cleanupStop = make(chan bool)
		limiter.cleanupTicker = time.NewTicker(config.CleanupInterval)

		// Start cleanup goroutine
		go func() {
			for {
				select {
				case <-limiter.cleanupTicker.C:
					// Perform cleanup for all strategies
					for _, strategy := range limiter.strategies {
						strategy.Cleanup(config.CleanupInterval * 2) // Cleanup locks older than 2x the interval
					}
				case <-limiter.cleanupStop:
					return
				}
			}
		}()
	}

	return limiter, nil
}

// parseAccessOptions parses the provided access options and returns the configuration
func (m *MultiTierLimiter) parseAccessOptions(opts []AccessOption) (*accessOptions, error) {
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

// Allow checks if a request is allowed across all configured tiers
func (m *MultiTierLimiter) Allow(opts ...AccessOption) (bool, error) {
	// Parse access options
	accessOpts, err := m.parseAccessOptions(opts)
	if err != nil {
		return false, fmt.Errorf("failed to parse access options: %w", err)
	}

	// Check each tier
	results := make(map[string]Result)
	deniedTiers := []string{}

	for _, tier := range m.config.Tiers {
		tierName := getTierName(tier.Interval)

		// Create strategy-specific config for this tier
		config, err := m.createTierConfig(accessOpts.key, tierName, tier.Limit, tier.Interval)
		if err != nil {
			return false, fmt.Errorf("failed to create config for tier %s: %w", tierName, err)
		}

		// Check if request is allowed for this tier
		strategy := m.strategies[tierName]
		allowed, err := strategy.Allow(accessOpts.ctx, config)
		if err != nil {
			return false, fmt.Errorf("tier %s check failed: %w", tierName, err)
		}

		results[tierName] = Result{
			Allowed: allowed,
		}

		if !allowed {
			deniedTiers = append(deniedTiers, tierName)
		}
	}

	// Request is only allowed if ALL tiers allow it
	allowed := len(deniedTiers) == 0

	return allowed, nil
}

// createTierConfig creates strategy-specific configuration for a tier
func (m *MultiTierLimiter) createTierConfig(dynamicKey string, tierName string, limit int, interval time.Duration) (any, error) {
	var keyBuilder strings.Builder
	keyBuilder.Grow(len(m.config.BaseKey) + len(dynamicKey) + len(tierName) + 2) // +2 for the colons
	keyBuilder.WriteString(m.config.BaseKey)
	keyBuilder.WriteByte(':')
	keyBuilder.WriteString(dynamicKey)
	keyBuilder.WriteByte(':')
	keyBuilder.WriteString(tierName)
	key := keyBuilder.String()

	switch m.config.Strategy {
	case StrategyFixedWindow:
		return strategies.FixedWindowConfig{
			RateLimitConfig: strategies.RateLimitConfig{
				Key:   key,
				Limit: limit,
			},
			Window: interval,
		}, nil

	case StrategyTokenBucket:
		return strategies.TokenBucketConfig{
			RateLimitConfig: strategies.RateLimitConfig{
				Key:   key,
				Limit: limit,
			},
			BurstSize:  m.config.BurstSize,
			RefillRate: m.config.RefillRate,
		}, nil

	case StrategyLeakyBucket:
		return strategies.LeakyBucketConfig{
			RateLimitConfig: strategies.RateLimitConfig{
				Key:   key,
				Limit: limit,
			},
			Capacity: m.config.Capacity,
			LeakRate: m.config.LeakRate,
		}, nil

	default:
		return nil, fmt.Errorf("unknown strategy: %s", m.config.Strategy)
	}
}

// getTierName returns a human-readable name for the tier
func getTierName(interval time.Duration) string {
	switch interval {
	case time.Minute:
		return "minute"
	case time.Hour:
		return "hour"
	case 24 * time.Hour:
		return "day"
	case 7 * 24 * time.Hour:
		return "week"
	case 30 * 24 * time.Hour:
		return "month"
	default:
		// For custom intervals, use duration string
		return interval.String()
	}
}

// createStrategy creates a strategy instance based on the type
func createStrategy(strategyType StrategyType, storage backends.Backend) (strategies.Strategy, error) {
	switch strategyType {
	case StrategyFixedWindow:
		return strategies.NewFixedWindow(storage), nil
	case StrategyTokenBucket:
		return strategies.NewTokenBucket(storage), nil
	case StrategyLeakyBucket:
		return strategies.NewLeakyBucket(storage), nil
	default:
		return nil, fmt.Errorf("unknown strategy type: %s", strategyType)
	}
}

// GetBackend returns the storage backend used by this rate limiter
func (m *MultiTierLimiter) GetBackend() backends.Backend {
	return m.config.Storage
}

// GetConfig returns the configuration used by this rate limiter
func (m *MultiTierLimiter) GetConfig() MultiTierConfig {
	return m.config
}

// GetStats returns detailed statistics for all tiers
func (m *MultiTierLimiter) GetStats(opts ...AccessOption) (map[string]TierResult, error) {
	// Parse access options
	accessOpts, err := m.parseAccessOptions(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to parse access options: %w", err)
	}

	stats := make(map[string]TierResult)

	for _, tier := range m.config.Tiers {
		tierName := getTierName(tier.Interval)

		// Create strategy-specific config for this tier
		config, err := m.createTierConfig(accessOpts.key, tierName, tier.Limit, tier.Interval)
		if err != nil {
			return nil, fmt.Errorf("failed to create config for tier %s: %w", tierName, err)
		}

		// Get stats from this tier's strategy
		strategy := m.strategies[tierName]
		result, err := strategy.GetResult(accessOpts.ctx, config)
		if err != nil {
			return nil, fmt.Errorf("failed to get stats for tier %s: %w", tierName, err)
		}

		stats[tierName] = TierResult{
			Allowed:   result.Allowed,
			Remaining: result.Remaining,
			Reset:     result.Reset,
			Total:     tier.Limit,
			Used:      tier.Limit - result.Remaining,
		}
	}

	return stats, nil
}

// Reset resets the rate limit counters for all tiers (mainly for testing)
func (m *MultiTierLimiter) Reset(opts ...AccessOption) error {
	// Parse access options
	accessOpts, err := m.parseAccessOptions(opts)
	if err != nil {
		return fmt.Errorf("failed to parse access options: %w", err)
	}

	for _, tier := range m.config.Tiers {
		tierName := getTierName(tier.Interval)

		// Create strategy-specific config for this tier
		config, err := m.createTierConfig(accessOpts.key, tierName, tier.Limit, tier.Interval)
		if err != nil {
			return fmt.Errorf("failed to create config for tier %s: %w", tierName, err)
		}

		// Reset this tier's strategy
		strategy := m.strategies[tierName]
		if err := strategy.Reset(accessOpts.ctx, config); err != nil {
			return fmt.Errorf("failed to reset tier %s: %w", tierName, err)
		}
	}

	return nil
}

// Close cleans up resources used by the rate limiter
func (m *MultiTierLimiter) Close() error {
	// Stop the cleanup ticker if it's running
	if m.cleanupTicker != nil {
		m.cleanupTicker.Stop()
		close(m.cleanupStop)
	}

	// Close the storage backend
	if m.config.Storage != nil {
		return m.config.Storage.Close()
	}
	return nil
}

// New creates a new rate limiter with functional options
func New(opts ...Option) (*MultiTierLimiter, error) {
	// Create default configuration
	config := MultiTierConfig{
		BaseKey:  "default",
		Strategy: StrategyFixedWindow,
		Tiers: []TierConfig{
			{
				Interval: time.Minute,
				Limit:    100,
			},
		},
		// Default strategy-specific values
		BurstSize:  10,
		RefillRate: 1.0,
		Capacity:   10,
		LeakRate:   1.0,
		// Default cleanup interval
		CleanupInterval: DefaultCleanupInterval,
	}

	// Apply provided options
	for _, opt := range opts {
		if err := opt(&config); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	// Create the limiter with the final configuration
	return newMultiTierLimiter(config)
}

// AllowWithResult checks if a request is allowed across all configured tiers and returns detailed results
func (m *MultiTierLimiter) AllowWithResult(opts ...AccessOption) (bool, map[string]strategies.Result, error) {
	// Parse access options
	accessOpts, err := m.parseAccessOptions(opts)
	if err != nil {
		return false, nil, fmt.Errorf("failed to parse access options: %w", err)
	}

	// Check each tier
	results := make(map[string]strategies.Result)
	deniedTiers := []string{}

	for _, tier := range m.config.Tiers {
		tierName := getTierName(tier.Interval)

		// Create strategy-specific config for this tier
		config, err := m.createTierConfig(accessOpts.key, tierName, tier.Limit, tier.Interval)
		if err != nil {
			return false, nil, fmt.Errorf("failed to create config for tier %s: %w", tierName, err)
		}

		// Check if request is allowed for this tier using AllowWithResult
		strategy := m.strategies[tierName]
		tierResult, err := strategy.AllowWithResult(accessOpts.ctx, config)
		if err != nil {
			return false, nil, fmt.Errorf("tier %s check failed: %w", tierName, err)
		}

		results[tierName] = tierResult

		if !tierResult.Allowed {
			deniedTiers = append(deniedTiers, tierName)
		}
	}

	// Request is only allowed if ALL tiers allow it
	allowed := len(deniedTiers) == 0

	return allowed, results, nil
}
