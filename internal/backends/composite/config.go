package composite

import (
	"fmt"
	"time"
)

// Validate validates the composite backend configuration
func (c *Config) Validate() error {
	if c.Primary == nil {
		return fmt.Errorf("primary backend is required")
	}
	if c.Secondary == nil {
		return fmt.Errorf("secondary backend is required")
	}

	// Validate circuit breaker config
	if c.CircuitBreaker.FailureThreshold <= 0 {
		return fmt.Errorf("circuit breaker failure threshold must be positive")
	}
	if c.CircuitBreaker.RecoveryTimeout <= 0 {
		return fmt.Errorf("circuit breaker recovery timeout must be positive")
	}

	// Validate health check config
	if c.HealthChecker.Interval < 0 {
		return fmt.Errorf("health check interval cannot be negative")
	}
	if c.HealthChecker.Timeout < 0 {
		return fmt.Errorf("health check timeout cannot be negative")
	}
	if c.HealthChecker.Interval > 0 && c.HealthChecker.Timeout >= c.HealthChecker.Interval {
		return fmt.Errorf("health check timeout must be less than interval")
	}

	return nil
}

// SetDefaults sets reasonable defaults for configuration values
func (c *Config) SetDefaults() {
	// Set circuit breaker defaults
	if c.CircuitBreaker.FailureThreshold == 0 {
		c.CircuitBreaker.FailureThreshold = 5
	}
	if c.CircuitBreaker.RecoveryTimeout == 0 {
		c.CircuitBreaker.RecoveryTimeout = 30 * time.Second
	}

	// Set health check defaults
	if c.HealthChecker.Interval == 0 {
		c.HealthChecker.Interval = 10 * time.Second
	}
	if c.HealthChecker.Timeout == 0 {
		c.HealthChecker.Timeout = 2 * time.Second
	}
	if c.HealthChecker.TestKey == "" {
		c.HealthChecker.TestKey = "health-check-key"
	}
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() Config {
	config := Config{}
	config.SetDefaults()
	return config
}
