package tokenbucket

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

func TestTokenBucket_GetResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		storage := memory.New()
		strategy := New(storage)

		config := strategies.TokenBucketConfig{
			Key:        "result-test-key",
			BurstSize:  10,
			RefillRate: 10.0, // 10 tokens per second
		}

		// Test GetResult with no existing data
		result, err := strategy.GetResult(ctx, config)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "Result should be allowed initially")
		assert.Equal(t, 10, result.Remaining, "Remaining should be 10 initially")

		// Make some requests
		for i := range 5 {
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "Request %d should be allowed", i)
		}

		// Test GetResult after requests
		result, err = strategy.GetResult(ctx, config)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "Result should still be allowed")
		assert.Equal(t, 5, result.Remaining, "Remaining should be 5 after 5 requests")

		// Use all tokens
		for i := range 5 {
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "Request %d should be allowed", i+5)
		}

		// Next request should be denied
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result.Allowed, "Request should be denied when no tokens")

		// Test invalid config type
		_, err = strategy.GetResult(ctx, struct{}{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires TokenBucketConfig")
	})
}

func TestTokenBucket_Reset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		storage := memory.New()
		strategy := New(storage)

		config := strategies.TokenBucketConfig{
			Key:        "reset-test-key",
			BurstSize:  4,
			RefillRate: 1.0, // 1 token per second
		}

		// Use all tokens
		for i := range 4 {
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "Request %d should be allowed", i)
		}

		// Next request should be denied
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result.Allowed, "Request should be denied (no tokens)")

		// Reset the bucket
		err = strategy.Reset(ctx, config)
		require.NoError(t, err)

		// After reset, requests should be allowed again
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "Request after reset should be allowed")

		// Test invalid config type
		err = strategy.Reset(ctx, struct{}{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires TokenBucketConfig")
	})
}

func TestTokenBucket_Allow(t *testing.T) {
	// Test initial bucket should allow requests
	t.Run("initial bucket should allow requests", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := t.Context()
			storage := memory.New()
			strategy := New(storage)

			config := strategies.TokenBucketConfig{
				Key:        "test_initial",
				BurstSize:  10,
				RefillRate: 10.0, // 10 tokens per second
			}

			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result.Allowed)
		})
	})

	// Test should respect capacity limit
	t.Run("should respect capacity limit", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := t.Context()
			storage := memory.New()
			strategy := New(storage)

			config := strategies.TokenBucketConfig{
				Key:        "test_capacity",
				BurstSize:  3,
				RefillRate: 1.0, // 1 token per second
			}

			// First 3 requests should be allowed
			for i := range 3 {
				result, err := strategy.Allow(ctx, config)
				require.NoError(t, err)
				assert.True(t, result.Allowed, "Request %d should be allowed", i)
			}

			// 4th request should be denied
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.False(t, result.Allowed)
		})
	})

	// Test basic refill functionality
	t.Run("basic refill functionality", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := t.Context()
			storage := memory.New()
			strategy := New(storage)

			config := strategies.TokenBucketConfig{
				Key:        "test_refill",
				BurstSize:  3,
				RefillRate: 1.0, // 1 token per second
			}

			// Use all tokens
			for i := range 3 {
				result, err := strategy.Allow(ctx, config)
				require.NoError(t, err)
				assert.True(t, result.Allowed, "Request %d should be allowed", i)
			}

			// Next request should be denied
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.False(t, result.Allowed)

			// Wait for a significant time to ensure refill
			time.Sleep(1500 * time.Millisecond)
			synctest.Wait()

			// At least one request should be allowed after waiting
			result, err = strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "At least one request should be allowed after refill")
		})
	})

	// Test should handle multiple keys independently
	t.Run("should handle multiple keys independently", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := t.Context()
			storage := memory.New()
			strategy := New(storage)

			config1 := strategies.TokenBucketConfig{
				Key:        "user1",
				BurstSize:  2,
				RefillRate: 2.0, // 2 tokens per second
			}
			config2 := strategies.TokenBucketConfig{
				Key:        "user2",
				BurstSize:  2,
				RefillRate: 2.0, // 2 tokens per second
			}

			// Use all tokens for key1
			for i := range 2 {
				result, err := strategy.Allow(ctx, config1)
				require.NoError(t, err)
				assert.True(t, result.Allowed, "Key1 request %d should be allowed", i)
			}

			// key1 should be denied
			result, err := strategy.Allow(ctx, config1)
			require.NoError(t, err)
			assert.False(t, result.Allowed)

			// key2 should still be allowed
			for i := range 2 {
				result, err := strategy.Allow(ctx, config2)
				require.NoError(t, err)
				assert.True(t, result.Allowed, "Key2 request %d should be allowed", i)
			}

			// key2 should now be denied
			result, err = strategy.Allow(ctx, config2)
			require.NoError(t, err)
			assert.False(t, result.Allowed)
		})
	})

	// Test fractional refill rate functionality
	t.Run("fractional refill rate functionality", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := t.Context()
			storage := memory.New()
			strategy := New(storage)

			config := strategies.TokenBucketConfig{
				Key:        "test_fractional",
				BurstSize:  10,
				RefillRate: 2.0, // 2 tokens per second
			}

			// Use all tokens
			for i := range 10 {
				result, err := strategy.Allow(ctx, config)
				require.NoError(t, err)
				assert.True(t, result.Allowed, "Request %d should be allowed", i)
			}

			// Next request should be denied
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.False(t, result.Allowed)

			// Wait for some time to get fractional tokens
			time.Sleep(1200 * time.Millisecond)
			synctest.Wait()

			// At least one request should be allowed after waiting
			result, err = strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "At least one request should be allowed after fractional refill")
		})
	})
}

func TestTokenBucket_ConcurrentAccess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := memory.New()
		strategy := New(storage)

		config := strategies.TokenBucketConfig{
			Key:        "concurrent-key",
			BurstSize:  5,
			RefillRate: 5.0,
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
				results <- result.Allowed
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
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result.Allowed, "11th request should be denied")
	})
}
