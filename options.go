package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

// Option is a functional option for configuring the rate limiter
type Option func(*MultiTierConfig) error

// WithMemoryBackend configures the rate limiter to use memory storage
func WithMemoryBackend() Option {
	return func(config *MultiTierConfig) error {
		config.Storage = backends.NewMemoryStorage()
		return nil
	}
}

// WithRedisBackend configures the rate limiter to use Redis storage
func WithRedisBackend(redisConfig backends.RedisConfig) Option {
	return func(config *MultiTierConfig) error {
		storage, err := backends.NewRedisStorage(redisConfig)
		if err != nil {
			return fmt.Errorf("failed to create Redis storage: %w", err)
		}
		config.Storage = storage
		return nil
	}
}

// WithPostgresBackend configures the rate limiter to use PostgreSQL storage
func WithPostgresBackend(postgresConfig backends.PostgresConfig) Option {
	return func(config *MultiTierConfig) error {
		storage, err := backends.NewPostgresStorage(postgresConfig)
		if err != nil {
			return fmt.Errorf("failed to create PostgreSQL storage: %w", err)
		}
		config.Storage = storage
		return nil
	}
}

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
func WithTokenBucketStrategy(burstSize int, refillRate float64, tiers ...TierConfig) Option {
	return func(config *MultiTierConfig) error {
		config.Strategy = StrategyTokenBucket
		config.BurstSize = burstSize
		config.RefillRate = refillRate

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

// WithLeakyBucketStrategy configures the rate limiter to use leaky bucket strategy
func WithLeakyBucketStrategy(capacity int, leakRate float64, tiers ...TierConfig) Option {
	return func(config *MultiTierConfig) error {
		config.Strategy = StrategyLeakyBucket
		config.Capacity = capacity
		config.LeakRate = leakRate

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
	ctx context.Context
	key string
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
