package strategies

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	_ "github.com/ajiwo/ratelimit/backends/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixedWindow_Allow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := testCreateMemoryStorage(t)
		strategy := NewFixedWindow(storage)

		config := FixedWindowConfig{
			RateLimitConfig: RateLimitConfig{
				Key:   "test-key",
				Limit: 5,
			},
			Window: time.Minute,
		}

		ctx := t.Context()

		// First 5 requests should be allowed
		for i := range 5 {
			allowed, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, allowed, "Request %d should be allowed", i)
		}

		// 6th request should be denied
		allowed, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, allowed, "6th request should be denied")
	})
}

func TestFixedWindow_WindowReset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := testCreateMemoryStorage(t)
		strategy := NewFixedWindow(storage)

		config := FixedWindowConfig{
			RateLimitConfig: RateLimitConfig{
				Key:   "test-key",
				Limit: 2,
			},
			Window: time.Second, // Window duration
		}

		ctx := t.Context()

		// Use up the limit
		for i := range 2 {
			allowed, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, allowed, "Request %d should be allowed", i)
		}

		// 3rd request should be denied
		allowed, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, allowed, "3rd request should be denied")

		// Advance time past the window duration
		time.Sleep(1100 * time.Millisecond)
		synctest.Wait()

		// New request should be allowed in new window
		allowed, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, allowed, "Request in new window should be allowed")
	})
}

func TestFixedWindow_MultipleKeys(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := testCreateMemoryStorage(t)
		strategy := NewFixedWindow(storage)

		config1 := FixedWindowConfig{
			RateLimitConfig: RateLimitConfig{
				Key:   "user1",
				Limit: 1,
			},
			Window: time.Minute,
		}

		config2 := FixedWindowConfig{
			RateLimitConfig: RateLimitConfig{
				Key:   "user2",
				Limit: 1,
			},
			Window: time.Minute,
		}

		ctx := t.Context()

		// First request for user1 should be allowed
		allowed, err := strategy.Allow(ctx, config1)
		require.NoError(t, err)
		assert.True(t, allowed, "First request for user1 should be allowed")

		// Second request for user1 should be denied
		allowed, err = strategy.Allow(ctx, config1)
		require.NoError(t, err)
		assert.False(t, allowed, "Second request for user1 should be denied")

		// First request for user2 should be allowed (different key)
		allowed, err = strategy.Allow(ctx, config2)
		require.NoError(t, err)
		assert.True(t, allowed, "First request for user2 should be allowed")
	})
}

func TestFixedWindow_InvalidConfig(t *testing.T) {
	storage := testCreateMemoryStorage(t)
	strategy := NewFixedWindow(storage)

	ctx := t.Context()

	// Test with wrong config type
	allowed, err := strategy.Allow(ctx, TokenBucketConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires FixedWindowConfig")
	assert.False(t, allowed)
}

func TestFixedWindow_ZeroLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := testCreateMemoryStorage(t)
		strategy := NewFixedWindow(storage)

		config := FixedWindowConfig{
			RateLimitConfig: RateLimitConfig{
				Key:   "test-key",
				Limit: 0,
			},
			Window: time.Minute,
		}

		ctx := t.Context()

		// Any request should be denied when limit is 0
		allowed, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, allowed, "Request should be denied when limit is 0")
	})
}

