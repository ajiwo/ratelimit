package strategies

import (
	"fmt"
	"time"
)

// StrategyType defines the rate limiting strategy to use
type StrategyType string

const (
	StrategyFixedWindow StrategyType = "fixed_window"
	StrategyTokenBucket StrategyType = "token_bucket"
	StrategyLeakyBucket StrategyType = "leaky_bucket"
)

// StrategyConfig defines the interface for all strategy configurations
type StrategyConfig interface {
	Validate() error
	Type() StrategyType
}

type FixedWindowConfig struct {
	Key    string
	Limit  int
	Window time.Duration
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

func (c LeakyBucketConfig) Type() StrategyType {
	return StrategyLeakyBucket
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

func (c TokenBucketConfig) Type() StrategyType {
	return StrategyTokenBucket
}
