package composite

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthChecker_FailuresAndRecovery(t *testing.T) {
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
			Interval: 20 * time.Millisecond,
			Timeout:  5 * time.Millisecond,
			TestKey:  "health-test-key",
		},
	}

	composite, err := New(config)
	require.NoError(t, err)
	defer composite.Close()

	ctx := t.Context()

	// Trip circuit breaker initially
	primary.setFail(true, errors.New("primary failed"))
	err = composite.Set(ctx, "test", "value", time.Minute)
	assert.NoError(t, err) // Should succeed via secondary
	assert.Equal(t, stateOpen, composite.GetCircuitBreakerState())

	// Wait for health checker to attempt a check (but fail due to primary being down)
	assert.Equal(t, stateOpen, composite.GetCircuitBreakerState())

	// Restore primary and set up health check key
	primary.setFail(false, nil)
	err = primary.Set(ctx, config.HealthChecker.TestKey, "health-ok", time.Minute)
	assert.NoError(t, err)

	// Advance time enough for health checker to detect recovery
	time.Sleep(30 * time.Millisecond) // One health check interval

	// Health checker should have detected recovery and closed circuit
	assert.Equal(t, stateClosed, composite.GetCircuitBreakerState())

	// Verify operations go back to primary
	err = composite.Set(ctx, "back-on-primary", "value", time.Minute)
	assert.NoError(t, err)

	val, err := primary.Get(ctx, "back-on-primary")
	assert.NoError(t, err)
	assert.Equal(t, "value", val)
}
