package fixedwindow

import (
	"fmt"
	"time"

	"github.com/ajiwo/ratelimit/strategies"
)

type Tier struct {
	Limit  int
	Window time.Duration
}

type Config struct {
	Key   string
	Tiers map[string]Tier
	role  strategies.StrategyRole
}

func (c Config) Validate() error {
	if c.Key == "" {
		return fmt.Errorf("fixed window key cannot be empty")
	}
	if len(c.Tiers) == 0 {
		return fmt.Errorf("fixed window must have at least one tier")
	}
	for name, tier := range c.Tiers {
		if tier.Limit <= 0 {
			return fmt.Errorf("fixed window tier '%s' limit must be positive, got %d", name, tier.Limit)
		}
		if tier.Window <= 0 {
			return fmt.Errorf("fixed window tier '%s' window must be positive, got %v", name, tier.Window)
		}
	}
	return nil
}

func (c Config) Name() string {
	return "fixed_window"
}

func (c Config) Capabilities() strategies.CapabilityFlags {
	return strategies.CapPrimary | strategies.CapTiers
}

func (c Config) GetRole() strategies.StrategyRole {
	return c.role
}

func (c Config) WithRole(role strategies.StrategyRole) strategies.StrategyConfig {
	c.role = role
	return c
}

// configBuilder provides a fluent interface for building multi-tier configurations
type configBuilder struct {
	key   string
	tiers map[string]Tier
}

// NewConfig creates a multi-tier FixedWindowConfig with a builder pattern
func NewConfig(key string) *configBuilder {
	return &configBuilder{
		key:   key,
		tiers: make(map[string]Tier),
	}
}

// AddTier adds a new tier to the configuration
func (b *configBuilder) AddTier(name string, limit int, window time.Duration) *configBuilder {
	b.tiers[name] = Tier{
		Limit:  limit,
		Window: window,
	}
	return b
}

// Build creates the FixedWindowConfig from the builder
func (b *configBuilder) Build() Config {
	return Config{
		Key:   b.key,
		Tiers: b.tiers,
	}
}

// TierConfig represents a tier configuration for the custom multi-tier builder
type TierConfig struct {
	Limit  int
	Window time.Duration
}