func TestFixedWindow_GetResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := testCreateMemoryStorage(t)
		strategy := NewFixedWindow(storage)

		config := FixedWindowConfig{
			RateLimitConfig: RateLimitConfig{
				Key:   "result-test-key",
				Limit: 5,
			},
			Window: time.Minute,
		}

		ctx := t.Context()

		// Test GetResult with no existing data
		result, err := strategy.GetResult(ctx, config)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "Result should be allowed initially")
		assert.Equal(t, 5, result.Remaining, "Remaining should be 5 initially")
		assert.WithinDuration(t, time.Now().Add(time.Minute), result.Reset, time.Second, "Reset time should be approximately 1 minute from now")

		// Make some requests
		for range 3 {
			allowed, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, allowed, "Request should be allowed")
		}

		// Test GetResult after requests
		result, err = strategy.GetResult(ctx, config)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "Result should still be allowed")
		assert.Equal(t, 2, result.Remaining, "Remaining should be 2 after 3 requests")

		// Make remaining requests to hit the limit
		for range 2 {
			allowed, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, allowed, "Request should be allowed")
		}

		// Next request should be denied
		allowed, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, allowed, "Request should be denied")

		// Test GetResult when at limit
		result, err = strategy.GetResult(ctx, config)
		require.NoError(t, err)
		assert.False(t, result.Allowed, "Result should not be allowed when at limit")
		assert.Equal(t, 0, result.Remaining, "Remaining should be 0 when at limit")

		// Test invalid config type
		_, err = strategy.GetResult(ctx, TokenBucketConfig{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires FixedWindowConfig")
	})
}

func TestFixedWindow_Reset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := testCreateMemoryStorage(t)
		strategy := NewFixedWindow(storage)

		config := FixedWindowConfig{
			RateLimitConfig: RateLimitConfig{
				Key:   "reset-test-key",
				Limit: 2,
			},
			Window: time.Minute,
		}

		ctx := t.Context()

		// Make requests to use up the limit
		for i := range 2 {
			allowed, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, allowed, "Request %d should be allowed", i)
		}

		// Next request should be denied
		allowed, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, allowed, "Request should be denied (limit exceeded)")

		// Reset the counter
		err = strategy.Reset(ctx, config)
		require.NoError(t, err)

		// After reset, requests should be allowed again
		allowed, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, allowed, "Request after reset should be allowed")

		// Test invalid config type
		err = strategy.Reset(ctx, TokenBucketConfig{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires FixedWindowConfig")
	})
}

func TestFixedWindow_ConcurrentAccess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := testCreateMemoryStorage(t)
		strategy := NewFixedWindow(storage)

		config := FixedWindowConfig{
			RateLimitConfig: RateLimitConfig{
				Key:   "concurrent-key",
				Limit: 5,
			},
			Window: time.Minute,
		}

		ctx := t.Context()

		// Start multiple goroutines that will try to make requests concurrently
		results := make(chan bool, 10)
		waitGroup := &sync.WaitGroup{}

		// Launch 10 goroutines that will all try to make a request
		for range 10 {
			waitGroup.Go(func() {
				allowed, err := strategy.Allow(ctx, config)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
				results <- allowed
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
		allowed, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, allowed, "11th request should be denied")
	})
}

func TestFixedWindow_PreciseTiming(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := testCreateMemoryStorage(t)
		strategy := NewFixedWindow(storage)

		config := FixedWindowConfig{
			RateLimitConfig: RateLimitConfig{
				Key:   "timing-key",
				Limit: 3,
			},
			Window: 5 * time.Second,
		}

		ctx := t.Context()

		// Make requests at different times within the window
		allowed, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, allowed, "First request should be allowed")

		// Advance time by 2 seconds
		time.Sleep(2 * time.Second)
		synctest.Wait()

		allowed, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, allowed, "Second request should be allowed")

		// Advance time by 2 more seconds (4 seconds total)
		time.Sleep(2 * time.Second)
		synctest.Wait()

		allowed, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, allowed, "Third request should be allowed")

		// Advance time by 1 more second (5 seconds total, window should reset)
		time.Sleep(1 * time.Second)
		synctest.Wait()

		// Wait a bit more to ensure window reset
		time.Sleep(1 * time.Millisecond)
		synctest.Wait()

		// This request should be allowed because we're in a new window
		allowed, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, allowed, "Fourth request should be allowed in new window")

		// Use up the rest of the limit in the new window
		allowed, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, allowed, "Fifth request should be allowed")

		allowed, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, allowed, "Sixth request should be allowed")

		// This one should be denied
		allowed, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, allowed, "Seventh request should be denied")
	})
}
