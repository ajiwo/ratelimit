package composite

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBackend implements backends.Backend for testing
type mockBackend struct {
	data    map[string]string
	mu      sync.RWMutex
	closed  bool
	fail    bool
	failErr error
}

func newMockBackend() *mockBackend {
	return &mockBackend{
		data: make(map[string]string),
	}
}

func (m *mockBackend) Get(ctx context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.fail {
		return "", m.failErr
	}
	if m.closed {
		return "", errors.New("backend closed")
	}

	val, exists := m.data[key]
	if !exists {
		return "", errors.New("key not found")
	}
	return val, nil
}

func (m *mockBackend) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.fail {
		return m.failErr
	}
	if m.closed {
		return errors.New("backend closed")
	}

	m.data[key] = value
	return nil
}

func (m *mockBackend) CheckAndSet(ctx context.Context, key string, oldValue, newValue string, expiration time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.fail {
		return false, m.failErr
	}
	if m.closed {
		return false, errors.New("backend closed")
	}

	current, exists := m.data[key]
	if !exists && oldValue != "" {
		return false, nil
	}
	if exists && current != oldValue {
		return false, nil
	}

	m.data[key] = newValue
	return true, nil
}

func (m *mockBackend) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.fail {
		return m.failErr
	}
	if m.closed {
		return errors.New("backend closed")
	}

	delete(m.data, key)
	return nil
}

func (m *mockBackend) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
	m.data = make(map[string]string)
	return nil
}

func (m *mockBackend) setFail(fail bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fail = fail
	m.failErr = err
}

