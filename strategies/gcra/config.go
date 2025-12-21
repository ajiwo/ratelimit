package gcra

import (
	"github.com/ajiwo/ratelimit/strategies"
)

// Config implements the Config interface for Generic Cell Rate Algorithm (GCRA) rate limiting.
//
// GCRA uses a theoretical arrival time (TAT) to track when the next request would be allowed.
// The algorithm updates the TAT based on the configured rate and burst parameters.
type Config struct {
	Key        string  // Storage key for GCRA state (theoretical arrival time)
	Rate       float64 // Requests per second (sustained rate limit)
	Burst      int     // Maximum burst size (concurrent request tolerance)
	MaxRetries int     // Maximum retry attempts for atomic operations, 0 means use default
}

// Validate performs configuration validation for the GCRA strategy.
//
// Returns an error if any of the following conditions are met:
//   - Rate <= 0 (NewInvalidRateError)
//   - Burst <= 0 (NewInvalidBurstError)
//
// Note: The Key field is not validated here as it may be set later
// using WithKey() for dynamic key assignment.
func (c *Config) Validate() error {
	if c.Rate <= 0 {
		return NewInvalidRateError(c.Rate)
	}
	if c.Burst <= 0 {
		return NewInvalidBurstError(c.Burst)
	}
	return nil
}

// ID returns the unique identifier for the GCRA strategy.
//
// This method implements the Config interface and returns StrategyGCRA,
// which is used for logging, debugging, and strategy selection.
func (c *Config) ID() strategies.ID {
	return strategies.StrategyGCRA
}

// Capabilities returns the supported capabilities of the GCRA strategy.
//
// This strategy supports primary and secondary roles but does not support
// multi-quota configurations.
func (c *Config) Capabilities() strategies.CapabilityFlags {
	return strategies.CapPrimary | strategies.CapSecondary
}

// WithKey returns a copy of the config with the provided key applied.
//
// The key is used as-is for storage without modification or prefixing.
// This allows direct control over storage keys for backend compatibility.
func (c *Config) WithKey(key string) strategies.Config {
	cfg := *c
	cfg.Key = key
	return &cfg
}

// WithMaxRetries returns a copy of the config with the provided retry limit applied.
//
// This controls the maximum number of retry attempts for atomic operations
// (CheckAndSet) when storage conflicts occur. Set to 0 to use the default
// retry limit. Higher values may help in high-contention scenarios.
func (c *Config) WithMaxRetries(retries int) strategies.Config {
	cfg := *c
	cfg.MaxRetries = retries
	return &cfg
}

// GetBurst returns the maximum burst size for the GCRA strategy.
//
// This method implements the `internal.Config` interface used by the GCRA
// algorithm and defines the maximum number of requests that can be handled
// in a short burst before the rate limit enforcement begins.
func (c *Config) GetBurst() int {
	return c.Burst
}

// GetKey returns the storage key for the GCRA state.
//
// This method implements the `internal.Config` interface used by the GCRA
// algorithm. The key is set by the top-level ratelimit package via WithKey()
// during rate limiter construction and is used for storing the theoretical
// arrival time (TAT) in the backend.
func (c *Config) GetKey() string {
	return c.Key
}

// GetRate returns the sustained rate limit for the GCRA strategy.
//
// This method implements the `internal.Config` interface used by the GCRA
// algorithm and defines the maximum number of requests per second over the
// long term for rate limiting calculations.
func (c *Config) GetRate() float64 {
	return c.Rate
}

// GetMaxRetries returns the configured maximum retry attempts for atomic operations.
//
// When MaxRetries is 0 (default), returns the Burst + 1 value as the optimal retry count
// for GCRA operations. When MaxRetries > 0, returns the explicitly configured value.
func (c *Config) GetMaxRetries() int {
	if c.MaxRetries > 0 {
		return c.MaxRetries
	}
	return c.Burst + 1
}
