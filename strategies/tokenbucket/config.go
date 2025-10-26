package tokenbucket

import (
	"github.com/ajiwo/ratelimit/strategies"
)

type Config struct {
	Key        string
	BurstSize  int     // Maximum tokens the bucket can hold
	RefillRate float64 // Tokens to add per second
	role       strategies.StrategyRole
	maxRetries int // Maximum retry attempts for atomic operations, 0 means use default
}

func (c Config) Validate() error {
	if c.BurstSize <= 0 {
		return NewInvalidBurstSizeError(c.BurstSize)
	}
	if c.RefillRate <= 0 {
		return NewInvalidRefillRateError(c.RefillRate)
	}
	return nil
}

func (c Config) ID() strategies.StrategyID {
	return strategies.StrategyTokenBucket
}

func (c Config) Capabilities() strategies.CapabilityFlags {
	return strategies.CapPrimary | strategies.CapSecondary
}

func (c Config) GetRole() strategies.StrategyRole {
	return c.role
}

func (c Config) WithRole(role strategies.StrategyRole) strategies.StrategyConfig {
	c.role = role
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

// These 4 methods implement `internal.Config`
func (c Config) GetKey() string {
	return c.Key
}

func (c Config) GetBurstSize() int {
	return c.BurstSize
}

func (c Config) GetRefillRate() float64 {
	return c.RefillRate
}

func (c Config) MaxRetries() int {
	return c.maxRetries
}
