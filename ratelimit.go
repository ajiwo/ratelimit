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
	// Secondary strategy for smoothing/shaping (optional)
	secondaryStrategy strategies.Strategy
}

// newMultiTierLimiter creates a new multi-tier rate limiter
func newMultiTierLimiter(config MultiTierConfig) (*MultiTierLimiter, error) {
	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, err
	}

	limiter := &MultiTierLimiter{
		config:     config,
		strategies: make(map[string]strategies.Strategy),
	}

	// Setup primary strategies
	if err := limiter.setupPrimaryStrategies(config); err != nil {
		return nil, err
	}

	// Setup secondary strategy if configured
	if err := limiter.setupSecondaryStrategy(config); err != nil {
		return nil, err
	}

	return limiter, nil
}

// setupPrimaryStrategies creates the primary strategy(ies) based on the primary configuration
func (m *MultiTierLimiter) setupPrimaryStrategies(config MultiTierConfig) error {
	primaryConfig := config.PrimaryConfig
	primaryStrategyType := primaryConfig.Type()

	switch primaryStrategyType {
	case StrategyTokenBucket, StrategyLeakyBucket:
		strategy, err := createStrategy(primaryStrategyType, config.Storage)
		if err != nil {
			return fmt.Errorf("failed to create %s strategy: %w", primaryStrategyType, err)
		}
		m.strategies["default"] = strategy

	case StrategyFixedWindow:
		return m.setupFixedWindowStrategies(config, primaryStrategyType)

	default:
		return fmt.Errorf("unsupported primary strategy type: %s", primaryStrategyType)
	}

	return nil
}

// setupFixedWindowStrategies creates strategies for each tier in fixed window configuration
func (m *MultiTierLimiter) setupFixedWindowStrategies(config MultiTierConfig, primaryStrategyType StrategyType) error {
	fixedWindowConfig, ok := config.PrimaryConfig.(FixedWindowConfig)
	if !ok {
		return fmt.Errorf("invalid configuration type for fixed window strategy")
	}

	for _, tier := range fixedWindowConfig.Tiers {
		tierName := getTierName(tier.Interval)

		strategy, err := createStrategy(primaryStrategyType, config.Storage)
		if err != nil {
			return fmt.Errorf("failed to create strategy for tier %s: %w", tierName, err)
		}

		m.strategies[tierName] = strategy
	}

	return nil
}

// setupSecondaryStrategy creates the secondary strategy if configured
func (m *MultiTierLimiter) setupSecondaryStrategy(config MultiTierConfig) error {
	if config.SecondaryConfig == nil {
		return nil
	}

	secondaryStrategyType := config.SecondaryConfig.Type()
	secondaryStrategy, err := createStrategy(secondaryStrategyType, config.Storage)
	if err != nil {
		return fmt.Errorf("failed to create secondary strategy: %w", err)
	}
	m.secondaryStrategy = secondaryStrategy

	return nil
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
	// Parse access options to check if results are requested
	accessOpts, err := m.parseAccessOptions(opts)
	if err != nil {
		return false, fmt.Errorf("failed to parse access options: %w", err)
	}

	// Check if results are requested
	if accessOpts.result != nil {
		// Use allowWithResult and populate the results map
		allowed, results, err := m.allowWithResult(opts...)
		if err != nil {
			return false, err
		}
		*accessOpts.result = results
		return allowed, nil
	}

	// No results requested, use simple version
	allowed, _, err := m.allowWithResult(opts...)
	return allowed, err
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

	primaryConfig := m.config.PrimaryConfig
	switch primaryConfig.Type() {
	case StrategyFixedWindow:
		return strategies.FixedWindowConfig{
			RateLimitConfig: strategies.RateLimitConfig{
				Key:   key,
				Limit: limit,
			},
			Window: interval,
		}, nil

	case StrategyTokenBucket:
		tokenConfig, ok := primaryConfig.(TokenBucketConfig)
		if !ok {
			return nil, fmt.Errorf("invalid token bucket configuration")
		}
		return strategies.TokenBucketConfig{
			RateLimitConfig: strategies.RateLimitConfig{
				Key:   key,
				Limit: limit,
			},
			BurstSize:  tokenConfig.BurstSize,
			RefillRate: tokenConfig.RefillRate,
		}, nil

	case StrategyLeakyBucket:
		leakyConfig, ok := primaryConfig.(LeakyBucketConfig)
		if !ok {
			return nil, fmt.Errorf("invalid leaky bucket configuration")
		}
		return strategies.LeakyBucketConfig{
			RateLimitConfig: strategies.RateLimitConfig{
				Key:   key,
				Limit: limit,
			},
			Capacity: leakyConfig.Capacity,
			LeakRate: leakyConfig.LeakRate,
		}, nil

	default:
		return nil, fmt.Errorf("unknown strategy: %s", primaryConfig.Type())
	}
}

