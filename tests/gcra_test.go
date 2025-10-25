package tests

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/ajiwo/ratelimit/strategies/gcra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGCRA_MemoryBackend tests GCRA strategy with memory backend
func TestGCRA_MemoryBackend(t *testing.T) {
	ctx := context.Background()
	storage := memory.New()
	defer storage.Close()

	strategy := gcra.New(storage)
	config := gcra.Config{
		Key:   "test_memory",
		Rate:  10.0, // 10 requests per second
		Burst: 5,    // burst of 5
	}

	// Test initial state - should allow requests
	result, err := strategy.Allow(ctx, config)
	require.NoError(t, err, "Unexpected error")
	assert.True(t, result["default"].Allowed, "Expected first request to be allowed")
	assert.Equal(t, 4, result["default"].Remaining, "Expected 4 remaining requests")

	// Test burst consumption
	for i := 0; i < 4; i++ {
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err, "Unexpected error on burst request %d", i)
		assert.True(t, result["default"].Allowed, "Expected burst request %d to be allowed", i+1)
	}

	// Next request should be denied (burst exhausted)
	result, err = strategy.Allow(ctx, config)
	require.NoError(t, err, "Unexpected error")
	assert.False(t, result["default"].Allowed, "Expected request to be denied after burst exhaustion")
	assert.Equal(t, 0, result["default"].Remaining, "Expected 0 remaining requests")
}

// TestGCRA_PeekBehavior tests Peek vs Allow behavior
func TestGCRA_PeekBehavior(t *testing.T) {
	ctx := context.Background()
	storage := memory.New()
	defer storage.Close()

	strategy := gcra.New(storage)
	config := gcra.Config{
		Key:   "test_peek",
		Rate:  5.0,
		Burst: 3,
	}

	// Initial peek should show full burst
	result, err := strategy.Peek(ctx, config)
	require.NoError(t, err, "Unexpected error")
	assert.True(t, result["default"].Allowed, "Peek should show request as allowed")
	assert.Equal(t, 3, result["default"].Remaining, "Expected 3 remaining")

	// Peek again should show same state (no consumption)
	result, err = strategy.Peek(ctx, config)
	require.NoError(t, err, "Unexpected error")
	assert.Equal(t, 3, result["default"].Remaining, "Peek should not consume quota, expected 3 remaining")

	// Allow should consume quota
	result, err = strategy.Allow(ctx, config)
	require.NoError(t, err, "Unexpected error")
	assert.Equal(t, 2, result["default"].Remaining, "Expected 2 remaining after allow")

	// Peek should reflect consumed state
	result, err = strategy.Peek(ctx, config)
	require.NoError(t, err, "Unexpected error")
	assert.Equal(t, 2, result["default"].Remaining, "Peek should reflect consumed state, expected 2 remaining")
}

// TestGCRA_Reset tests reset functionality
func TestGCRA_Reset(t *testing.T) {
	ctx := context.Background()
	storage := memory.New()
	defer storage.Close()

	strategy := gcra.New(storage)
	config := gcra.Config{
		Key:   "test_reset",
		Rate:  2.0,
		Burst: 2,
	}

	// Consume all burst
	for i := 0; i < 2; i++ {
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err, "Unexpected error")
		assert.True(t, result["default"].Allowed, "Expected request %d to be allowed", i+1)
	}

	// Should be denied now
	result, err := strategy.Allow(ctx, config)
	require.NoError(t, err, "Unexpected error")
	assert.False(t, result["default"].Allowed, "Expected request to be denied after burst consumption")

	// Reset should clear the state
	err = strategy.Reset(ctx, config)
	require.NoError(t, err, "Unexpected error during reset")

	// Should now be allowed again with full burst
	result, err = strategy.Allow(ctx, config)
	require.NoError(t, err, "Unexpected error")
	assert.True(t, result["default"].Allowed, "Expected request to be allowed after reset")
	assert.Equal(t, 1, result["default"].Remaining, "Expected 1 remaining after reset")
}

