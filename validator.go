package ratelimit

import (
	"fmt"
)

// allowedCharsArray is a precomputed boolean array for O(1) character validation
var allowedCharsArray [128]bool

func init() {
	// Initialize all characters as not allowed
	for i := range 128 {
		allowedCharsArray[i] = false
	}

	// Set allowed characters to true
	for _, c := range "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-:.@" {
		allowedCharsArray[c] = true
	}
}

// validateKey validates that a key meets the requirements:
// - Maximum 64 bytes length
// - Contains only alphanumeric ASCII characters, underscore (_), hyphen (-), and colon (:)
func validateKey(key string, keyType string) error {
	if len(key) == 0 {
		return fmt.Errorf("%s cannot be empty", keyType)
	}

	if len(key) > 64 {
		return fmt.Errorf("%s cannot exceed 64 bytes, got %d bytes", keyType, len(key))
	}

	const hint = "Only alphanumeric ASCII, underscore (_), hyphen (-), colon (:), period (.), and at (@) are allowed"

	for i, r := range key {
		// Check if character is within ASCII range
		if r >= 128 {
			return fmt.Errorf("%s contains invalid character '%c' at position %d. %s", keyType, r, i, hint)
		}

		// Check if character is allowed
		if !allowedCharsArray[r] {
			return fmt.Errorf("%s contains invalid character '%c' at position %d. %s", keyType, r, i, hint)
		}
	}

	return nil
}

// validateMultiTierConfig validates the multi-tier configuration
func validateMultiTierConfig(config MultiTierConfig) error {
	if err := validateBasicConfig(config); err != nil {
		return err
	}
	if err := validateStrategyConfig(config); err != nil {
		return err
	}
	// Only validate tiers if the strategy uses them
	if len(config.Tiers) > 0 {
		if err := validateTiers(config.Tiers); err != nil {
			return err
		}
	}
	return nil
}

// validateBasicConfig validates basic configuration fields
func validateBasicConfig(config MultiTierConfig) error {
	if err := validateKey(config.BaseKey, "base key"); err != nil {
		return err
	}
	if config.Storage == nil {
		return fmt.Errorf("storage backend cannot be nil")
	}

	// Bucket strategies don't require tiers
	if config.Strategy != StrategyTokenBucket && config.Strategy != StrategyLeakyBucket {
		if len(config.Tiers) == 0 {
			return fmt.Errorf("at least one tier must be configured")
		}
	}

	if len(config.Tiers) > MaxTiers {
		return fmt.Errorf("maximum %d tiers allowed, got %d", MaxTiers, len(config.Tiers))
	}
	return nil
}

// validateTiers validates individual tier configurations
func validateTiers(tiers []TierConfig) error {
	for i, tier := range tiers {
		if tier.Interval < MinInterval {
			return fmt.Errorf("tier %d: interval must be at least %v, got %v", i+1, MinInterval, tier.Interval)
		}
		if tier.Limit <= 0 {
			return fmt.Errorf("tier %d: limit must be positive, got %d", i+1, tier.Limit)
		}
	}
	return nil
}

// validateStrategyConfig validates strategy-specific configuration
func validateStrategyConfig(config MultiTierConfig) error {
	// Validate primary strategy
	primaryCfg := StrategyConfig{
		Strategy:   config.Strategy,
		BurstSize:  config.BurstSize,
		RefillRate: config.RefillRate,
		Capacity:   config.Capacity,
		LeakRate:   config.LeakRate,
		Prefix:     "",
	}
	if err := validateSingleStrategy(primaryCfg); err != nil {
		return err
	}

	// Validate secondary strategy if configured
	if config.SecondaryStrategy != nil {
		if *config.SecondaryStrategy == StrategyFixedWindow {
			return fmt.Errorf("fixed window cannot be used as secondary strategy")
		}
		secondaryCfg := StrategyConfig{
			Strategy:   *config.SecondaryStrategy,
			BurstSize:  config.SecondaryBurstSize,
			RefillRate: config.SecondaryRefillRate,
			Capacity:   config.SecondaryCapacity,
			LeakRate:   config.SecondaryLeakRate,
			Prefix:     "secondary ",
		}
		return validateSingleStrategy(secondaryCfg)
	}

	return nil
}

// validateSingleStrategy validates a single strategy configuration
type StrategyConfig struct {
	Strategy   StrategyType
	BurstSize  int
	RefillRate float64
	Capacity   int
	LeakRate   float64
	Prefix     string
}

func validateSingleStrategy(cfg StrategyConfig) error {
	switch cfg.Strategy {
	case StrategyTokenBucket:
		if cfg.BurstSize <= 0 {
			return fmt.Errorf("%sburst size must be positive for token bucket", cfg.Prefix)
		}
		if cfg.RefillRate <= 0 {
			return fmt.Errorf("%srefill rate must be positive for token bucket", cfg.Prefix)
		}
	case StrategyLeakyBucket:
		if cfg.Capacity <= 0 {
			return fmt.Errorf("%scapacity must be positive for leaky bucket", cfg.Prefix)
		}
		if cfg.LeakRate <= 0 {
			return fmt.Errorf("%sleak rate must be positive for leaky bucket", cfg.Prefix)
		}
	}
	return nil
}
