package ratelimit

import (
	"fmt"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
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

// FixedWindowConfig implements StrategyConfig for multi-tiers fixed window rate limiting
// used to create `len(Tiers)` instances of `strategies.FixedWindowConfig`
type FixedWindowConfig struct {
	Tiers []TierConfig `json:"tiers"`
}

func (c FixedWindowConfig) Validate() error {
	if len(c.Tiers) == 0 {
		return fmt.Errorf("fixed window strategy requires at least one tier")
	}
	if len(c.Tiers) > MaxTiers {
		return fmt.Errorf("fixed window strategy supports maximum %d tiers, got %d", MaxTiers, len(c.Tiers))
	}

	for i, tier := range c.Tiers {
		if tier.Interval < MinInterval {
			return fmt.Errorf("tier %d: interval %v is below minimum %v", i, tier.Interval, MinInterval)
		}
		if tier.Limit <= 0 {
			return fmt.Errorf("tier %d: limit must be positive, got %d", i, tier.Limit)
		}
		if tier.Name != "" {
			if len(tier.Name) > 16 {
				return fmt.Errorf("tier %d: name must be 16 characters or less, got %d", i, len(tier.Name))
			}
			if err := validateKey(tier.Name, "tier name"); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c FixedWindowConfig) Type() strategies.StrategyType {
	return strategies.StrategyFixedWindow
}

// MultiTierConfig defines the configuration for multi-tier rate limiting
type MultiTierConfig struct {
	BaseKey         string                    `json:"base_key"`
	Storage         backends.Backend          `json:"-"`
	PrimaryConfig   strategies.StrategyConfig `json:"primary_config"`
	SecondaryConfig strategies.StrategyConfig `json:"secondary_config,omitempty"`
}

// Validate validates the entire multi-tier configuration
func (c MultiTierConfig) Validate() error {
	if c.BaseKey == "" {
		return fmt.Errorf("base key cannot be empty")
	}
	if c.Storage == nil {
		return fmt.Errorf("storage backend cannot be nil")
	}
	if c.PrimaryConfig == nil {
		return fmt.Errorf("primary strategy config cannot be nil")
	}

	// Validate primary strategy config
	if err := c.PrimaryConfig.Validate(); err != nil {
		return fmt.Errorf("primary strategy config validation failed: %w", err)
	}

	// Validate secondary strategy config if present
	if c.SecondaryConfig != nil {
		if err := c.SecondaryConfig.Validate(); err != nil {
			return fmt.Errorf("secondary strategy config validation failed: %w", err)
		}

		// Secondary strategy must be a bucket-based strategy (for smoothing)
		if c.SecondaryConfig.Type() != strategies.StrategyTokenBucket && c.SecondaryConfig.Type() != strategies.StrategyLeakyBucket {
			return fmt.Errorf("secondary strategy must be token bucket or leaky bucket, got %s", c.SecondaryConfig.Type())
		}

		// Primary strategy cannot be bucket-based if secondary is also bucket-based
		if c.PrimaryConfig.Type() == strategies.StrategyTokenBucket || c.PrimaryConfig.Type() == strategies.StrategyLeakyBucket {
			return fmt.Errorf("cannot use bucket strategy as primary when secondary strategy is also specified")
		}
	}

	return nil
}
