package fixedwindow

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixedWindow_Allow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := memory.New()
		strategy := New(storage)

		config := strategies.FixedWindowConfig{
			Key:    "test-key",
			Limit:  5,
			Window: time.Minute,
		}

		ctx := t.Context()

		// First 5 requests should be allowed
		for i := range 5 {
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
		}

		// 6th request should be denied
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "6th request should be denied")
	})
}

func TestFixedWindow_WindowReset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := memory.New()
		strategy := New(storage)

		config := strategies.FixedWindowConfig{
			Key:    "test-key",
			Limit:  2,
			Window: time.Second, // Window duration
		}

		ctx := t.Context()

		// Use up the limit
		for i := range 2 {
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
		}

		// 3rd request should be denied
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "3rd request should be denied")

		// Advance time past the window duration
		time.Sleep(1100 * time.Millisecond)
		synctest.Wait()

		// New request should be allowed in new window
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Request in new window should be allowed")
	})
}

func TestFixedWindow_MultipleKeys(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := memory.New()
		strategy := New(storage)

		config1 := strategies.FixedWindowConfig{
			Key:    "user1",
			Limit:  1,
			Window: time.Minute,
		}

		config2 := strategies.FixedWindowConfig{
			Key:    "user2",
			Limit:  1,
			Window: time.Minute,
		}

		ctx := t.Context()

		// First request for user1 should be allowed
		result, err := strategy.Allow(ctx, config1)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "First request for user1 should be allowed")

		// Second request for user1 should be denied
		result, err = strategy.Allow(ctx, config1)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Second request for user1 should be denied")

		// First request for user2 should be allowed (different key)
		result, err = strategy.Allow(ctx, config2)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "First request for user2 should be allowed")
	})
}

func TestFixedWindow_InvalidConfig(t *testing.T) {
	storage := memory.New()
	strategy := New(storage)

	ctx := t.Context()

	// Test with wrong config type
	result, err := strategy.Allow(ctx, struct{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires FixedWindowConfig")
	assert.False(t, result["default"].Allowed)
}

func TestFixedWindow_ZeroLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := memory.New()
		strategy := New(storage)

		config := strategies.FixedWindowConfig{
			Key:    "test-key",
			Limit:  0,
			Window: time.Minute,
		}

		ctx := t.Context()

		// Any request should be denied when limit is 0
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Request should be denied when limit is 0")
	})
}

func TestFixedWindow_GetResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := memory.New()
		strategy := New(storage)

		config := strategies.FixedWindowConfig{
			Key:    "result-test-key",
			Limit:  5,
			Window: time.Minute,
		}

		ctx := t.Context()

		// Test GetResult with no existing data
		result, err := strategy.GetResult(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Result should be allowed initially")
		assert.Equal(t, 5, result["default"].Remaining, "Remaining should be 5 initially")
		assert.WithinDuration(t, time.Now().Add(time.Minute), result["default"].Reset, time.Second, "Reset time should be approximately 1 minute from now")

		// Make some requests
		for range 3 {
			result, err = strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Request should be allowed")
		}

		// Test GetResult after requests
		result, err = strategy.GetResult(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Result should still be allowed")
		assert.Equal(t, 2, result["default"].Remaining, "Remaining should be 2 after 3 requests")

		// Make remaining requests to hit the limit
		for range 2 {
			result, err = strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Request should be allowed")
		}

		// Next request should be denied
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Request should be denied")

		// Test GetResult when at limit
		result, err = strategy.GetResult(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Result should not be allowed when at limit")
		assert.Equal(t, 0, result["default"].Remaining, "Remaining should be 0 when at limit")

		// Test invalid config type
		_, err = strategy.GetResult(ctx, struct{}{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires FixedWindowConfig")
	})
}

func TestFixedWindow_Reset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := memory.New()
		strategy := New(storage)

		config := strategies.FixedWindowConfig{
			Key:    "reset-test-key",
			Limit:  2,
			Window: time.Minute,
		}

		ctx := t.Context()

		// Make requests to use up the limit
		for i := range 2 {
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
		}

		// Next request should be denied
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Request should be denied (limit exceeded)")

		// Reset the counter
		err = strategy.Reset(ctx, config)
		require.NoError(t, err)

		// After reset, requests should be allowed again
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Request after reset should be allowed")

		// Test invalid config type
		err = strategy.Reset(ctx, struct{}{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires FixedWindowConfig")
	})
}

func TestFixedWindow_ConcurrentAccess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := memory.New()
		strategy := New(storage)

		config := strategies.FixedWindowConfig{
			Key:    "concurrent-key",
			Limit:  5,
			Window: time.Minute,
		}

		ctx := t.Context()

		// Start multiple goroutines that will try to make requests concurrently
		results := make(chan bool, 10)
		waitGroup := &sync.WaitGroup{}

		// Launch 10 goroutines that will all try to make a request
		for range 10 {
			waitGroup.Go(func() {
				result, err := strategy.Allow(ctx, config)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
				results <- result["default"].Allowed
			})
		}

		// Wait for all goroutines to complete
		waitGroup.Wait()
		close(results)

		// Collect results
		var allowedCount int
		for allowed := range results {
			if allowed {
				allowedCount++
			}
		}

		// Exactly 5 requests should be allowed (the limit)
		assert.Equal(t, 5, allowedCount, "Exactly 5 requests should be allowed")

		// Verify that we can't make any more requests
		result2, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result2["default"].Allowed, "11th request should be denied")
	})
}

func TestFixedWindow_PreciseTiming(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := memory.New()
		strategy := New(storage)

		config := strategies.FixedWindowConfig{
			Key:    "timing-key",
			Limit:  3,
			Window: 5 * time.Second,
		}

		ctx := t.Context()

		// Make requests at different times within the window
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "First request should be allowed")

		// Advance time by 2 seconds
		time.Sleep(2 * time.Second)
		synctest.Wait()

		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Second request should be allowed")

		// Advance time by 2 more seconds (4 seconds total)
		time.Sleep(2 * time.Second)
		synctest.Wait()

		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Third request should be allowed")

		// Advance time by 1 more second (5 seconds total, window should reset)
		time.Sleep(1 * time.Second)
		synctest.Wait()

		// Wait a bit more to ensure window reset
		time.Sleep(1 * time.Millisecond)
		synctest.Wait()

		// This request should be allowed because we're in a new window
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Fourth request should be allowed in new window")

		// Use up the rest of the limit in the new window
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Fifth request should be allowed")

		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Sixth request should be allowed")

		// This one should be denied
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Seventh request should be denied")
	})
}
