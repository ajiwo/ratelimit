package tokenbucket

import (
	"github.com/ajiwo/ratelimit/strategies"
)

// Config implements the Config interface for token bucket rate limiting.
//
// Token bucket rate limiting uses a bucket that holds tokens representing
// allowed requests. Tokens are added to the bucket at a constant rate,
// up to a maximum burst size. Each request consumes one token.
type Config struct {
	Key        string          // Storage key for the token bucket state
	Burst      int             // Maximum tokens the bucket can hold
	Rate       float64         // Tokens to add per second (rate limit)
	role       strategies.Role // Strategy role (primary or secondary)
	MaxRetries int             // Maximum retry attempts for atomic operations, 0 means use default
}

// Validate performs configuration validation for the token bucket.
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

// ID returns the unique identifier for the token bucket strategy.
//
// This method implements the Config interface and returns StrategyTokenBucket,
// which is used for logging, debugging, and strategy selection.
func (c *Config) ID() strategies.ID {
	return strategies.StrategyTokenBucket
}

// Capabilities returns the supported capabilities of the token bucket strategy.
//
// This strategy supports primary and secondary roles but does not support
// multi-quota configurations.
func (c *Config) Capabilities() strategies.CapabilityFlags {
	return strategies.CapPrimary | strategies.CapSecondary
}

// GetRole returns the current role of the token bucket strategy.
//
// The role determines whether this strategy acts as a primary limiter
// or secondary smoothing strategy in dual-strategy configurations.
func (c *Config) GetRole() strategies.Role {
	return c.role
}

// WithRole returns a copy of the config with the specified role applied.
//
// This method allows the same token bucket configuration to be used
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
	cfg.MaxRetries = retries
	return &cfg
}

// GetKey returns the storage key for the token bucket state.
//
// This method implements the internal.Config interface used by the token bucket
// algorithm. The key is set by the top-level ratelimit package via WithKey()
// during rate limiter construction and is used for storing token count and
// last refill timestamp in the backend.
func (c *Config) GetKey() string {
	return c.Key
}

// GetBurst returns the maximum number of tokens the bucket can hold.
//
// This method implements the internal.Config interface used by the token bucket
// algorithm and defines the burst capacity of the token bucket for managing
// concurrent requests.
func (c *Config) GetBurst() int {
	return c.Burst
}

// GetRate returns the rate at which tokens are added to the bucket.
//
// This method implements the internal.Config interface used by the token bucket
// algorithm and defines the sustained rate limit in tokens per second for
// long-term request processing.
func (c *Config) GetRate() float64 {
	return c.Rate
}

// GetMaxRetries returns the configured maximum retry attempts for atomic operations.
//
// When MaxRetries is 0 (default), returns the Burst + 1 value as the optimal retry count
// for token bucket operations. When MaxRetries > 0, returns the explicitly configured value.
func (c *Config) GetMaxRetries() int {
	if c.MaxRetries > 0 {
		return c.MaxRetries
	}
	return c.Burst + 1
}