// createBucketConfig creates strategy-specific configuration for bucket strategies
func (m *MultiTierLimiter) createBucketConfig(dynamicKey string) (any, error) {
	var keyBuilder strings.Builder
	keyBuilder.Grow(len(m.config.BaseKey) + len(dynamicKey) + 1) // +1 for the colon
	keyBuilder.WriteString(m.config.BaseKey)
	keyBuilder.WriteByte(':')
	keyBuilder.WriteString(dynamicKey)
	key := keyBuilder.String()

	primaryConfig := m.config.PrimaryConfig
	switch primaryConfig.Type() {
	case StrategyTokenBucket:
		tokenConfig, ok := primaryConfig.(TokenBucketConfig)
		if !ok {
			return nil, fmt.Errorf("invalid token bucket configuration")
		}
		return strategies.TokenBucketConfig{
			RateLimitConfig: strategies.RateLimitConfig{
				Key:   key,
				Limit: int(tokenConfig.RefillRate * 3600), // Convert to hourly limit for display purposes
			},
			BurstSize:  tokenConfig.BurstSize,
			RefillRate: tokenConfig.RefillRate,
		}, nil

	case StrategyLeakyBucket:
		leakyConfig, ok := primaryConfig.(LeakyBucketConfig)
		if !ok {
			return nil, fmt.Errorf("invalid leaky bucket configuration")
		}
		return strategies.LeakyBucketConfig{
			RateLimitConfig: strategies.RateLimitConfig{
				Key:   key,
				Limit: int(leakyConfig.LeakRate * 3600), // Convert to hourly limit for display purposes
			},
			Capacity: leakyConfig.Capacity,
			LeakRate: leakyConfig.LeakRate,
		}, nil

	default:
		return nil, fmt.Errorf("unknown bucket strategy: %s", primaryConfig.Type())
	}
}

