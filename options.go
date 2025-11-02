package ratelimit

import (
	"fmt"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

// Option is a functional option for configuring the rate limiter
type Option func(*Config) error

// WithPrimaryStrategy configures the primary rate limiting strategy with custom configuration
func WithPrimaryStrategy(strategyConfig strategies.StrategyConfig) Option {
	return func(config *Config) error {
		if strategyConfig == nil {
			return fmt.Errorf("primary strategy config cannot be nil")
		}
		config.PrimaryConfig = strategyConfig
		return nil
	}
}

// WithSecondaryStrategy configures the secondary smoother strategy
func WithSecondaryStrategy(strategyConfig strategies.StrategyConfig) Option {
	return func(config *Config) error {
		if strategyConfig == nil {
			return fmt.Errorf("secondary strategy config cannot be nil")
		}

		// Secondary strategy must have CapSecondary capability
		if !strategyConfig.Capabilities().Has(strategies.CapSecondary) {
			return fmt.Errorf("strategy '%s' doesn't have secondary capability", strategyConfig.ID().String())
		}

		config.SecondaryConfig = strategyConfig
		return nil
	}
}

// WithBaseKey sets the base key for rate limiting
func WithBaseKey(key string) Option {
	return func(config *Config) error {
		config.BaseKey = key
		return nil
	}
}

// AccessOptions holds the configuration for a rate limiter access operation
type AccessOptions struct {
	Key            string                        // Dynamic key
	SkipValidation bool                          // Skip key validation
	Result         *map[string]strategies.Result // Optional results pointer
}

// WithBackend configures the rate limiter to use a custom backend
func WithBackend(backend backends.Backend) Option {
	return func(config *Config) error {
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

// WithMaxRetries configures the maximum number of retry attempts for atomic CheckAndSet operations.
// This is used by all strategies that perform optimistic locking (Fixed Window, Token Bucket, Leaky Bucket, GCRA).
//
// Under high concurrency, CheckAndSet operations may need to retry if the state changes between read and write.
// The retry loop uses exponential backoff with delays based on time since last failed attempt, clamped
// between 32ns and 32ms, and checks for context cancellation on each attempt.
//
// Recommended values:
//   - Low contention (< 10 concurrent users): 10-15 retries
//   - Medium contention (10-100 concurrent users): 100-175 retries
//   - High contention (100+ concurrent users): (2 x concurrent users) retries
//
// Note: Context cancellation will stop retries early regardless of this limit.
// If retries is 0 or not set, the default value (300) will be used.
func WithMaxRetries(retries int) Option {
	return func(config *Config) error {
		if retries < 0 {
			return fmt.Errorf("check and set retries cannot be negative, got %d", retries)
		}
		if retries > strategies.MaxRetries {
			return fmt.Errorf(
				"check and set retries cannot exceed %d, got %d",
				strategies.MaxRetries, retries,
			)
		}
		config.maxRetries = retries
		return nil
	}
}
