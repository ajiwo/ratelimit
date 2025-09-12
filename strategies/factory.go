package strategies

import (
	"fmt"
)

// NewStrategy creates a new strategy instance based on the configuration
func NewStrategy(config StrategyConfig) (Strategy, error) {
	if err := ValidateStrategyConfig(config); err != nil {
		return nil, err
	}

	switch config.Type {
	case "token_bucket":
		return NewTokenBucketStrategy(config)
	case "fixed_window":
		return NewFixedWindowStrategy(config)
	default:
		return nil, fmt.Errorf("unsupported strategy type: %s", config.Type)
	}
}

// ValidateStrategyConfig validates the strategy configuration
func ValidateStrategyConfig(config StrategyConfig) error {
	switch config.Type {
	case "token_bucket":
		if config.RefillRate < 0 {
			return fmt.Errorf("token bucket refill_rate must be non-negative")
		}
		if config.RefillAmount < 0 {
			return fmt.Errorf("token bucket refill_amount must be non-negative")
		}
		return nil
	case "fixed_window":
		if config.WindowDuration <= 0 {
			return fmt.Errorf("fixed window window_duration must be positive")
		}
		return nil
	default:
		return fmt.Errorf("unsupported strategy type: %s", config.Type)
	}
}

// GetSupportedStrategies returns a list of supported strategy types
func GetSupportedStrategies() []string {
	return []string{"token_bucket", "fixed_window"}
}

// GetRecommendedStrategy returns the recommended strategy for given requirements
func GetRecommendedStrategy(requestsPerMinute int64, burstSize int64, useCase string) StrategyConfig {
	switch useCase {
	case "ai", "expensive":
		// AI operations benefit from burst capability
		refillRate, refillAmount := CalculateOptimalRefillRate(requestsPerMinute, burstSize)
		return StrategyConfig{
			Type:         "token_bucket",
			RefillRate:   refillRate,
			RefillAmount: refillAmount,
			BucketSize:   burstSize,
		}
	case "api", "general":
		// General API usage - token bucket with moderate burst
		refillRate, refillAmount := CalculateOptimalRefillRate(requestsPerMinute, burstSize)
		return StrategyConfig{
			Type:         "token_bucket",
			RefillRate:   refillRate,
			RefillAmount: refillAmount,
			BucketSize:   burstSize,
		}
	default:
		// Default to token bucket
		refillRate, refillAmount := CalculateOptimalRefillRate(requestsPerMinute, burstSize)
		return StrategyConfig{
			Type:         "token_bucket",
			RefillRate:   refillRate,
			RefillAmount: refillAmount,
			BucketSize:   burstSize,
		}
	}
}
