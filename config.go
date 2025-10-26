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
func validateKey(key, keyType string) error {
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
	maxRetries      int
}

// Validate validates the entire configuration
func (c Config) Validate() error {
	if err := validateKey(c.BaseKey, "base key"); err != nil {
		return err
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

		// Secondary strategy must have CapSecondary capability (for smoothing)
		if !c.SecondaryConfig.Capabilities().Has(strategies.CapSecondary) {
			return fmt.Errorf("secondary strategy must support secondary capability, got %s", c.SecondaryConfig.ID().String())
		}

		// Primary strategy cannot have CapSecondary if secondary is also specified
		if c.PrimaryConfig.Capabilities().Has(strategies.CapSecondary) {
			return fmt.Errorf("cannot use strategy with secondary capability as primary when secondary strategy is also specified")
		}
	}

	return nil
}
