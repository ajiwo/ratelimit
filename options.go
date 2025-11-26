package ratelimit

import (
	"fmt"
	"math"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/ajiwo/ratelimit/internal/backends/composite"
	"github.com/ajiwo/ratelimit/internal/healthchecker"
	"github.com/ajiwo/ratelimit/strategies"
)

// Option is a functional option for configuring the rate limiter
type Option func(*Config) error

// WithPrimaryStrategy configures the primary rate limiting strategy with custom configuration
func WithPrimaryStrategy(strategyConfig strategies.Config) Option {
	return func(config *Config) error {
		if strategyConfig == nil {
			return fmt.Errorf("primary strategy config cannot be nil")
		}
		config.PrimaryConfig = strategyConfig
		return nil
	}
}

// WithSecondaryStrategy configures the secondary smoother strategy
func WithSecondaryStrategy(strategyConfig strategies.Config) Option {
	return func(config *Config) error {
		if strategyConfig == nil {
			return fmt.Errorf("secondary strategy config cannot be nil")
		}

		// Secondary strategy must have CapSecondary capability
		if !strategyConfig.Capabilities().Has(strategies.CapSecondary) {
			return fmt.Errorf("strategy '%s' doesn't have secondary capability", strategyConfig.ID().String())
		}

		config.SecondaryConfig = strategyConfig
		return nil
	}
}

// WithBaseKey sets the base key for rate limiting
func WithBaseKey(key string) Option {
	return func(config *Config) error {
		config.BaseKey = key
		return nil
	}
}

// AccessOptions holds the configuration for a rate limiter access operation
type AccessOptions struct {
	Key            string                        // Dynamic key
	SkipValidation bool                          // Skip key validation
	Result         *map[string]strategies.Result // Optional results pointer
}

// WithBackend configures the rate limiter to use a custom backend
func WithBackend(backend backends.Backend) Option {
	return func(config *Config) error {
		if backend == nil {
			return fmt.Errorf("backend cannot be nil")
		}
		if config.Storage != nil {
			err := config.Storage.Close()
			if err != nil {
				return fmt.Errorf("failed to close existing backend: %w", err)
			}
		}
		config.Storage = backend
		return nil
	}
}

// WithMaxRetries configures the maximum number of retry attempts for atomic CheckAndSet operations.
// This is used by all strategies that perform optimistic locking (Fixed Window, Token Bucket, Leaky Bucket, GCRA).
//
// Under high concurrency, CheckAndSet operations may need to retry if the state changes between read and write.
// The retry loop uses exponential backoff with delays based on time since last failed attempt, clamped
// between 30ns and 10s, and checks for context cancellation (except for short delays, which bypass context checks)
// on each attempt.
//
// Recommended values, not strictly:
//   - Low contention (< 10 concurrent users): 5-8 retries
//   - Medium contention (10-100 concurrent users): 75% of concurrent users retries
//   - High contention (100+ concurrent users): max(30, 75% of concurrent users) retries
//
// Examples:
//   - 20 concurrent users: ~15 retries
//   - 50 concurrent users: ~38 retries
//   - 100 concurrent users: 75 retries
//   - 500 concurrent users: 375 retries
//
// If retries is 0 or not set, the default value (30) will be used.
func WithMaxRetries(retries int) Option {
	return func(config *Config) error {
		if retries < 0 {
			return fmt.Errorf("check and set retries cannot be negative, got %d", retries)
		}
		if retries > strategies.MaxRetries {
			return fmt.Errorf(
				"check and set retries cannot exceed %d, got %d",
				strategies.MaxRetries, retries,
			)
		}
		config.maxRetries = retries
		return nil
	}
}

// MemoryFailoverOption configures memory failover behavior
type MemoryFailoverOption func(*failoverConfig)

// failoverConfig holds configuration for memory failover
type failoverConfig struct {
	failureThreshold int32
	recoveryTimeout  time.Duration
	healthInterval   time.Duration
	healthTimeout    time.Duration
	healthTestKey    string
}

