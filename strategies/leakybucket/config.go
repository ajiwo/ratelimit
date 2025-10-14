package leakybucket

import (
	"fmt"

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
		return fmt.Errorf("leaky bucket capacity must be positive, got %d", c.Capacity)
	}
	if c.LeakRate <= 0 {
		return fmt.Errorf("leaky bucket leak rate must be positive, got %f", c.LeakRate)
	}
	return nil
}

func (c Config) Name() string {
	return "leaky_bucket"
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
