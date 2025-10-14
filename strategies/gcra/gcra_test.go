package gcra

import (
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGCRA_Allow(t *testing.T) {
	// Test initial state should allow requests
	t.Run("initial state should allow requests", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := t.Context()
			storage := memory.New()
			strategy := New(storage)

			config := Config{
				Key:   "test_initial",
				Rate:  10.0, // 10 requests per second
				Burst: 10,   // burst size of 10
			}

			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed)
		})
	})

	// Test should respect burst limit
	t.Run("should respect burst limit", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := t.Context()
			storage := memory.New()
			strategy := New(storage)

			config := Config{
				Key:   "test_burst",
				Rate:  5.0, // 5 requests per second
				Burst: 3,   // burst size of 3
			}

			// First 3 requests should be allowed
			for i := range 3 {
				result, err := strategy.Allow(ctx, config)
				require.NoError(t, err)
				assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
			}

			// 4th request should be denied
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.False(t, result["default"].Allowed)
		})
	})

	// Test should handle multiple keys independently
	t.Run("should handle multiple keys independently", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := t.Context()
			storage := memory.New()
			strategy := New(storage)

			config1 := Config{
				Key:   "user1",
				Rate:  2.0, // 2 requests per second
				Burst: 2,   // burst size of 2
			}
			config2 := Config{
				Key:   "user2",
				Rate:  2.0, // 2 requests per second
				Burst: 2,   // burst size of 2
			}

			// Use all burst for key1
			for i := range 2 {
				result, err := strategy.Allow(ctx, config1)
				require.NoError(t, err)
				assert.True(t, result["default"].Allowed, "Key1 request %d should be allowed", i)
			}

			// key1 should be denied
			result, err := strategy.Allow(ctx, config1)
			require.NoError(t, err)
			assert.False(t, result["default"].Allowed)

			// key2 should still be allowed
			for i := range 2 {
				result, err := strategy.Allow(ctx, config2)
				require.NoError(t, err)
				assert.True(t, result["default"].Allowed, "Key2 request %d should be allowed", i)
			}

			// key2 should now be denied
			result, err = strategy.Allow(ctx, config2)
			require.NoError(t, err)
			assert.False(t, result["default"].Allowed)
		})
	})

	// Test gradual recovery after burst exhaustion
	t.Run("gradual recovery after burst exhaustion", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := t.Context()
			storage := memory.New()
			strategy := New(storage)

			config := Config{
				Key:   "test_recovery",
				Rate:  10.0, // 10 requests per second
				Burst: 5,    // burst size of 5
			}

			// Use all burst capacity
			for i := range 5 {
				result, err := strategy.Allow(ctx, config)
				require.NoError(t, err)
				assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
			}

			// Next request should be denied
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.False(t, result["default"].Allowed)

			// Wait for emission interval (100ms for 10 req/s)
			time.Sleep(150 * time.Millisecond)
			synctest.Wait()

			// Should allow one request after waiting
			result, err = strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Should allow one request after waiting emission interval")

			// Next request should be denied again
			result, err = strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.False(t, result["default"].Allowed, "Should be denied again immediately after one request")
		})
	})

	// Test high rate limiting
	t.Run("high rate limiting", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := t.Context()
			storage := memory.New()
			strategy := New(storage)

			config := Config{
				Key:   "test_high_rate",
				Rate:  100.0, // 100 requests per second
				Burst: 10,    // burst size of 10
			}

			// Should allow burst capacity
			for i := range 10 {
				result, err := strategy.Allow(ctx, config)
				require.NoError(t, err)
				assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
			}

			// Should be denied after burst
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.False(t, result["default"].Allowed)

			// Wait for one emission interval (10ms for 100 req/s)
			time.Sleep(15 * time.Millisecond)
			synctest.Wait()

			// Should allow one request
			result, err = strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed)
		})
	})

	// Test low rate limiting
	t.Run("low rate limiting", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := t.Context()
			storage := memory.New()
			strategy := New(storage)

			config := Config{
				Key:   "test_low_rate",
				Rate:  0.5, // 0.5 requests per second (1 request every 2 seconds)
				Burst: 2,   // burst size of 2
			}

			// Should allow burst capacity
			for i := range 2 {
				result, err := strategy.Allow(ctx, config)
				require.NoError(t, err)
				assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
			}

			// Should be denied after burst
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.False(t, result["default"].Allowed)

			// Wait for emission interval (2 seconds for 0.5 req/s)
			time.Sleep(2100 * time.Millisecond)
			synctest.Wait()

			// Should allow one request after waiting
			result, err = strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed)
		})
	})
}