// WithFailureThreshold configures the number of consecutive failures before opening circuit
func WithFailureThreshold(threshold int) MemoryFailoverOption {
	return func(fc *failoverConfig) {
		if threshold > math.MaxInt32 {
			// Clamp to maximum int32 value to prevent overflow
			threshold = math.MaxInt32
		}
		// #nosec G115 -- overflow is prevented by the clamping above
		fc.failureThreshold = int32(threshold)
	}
}

// WithRecoveryTimeout configures how long to wait in OPEN state before attempting recovery
func WithRecoveryTimeout(timeout time.Duration) MemoryFailoverOption {
	return func(fc *failoverConfig) {
		fc.recoveryTimeout = timeout
	}
}

// WithHealthCheckInterval configures health check frequency
func WithHealthCheckInterval(interval time.Duration) MemoryFailoverOption {
	return func(fc *failoverConfig) {
		fc.healthInterval = interval
	}
}

// WithHealthCheckTimeout configures individual health check timeout
func WithHealthCheckTimeout(timeout time.Duration) MemoryFailoverOption {
	return func(fc *failoverConfig) {
		fc.healthTimeout = timeout
	}
}

// WithHealthTestKey configures the key used for health checking
func WithHealthTestKey(key string) MemoryFailoverOption {
	return func(fc *failoverConfig) {
		fc.healthTestKey = key
	}
}

// WithMemoryFailover configures automatic failover to a memory backend when the primary backend fails.
// This provides resilience by falling back to in-memory storage during backend outages.
//
// The memory backend will be used automatically when:
//   - The primary backend fails consecutively (default: 5 failures)
//   - The circuit breaker is OPEN (default recovery timeout: 30s)
//
// This option prioritizes service availability over strict state consistency:
//   - No State Synchronization: Primary and memory backends maintain independent state
//   - State Fragmentation During Failover: Users may get full or partial quota resets when switching backends
//   - Self-Correction Over Time: State consistency resumes naturally through normal rate limiting operations
//
// Skip this option if you require:
//   - Strict rate limiting consistency across all storage backends
//   - Zero tolerance for temporary state fragmentation
//   - Enterprise-grade state synchronization guarantees
//
// This option is designed for:
//   - Applications prioritizing service availability over perfect rate limiting
//   - Small teams without infrastructure support for state synchronization
//   - Systems where temporary over-allocation is preferable to service downtime
//   - Simple deployments needing basic resilience
func WithMemoryFailover(opts ...MemoryFailoverOption) Option {
	return func(config *Config) error {
		if config.Storage == nil {
			return fmt.Errorf("primary backend not found - call WithBackend() before WithMemoryFailover()")
		}

		// Prevent failover from memory to memory
		if _, ok := config.Storage.(*memory.Backend); ok {
			return fmt.Errorf("memory failover is not applicable when the primary backend is already a memory backend")
		}

		// Prevent nested failovers
		if _, ok := config.Storage.(*composite.Backend); ok {
			return fmt.Errorf("memory failover cannot be enabled on a composite backend to prevent nested failovers")
		}

		// Set defaults
		fc := &failoverConfig{
			failureThreshold: 5,
			recoveryTimeout:  30 * time.Second,
			healthInterval:   10 * time.Second,
			healthTimeout:    2 * time.Second,
			healthTestKey:    "health-check-key",
		}

		// Apply custom options
		for _, opt := range opts {
			opt(fc)
		}

		// Create memory backend as secondary
		memoryBackend := memory.New()

		// Create composite backend with failover configuration
		compositeBackend, err := composite.New(composite.Config{
			Primary:   config.Storage,
			Secondary: memoryBackend,
			CircuitBreaker: composite.BreakerConfig{
				FailureThreshold: fc.failureThreshold,
				RecoveryTimeout:  fc.recoveryTimeout,
			},
			HealthChecker: healthchecker.Config{
				Interval: fc.healthInterval,
				Timeout:  fc.healthTimeout,
				TestKey:  fc.healthTestKey,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create memory failover backend: %w", err)
		}

		// Replace the storage backend with the composite backend
		config.Storage = compositeBackend
		return nil
	}
}
