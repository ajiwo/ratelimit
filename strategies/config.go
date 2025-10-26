package strategies

import (
	"strings"
)

// StrategyID uniquely identifies a strategy implementation
type StrategyID uint8

const (
	StrategyUnknown StrategyID = iota
	StrategyTokenBucket
	StrategyFixedWindow
	StrategyLeakyBucket
	StrategyGCRA
	StrategyComposite
)

// String returns the canonical string representation of the strategy ID
func (id StrategyID) String() string {
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

// StrategyRole defines the role a strategy instance will play
type StrategyRole int

const (
	RolePrimary StrategyRole = iota
	RoleSecondary
)

// StrategyConfig defines the interface for all strategy configurations
type StrategyConfig interface {
	Validate() error
	ID() StrategyID
	Capabilities() CapabilityFlags

	// Role-based methods
	GetRole() StrategyRole
	WithRole(role StrategyRole) StrategyConfig

	// WithKey returns a copy of the config with the provided fully-qualified key applied
	WithKey(key string) StrategyConfig

	// MaxRetries returns the maximum retry attempts for atomic operations
	// Returns 0 if not configured, which means use DefaultCheckAndSetRetries
	MaxRetries() int

	// WithMaxRetries returns a copy of the config with the provided retry limit
	WithMaxRetries(retries int) StrategyConfig
}

// CapabilityFlags defines the capabilities and roles a strategy can fulfill
type CapabilityFlags uint8

const (
	// CapPrimary indicates the strategy can be used as a primary strategy
	CapPrimary CapabilityFlags = 1 << iota
	// CapSecondary indicates the strategy can be used as a secondary (smoother) strategy
	CapSecondary
	// CapQuotas indicates the strategy supports multi-quota configurations
	CapQuotas
)

// Has checks if the flags contain a specific capability
func (flags CapabilityFlags) Has(cap CapabilityFlags) bool {
	return flags&cap != 0
}

// String returns a human-readable representation of capabilities
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
