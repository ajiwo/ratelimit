package composite

import (
	"fmt"

	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/utils/builderpool"
)

// Config represents a dual-strategy configuration
type Config struct {
	BaseKey      string
	Primary      strategies.Config
	Secondary    strategies.Config
	compositeKey string // cached composite key
}

func (c Config) Validate() error {
	if c.BaseKey == "" {
		return fmt.Errorf("composite config base key cannot be empty")
	}
	if c.Primary == nil {
		return fmt.Errorf("composite config primary strategy cannot be nil")
	}
	if c.Secondary == nil {
		return fmt.Errorf("composite config secondary strategy cannot be nil")
	}

	// Validate individual configs
	if err := c.Primary.Validate(); err != nil {
		return fmt.Errorf("primary config validation failed: %w", err)
	}
	if err := c.Secondary.Validate(); err != nil {
		return fmt.Errorf("secondary config validation failed: %w", err)
	}

	// Check capabilities
	if !c.Primary.Capabilities().Has(strategies.CapPrimary) {
		return fmt.Errorf("primary strategy must support primary capability")
	}
	if !c.Secondary.Capabilities().Has(strategies.CapSecondary) {
		return fmt.Errorf("secondary strategy must support secondary capability")
	}

	return nil
}

func (c Config) ID() strategies.ID {
	return strategies.StrategyComposite
}

func (c Config) Capabilities() strategies.CapabilityFlags {
	return strategies.CapPrimary | strategies.CapSecondary
}

func (c Config) GetRole() strategies.Role {
	return strategies.RolePrimary // Composite always acts as primary
}

func (c Config) WithRole(role strategies.Role) strategies.Config {
	// Composite strategies don't change roles
	return c
}

// WithKey applies a new fully-qualified-key to the composite config.
// The new key is then prefixed with BaseKey and suffixed with ":c" for
// composite storage. Primary and secondary configs are not given keys
// since they will use the singleKeyAdapter.
func (c Config) WithKey(key string) strategies.Config {
	sb := builderpool.Get()
	defer func() {
		builderpool.Put(sb)
	}()
	sb.WriteString(c.BaseKey)
	sb.WriteString(":")
	sb.WriteString(key)
	sb.WriteString(":c")
	c.compositeKey = sb.String()

	return c
}

// CompositeKey returns the composite storage key
func (c Config) CompositeKey() string {
	return c.compositeKey
}

// MaxRetries returns the retry limit from the primary config
func (c Config) MaxRetries() int {
	return c.Primary.MaxRetries()
}

// WithMaxRetries applies the retry limit to both primary and secondary configs
func (c Config) WithMaxRetries(retries int) strategies.Config {
	c.Primary = c.Primary.WithMaxRetries(retries)
	c.Secondary = c.Secondary.WithMaxRetries(retries)
	return c
}
