package strategies

import (
	"context"
	"fmt"

	"github.com/ajiwo/ratelimit/backends"
)

// CompositeConfig represents a dual-strategy configuration
type CompositeConfig struct {
	BaseKey   string
	Primary   StrategyConfig
	Secondary StrategyConfig
}

func (c CompositeConfig) Validate() error {
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
	if !c.Primary.Capabilities().Has(CapPrimary) {
		return fmt.Errorf("primary strategy must support primary capability")
	}
	if !c.Secondary.Capabilities().Has(CapSecondary) {
		return fmt.Errorf("secondary strategy must support secondary capability")
	}

	return nil
}

func (c CompositeConfig) ID() StrategyID {
	return StrategyComposite
}

func (c CompositeConfig) Capabilities() CapabilityFlags {
	return CapPrimary | CapSecondary
}

func (c CompositeConfig) GetRole() StrategyRole {
	return RolePrimary // Composite always acts as primary
}

func (c CompositeConfig) WithRole(role StrategyRole) StrategyConfig {
	// Composite strategies don't change roles
	return c
}

// WithKey applies the provided fully-qualified key to both primary and secondary configs
func (c CompositeConfig) WithKey(key string) StrategyConfig {
	c.Primary = c.Primary.WithKey(key)
	c.Secondary = c.Secondary.WithKey(key)
	return c
}

// MaxRetries returns the retry limit from the primary config
func (c CompositeConfig) MaxRetries() int {
	if c.Primary != nil {
		return c.Primary.MaxRetries()
	}
	return 0
}

// WithMaxRetries applies the retry limit to both primary and secondary configs
func (c CompositeConfig) WithMaxRetries(retries int) StrategyConfig {
	if c.Primary != nil {
		c.Primary = c.Primary.WithMaxRetries(retries)
	}
	if c.Secondary != nil {
		c.Secondary = c.Secondary.WithMaxRetries(retries)
	}
	return c
}

// compositeStrategy implements dual-strategy behavior
type compositeStrategy struct {
	storage   backends.Backend
	primary   Strategy
	secondary Strategy
}

// NewComposite creates a new composite strategy
func NewComposite(storage backends.Backend) Strategy {
	return &compositeStrategy{
		storage: storage,
	}
}

// SetPrimaryStrategy sets the primary strategy instance
func (cs *compositeStrategy) SetPrimaryStrategy(strategy Strategy) {
	cs.primary = strategy
}

// SetSecondaryStrategy sets the secondary strategy instance
func (cs *compositeStrategy) SetSecondaryStrategy(strategy Strategy) {
	cs.secondary = strategy
}

// Allow implements the dual-strategy logic
func (cs *compositeStrategy) Allow(ctx context.Context, config StrategyConfig) (map[string]Result, error) {
	compositeConfig, ok := config.(CompositeConfig)
	if !ok {
		return nil, fmt.Errorf("composite strategy requires CompositeConfig")
	}

	// Ensure strategies are set
	if cs.primary == nil || cs.secondary == nil {
		return nil, fmt.Errorf("composite strategy requires both primary and secondary strategies to be set")
	}

	results := make(map[string]Result)

	// Step 1: Check primary strategy (no consumption yet)
	primaryConfig := cs.createPrimaryConfig(compositeConfig)
	primaryResults, err := cs.primary.Peek(ctx, primaryConfig)
	if err != nil {
		return nil, fmt.Errorf("primary strategy check failed: %w", err)
	}

	// Check if all primary quotas allow the request
	allAllowed := true
	for quotaName, result := range primaryResults {
		results["primary_"+quotaName] = result
		if !result.Allowed {
			allAllowed = false
		}
	}

	// If primary denies, don't check secondary
	if !allAllowed {
		return results, nil
	}

	// Step 2: Check secondary strategy (no consumption yet)
	secondaryConfig := cs.createSecondaryConfig(compositeConfig)
	secondaryResults, err := cs.secondary.Peek(ctx, secondaryConfig)
	if err != nil {
		return nil, fmt.Errorf("secondary strategy check failed: %w", err)
	}

	// Check if all secondary quotas allow the request
	secondaryAllAllowed := true
	for quotaName, result := range secondaryResults {
		results["secondary_"+quotaName] = result
		if !result.Allowed {
			secondaryAllAllowed = false
		}
	}

	// If secondary denies, don't consume from either strategy
	if !secondaryAllAllowed {
		return results, nil
	}

	// Step 3: Both strategies allow, now consume quota from both
	return cs.consumeQuotas(ctx, primaryConfig, secondaryConfig, results)
}