// createSecondaryBucketConfig creates strategy-specific configuration for secondary bucket strategies
func (m *MultiTierLimiter) createSecondaryBucketConfig(dynamicKey string) (any, error) {
	if m.config.SecondaryConfig == nil {
		return nil, fmt.Errorf("secondary strategy not configured")
	}

	var keyBuilder strings.Builder
	keyBuilder.Grow(len(m.config.BaseKey) + len(dynamicKey) + 10) // +10 for ":smoother" suffix
	keyBuilder.WriteString(m.config.BaseKey)
	keyBuilder.WriteByte(':')
	keyBuilder.WriteString(dynamicKey)
	keyBuilder.WriteString(":smoother")
	key := keyBuilder.String()

	secondaryConfig := m.config.SecondaryConfig
	switch secondaryConfig.Type() {
	case StrategyTokenBucket:
		tokenConfig, ok := secondaryConfig.(TokenBucketConfig)
		if !ok {
			return nil, fmt.Errorf("invalid secondary token bucket configuration")
		}
		return strategies.TokenBucketConfig{
			RateLimitConfig: strategies.RateLimitConfig{
				Key:   key,
				Limit: int(tokenConfig.RefillRate * 3600), // Convert to hourly limit for display purposes
			},
			BurstSize:  tokenConfig.BurstSize,
			RefillRate: tokenConfig.RefillRate,
		}, nil

	case StrategyLeakyBucket:
		leakyConfig, ok := secondaryConfig.(LeakyBucketConfig)
		if !ok {
			return nil, fmt.Errorf("invalid secondary leaky bucket configuration")
		}
		return strategies.LeakyBucketConfig{
			RateLimitConfig: strategies.RateLimitConfig{
				Key:   key,
				Limit: int(leakyConfig.LeakRate * 3600), // Convert to hourly limit for display purposes
			},
			Capacity: leakyConfig.Capacity,
			LeakRate: leakyConfig.LeakRate,
		}, nil

	default:
		return nil, fmt.Errorf("unknown secondary strategy: %s", secondaryConfig.Type())
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

	// Only fixed window strategy has tiers
	if m.config.PrimaryConfig.Type() == StrategyFixedWindow {
		fixedWindowConfig, ok := m.config.PrimaryConfig.(FixedWindowConfig)
		if !ok {
			return nil, fmt.Errorf("invalid fixed window configuration")
		}

		for _, tier := range fixedWindowConfig.Tiers {
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
	} else {
		// For bucket strategies, create a single "default" stat
		config, err := m.createBucketConfig(accessOpts.key)
		if err != nil {
			return nil, fmt.Errorf("failed to create bucket config: %w", err)
		}

		strategy := m.strategies["default"]
		result, err := strategy.GetResult(accessOpts.ctx, config)
		if err != nil {
			return nil, fmt.Errorf("failed to get stats: %w", err)
		}

		var total int
		switch m.config.PrimaryConfig.Type() {
		case StrategyTokenBucket:
			tokenConfig := m.config.PrimaryConfig.(TokenBucketConfig)
			total = tokenConfig.BurstSize
		case StrategyLeakyBucket:
			leakyConfig := m.config.PrimaryConfig.(LeakyBucketConfig)
			total = leakyConfig.Capacity
		}

		stats["default"] = TierResult{
			Allowed:   result.Allowed,
			Remaining: result.Remaining,
			Reset:     result.Reset,
			Total:     total,
			Used:      total - result.Remaining,
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

	// Only fixed window strategy has tiers
	if m.config.PrimaryConfig.Type() == StrategyFixedWindow {
		fixedWindowConfig, ok := m.config.PrimaryConfig.(FixedWindowConfig)
		if !ok {
			return fmt.Errorf("invalid fixed window configuration")
		}

		for _, tier := range fixedWindowConfig.Tiers {
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
	} else {
		// For bucket strategies, reset the single strategy
		config, err := m.createBucketConfig(accessOpts.key)
		if err != nil {
			return fmt.Errorf("failed to create bucket config: %w", err)
		}

		strategy := m.strategies["default"]
		if err := strategy.Reset(accessOpts.ctx, config); err != nil {
			return fmt.Errorf("failed to reset bucket strategy: %w", err)
		}
	}

	return nil
}

// Close cleans up resources used by the rate limiter
func (m *MultiTierLimiter) Close() error {
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
		BaseKey: "default",
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

// allowWithResult checks if a request is allowed across all configured tiers and returns detailed results
func (m *MultiTierLimiter) allowWithResult(opts ...AccessOption) (bool, map[string]TierResult, error) {
	// Parse access options
	accessOpts, err := m.parseAccessOptions(opts)
	if err != nil {
		return false, nil, fmt.Errorf("failed to parse access options: %w", err)
	}

	results := make(map[string]TierResult)

	// Handle single bucket strategy (primary only, no secondary)
	primaryType := m.config.PrimaryConfig.Type()
	if (primaryType == StrategyTokenBucket || primaryType == StrategyLeakyBucket) && m.config.SecondaryConfig == nil {
		return m.handleSingleBucketStrategy(accessOpts, results)
	}

	// Handle dual strategy: primary (tier-based) + secondary (bucket)
	if m.config.SecondaryConfig != nil {
		return m.handleDualStrategy(accessOpts, results)
	}

	// Handle single tier-based strategy (original behavior)
	return m.handleSingleTierStrategy(accessOpts, results)
}

// handleSingleBucketStrategy handles the case where only a bucket strategy is used as primary
func (m *MultiTierLimiter) handleSingleBucketStrategy(accessOpts *accessOptions, results map[string]TierResult) (bool, map[string]TierResult, error) {
	// Create strategy-specific config for bucket strategy
	config, err := m.createBucketConfig(accessOpts.key)
	if err != nil {
		return false, nil, fmt.Errorf("failed to create config for bucket strategy: %w", err)
	}

	// Check if request is allowed using the bucket strategy
	strategy := m.strategies["default"]
	tierResult, err := strategy.AllowWithResult(accessOpts.ctx, config)
	if err != nil {
		return false, nil, fmt.Errorf("bucket strategy check failed: %w", err)
	}

	// Calculate Total and Used based on the strategy type
	var total, used int
	primaryConfig := m.config.PrimaryConfig
	switch primaryConfig.Type() {
	case StrategyTokenBucket:
		tokenConfig := primaryConfig.(TokenBucketConfig)
		total = tokenConfig.BurstSize
		used = total - tierResult.Remaining
	case StrategyLeakyBucket:
		leakyConfig := primaryConfig.(LeakyBucketConfig)
		total = leakyConfig.Capacity
		used = total - tierResult.Remaining
	}

	results["default"] = TierResult{
		Allowed:   tierResult.Allowed,
		Remaining: tierResult.Remaining,
		Reset:     tierResult.Reset,
		Total:     total,
		Used:      used,
	}

	return tierResult.Allowed, results, nil
}

// handleDualStrategy handles the case where we have a primary tier-based strategy + secondary bucket strategy
func (m *MultiTierLimiter) handleDualStrategy(accessOpts *accessOptions, results map[string]TierResult) (bool, map[string]TierResult, error) {
	// First check primary strategy using CheckOnly (no quota consumption)
	primaryAllowed := true

	fixedWindowConfig, ok := m.config.PrimaryConfig.(FixedWindowConfig)
	if !ok {
		return false, nil, fmt.Errorf("dual strategy only supports FixedWindow as primary strategy")
	}

	for _, tier := range fixedWindowConfig.Tiers {
		tierName := getTierName(tier.Interval)

		// Create FixedWindowConfig for this tier
		config, err := m.createTierConfig(accessOpts.key, tierName, tier.Limit, tier.Interval)
		if err != nil {
			return false, nil, fmt.Errorf("failed to create config for tier %s: %w", tierName, err)
		}

		fixedWindowConfig, ok := config.(strategies.FixedWindowConfig)
		if !ok {
			return false, nil, fmt.Errorf("dual strategy only supports FixedWindow as primary strategy")
		}

		// Type assert to FixedWindowStrategy and use CheckOnly
		fixedWindowStrategy, ok := m.strategies[tierName].(*strategies.FixedWindowStrategy)
		if !ok {
			return false, nil, fmt.Errorf("expected FixedWindowStrategy for tier %s", tierName)
		}

		tierResult, err := fixedWindowStrategy.CheckOnly(accessOpts.ctx, fixedWindowConfig)
		if err != nil {
			return false, nil, fmt.Errorf("tier %s check failed: %w", tierName, err)
		}

		results[tierName] = TierResult{
			Allowed:   tierResult.Allowed,
			Remaining: tierResult.Remaining,
			Reset:     tierResult.Reset,
			Total:     tier.Limit,
			Used:      tier.Limit - tierResult.Remaining,
		}

		if !tierResult.Allowed {
			primaryAllowed = false
		}
	}

	// If primary strategy doesn't allow, don't check secondary
	if !primaryAllowed {
		return false, results, nil
	}

	// Primary allowed, now check secondary strategy (smoother) using GetResult
	secondaryConfig, err := m.createSecondaryBucketConfig(accessOpts.key)
	if err != nil {
		return false, nil, fmt.Errorf("failed to create config for secondary strategy: %w", err)
	}

	secondaryResult, err := m.secondaryStrategy.GetResult(accessOpts.ctx, secondaryConfig)
	if err != nil {
		return false, nil, fmt.Errorf("secondary strategy check failed: %w", err)
	}

	// Calculate Total and Used for secondary strategy
	var total, used int
	switch m.config.SecondaryConfig.Type() {
	case StrategyTokenBucket:
		tokenConfig := m.config.SecondaryConfig.(TokenBucketConfig)
		total = tokenConfig.BurstSize
		used = total - secondaryResult.Remaining
	case StrategyLeakyBucket:
		leakyConfig := m.config.SecondaryConfig.(LeakyBucketConfig)
		total = leakyConfig.Capacity
		used = total - secondaryResult.Remaining
	}

	results["smoother"] = TierResult{
		Allowed:   secondaryResult.Allowed,
		Remaining: secondaryResult.Remaining,
		Reset:     secondaryResult.Reset,
		Total:     total,
		Used:      used,
	}

	// If secondary strategy doesn't allow, don't consume from either strategy
	if !secondaryResult.Allowed {
		return false, results, nil
	}

	// Both strategies allow, now consume quota from both
	return m.consumeFromBothStrategies(accessOpts, secondaryConfig, results)
}

// consumeFromBothStrategies consumes quota from both primary and secondary strategies
func (m *MultiTierLimiter) consumeFromBothStrategies(accessOpts *accessOptions, secondaryConfig any, results map[string]TierResult) (bool, map[string]TierResult, error) {
	// Consume from primary strategy
	fixedWindowConfig := m.config.PrimaryConfig.(FixedWindowConfig)
	for _, tier := range fixedWindowConfig.Tiers {
		tierName := getTierName(tier.Interval)
		config, err := m.createTierConfig(accessOpts.key, tierName, tier.Limit, tier.Interval)
		if err != nil {
			return false, nil, fmt.Errorf("failed to create config for tier %s: %w", tierName, err)
		}

		strategy := m.strategies[tierName]
		_, err = strategy.AllowWithResult(accessOpts.ctx, config)
		if err != nil {
			return false, nil, fmt.Errorf("tier %s quota consumption failed: %w", tierName, err)
		}
	}

	// Consume from secondary strategy
	_, err := m.secondaryStrategy.AllowWithResult(accessOpts.ctx, secondaryConfig)
	if err != nil {
		return false, nil, fmt.Errorf("secondary strategy quota consumption failed: %w", err)
	}

	return true, results, nil
}

// handleSingleTierStrategy handles the case where only a single tier-based strategy is used
func (m *MultiTierLimiter) handleSingleTierStrategy(accessOpts *accessOptions, results map[string]TierResult) (bool, map[string]TierResult, error) {
	deniedTiers := []string{}
	fixedWindowConfig := m.config.PrimaryConfig.(FixedWindowConfig)
	for _, tier := range fixedWindowConfig.Tiers {
		tierName := getTierName(tier.Interval)

		// Create strategy-specific config for this tier
		config, err := m.createTierConfig(accessOpts.key, tierName, tier.Limit, tier.Interval)
		if err != nil {
			return false, nil, fmt.Errorf("failed to create config for tier %s: %w", tierName, err)
		}

		// Check if request is allowed for this tier
		strategy := m.strategies[tierName]
		tierResult, err := strategy.AllowWithResult(accessOpts.ctx, config)
		if err != nil {
			return false, nil, fmt.Errorf("tier %s check failed: %w", tierName, err)
		}

		results[tierName] = TierResult{
			Allowed:   tierResult.Allowed,
			Remaining: tierResult.Remaining,
			Reset:     tierResult.Reset,
			Total:     tier.Limit,
			Used:      tier.Limit - tierResult.Remaining,
		}

		if !tierResult.Allowed {
			deniedTiers = append(deniedTiers, tierName)
		}
	}

	// Request is only allowed if ALL tiers allow it
	allowed := len(deniedTiers) == 0
	return allowed, results, nil
}
