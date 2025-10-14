package fixedwindow

import (
	"fmt"
	"time"

	"github.com/ajiwo/ratelimit/strategies"
)

type Quota struct {
	Limit  int
	Window time.Duration
}

type Config struct {
	Key    string
	Quotas map[string]Quota
	role   strategies.StrategyRole
}

func (c Config) Validate() error {
	if c.Key == "" {
		return fmt.Errorf("fixed window key cannot be empty")
	}
	if len(c.Quotas) == 0 {
		return fmt.Errorf("fixed window must have at least one quota")
	}
	for name, quota := range c.Quotas {
		if quota.Limit <= 0 {
			return fmt.Errorf("fixed window quota '%s' limit must be positive, got %d", name, quota.Limit)
		}
		if quota.Window <= 0 {
			return fmt.Errorf("fixed window quota '%s' window must be positive, got %v", name, quota.Window)
		}
	}
	return nil
}

func (c Config) Name() string {
	return "fixed_window"
}

func (c Config) Capabilities() strategies.CapabilityFlags {
	return strategies.CapPrimary | strategies.CapQuotas
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

// configBuilder provides a fluent interface for building multi-quota configurations
type configBuilder struct {
	key    string
	quotas map[string]Quota
}

// NewConfig creates a multi-quota FixedWindowConfig with a builder pattern
func NewConfig(key string) *configBuilder {
	return &configBuilder{
		key:    key,
		quotas: make(map[string]Quota),
	}
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
