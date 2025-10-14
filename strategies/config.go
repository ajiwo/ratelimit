package strategies

import (
	"strings"
)

// StrategyConfig defines the interface for all strategy configurations
type StrategyConfig interface {
	Validate() error
	Name() string
	Capabilities() CapabilityFlags
}

// CapabilityFlags defines the capabilities and roles a strategy can fulfill
type CapabilityFlags uint8

const (
	// CapPrimary indicates the strategy can be used as a primary strategy
	CapPrimary CapabilityFlags = 1 << iota
	// CapSecondary indicates the strategy can be used as a secondary (smoother) strategy
	CapSecondary
	// CapTiers indicates the strategy supports multi-tier configurations
	CapTiers
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
	if flags.Has(CapTiers) {
		caps = append(caps, "Tiers")
	}
	if len(caps) == 0 {
		return "None"
	}
	return strings.Join(caps, "|")
}
