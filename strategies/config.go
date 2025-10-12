package strategies

import (
	"fmt"
	"strings"
	"time"
)

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

// StrategyConfig defines the interface for all strategy configurations
type StrategyConfig interface {
	Validate() error
	Name() string
	Capabilities() CapabilityFlags
}

type FixedWindowConfig struct {
	Key    string
	Limit  int
	Window time.Duration
}

func (c FixedWindowConfig) Validate() error {
	if c.Key == "" {
		return fmt.Errorf("fixed window key cannot be empty")
	}
	if c.Limit <= 0 {
		return fmt.Errorf("fixed window limit must be positive, got %d", c.Limit)
	}
	if c.Window <= 0 {
		return fmt.Errorf("fixed window window must be positive, got %v", c.Window)
	}
	return nil
}

func (c FixedWindowConfig) Name() string {
	return "fixed_window"
}

func (c FixedWindowConfig) Capabilities() CapabilityFlags {
	return CapPrimary | CapTiers
}

type LeakyBucketConfig struct {
	Key      string
	Capacity int     // Maximum requests the bucket can hold
	LeakRate float64 // Requests to process per second
}

func (c LeakyBucketConfig) Validate() error {
	if c.Capacity <= 0 {
		return fmt.Errorf("leaky bucket capacity must be positive, got %d", c.Capacity)
	}
	if c.LeakRate <= 0 {
		return fmt.Errorf("leaky bucket leak rate must be positive, got %f", c.LeakRate)
	}
	return nil
}

func (c LeakyBucketConfig) Name() string {
	return "leaky_bucket"
}

func (c LeakyBucketConfig) Capabilities() CapabilityFlags {
	return CapPrimary | CapSecondary
}

type TokenBucketConfig struct {
	Key        string
	BurstSize  int     // Maximum tokens the bucket can hold
	RefillRate float64 // Tokens to add per second
}

func (c TokenBucketConfig) Validate() error {
	if c.BurstSize <= 0 {
		return fmt.Errorf("token bucket burst size must be positive, got %d", c.BurstSize)
	}
	if c.RefillRate <= 0 {
		return fmt.Errorf("token bucket refill rate must be positive, got %f", c.RefillRate)
	}
	return nil
}

func (c TokenBucketConfig) Name() string {
	return "token_bucket"
}

func (c TokenBucketConfig) Capabilities() CapabilityFlags {
	return CapPrimary | CapSecondary
}
