package ratelimit

import (
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