// TestGCRA_ConcurrentAccess tests concurrent access to GCRA
func TestGCRA_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	storage := memory.New()
	defer storage.Close()

	strategy := gcra.New(storage)
	config := gcra.Config{
		Key:   "test_concurrent",
		Rate:  100.0, // High rate for concurrent testing
		Burst: 50,
	}

	const numGoroutines = 10
	const requestsPerGoroutine = 20

	var wg sync.WaitGroup
	var allowedCount int64
	var errorCount int64
	var mu sync.Mutex

	// Launch multiple goroutines making concurrent requests
	for range numGoroutines {
		wg.Go(func() {
			for range requestsPerGoroutine {
				result, err := strategy.Allow(ctx, config)
				if err != nil {
					mu.Lock()
					errorCount++
					mu.Unlock()
					continue
				}
				if result["default"].Allowed {
					mu.Lock()
					allowedCount++
					mu.Unlock()
				}
			}
		})
	}

	wg.Wait()

	totalRequests := int64(numGoroutines * requestsPerGoroutine)
	assert.Equal(t, int64(0), errorCount, "Encountered %d errors during concurrent access", errorCount)

	// Should not have allowed more than burst + rate*time elapsed requests
	// This is a rough check - the exact number depends on timing
	if allowedCount > int64(config.Burst) {
		t.Logf("Allowed %d requests out of %d total", allowedCount, totalRequests)
	}
}

// TestGCRA_RateLimiting tests actual rate limiting over time
func TestGCRA_RateLimiting(t *testing.T) {
	ctx := context.Background()
	storage := memory.New()
	defer storage.Close()

	strategy := gcra.New(storage)
	config := gcra.Config{
		Key:   "test_rate",
		Rate:  2.0, // 2 requests per second
		Burst: 1,   // Small burst
	}

	// Consume burst first
	result, err := strategy.Allow(ctx, config)
	require.NoError(t, err, "Unexpected error")
	assert.True(t, result["default"].Allowed, "Expected first request to be allowed")

	// Should be denied now
	result, err = strategy.Allow(ctx, config)
	require.NoError(t, err, "Unexpected error")
	assert.False(t, result["default"].Allowed, "Expected request to be denied")

	// Wait for rate to allow new request
	time.Sleep(600 * time.Millisecond) // More than 500ms interval (2.0 req/sec = 500ms per request)

	// Should now be allowed (enough time passed for 1 request)
	result, err = strategy.Allow(ctx, config)
	require.NoError(t, err, "Unexpected error")
	assert.True(t, result["default"].Allowed, "Expected request to be allowed after rate interval")

	// Should be denied again immediately (no burst left)
	result, err = strategy.Allow(ctx, config)
	require.NoError(t, err, "Unexpected error")
	assert.False(t, result["default"].Allowed, "Expected request to be denied again")
}

// TestGCRA_ErrorHandling tests error scenarios
func TestGCRA_ErrorHandling(t *testing.T) {
	ctx := context.Background()
	storage := memory.New()
	defer storage.Close()

	strategy := gcra.New(storage)

	// Test invalid config
	invalidConfig := gcra.Config{
		Key:   "test_invalid",
		Rate:  -1.0, // Invalid rate
		Burst: 5,
	}

	err := invalidConfig.Validate()
	assert.Error(t, err, "Expected error for invalid rate")

	// Test context cancellation
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // Cancel immediately

	validConfig := gcra.Config{
		Key:   "test_cancel",
		Rate:  10.0,
		Burst: 5,
	}

	_, err = strategy.Allow(cancelCtx, validConfig)
	assert.Error(t, err, "Expected error for cancelled context")
	assert.True(t, errors.Is(err, context.Canceled), "Expected context.Canceled error, got %v", err)
}

// TestGCRA_MultipleKeys tests GCRA with different keys
func TestGCRA_MultipleKeys(t *testing.T) {
	ctx := context.Background()
	storage := memory.New()
	defer storage.Close()

	strategy := gcra.New(storage)

	configs := []gcra.Config{
		{Key: "user1", Rate: 5.0, Burst: 3},
		{Key: "user2", Rate: 2.0, Burst: 2},
		{Key: "user3", Rate: 10.0, Burst: 5},
	}

	// Test that different keys work independently
	for i, config := range configs {
		// Consume burst for each user
		for j := 0; j < config.Burst; j++ {
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err, "Unexpected error for user %d", i+1)
			assert.True(t, result["default"].Allowed, "Expected user %d request %d to be allowed", i+1, j+1)
		}

		// Should be denied for each user
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err, "Unexpected error for user %d", i+1)
		assert.False(t, result["default"].Allowed, "Expected user %d to be denied after burst", i+1)
	}
}

// TestGCRA_BackendCompatibility tests GCRA with different backends
func TestGCRA_BackendCompatibility(t *testing.T) {
	ctx := context.Background()

	backends := []string{"memory", "redis", "postgres"}

	for _, backendName := range backends {
		t.Run(backendName+"Backend", func(t *testing.T) {
			storage := UseBackend(t, backendName)
			defer storage.Close()

			strategy := gcra.New(storage)
			config := gcra.Config{Key: "test_" + backendName, Rate: 10.0, Burst: 5}

			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err, "%s backend error", backendName)
			assert.True(t, result["default"].Allowed, "%s backend should allow request", backendName)
		})
	}
}
