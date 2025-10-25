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
