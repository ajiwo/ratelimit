package strategies

import (
	"strings"
)

// ID uniquely identifies a strategy implementation
type ID uint8

const (
	StrategyUnknown ID = iota
	StrategyTokenBucket
	StrategyFixedWindow
	StrategyLeakyBucket
	StrategyGCRA
	StrategyComposite
)

// String returns the canonical string representation of the strategy ID
func (id ID) String() string {
	switch id {
	case StrategyTokenBucket:
		return "token_bucket"
	case StrategyFixedWindow:
		return "fixed_window"
	case StrategyLeakyBucket:
		return "leaky_bucket"
	case StrategyGCRA:
		return "gcra"
	case StrategyComposite:
		return "composite"
	default:
		return "unknown"
	}
}

// Role defines the role a strategy instance will play in dual-strategy configurations.
type Role int

const (
	// RolePrimary indicates the strategy acts as the primary hard limiter.
	//
	// Primary strategy is evaluated first and enforces strict rate limits.
	RolePrimary Role = iota

	// RoleSecondary indicates the strategy acts as a secondary limiter.
	//
	// Secondary strategies are evaluated after primary limits pass and provide
	// additional smoothing or burst handling capabilities.
	RoleSecondary
)

// Config defines the interface for all strategy configurations
type Config interface {
	// Validate performs configuration validation and returns an error if invalid.
	//
	// Implementations should validate all required parameters and their relationships.
	Validate() error

	// ID returns the unique identifier for the strategy implementation.
	//
	// This is used for logging, debugging, and strategy selection.
	ID() ID

	// Capabilities returns the capabilities and roles this strategy supports.
	//
	// Strategies declare whether they can be used as primary, secondary, or support quotas.
	Capabilities() CapabilityFlags

	// Role-based methods for managing strategy roles in dual-strategy configurations

	// GetRole returns the current role of the strategy instance.
	GetRole() Role

	// WithRole returns a new config with the specified role applied.
	//
	// This enables role-based configuration without modifying the original.
	WithRole(role Role) Config

	// WithKey returns a copy of the config with the provided fully-qualified key applied.
	//
	// The key is used for storage and retrieval of rate limiting state.
	// Implementations should ensure the key is properly formatted for their storage needs.
	WithKey(key string) Config

	// GetMaxRetries returns the maximum retry attempts for `backends.CheckAndSet` operations.
	//
	// Returns 0 if not configured, which indicates that `DefaultMaxRetries` should be used
	// for retry counts.
	GetMaxRetries() int

	// WithMaxRetries returns a copy of the config with the provided retry limit applied.
	//
	// This allows tuning retry behavior for specific use cases or environments.
	WithMaxRetries(retries int) Config
}

// CapabilityFlags defines the capabilities and roles a strategy can fulfill
type CapabilityFlags uint8

const (
	// CapPrimary indicates the strategy can be used as a primary strategy.
	//
	// Primary strategies enforce hard rate limits and are evaluated first in dual-strategy mode.
	// They must be capable of rejecting requests that exceed their configured limits.
	CapPrimary CapabilityFlags = 1 << iota

	// CapSecondary indicates the strategy can be used as a secondary (smoother) strategy.
	//
	// Secondary strategies provide additional smoothing, burst handling, or complementary
	// rate limiting behavior and are evaluated after primary strategies in dual-strategy mode.
	CapSecondary

	// CapQuotas indicates the strategy supports multi-quota configurations.
	//
	// Strategies with this capability can enforce multiple different rate limits on the same key,
	// such as different limits for different API endpoints or user types.
	CapQuotas
)

// Has checks if the flags contain a specific capability.
//
// This method enables capability checking for strategy validation and role assignment.
// Returns true if the specified capability bit is set, false otherwise.
func (flags CapabilityFlags) Has(cap CapabilityFlags) bool {
	return flags&cap != 0
}

// String returns a human-readable representation of capabilities.
//
// This method formats the set capabilities as a pipe-separated string,
// useful for debugging, logging, and configuration validation.
// Returns "None" if no capabilities are set.
func (flags CapabilityFlags) String() string {
	var caps []string
	if flags.Has(CapPrimary) {
		caps = append(caps, "Primary")
	}
	if flags.Has(CapSecondary) {
		caps = append(caps, "Secondary")
	}
	if flags.Has(CapQuotas) {
		caps = append(caps, "Quotas")
	}
	if len(caps) == 0 {
		return "None"
	}
	return strings.Join(caps, "|")
}
