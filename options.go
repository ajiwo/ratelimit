package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

// Option is a functional option for configuring the rate limiter
type Option func(*MultiTierConfig) error

// WithFixedWindowStrategy configures the rate limiter to use fixed window strategy
func WithFixedWindowStrategy(tiers ...TierConfig) Option {
	return func(config *MultiTierConfig) error {
		config.Strategy = StrategyFixedWindow

		// Default tier if none provided
		if len(tiers) == 0 {
			config.Tiers = []TierConfig{
				{
					Interval: time.Minute,
					Limit:    100,
				},
			}
		} else {
			config.Tiers = tiers
		}

		return nil
	}
}

// WithTokenBucketStrategy configures the rate limiter to use token bucket strategy
// If used with another strategy (like Fixed Window), it becomes the secondary smoother strategy
func WithTokenBucketStrategy(burstSize int, refillRate float64) Option {
	return func(config *MultiTierConfig) error {
		// If no primary strategy is set yet, set this as primary
		if config.Strategy == "" {
			config.Strategy = StrategyTokenBucket
			config.BurstSize = burstSize
			config.RefillRate = refillRate
			config.Tiers = nil // Token bucket doesn't use tiers
		} else {
			// Set as secondary strategy
			strategy := StrategyTokenBucket
			config.SecondaryStrategy = &strategy
			config.SecondaryBurstSize = burstSize
			config.SecondaryRefillRate = refillRate
		}

		return nil
	}
}

// WithLeakyBucketStrategy configures the rate limiter to use leaky bucket strategy
// If used with another strategy (like Fixed Window), it becomes the secondary smoother strategy
func WithLeakyBucketStrategy(capacity int, leakRate float64) Option {
	return func(config *MultiTierConfig) error {
		// If no primary strategy is set yet, set this as primary
		if config.Strategy == "" {
			config.Strategy = StrategyLeakyBucket
			config.Capacity = capacity
			config.LeakRate = leakRate
			config.Tiers = nil // Leaky bucket doesn't use tiers
		} else {
			// Set as secondary strategy
			strategy := StrategyLeakyBucket
			config.SecondaryStrategy = &strategy
			config.SecondaryCapacity = capacity
			config.SecondaryLeakRate = leakRate
		}

		return nil
	}
}

// WithPrimaryStrategy configures the primary rate limiting strategy (explicitly)
func WithPrimaryStrategy(strategy StrategyType, tiers ...TierConfig) Option {
	return func(config *MultiTierConfig) error {
		config.Strategy = strategy

		if strategy == StrategyFixedWindow {
			// Default tier if none provided for fixed window
			if len(tiers) == 0 {
				config.Tiers = []TierConfig{
					{
						Interval: time.Minute,
						Limit:    100,
					},
				}
			} else {
				config.Tiers = tiers
			}
		} else {
			// For bucket strategies used as primary, no tiers needed
			config.Tiers = nil
		}

		return nil
	}
}

// WithBaseKey sets the base key for rate limiting
func WithBaseKey(key string) Option {
	return func(config *MultiTierConfig) error {
		config.BaseKey = key
		return nil
	}
}

// WithTiers overrides the default tiers for any strategy
func WithTiers(tiers ...TierConfig) Option {
	return func(config *MultiTierConfig) error {
		if len(tiers) == 0 {
			return fmt.Errorf("at least one tier must be provided")
		}
		config.Tiers = tiers
		return nil
	}
}

// WithCleanupInterval configures the interval for cleaning up stale locks
// Set to 0 to disable automatic cleanup
func WithCleanupInterval(interval time.Duration) Option {
	return func(config *MultiTierConfig) error {
		config.CleanupInterval = interval
		return nil
	}
}

// AccessOption defines a functional option for rate limiter access methods
type AccessOption func(*accessOptions) error

// accessOptions holds the configuration for a rate limiter access operation
type accessOptions struct {
	ctx    context.Context
	key    string
	result *map[string]TierResult // Optional results pointer
}

// WithContext provides a context for the rate limiter operation
func WithContext(ctx context.Context) AccessOption {
	return func(opts *accessOptions) error {
		if ctx == nil {
			return fmt.Errorf("context cannot be nil")
		}
		opts.ctx = ctx
		return nil
	}
}

// WithKey provides a dynamic key for the rate limiter operation
func WithKey(key string) AccessOption {
	return func(opts *accessOptions) error {
		if err := validateKey(key, "dynamic key"); err != nil {
			return err
		}
		opts.key = key
		return nil
	}
}

// WithResult provides a pointer to a map that will be populated with detailed results
func WithResult(results *map[string]TierResult) AccessOption {
	return func(opts *accessOptions) error {
		if results == nil {
			return fmt.Errorf("results pointer cannot be nil")
		}
		opts.result = results
		return nil
	}
}

// WithBackend configures the rate limiter to use a custom backend
func WithBackend(backend backends.Backend) Option {
	return func(config *MultiTierConfig) error {
		if backend == nil {
			return fmt.Errorf("backend cannot be nil")
		}
		if config.Storage != nil {
			err := config.Storage.Close()
			if err != nil {
				return fmt.Errorf("failed to close existing backend: %w", err)
			}
		}
		config.Storage = backend
		return nil
	}
}