// Peek inspects current state without consuming quota
func (cs *compositeStrategy) Peek(ctx context.Context, config StrategyConfig) (map[string]Result, error) {
	compositeConfig, ok := config.(CompositeConfig)
	if !ok {
		return nil, fmt.Errorf("composite strategy requires CompositeConfig")
	}

	if cs.primary == nil || cs.secondary == nil {
		return nil, fmt.Errorf("composite strategy requires both primary and secondary strategies to be set")
	}

	results := make(map[string]Result)

	// Get primary results
	primaryConfig := cs.createPrimaryConfig(compositeConfig)
	primaryResults, err := cs.primary.Peek(ctx, primaryConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get primary results: %w", err)
	}
	for key, result := range primaryResults {
		results["primary_"+key] = result
	}

	// Get secondary results
	secondaryConfig := cs.createSecondaryConfig(compositeConfig)
	secondaryResults, err := cs.secondary.Peek(ctx, secondaryConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get secondary results: %w", err)
	}
	for key, result := range secondaryResults {
		results["secondary_"+key] = result
	}

	return results, nil
}

// Reset resets both strategies
func (cs *compositeStrategy) Reset(ctx context.Context, config StrategyConfig) error {
	compositeConfig, ok := config.(CompositeConfig)
	if !ok {
		return fmt.Errorf("composite strategy requires CompositeConfig")
	}

	if cs.primary == nil || cs.secondary == nil {
		return fmt.Errorf("composite strategy requires both primary and secondary strategies to be set")
	}

	// Reset primary strategy
	primaryConfig := cs.createPrimaryConfig(compositeConfig)
	if err := cs.primary.Reset(ctx, primaryConfig); err != nil {
		return fmt.Errorf("failed to reset primary strategy: %w", err)
	}

	// Reset secondary strategy
	secondaryConfig := cs.createSecondaryConfig(compositeConfig)
	if err := cs.secondary.Reset(ctx, secondaryConfig); err != nil {
		return fmt.Errorf("failed to reset secondary strategy: %w", err)
	}

	return nil
}

// createPrimaryConfig creates a role-aware config for the primary strategy
func (cs *compositeStrategy) createPrimaryConfig(compositeConfig CompositeConfig) StrategyConfig {
	return compositeConfig.Primary.WithRole(RolePrimary)
}

// createSecondaryConfig creates a role-aware config for the secondary strategy
func (cs *compositeStrategy) createSecondaryConfig(compositeConfig CompositeConfig) StrategyConfig {
	return compositeConfig.Secondary.WithRole(RoleSecondary)
}

// consumeQuotas consumes quota from both primary and secondary strategies
func (cs *compositeStrategy) consumeQuotas(ctx context.Context, primaryConfig, secondaryConfig StrategyConfig, results map[string]Result) (map[string]Result, error) {
	primaryAllowResults, err := cs.primary.Allow(ctx, primaryConfig)
	if err != nil {
		return nil, fmt.Errorf("primary strategy quota consumption failed: %w", err)
	}

	secondaryAllowResults, err := cs.secondary.Allow(ctx, secondaryConfig)
	if err != nil {
		return nil, fmt.Errorf("secondary strategy quota consumption failed: %w", err)
	}

	// Update results with the consumed values from Allow operations
	for quotaName, result := range primaryAllowResults {
		results["primary_"+quotaName] = result
	}
	for quotaName, result := range secondaryAllowResults {
		results["secondary_"+quotaName] = result
	}

	return results, nil
}
