package leakybucket

import (
	"github.com/ajiwo/ratelimit/strategies"
)

// Config implements the Config interface for leaky bucket rate limiting.
//
// Leaky bucket rate limiting treats requests as water flowing into a bucket
// with a hole at the bottom. The bucket has a maximum capacity, and requests
// leak out at a constant rate. If the bucket overflows, requests are rejected.
type Config struct {
	Key        string          // Storage key for the leaky bucket state
	Burst      int             // Maximum requests the bucket can hold
	Rate       float64         // Requests to process per second (output rate)
	role       strategies.Role // Strategy role (primary or secondary)
	maxRetries int             // Maximum retry attempts for atomic operations, 0 means use default
}

// Validate performs configuration validation for the leaky bucket.
//
// Returns an error if any of the following conditions are met:
//   - Burst <= 0 (NewInvalidBurstError)
//   - Rate <= 0 (NewInvalidRateError)
//
// Note: The Key field is not validated here as it may be set later
// using WithKey() for dynamic key assignment.
func (c *Config) Validate() error {
	if c.Burst <= 0 {
		return NewInvalidBurstError(c.Burst)
	}
	if c.Rate <= 0 {
		return NewInvalidRateError(c.Rate)
	}
	return nil
}

// ID returns the unique identifier for the leaky bucket strategy.
//
// This method implements the Config interface and returns StrategyLeakyBucket,
// which is used for logging, debugging, and strategy selection.
func (c *Config) ID() strategies.ID {
	return strategies.StrategyLeakyBucket
}

// Capabilities returns the supported capabilities of the leaky bucket strategy.
//
// This strategy supports primary and secondary roles but does not support
// multi-quota configurations.
func (c *Config) Capabilities() strategies.CapabilityFlags {
	return strategies.CapPrimary | strategies.CapSecondary
}

// GetRole returns the current role of the leaky bucket strategy.
//
// The role determines whether this strategy acts as a primary limiter
// or secondary smoothing strategy in dual-strategy configurations.
func (c *Config) GetRole() strategies.Role {
	return c.role
}

// WithRole returns a copy of the config with the specified role applied.
//
// This method allows the same leaky bucket configuration to be used
// in different roles (primary or secondary) without modifying the original.
func (c *Config) WithRole(role strategies.Role) strategies.Config {
	cfg := *c
	cfg.role = role
	return &cfg
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
	cfg.maxRetries = retries
	return &cfg
}

// GetKey returns the storage key for the leaky bucket state.
//
// This method implements the internal.Config interface used by the leaky bucket
// algorithm. The key is set by the top-level ratelimit package via WithKey()
// during rate limiter construction and is used for storing bucket level and
// last leak timestamp in the backend.
func (c *Config) GetKey() string {
	return c.Key
}

// GetBurst returns the maximum number of requests the bucket can hold.
//
// This method implements the internal.Config interface used by the leaky bucket
// algorithm and defines the maximum burst capacity for absorbing temporary
// request spikes.
func (c *Config) GetBurst() int {
	return c.Burst
}

// GetRate returns the rate at which requests leak out of the bucket.
//
// This method implements the internal.Config interface used by the leaky bucket
// algorithm and defines the sustained output rate in requests per second for
// steady-state traffic processing.
func (c *Config) GetRate() float64 {
	return c.Rate
}

// MaxRetries returns the configured maximum retry attempts for atomic operations.
//
// Returns 0 if not configured, which indicates that `strategies.DefaultMaxRetries`
// should be used for retry counts.
func (c *Config) MaxRetries() int {
	return c.maxRetries
}
