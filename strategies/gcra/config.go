package gcra

import (
	"fmt"

	"github.com/ajiwo/ratelimit/strategies"
)

type Config struct {
	Key   string
	Rate  float64 // Requests per second
	Burst int     // Maximum burst size
}

func (c Config) Validate() error {
	if c.Rate <= 0 {
		return fmt.Errorf("gcra rate must be positive, got %f", c.Rate)
	}
	if c.Burst <= 0 {
		return fmt.Errorf("gcra burst must be positive, got %d", c.Burst)
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
