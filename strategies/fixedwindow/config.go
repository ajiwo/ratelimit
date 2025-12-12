package fixedwindow

import (
	"math"
	"time"

	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/fixedwindow/internal"
	"github.com/ajiwo/ratelimit/utils"
)

// Quota represents a rate limit quota with a maximum number of requests per time window.
//
// Fixed window rate limiting divides time into discrete windows of equal duration
// and allows a fixed number of requests within each window. When the window
// ends, the count resets to zero. Each quota maintains its own counter and
// window timestamp, tracked independently from other quotas.
//
// Multi-Quota Support:
//
//	This strategy supports up to 8 named quotas per key, each enforcing
//	a distinct rate limit based on requests per window duration.
type Quota = internal.Quota

// Config implements the Config interface for fixed window rate limiting with multi-quota support.
//
// Config supports up to 8 named quotas per key. Each quota is tracked independently
// with its own counter and window state. Quotas must have unique rate ratios
// (requests per second) to prevent duplicate rate limits.
type Config struct {
	Key        string  // Storage key for the rate limit state
	Quotas     []Quota // Named quotas with their limits and windows (sorted for determinism)
	maxRetries int     // Maximum retry attempts for atomic operations, 0 means use default
}

// GetKey returns the storage key for rate limit state.
//
// This method implements the internal.Config interface used by the fixed window
// algorithm. The key is set by the top-level ratelimit package via WithKey()
// during rate limiter construction and is used for storing rate limit counters
// and window timestamps in the backend.
func (c *Config) GetKey() string {
	return c.Key
}

// GetQuotas returns the configured quotas for rate limiting.
//
// This method implements the internal.Config interface used by the fixed window
// algorithm and provides access to all named quotas that will be tracked
// independently for this key. Each quota maintains its own counter and window
// state.
func (c *Config) GetQuotas() []internal.Quota {
	return c.Quotas
}

// Validate performs comprehensive configuration validation.
//
// Returns an error if any of the following conditions are met:
//   - No quotas are configured (ErrNoQuotas)
//   - More than 8 quotas are configured (ErrTooManyQuotas)
//   - Any quota has a limit <= 0 (NewInvalidQuotaLimitError)
//   - Any quota has a window duration <= 0 (NewInvalidQuotaWindowError)
//   - Any quota name is invalid (utils.ValidateQuotaName)
//   - Multiple quotas have the same rate ratio (NewDuplicateRateRatioError)
//
// Rate ratio validation ensures each quota enforces a distinct rate limit
// by checking that requests per second values are unique (with 1e-9 tolerance
// for floating-point precision).
func (c *Config) Validate() error {
	if len(c.Quotas) == 0 {
		return ErrNoQuotas
	}

	// Validate maximum of 8 quotas per key
	if len(c.Quotas) > internal.MaxQuota {
		return ErrTooManyQuotas
	}

	for _, quota := range c.Quotas {
		// Validate quota name
		if err := utils.ValidateQuotaName(quota.Name); err != nil {
			return err
		}
		if quota.Limit <= 0 {
			return NewInvalidQuotaLimitError(quota.Name, quota.Limit)
		}
		if quota.Window <= 0 {
			return NewInvalidQuotaWindowError(quota.Name, quota.Window)
		}
	}

	// Validate for duplicate rate ratios (requests per second)
	if err := c.validateUniqueRateRatios(); err != nil {
		return err
	}

	return nil
}

// validateUniqueRateRatios ensures each quota has a unique rate ratio.
//
// This method calculates the rate ratio (requests per second) for each quota
// and ensures no two quotas have identical rates. This prevents redundant
// quotas that would enforce the same rate limit with different names.
func (c *Config) validateUniqueRateRatios() error {
	// Map to track rate ratios: normalized requests per second
	rateRatios := make(map[float64]string)

	for _, quota := range c.Quotas {
		// Calculate rate as requests per second
		ratePerSecond := float64(quota.Limit) / quota.Window.Seconds()

		// Check for existing rate ratio (with small tolerance for floating point precision)
		tolerance := 1e-9
		for existingRate, existingName := range rateRatios {
			if math.Abs(ratePerSecond-existingRate) < tolerance {
				return NewDuplicateRateRatioError(quota.Name, existingName, ratePerSecond)
			}
		}

		rateRatios[ratePerSecond] = quota.Name
	}

	return nil
}

// ID returns the unique identifier for the fixed window strategy.
//
// This method implements the Config interface and returns StrategyFixedWindow,
// which is used for logging, debugging, and strategy selection.
func (c *Config) ID() strategies.ID {
	return strategies.StrategyFixedWindow
}

// Capabilities returns the supported capabilities of the fixed window strategy.
//
// This strategy supports:
//   - CapPrimary: Can be used as a primary hard limiter
//   - CapQuotas: Supports multiple named quotas per key
//
// Note: This strategy does NOT support CapSecondary and cannot be used
// as a secondary smoothing strategy in dual-strategy configurations.
func (c *Config) Capabilities() strategies.CapabilityFlags {
	return strategies.CapPrimary | strategies.CapQuotas
}

// GetRole returns the current role of the fixed window strategy.
//
// Fixed window strategy always returns RolePrimary since it only supports
// primary operation and cannot be used as a secondary strategy.
func (c *Config) GetRole() strategies.Role {
	return strategies.RolePrimary
}

// WithRole returns a copy of the config with the specified role applied.
//
// For fixed window strategy, this method ignores the role parameter and
// returns the original config since this strategy only supports primary roles.
// Attempting to use it as a secondary strategy will fail validation.
func (c *Config) WithRole(role strategies.Role) strategies.Config {
	return c // Fixed window strategy only supports primary role
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

// MaxRetries returns the configured maximum retry attempts for atomic operations.
//
// Returns 0 if not configured, which indicates that `strategies.DefaultMaxRetries`
// should be used for retry counts.
func (c *Config) MaxRetries() int {
	return c.maxRetries
}

// configBuilder provides a fluent interface for building multi-quota configurations
type configBuilder struct {
	key    string
	quotas []Quota
}

// NewConfig creates a multi-quota FixedWindowConfig with a builder pattern
func NewConfig() *configBuilder {
	return &configBuilder{
		quotas: make([]Quota, 0),
	}
}

// SetKey sets the key for the configuration
func (b *configBuilder) SetKey(key string) *configBuilder {
	b.key = key
	return b
}

// AddQuota adds a new quota to the configuration
func (b *configBuilder) AddQuota(name string, limit int, window time.Duration) *configBuilder {
	b.quotas = append(b.quotas, Quota{
		Name:   name,
		Limit:  limit,
		Window: window,
	})
	return b
}

// Build creates the FixedWindowConfig from the builder
func (b *configBuilder) Build() *Config {
	return &Config{
		Key:    b.key,
		Quotas: b.quotas,
	}
}

// QuotaConfig represents a quota configuration for the custom multi-quota builder
type QuotaConfig struct {
	Limit  int
	Window time.Duration
}
