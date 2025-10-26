package ratelimit

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/ajiwo/ratelimit/strategies"
)

type Limiter = RateLimiter

// keyBuilderPool reduces allocations in key construction
var keyBuilderPool = sync.Pool{
	New: func() any {
		return &strings.Builder{}
	},
}

// RateLimiter implements single or dual strategy rate limiting
type RateLimiter struct {
	config     Config
	strategy   strategies.Strategy
	basePrefix string // cached BaseKey + ":" for fast key construction
}

func Ptr[T any](val T) *T {
	return &val
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
func (r *RateLimiter) Allow(ctx context.Context, options AccessOptions) (bool, error) {
	dynamicKey, err := checkDynamicKey(options)
	if err != nil {
		return false, err
	}

	allowed, results, err := r.allowWithResult(ctx, dynamicKey)
	if err != nil {
		return false, err
	}

	// If result map is provided, populate it
	if options.Result != nil {
		*options.Result = results
	}

	return allowed, err
}

// Peek retrieves strategy results without consuming quota and returns an overall allowed boolean
func (r *RateLimiter) Peek(ctx context.Context, options AccessOptions) (bool, error) {
	dynamicKey, err := checkDynamicKey(options)
	if err != nil {
		return false, err
	}

	strategyConfig := r.buildStrategyConfig(dynamicKey)

	// Get stats from the strategy (composite or single)
	results, err := r.strategy.Peek(ctx, strategyConfig)
	if err != nil {
		return false, fmt.Errorf("failed to get stats: %w", err)
	}
	if options.Result != nil {
		*options.Result = results
	}
	// Determine overall allowed similarly to Allow
	allAllowed := true
	for _, res := range results {
		if !res.Allowed {
			allAllowed = false
			break
		}
	}
	return allAllowed, nil
}

// Reset resets the rate limit counters for all strategies (mainly for testing)
func (r *RateLimiter) Reset(ctx context.Context, options AccessOptions) error {
	dynamicKey, err := checkDynamicKey(options)
	if err != nil {
		return err
	}

	strategyConfig := r.buildStrategyConfig(dynamicKey)

	// Reset the strategy (composite or single)
	if err := r.strategy.Reset(ctx, strategyConfig); err != nil {
		return fmt.Errorf("failed to reset strategy: %w", err)
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

// allowWithResult1 checks if a request is allowed and returns detailed results
func (r *RateLimiter) allowWithResult(ctx context.Context, dynamicKey string) (bool, strategies.Results, error) {
	strategyConfig := r.buildStrategyConfig(dynamicKey)

	// Use the strategy (composite or single)
	results, err := r.strategy.Allow(ctx, strategyConfig)
	if err != nil {
		return false, nil, fmt.Errorf("strategy check failed: %w", err)
	}

	// Determine if the request was allowed by checking if all results allow it
	allAllowed := true
	for _, result := range results {
		if !result.Allowed {
			allAllowed = false
			break
		}
	}

	return allAllowed, results, nil
}

// buildStrategyConfig builds the appropriate strategy config (composite or single)
func (r *RateLimiter) buildStrategyConfig(dynamicKey string) strategies.StrategyConfig {
	prefix := r.basePrefix
	if prefix == "" {
		prefix = r.config.BaseKey + ":"
	}

	// Use pooled string builder to reduce allocations
	keyBuilder := keyBuilderPool.Get().(*strings.Builder)
	defer func() {
		keyBuilder.Reset()
		keyBuilderPool.Put(keyBuilder)
	}()

	// a multiply of 8 is faster because it would avoid internal padding/alignment of `strings.Builder`
	// simply use a static value of 64 (max allowed by `validateKey`)
	keyBuilder.Grow(64)
	keyBuilder.WriteString(prefix)
	keyBuilder.WriteString(dynamicKey)
	key1 := keyBuilder.String()

	if r.config.SecondaryConfig == nil {
		config := r.config.PrimaryConfig.WithKey(key1)
		if r.config.maxRetries > 0 {
			config = config.WithMaxRetries(r.config.maxRetries)
		}
		return config
	}

	keyBuilder.WriteString("s")
	key2 := keyBuilder.String()

	primaryConfig := r.config.PrimaryConfig.WithKey(key1)
	secondaryConfig := r.config.SecondaryConfig.WithKey(key2)

	if r.config.maxRetries > 0 {
		primaryConfig = primaryConfig.WithMaxRetries(r.config.maxRetries)
		secondaryConfig = secondaryConfig.WithMaxRetries(r.config.maxRetries)
	}

	return strategies.CompositeConfig{
		BaseKey:   r.config.BaseKey,
		Primary:   primaryConfig,
		Secondary: secondaryConfig,
	}
}

// checkDynamicKey validates (if enabled) and returns the dynamic key
func checkDynamicKey(options AccessOptions) (string, error) {
	if options.Key != "" {
		if !options.SkipValidation {
			if err := validateKey(options.Key, "dynamic key"); err != nil {
				return "", err
			}
		}
		return options.Key, nil
	}
	return "default", nil
}

// newRateLimiter creates a new rate limiter
func newRateLimiter(config Config) (*RateLimiter, error) {
	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, err
	}

	limiter := &RateLimiter{
		config:     config,
		basePrefix: config.BaseKey + ":",
	}

	// Check if we have a dual-strategy configuration
	if config.SecondaryConfig != nil {
		// Use composite strategy for dual-strategy behavior
		compositeStrategy := strategies.NewComposite(config.Storage)
		limiter.strategy = compositeStrategy

		// Create and configure the individual strategies
		primaryStrategy, err := strategies.Create(config.PrimaryConfig.ID(), config.Storage)
		if err != nil {
			return nil, fmt.Errorf("failed to create primary strategy: %w", err)
		}

		secondaryStrategy, err := strategies.Create(config.SecondaryConfig.ID(), config.Storage)
		if err != nil {
			return nil, fmt.Errorf("failed to create secondary strategy: %w", err)
		}

		// Set the individual strategies on the composite
		if comp, ok := compositeStrategy.(interface {
			SetPrimaryStrategy(strategies.Strategy)
			SetSecondaryStrategy(strategies.Strategy)
		}); ok {
			comp.SetPrimaryStrategy(primaryStrategy)
			comp.SetSecondaryStrategy(secondaryStrategy)
		} else {
			return nil, fmt.Errorf("composite strategy type assertion failed")
		}

		return limiter, nil
	}

	// Single strategy case
	primaryStrategyID := config.PrimaryConfig.ID()
	primaryStrategy, err := strategies.Create(primaryStrategyID, config.Storage)
	if err != nil {
		return nil, fmt.Errorf("failed to create primary strategy: %w", err)
	}
	limiter.strategy = primaryStrategy

	return limiter, nil
}
