package fixedwindow

import (
	"math"
	"time"

	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/fixedwindow/internal"
	"github.com/ajiwo/ratelimit/utils"
)

type Quota = internal.Quota

type Config struct {
	Key        string
	Quotas     map[string]Quota
	maxRetries int // Maximum retry attempts for atomic operations, 0 means use default
}

// GetKey implements internal.Config interface
func (c Config) GetKey() string {
	return c.Key
}

// GetQuotas implements internal.Config interface
func (c Config) GetQuotas() map[string]internal.Quota {
	return c.Quotas
}

func (c Config) Validate() error {
	if len(c.Quotas) == 0 {
		return ErrNoQuotas
	}

	// Validate maximum of 8 quotas per key
	if len(c.Quotas) > internal.MaxQuota {
		return ErrTooManyQuotas
	}

	for name, quota := range c.Quotas {
		// Validate quota name
		if err := utils.ValidateQuotaName(name); err != nil {
			return err
		}
		if quota.Limit <= 0 {
			return NewInvalidQuotaLimitError(name, quota.Limit)
		}
		if quota.Window <= 0 {
			return NewInvalidQuotaWindowError(name, quota.Window)
		}
	}

	// Validate for duplicate rate ratios (requests per second)
	if err := c.validateUniqueRateRatios(); err != nil {
		return err
	}

	return nil
}

// validateUniqueRateRatios ensures each quota has a unique rate ratio
func (c Config) validateUniqueRateRatios() error {
	// Map to track rate ratios: normalized requests per second
	rateRatios := make(map[float64]string)

	for name, quota := range c.Quotas {
		// Calculate rate as requests per second
		ratePerSecond := float64(quota.Limit) / quota.Window.Seconds()

		// Check for existing rate ratio (with small tolerance for floating point precision)
		tolerance := 1e-9
		for existingRate, existingName := range rateRatios {
			if math.Abs(ratePerSecond-existingRate) < tolerance {
				return NewDuplicateRateRatioError(name, existingName, ratePerSecond)
			}
		}

		rateRatios[ratePerSecond] = name
	}

	return nil
}

func (c Config) ID() strategies.StrategyID {
	return strategies.StrategyFixedWindow
}

func (c Config) Capabilities() strategies.CapabilityFlags {
	return strategies.CapPrimary | strategies.CapQuotas
}

func (c Config) GetRole() strategies.StrategyRole {
	return strategies.RolePrimary
}

func (c Config) WithRole(role strategies.StrategyRole) strategies.StrategyConfig {
	return c
}

func (c Config) WithKey(key string) strategies.StrategyConfig {
	c.Key = key
	return c
}

func (c Config) WithMaxRetries(retries int) strategies.StrategyConfig {
	c.maxRetries = retries
	return c
}

func (c Config) MaxRetries() int {
	return c.maxRetries
}

// configBuilder provides a fluent interface for building multi-quota configurations
type configBuilder struct {
	key    string
	quotas map[string]Quota
}

// NewConfig creates a multi-quota FixedWindowConfig with a builder pattern
func NewConfig() *configBuilder {
	return &configBuilder{
		quotas: make(map[string]Quota),
	}
}

// SetKey sets the key for the configuration
func (b *configBuilder) SetKey(key string) *configBuilder {
	b.key = key
	return b
}

// AddQuota adds a new quota to the configuration
func (b *configBuilder) AddQuota(name string, limit int, window time.Duration) *configBuilder {
	b.quotas[name] = Quota{
		Limit:  limit,
		Window: window,
	}
	return b
}

// Build creates the FixedWindowConfig from the builder
func (b *configBuilder) Build() Config {
	return Config{
		Key:    b.key,
		Quotas: b.quotas,
	}
}

// QuotaConfig represents a quota configuration for the custom multi-quota builder
type QuotaConfig struct {
	Limit  int
	Window time.Duration
}
