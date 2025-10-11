package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

// Option is a functional option for configuring the rate limiter
type Option func(*MultiTierConfig) error

// WithFixedWindowStrategy configures the rate limiter to use fixed window strategy
func WithFixedWindowStrategy(tiers ...TierConfig) Option {
	return func(config *MultiTierConfig) error {
		// Default tier if none provided
		if len(tiers) == 0 {
			config.PrimaryConfig = FixedWindowConfig{
				Tiers: []TierConfig{
					{
						Interval: time.Minute,
						Limit:    100,
					},
				},
			}
		} else {
			config.PrimaryConfig = FixedWindowConfig{
				Tiers: tiers,
			}
		}

		return nil
	}
}

// WithTokenBucketStrategy configures the rate limiter to use token bucket strategy
// If used with another strategy (like Fixed Window), it becomes the secondary smoother strategy
func WithTokenBucketStrategy(burstSize int, refillRate float64) Option {
	return func(config *MultiTierConfig) error {
		tokenConfig := strategies.TokenBucketConfig{
			BurstSize:  burstSize,
			RefillRate: refillRate,
		}

		// If no primary strategy is set yet, set this as primary
		if config.PrimaryConfig == nil {
			config.PrimaryConfig = tokenConfig
		} else {
			// Set as secondary strategy
			config.SecondaryConfig = tokenConfig
		}

		return nil
	}
}

// WithLeakyBucketStrategy configures the rate limiter to use leaky bucket strategy
// If used with another strategy (like Fixed Window), it becomes the secondary smoother strategy
func WithLeakyBucketStrategy(capacity int, leakRate float64) Option {
	return func(config *MultiTierConfig) error {
		leakyConfig := strategies.LeakyBucketConfig{
			Capacity: capacity,
			LeakRate: leakRate,
		}

		// If no primary strategy is set yet, set this as primary
		if config.PrimaryConfig == nil {
			config.PrimaryConfig = leakyConfig
		} else {
			// Set as secondary strategy
			config.SecondaryConfig = leakyConfig
		}

		return nil
	}
}

// WithPrimaryStrategy configures the primary rate limiting strategy with custom configuration
func WithPrimaryStrategy(strategyConfig strategies.StrategyConfig) Option {
	return func(config *MultiTierConfig) error {
		if strategyConfig == nil {
			return fmt.Errorf("primary strategy config cannot be nil")
		}
		config.PrimaryConfig = strategyConfig
		return nil
	}
}

// WithSecondaryStrategy configures the secondary smoother strategy
func WithSecondaryStrategy(strategyConfig strategies.StrategyConfig) Option {
	return func(config *MultiTierConfig) error {
		if strategyConfig == nil {
			return fmt.Errorf("secondary strategy config cannot be nil")
		}

		// Secondary strategy must be a bucket-based strategy
		if strategyConfig.Type() != strategies.StrategyTokenBucket && strategyConfig.Type() != strategies.StrategyLeakyBucket {
			return fmt.Errorf("secondary strategy must be token bucket or leaky bucket, got %s", strategyConfig.Type())
		}

		config.SecondaryConfig = strategyConfig
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
