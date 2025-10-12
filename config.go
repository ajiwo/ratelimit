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

// Config defines the configuration for single or dual strategy rate limiting
type Config struct {
	BaseKey         string                    `json:"base_key"`
	Storage         backends.Backend          `json:"-"`
	PrimaryConfig   strategies.StrategyConfig `json:"primary_config"`
	SecondaryConfig strategies.StrategyConfig `json:"secondary_config,omitempty"`
}

// Validate validates the entire configuration
func (c Config) Validate() error {
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
