package composite

import (
	"context"
	"fmt"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/internal/healthchecker"
)

// CheckerConfig is an alias for healthchecker.Config for backward compatibility
type CheckerConfig = healthchecker.Config

// Config holds configuration for the composite backend
type Config struct {
	Primary        backends.Backend // Primary/preferred backend
	Secondary      backends.Backend // Secondary/fallback backend
	CircuitBreaker BreakerConfig    // Circuit breaker configuration
	HealthChecker  CheckerConfig    // Health check configuration (alias for backward compatibility)
}

// Backend provides automatic failover capability for rate limiting storage
type Backend struct {
	config         Config
	primary        backends.Backend
	secondary      backends.Backend
	circuitBreaker *circuitBreaker
	healthChecker  *healthchecker.Checker
}

// New creates a new composite backend
func New(config Config) (*Backend, error) {
	if config.Primary == nil {
		return nil, fmt.Errorf("primary backend is required")
	}
	if config.Secondary == nil {
		return nil, fmt.Errorf("secondary backend is required")
	}

	// Set default circuit breaker config if not provided
	if config.CircuitBreaker.FailureThreshold <= 0 {
		config.CircuitBreaker.FailureThreshold = 5
	}
	if config.CircuitBreaker.RecoveryTimeout <= 0 {
		config.CircuitBreaker.RecoveryTimeout = 30 * time.Second
	}

	// Set default health check config if not provided
	if config.HealthChecker.Interval <= 0 {
		config.HealthChecker.Interval = 10 * time.Second
	}
	if config.HealthChecker.Timeout <= 0 {
		config.HealthChecker.Timeout = 2 * time.Second
	}

	composite := &Backend{
		config:         config,
		primary:        config.Primary,
		secondary:      config.Secondary,
		circuitBreaker: newCircuitBreaker(config.CircuitBreaker),
	}

	// Initialize health checker
	composite.healthChecker = healthchecker.New(
		composite.primary,
		healthchecker.Config(config.HealthChecker),
		composite.onPrimaryHealthy,
	)

	// Start health monitoring
	composite.healthChecker.Start()

	return composite, nil
}

// Get retrieves value from backend with failover logic
func (c *Backend) Get(ctx context.Context, key string) (string, error) {
	if c.circuitBreaker.IsOpen() {
		// Circuit is open - use secondary
		return c.secondary.Get(ctx, key)
	}

	// Try primary first
	result, err := c.primary.Get(ctx, key)
	if c.circuitBreaker.ShouldTrip(err) {
		// Circuit breaker was tripped, use secondary
		return c.secondary.Get(ctx, key)
	}

	// Circuit breaker in HALF-OPEN - test succeeded
	if c.circuitBreaker.GetState() == stateHalfOpen {
		c.circuitBreaker.Close()
	}

	return result, err
}

// Set stores value in backend with failover logic
func (c *Backend) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	if c.circuitBreaker.IsOpen() {
		// Circuit is open - use secondary
		return c.secondary.Set(ctx, key, value, expiration)
	}

	// Try primary first
	err := c.primary.Set(ctx, key, value, expiration)
	if c.circuitBreaker.ShouldTrip(err) {
		// Circuit breaker was tripped, use secondary
		return c.secondary.Set(ctx, key, value, expiration)
	}

	// Circuit breaker in HALF-OPEN - test succeeded
	if c.circuitBreaker.GetState() == stateHalfOpen {
		c.circuitBreaker.Close()
	}

	return err
}

// CheckAndSet performs atomic compare-and-set with failover logic
func (c *Backend) CheckAndSet(ctx context.Context, key string, expected string, newValue string, expiration time.Duration) (bool, error) {
	if c.circuitBreaker.IsOpen() {
		// Circuit is open - use secondary
		return c.secondary.CheckAndSet(ctx, key, expected, newValue, expiration)
	}

	// Try primary first
	result, err := c.primary.CheckAndSet(ctx, key, expected, newValue, expiration)
	if c.circuitBreaker.ShouldTrip(err) {
		// Circuit breaker was tripped, use secondary
		return c.secondary.CheckAndSet(ctx, key, expected, newValue, expiration)
	}

	// Circuit breaker in HALF-OPEN - primary test succeeded
	if c.circuitBreaker.GetState() == stateHalfOpen {
		c.circuitBreaker.Close()
	}

	return result, err
}

// Delete removes key from backend with failover logic
func (c *Backend) Delete(ctx context.Context, key string) error {
	if c.circuitBreaker.IsOpen() {
		// Circuit is open - use secondary
		return c.secondary.Delete(ctx, key)
	}

	// Try primary first
	err := c.primary.Delete(ctx, key)
	if c.circuitBreaker.ShouldTrip(err) {
		// Circuit breaker was tripped, use secondary
		return c.secondary.Delete(ctx, key)
	}

	// Circuit breaker in HALF-OPEN - test succeeded
	if c.circuitBreaker.GetState() == stateHalfOpen {
		c.circuitBreaker.Close()
	}

	return err
}

// Close closes both backends and stops health monitoring
func (c *Backend) Close() error {
	// Stop health monitoring
	if c.healthChecker != nil {
		c.healthChecker.Stop()
	}

	// Close both backends
	var primaryErr, secondaryErr error
	if c.primary != nil {
		primaryErr = c.primary.Close()
	}
	if c.secondary != nil {
		secondaryErr = c.secondary.Close()
	}

	// Return first error if any
	if primaryErr != nil {
		return primaryErr
	}
	return secondaryErr
}

// onPrimaryHealthy is called when health checker detects primary is healthy
func (c *Backend) onPrimaryHealthy() {
	// Reset circuit breaker if it's open
	if c.circuitBreaker.GetState() == stateOpen {
		c.circuitBreaker.Close()
	}
}

// GetCircuitBreakerState returns current circuit breaker state (for monitoring)
func (c *Backend) GetCircuitBreakerState() breakerState {
	return c.circuitBreaker.GetState()
}

// GetCircuitBreakerFailureCount returns the internal failure count (for testing/debugging)
func (c *Backend) GetCircuitBreakerFailureCount() int32 {
	return c.circuitBreaker.GetFailureCount()
}
