package tokenbucket

import (
	"fmt"

	"github.com/ajiwo/ratelimit/strategies"
)

type Config struct {
	Key        string
	BurstSize  int     // Maximum tokens the bucket can hold
	RefillRate float64 // Tokens to add per second
	role       strategies.StrategyRole
}

func (c Config) Validate() error {
	if c.BurstSize <= 0 {
		return fmt.Errorf("token bucket burst size must be positive, got %d", c.BurstSize)
	}
	if c.RefillRate <= 0 {
		return fmt.Errorf("token bucket refill rate must be positive, got %f", c.RefillRate)
	}
	return nil
}

func (c Config) Name() string {
	return "token_bucket"
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
