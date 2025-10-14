package ratelimit

import (
	"context"
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

// AccessOption defines a functional option for rate limiter access methods
type AccessOption func(*accessOptions) error

// accessOptions holds the configuration for a rate limiter access operation
type accessOptions struct {
	ctx    context.Context
	key    string
	result *map[string]strategies.Result // Optional results pointer
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
func WithResult(results *map[string]strategies.Result) AccessOption {
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
