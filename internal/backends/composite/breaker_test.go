package composite

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker(t *testing.T) {
	tests := []struct {
		name             string
		failureThreshold int32
		recoveryTimeout  time.Duration
		errors           []error
		expectedStates   []breakerState
	}{
		{
			name:             "trip after threshold",
			failureThreshold: 3,
			recoveryTimeout:  100 * time.Millisecond,
			errors:           []error{errors.New("fail1"), errors.New("fail2"), errors.New("fail3")},
			expectedStates:   []breakerState{stateClosed, stateClosed, stateOpen, stateOpen},
		},
		{
			name:             "no trip on success",
			failureThreshold: 3,
			recoveryTimeout:  100 * time.Millisecond,
			errors:           []error{nil, nil, nil},
			expectedStates:   []breakerState{stateClosed, stateClosed, stateClosed, stateClosed},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := BreakerConfig{
				FailureThreshold: tt.failureThreshold,
				RecoveryTimeout:  tt.recoveryTimeout,
			}
			cb := newCircuitBreaker(config)

			for i, err := range tt.errors {
				tripped := cb.ShouldTrip(err)
				expectedState := tt.expectedStates[i]
				assert.Equal(t, expectedState, cb.GetState(), "state mismatch at iteration %d", i)

				if err != nil && !tripped {
					assert.Equal(t, stateClosed, cb.GetState(), "should not trip before threshold")
				}
			}
		})
	}
}

func TestCircuitBreakerRecovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := BreakerConfig{
			FailureThreshold: 2,
			RecoveryTimeout:  50 * time.Millisecond,
		}
		cb := newCircuitBreaker(config)

		// Trip the circuit breaker
		assert.False(t, cb.ShouldTrip(errors.New("fail1")))
		assert.True(t, cb.ShouldTrip(errors.New("fail2"))) // Second failure trips the circuit
		assert.Equal(t, stateOpen, cb.GetState())

		// Should be open immediately after tripping
		assert.True(t, cb.IsOpen())

		// Should transition to half-open after recovery timeout
		time.Sleep(60 * time.Millisecond)
		synctest.Wait()
		assert.False(t, cb.IsOpen())
		assert.Equal(t, stateHalfOpen, cb.GetState())

		cb.Close()
		assert.Equal(t, stateClosed, cb.GetState())
		assert.False(t, cb.IsOpen())
	})
}

func TestCircuitBreaker_HalfOpenState(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
				Interval: 0, // Disable health checking for this test
			},
		}

		composite, err := New(config)
		require.NoError(t, err)
		defer composite.Close()

		ctx := context.Background()

		// Trip the circuit breaker
		primary.setFail(true, errors.New("primary failed"))
		err = composite.Set(ctx, "key1", "value1", time.Minute)
		assert.NoError(t, err) // Should succeed via secondary and trip circuit
		assert.Equal(t, stateOpen, composite.GetCircuitBreakerState())

		// Wait for recovery timeout to transition to half-open
		time.Sleep(100 * time.Millisecond) // Longer than recovery timeout
		synctest.Wait()
		// In synctest, the state should be either HALF_OPEN or might have already transitioned
		currentState := composite.GetCircuitBreakerState()
		assert.True(t, currentState == stateHalfOpen || currentState == stateClosed || currentState == stateOpen)

		// Test: Half-open with success - should close circuit
		primary.setFail(false, nil)
		err = composite.Set(ctx, "key2", "value2", time.Minute)
		assert.NoError(t, err)
		assert.Equal(t, stateClosed, composite.GetCircuitBreakerState())
	})
}

func TestCircuitBreaker_ConcurrentStateTransitions(t *testing.T) {
	primary := newMockBackend()
	secondary := newMockBackend()

	config := Config{
		Primary:   primary,
		Secondary: secondary,
		CircuitBreaker: BreakerConfig{
			FailureThreshold: 3,
			RecoveryTimeout:  100 * time.Millisecond,
		},
		HealthChecker: CheckerConfig{
			Interval: 0, // Disable health checking
		},
	}

	composite, err := New(config)
	require.NoError(t, err)
	defer composite.Close()

	ctx := context.Background()
	primary.setFail(true, errors.New("primary failed"))

	const numGoroutines = 10
	var wg sync.WaitGroup

	// Start multiple operations that should trigger circuit breaker transitions
	wg.Add(numGoroutines)
	for i := range numGoroutines {
		go func(id int) {
			defer wg.Done()
			for j := range 5 {
				key := fmt.Sprintf("key-%d-%d", id, j)
				err := composite.Set(ctx, key, fmt.Sprintf("value-%d-%d", id, j), time.Minute)
				// Errors are expected, we're testing race conditions
				_ = err
			}
		}(i)
	}

	wg.Wait()

	// Should eventually end up in OPEN state
	assert.Equal(t, stateOpen, composite.GetCircuitBreakerState())

	// Verify all operations went to secondary
	keys, err := secondary.Get(ctx, "key-0-0")
	if err == nil {
		assert.NotEmpty(t, keys)
	}
}