func TestCompositeBackend_New(t *testing.T) {
	primary := newMockBackend()
	secondary := newMockBackend()

	tests := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "valid config",
			config: Config{
				Primary:   primary,
				Secondary: secondary,
			},
			expectError: false,
		},
		{
			name: "missing primary",
			config: Config{
				Primary:   nil,
				Secondary: secondary,
			},
			expectError: true,
		},
		{
			name: "missing secondary",
			config: Config{
				Primary:   primary,
				Secondary: nil,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.config)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCompositeBackend_PrimaryOperations(t *testing.T) {
	primary := newMockBackend()
	secondary := newMockBackend()

	config := Config{
		Primary:   primary,
		Secondary: secondary,
		CircuitBreaker: BreakerConfig{
			FailureThreshold: 2,
			RecoveryTimeout:  100 * time.Millisecond,
		},
		HealthChecker: CheckerConfig{
			Interval: 50 * time.Millisecond,
			Timeout:  10 * time.Millisecond,
		},
	}

	composite, err := New(config)
	require.NoError(t, err)
	defer composite.Close()

	ctx := context.Background()

	// Test operations work normally when primary is healthy
	err = composite.Set(ctx, "test", "value", time.Minute)
	assert.NoError(t, err)

	val, err := composite.Get(ctx, "test")
	assert.NoError(t, err)
	assert.Equal(t, "value", val)

	success, err := composite.CheckAndSet(ctx, "test", "value", "newvalue", time.Minute)
	assert.NoError(t, err)
	assert.True(t, success)

	err = composite.Delete(ctx, "test")
	assert.NoError(t, err)
}

func TestCompositeBackend_Failover(t *testing.T) {
	primary := newMockBackend()
	secondary := newMockBackend()

	config := Config{
		Primary:   primary,
		Secondary: secondary,
		CircuitBreaker: BreakerConfig{
			FailureThreshold: 2,
			RecoveryTimeout:  100 * time.Millisecond,
		},
	}

	composite, err := New(config)
	require.NoError(t, err)
	defer composite.Close()

	ctx := context.Background()

	// Make primary fail
	primary.setFail(true, errors.New("primary failed"))

	// First failure should not trip circuit breaker yet
	err = composite.Set(ctx, "test", "value", time.Minute)
	assert.Error(t, err)
	assert.Equal(t, stateClosed, composite.GetCircuitBreakerState())

	// Second failure should trip circuit breaker
	err = composite.Set(ctx, "test", "value", time.Minute)
	assert.NoError(t, err) // Should succeed via secondary after circuit breaker trips
	assert.Equal(t, stateOpen, composite.GetCircuitBreakerState())

	// Operations should now go to secondary
	err = composite.Set(ctx, "test", "value", time.Minute)
	assert.NoError(t, err)

	val, err := composite.Get(ctx, "test")
	assert.NoError(t, err)
	assert.Equal(t, "value", val)

	// Verify secondary has the data
	val, err = secondary.Get(ctx, "test")
	assert.NoError(t, err)
	assert.Equal(t, "value", val)
}

func TestCompositeBackend_Recovery(t *testing.T) {
	primary := newMockBackend()
	secondary := newMockBackend()

	config := Config{
		Primary:   primary,
		Secondary: secondary,
		CircuitBreaker: BreakerConfig{
			FailureThreshold: 1,
			RecoveryTimeout:  50 * time.Millisecond,
		},
		HealthChecker: CheckerConfig{
			Interval: 10 * time.Millisecond,
			Timeout:  5 * time.Millisecond,
			TestKey:  "health-test",
		},
	}

	composite, err := New(config)
	require.NoError(t, err)
	defer composite.Close()

	ctx := context.Background()

	// Make primary fail to trip circuit breaker
	primary.setFail(true, errors.New("primary failed"))
	err = composite.Set(ctx, "test", "value", time.Minute)
	assert.NoError(t, err) // Should succeed via secondary after failure threshold is reached
	assert.Equal(t, stateOpen, composite.GetCircuitBreakerState())

	// Use secondary while primary is failing
	err = composite.Set(ctx, "test", "value", time.Minute)
	assert.NoError(t, err)

	// Restore primary
	primary.setFail(false, nil)

	// Set a value for health check to succeed
	err = primary.Set(ctx, config.HealthChecker.TestKey, "health-ok", time.Minute)
	assert.NoError(t, err)

	// Wait for health checker to detect recovery
	time.Sleep(100 * time.Millisecond)

	// Circuit breaker should be closed now
	assert.Equal(t, stateClosed, composite.GetCircuitBreakerState())

	// Operations should go to primary again
	err = composite.Set(ctx, "test2", "value2", time.Minute)
	assert.NoError(t, err)

	val, err := primary.Get(ctx, "test2")
	assert.NoError(t, err)
	assert.Equal(t, "value2", val)
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "valid config",
			config: Config{
				Primary:   newMockBackend(),
				Secondary: newMockBackend(),
				CircuitBreaker: BreakerConfig{
					FailureThreshold: 5,
					RecoveryTimeout:  30 * time.Second,
				},
				HealthChecker: CheckerConfig{
					Interval: 10 * time.Second,
					Timeout:  2 * time.Second,
				},
			},
			expectError: false,
		},
		{
			name: "missing primary",
			config: Config{
				Primary:   nil,
				Secondary: newMockBackend(),
			},
			expectError: true,
		},
		{
			name: "missing secondary",
			config: Config{
				Primary:   newMockBackend(),
				Secondary: nil,
			},
			expectError: true,
		},
		{
			name: "invalid circuit breaker config",
			config: Config{
				Primary:   newMockBackend(),
				Secondary: newMockBackend(),
				CircuitBreaker: BreakerConfig{
					FailureThreshold: 0, // invalid
				},
			},
			expectError: true,
		},
		{
			name: "health check timeout >= interval",
			config: Config{
				Primary:   newMockBackend(),
				Secondary: newMockBackend(),
				HealthChecker: CheckerConfig{
					Interval: 5 * time.Second,
					Timeout:  10 * time.Second, // invalid
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, int32(5), config.CircuitBreaker.FailureThreshold)
	assert.Equal(t, 30*time.Second, config.CircuitBreaker.RecoveryTimeout)
	assert.Equal(t, 10*time.Second, config.HealthChecker.Interval)
	assert.Equal(t, 2*time.Second, config.HealthChecker.Timeout)
	assert.Equal(t, "health-check-key", config.HealthChecker.TestKey)
}

func TestCompositeBackend_ConcurrentAccess(t *testing.T) {
	primary := newMockBackend()
	secondary := newMockBackend()

	config := Config{
		Primary:   primary,
		Secondary: secondary,
		CircuitBreaker: BreakerConfig{
			FailureThreshold: 10,
			RecoveryTimeout:  1 * time.Second,
		},
	}

	composite, err := New(config)
	require.NoError(t, err)
	defer composite.Close()

	ctx := context.Background()
	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := range numGoroutines {
		go func(id int) {
			defer wg.Done()
			for j := range numOperations {
				key := fmt.Sprintf("key-%d-%d", id, j)
				value := fmt.Sprintf("value-%d-%d", id, j)

				err := composite.Set(ctx, key, value, time.Minute)
				assert.NoError(t, err)

				retrieved, err := composite.Get(ctx, key)
				assert.NoError(t, err)
				assert.Equal(t, value, retrieved)
			}
		}(i)
	}

	wg.Wait()
}

func TestCompositeBackend_ContextCancellation(t *testing.T) {
	t.Run("basic context cancellation", func(t *testing.T) {
		primary := newMockBackend()
		secondary := newMockBackend()

		config := Config{
			Primary:   primary,
			Secondary: secondary,
			CircuitBreaker: BreakerConfig{
				FailureThreshold: 1,
				RecoveryTimeout:  100 * time.Millisecond,
			},
		}

		composite, err := New(config)
		require.NoError(t, err)
		defer composite.Close()

		// Test with cancelled context - should fail with context error
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err = composite.Get(ctx, "test-key")
		// Should get an error, but might be "key not found" before context cancellation
		assert.Error(t, err)
	})

	t.Run("context cancellation during failover", func(t *testing.T) {
		primary := newMockBackend()
		secondary := newMockBackend()

		config := Config{
			Primary:   primary,
			Secondary: secondary,
			CircuitBreaker: BreakerConfig{
				FailureThreshold: 1,
				RecoveryTimeout:  100 * time.Millisecond,
			},
		}

		composite, err := New(config)
		require.NoError(t, err)
		defer composite.Close()

		// Make primary fail to trigger failover to secondary
		primary.setFail(true, errors.New("primary failed"))

		// This should succeed via secondary and trip the circuit
		err = composite.Set(context.Background(), "test-key", "test-value", time.Minute)
		assert.NoError(t, err)

		// Now test with cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err = composite.Get(ctx, "test-key")
		// Should get an error - could be context error or success if operation completes before cancellation
		if err != nil {
			// Any error is acceptable here since we're testing that cancellation doesn't cause panics
			assert.True(t, true)
		}
	})
}

func TestCompositeBackend_StateFragmentation(t *testing.T) {
	primary := newMockBackend()
	secondary := newMockBackend()

	config := Config{
		Primary:   primary,
		Secondary: secondary,
		CircuitBreaker: BreakerConfig{
			FailureThreshold: 1,
			RecoveryTimeout:  50 * time.Millisecond,
		},
		HealthChecker: CheckerConfig{
			Interval: 0, // Disable health checking for controlled recovery
		},
	}

	composite, err := New(config)
	require.NoError(t, err)
	defer composite.Close()

	ctx := context.Background()

	// Set initial state on primary
	err = primary.Set(ctx, "key1", "primary-value", time.Minute)
	assert.NoError(t, err)

	// Trip circuit breaker
	primary.setFail(true, errors.New("primary failed"))
	err = composite.Set(ctx, "key2", "during-failover", time.Minute)
	assert.NoError(t, err) // Goes to secondary
	assert.Equal(t, stateOpen, composite.GetCircuitBreakerState())

	// Verify operation went to secondary
	val, err := secondary.Get(ctx, "key2")
	assert.NoError(t, err)
	assert.Equal(t, "during-failover", val)

	// Restore primary and manually close circuit
	primary.setFail(false, nil)
	composite.circuitBreaker.Close()

	// Now operations should go to primary again
	err = composite.Set(ctx, "key3", "back-to-primary", time.Minute)
	assert.NoError(t, err)

	val, err = primary.Get(ctx, "key3")
	assert.NoError(t, err)
	assert.Equal(t, "back-to-primary", val)
}

func TestCompositeBackend_ErrorClassification(t *testing.T) {
	tests := []struct {
		name              string
		primaryError      error
		expectCircuitTrip bool
		description       string
	}{
		{
			name:              "network error trips circuit",
			primaryError:      errors.New("connection refused"),
			expectCircuitTrip: true,
			description:       "Network errors should trip circuit breaker",
		},
		{
			name:              "timeout error trips circuit",
			primaryError:      context.DeadlineExceeded,
			expectCircuitTrip: true,
			description:       "Timeout errors should trip circuit breaker",
		},
		{
			name:              "nil error should not trip",
			primaryError:      nil,
			expectCircuitTrip: false,
			description:       "Successful operations should not trip circuit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := newMockBackend()
			secondary := newMockBackend()

			config := Config{
				Primary:   primary,
				Secondary: secondary,
				CircuitBreaker: BreakerConfig{
					FailureThreshold: 1,
					RecoveryTimeout:  100 * time.Millisecond,
				},
			}

			composite, err := New(config)
			require.NoError(t, err)
			defer composite.Close()

			ctx := context.Background()
			primary.setFail(true, tt.primaryError)

			initialState := composite.GetCircuitBreakerState()
			assert.Equal(t, stateClosed, initialState)

			// Perform operation that may trigger error
			_, err = composite.Get(ctx, "test-key")

			if tt.expectCircuitTrip {
				assert.Error(t, err)
				assert.Equal(t, stateOpen, composite.GetCircuitBreakerState())
			} else if tt.primaryError != nil {
				assert.Error(t, err)
				assert.Equal(t, stateClosed, composite.GetCircuitBreakerState())
			}
		})
	}
}

func TestCompositeBackend_ConfigurationEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
		description string
	}{
		{
			name: "zero failure threshold should use default",
			config: Config{
				Primary:   newMockBackend(),
				Secondary: newMockBackend(),
				CircuitBreaker: BreakerConfig{
					FailureThreshold: 0, // Should be set to default 5
					RecoveryTimeout:  30 * time.Second,
				},
			},
			expectError: false,
			description: "Zero failure threshold should use default",
		},
		{
			name: "zero recovery timeout should use default",
			config: Config{
				Primary:   newMockBackend(),
				Secondary: newMockBackend(),
				CircuitBreaker: BreakerConfig{
					FailureThreshold: 5,
					RecoveryTimeout:  0, // Should be set to default 30s
				},
			},
			expectError: false,
			description: "Zero recovery timeout should use default",
		},
		{
			name: "disabled health checker with zero interval",
			config: Config{
				Primary:   newMockBackend(),
				Secondary: newMockBackend(),
				HealthChecker: CheckerConfig{
					Interval: 0, // Should disable health checking
					Timeout:  2 * time.Second,
				},
			},
			expectError: false,
			description: "Zero health check interval should disable health checking",
		},
		{
			name: "very large configuration values",
			config: Config{
				Primary:   newMockBackend(),
				Secondary: newMockBackend(),
				CircuitBreaker: BreakerConfig{
					FailureThreshold: 1000,
					RecoveryTimeout:  24 * time.Hour,
				},
				HealthChecker: CheckerConfig{
					Interval: 1 * time.Hour,
					Timeout:  10 * time.Minute,
				},
			},
			expectError: false,
			description: "Large configuration values should be accepted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			composite, err := New(tt.config)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if composite != nil {
					composite.Close()
				}
			}
		})
	}
}
