package ratelimit

import "fmt"

// validateMultiTierConfig validates the multi-tier configuration
func validateMultiTierConfig(config MultiTierConfig) error {
	if err := validateBasicConfig(config); err != nil {
		return err
	}
	if err := validateTiers(config.Tiers); err != nil {
		return err
	}
	if err := validateStrategyConfig(config); err != nil {
		return err
	}
	return nil
}

// validateBasicConfig validates basic configuration fields
func validateBasicConfig(config MultiTierConfig) error {
	if config.BaseKey == "" {
		return fmt.Errorf("base key cannot be empty")
	}
	if config.Storage == nil {
		return fmt.Errorf("storage backend cannot be nil")
	}
	if len(config.Tiers) == 0 {
		return fmt.Errorf("at least one tier must be configured")
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
	switch config.Strategy {
	case StrategyTokenBucket:
		if config.BurstSize <= 0 {
			return fmt.Errorf("burst size must be positive for token bucket")
		}
		if config.RefillRate <= 0 {
			return fmt.Errorf("refill rate must be positive for token bucket")
		}
	case StrategyLeakyBucket:
		if config.Capacity <= 0 {
			return fmt.Errorf("capacity must be positive for leaky bucket")
		}
		if config.LeakRate <= 0 {
			return fmt.Errorf("leak rate must be positive for leaky bucket")
		}
	}
	return nil
}