func TestGCRA_GetResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		storage := memory.New()
		strategy := New(storage)

		config := Config{
			Key:   "result-test-key",
			Rate:  10.0, // 10 requests per second
			Burst: 10,   // burst size of 10
		}

		// Test GetResult with no existing data
		result, err := strategy.GetResult(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Result should be allowed initially")
		assert.Equal(t, 10, result["default"].Remaining, "Remaining should be 10 initially")

		// Make some requests
		for i := range 5 {
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
		}

		// Test GetResult after requests
		result, err = strategy.GetResult(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Result should still be allowed")
		assert.Equal(t, 5, result["default"].Remaining, "Remaining should be 5 after 5 requests")

		// Use all burst capacity
		for i := range 5 {
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Request %d should be allowed", i+5)
		}

		// Next request should be denied
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Request should be denied when burst is exhausted")
	})
}

func TestGCRA_Reset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		storage := memory.New()
		strategy := New(storage)

		config := Config{
			Key:   "reset-test-key",
			Rate:  5.0, // 5 requests per second
			Burst: 3,   // burst size of 3
		}

		// Use all burst capacity
		for i := range 3 {
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
		}

		// Next request should be denied
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Request should be denied (burst exhausted)")

		// Reset the GCRA state
		err = strategy.Reset(ctx, config)
		require.NoError(t, err)

		// After reset, requests should be allowed again
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Request after reset should be allowed")
	})
}

func TestGCRA_ConcurrentAccess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := memory.New()
		strategy := New(storage)

		config := Config{
			Key:   "concurrent-key",
			Rate:  10.0, // 10 requests per second
			Burst: 5,    // burst size of 5
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

		// Exactly 5 requests should be allowed (the burst size)
		assert.Equal(t, 5, allowedCount, "Exactly 5 requests should be allowed")

		// Verify that we can't make any more requests immediately
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "11th request should be denied")
	})
}

func TestGCRA_EmissionIntervalCalculation(t *testing.T) {
	// Test emission interval calculation for various rates
	testCases := []struct {
		rate             float64
		expectedInterval time.Duration
		tolerance        time.Duration
	}{
		{1.0, 1 * time.Second, 10 * time.Millisecond},
		{10.0, 100 * time.Millisecond, 1 * time.Millisecond},
		{100.0, 10 * time.Millisecond, 100 * time.Microsecond},
		{0.5, 2 * time.Second, 10 * time.Millisecond},
		{0.1, 10 * time.Second, 100 * time.Millisecond},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("rate %.1f", tc.rate), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := t.Context()
				storage := memory.New()
				strategy := New(storage)

				config := Config{
					Key:   fmt.Sprintf("test_rate_%.1f", tc.rate),
					Rate:  tc.rate,
					Burst: 2,
				}

				// Use burst capacity
				for i := range 2 {
					result, err := strategy.Allow(ctx, config)
					require.NoError(t, err)
					assert.True(t, result["default"].Allowed, "Initial burst request %d should be allowed", i)
				}

				// Should be denied
				result, err := strategy.Allow(ctx, config)
				require.NoError(t, err)
				assert.False(t, result["default"].Allowed)

				// Wait for calculated emission interval
				time.Sleep(tc.expectedInterval + tc.tolerance)
				synctest.Wait()

				// Should allow one request
				result, err = strategy.Allow(ctx, config)
				require.NoError(t, err)
				assert.True(t, result["default"].Allowed, "Should allow request after emission interval")
			})
		})
	}
}
