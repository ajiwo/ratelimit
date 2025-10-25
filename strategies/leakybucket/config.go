package leakybucket

import (
	"github.com/ajiwo/ratelimit/strategies"
)

type Config struct {
	Key      string
	Capacity int     // Maximum requests the bucket can hold
	LeakRate float64 // Requests to process per second
	role     strategies.StrategyRole
}

func (c Config) Validate() error {
	if c.Capacity <= 0 {
		return NewInvalidCapacityError(c.Capacity)
	}
	if c.LeakRate <= 0 {
		return NewInvalidLeakRateError(c.LeakRate)
	}
	return nil
}

func (c Config) ID() strategies.StrategyID {
	return strategies.StrategyLeakyBucket
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
