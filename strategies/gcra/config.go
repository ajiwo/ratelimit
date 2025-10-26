package gcra

import (
	"github.com/ajiwo/ratelimit/strategies"
)

type Config struct {
	Key        string
	Rate       float64 // Requests per second
	Burst      int     // Maximum burst size
	maxRetries int     // Maximum retry attempts for atomic operations, 0 means use default
}

func (c Config) Validate() error {
	if c.Rate <= 0 {
		return NewInvalidRateError(c.Rate)
	}
	if c.Burst <= 0 {
		return NewInvalidBurstError(c.Burst)
	}
	return nil
}

func (c Config) ID() strategies.StrategyID {
	return strategies.StrategyGCRA
}

func (c Config) Capabilities() strategies.CapabilityFlags {
	return strategies.CapPrimary
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

// These 4 methods implement `internal.Config`
func (c Config) GetBurst() int {
	return c.Burst
}
func (c Config) GetKey() string {
	return c.Key
}
func (c Config) GetRate() float64 {
	return c.Rate
}
func (c Config) MaxRetries() int {
	return c.maxRetries
}
