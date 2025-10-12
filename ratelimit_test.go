package ratelimit

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiter_Allow_FixedWindow(t *testing.T) {
	backend := memory.New()

	limiter, err := New(
		WithBackend(backend),
		WithPrimaryStrategy(strategies.NewFixedWindowConfig("test:user:123", 5, time.Minute)),
		WithBaseKey("test"),
	)
	require.NoError(t, err)
	defer limiter.Close()

	// Should allow first 5 requests
	for i := range 5 {
		allowed, err := limiter.Allow(WithContext(context.Background()))
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// Should deny 6th request
	allowed, err := limiter.Allow(WithContext(context.Background()))
	require.NoError(t, err)
	assert.False(t, allowed, "Request 6 should be denied")
}

func TestRateLimiter_Allow_TokenBucket(t *testing.T) {
	backend := memory.New()

	limiter, err := New(
		WithBackend(backend),
		WithPrimaryStrategy(strategies.TokenBucketConfig{
			Key:        "test:user:456",
			BurstSize:  10,
			RefillRate: 1.0,
		}),
		WithBaseKey("test"),
	)
	require.NoError(t, err)
	defer limiter.Close()

	// Should allow first 10 requests (burst)
	for i := range 10 {
		allowed, err := limiter.Allow(WithContext(context.Background()))
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// Should deny 11th request
	allowed, err := limiter.Allow(WithContext(context.Background()))
	require.NoError(t, err)
	assert.False(t, allowed, "Request 11 should be denied")
}

func TestRateLimiter_Allow_LeakyBucket(t *testing.T) {
	backend := memory.New()

	limiter, err := New(
		WithBackend(backend),
		WithPrimaryStrategy(strategies.LeakyBucketConfig{
			Key:      "test:user:789",
			Capacity: 5,
			LeakRate: 1.0,
		}),
		WithBaseKey("test"),
	)
	require.NoError(t, err)
	defer limiter.Close()

	// Should allow first 5 requests (capacity)
	for i := range 5 {
		allowed, err := limiter.Allow(WithContext(context.Background()))
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// Should deny 6th request
	allowed, err := limiter.Allow(WithContext(context.Background()))
	require.NoError(t, err)
	assert.False(t, allowed, "Request 6 should be denied")
}

func TestRateLimiter_GetStats(t *testing.T) {
	backend := memory.New()

	limiter, err := New(
		WithBackend(backend),
		WithPrimaryStrategy(strategies.TokenBucketConfig{
			Key:        "test:stats:user",
			BurstSize:  10,
			RefillRate: 2.0,
		}),
		WithBaseKey("test"),
	)
	require.NoError(t, err)
	defer limiter.Close()

	// Get initial stats
	stats, err := limiter.GetStats(WithContext(context.Background()))
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, 10, stats["primary_default"].Remaining)

	// Make some requests
	for i := range 5 {
		allowed, err := limiter.Allow(WithContext(context.Background()))
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// Check stats again
	stats, err = limiter.GetStats(WithContext(context.Background()))
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, 5, stats["primary_default"].Remaining)
}

func TestRateLimiter_Reset(t *testing.T) {
	backend := memory.New()

	limiter, err := New(
		WithBackend(backend),
		WithPrimaryStrategy(strategies.TokenBucketConfig{
			Key:        "test:reset:user",
			BurstSize:  5,
			RefillRate: 1.0,
		}),
		WithBaseKey("test"),
	)
	require.NoError(t, err)
	defer limiter.Close()

	// Consume all tokens
	for i := range 5 {
		allowed, err := limiter.Allow(WithContext(context.Background()))
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// Should be denied
	allowed, err := limiter.Allow(WithContext(context.Background()))
	require.NoError(t, err)
	assert.False(t, allowed)

	// Reset the limiter
	err = limiter.Reset(WithContext(context.Background()))
	require.NoError(t, err)

	// Should be allowed again
	allowed, err = limiter.Allow(WithContext(context.Background()))
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	backend := memory.New()

	limiter, err := New(
		WithBackend(backend),
		WithPrimaryStrategy(strategies.NewFixedWindowConfig("test:concurrent:user", 100, time.Minute)),
		WithBaseKey("test"),
	)
	require.NoError(t, err)
	defer limiter.Close()

	var wg sync.WaitGroup
	var mu sync.Mutex
	allowedCount := 0
	totalRequests := 200

	// Launch multiple goroutines making requests
	for i := range totalRequests {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			allowed, err := limiter.Allow(
				WithContext(context.Background()),
				WithKey("concurrent"),
			)
			if err == nil && allowed {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// Should allow exactly 100 requests
	assert.Equal(t, 100, allowedCount)
}

func TestRateLimiter_MultipleKeys(t *testing.T) {
	backend := memory.New()

	limiter, err := New(
		WithBackend(backend),
		WithPrimaryStrategy(strategies.NewFixedWindowConfig("test:multikey", 3, time.Minute)),
		WithBaseKey("test"),
	)
	require.NoError(t, err)
	defer limiter.Close()

	// Make requests with different keys
	// Key "user1"
	for i := range 3 {
		allowed, err := limiter.Allow(
			WithContext(context.Background()),
			WithKey("user1"),
		)
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d for user1 should be allowed", i+1)
	}

	// Key "user1" should be denied now
	allowed, err := limiter.Allow(
		WithContext(context.Background()),
		WithKey("user1"),
	)
	require.NoError(t, err)
	assert.False(t, allowed)

	// Key "user2" should still be allowed
	allowed, err = limiter.Allow(
		WithContext(context.Background()),
		WithKey("user2"),
	)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestRateLimiter_TimeBehavior(t *testing.T) {
	backend := memory.New()

	limiter, err := New(
		WithBackend(backend),
		WithPrimaryStrategy(strategies.NewFixedWindowConfig("test:time:user", 3, 2*time.Second)),
		WithBaseKey("test"),
	)
	require.NoError(t, err)
	defer limiter.Close()

	// Consume all allowed requests
	for i := range 3 {
		allowed, err := limiter.Allow(WithContext(context.Background()))
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// Should be denied
	allowed, err := limiter.Allow(WithContext(context.Background()))
	require.NoError(t, err)
	assert.False(t, allowed)

	// Wait for window to reset
	time.Sleep(3 * time.Second)

	// Should be allowed again
	allowed, err = limiter.Allow(WithContext(context.Background()))
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestRateLimiter_InvalidConfiguration(t *testing.T) {
	t.Run("No primary strategy", func(t *testing.T) {
		backend := memory.New()

		_, err := New(
			WithBackend(backend),
			WithBaseKey("test"),
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "primary strategy config cannot be nil")
	})

	t.Run("Invalid strategy config", func(t *testing.T) {
		backend := memory.New()

		_, err := New(
			WithBackend(backend),
			WithPrimaryStrategy(strategies.TokenBucketConfig{
				BurstSize:  0, // Invalid
				RefillRate: 1.0,
			}),
		)
		assert.Error(t, err)
	})

	t.Run("Invalid dual strategy config", func(t *testing.T) {
		backend := memory.New()

		_, err := New(
			WithBackend(backend),
			WithPrimaryStrategy(strategies.TokenBucketConfig{
				BurstSize:  10,
				RefillRate: 1.0,
			}),
			WithSecondaryStrategy(strategies.LeakyBucketConfig{
				Capacity: 5,
				LeakRate: 1.0,
			}),
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot use strategy with secondary capability as primary when secondary strategy is also specified")
	})
}

func TestRateLimiter_BackendOperations(t *testing.T) {
	backend := memory.New()

	limiter, err := New(
		WithBackend(backend),
		WithPrimaryStrategy(strategies.NewFixedWindowConfig("test:backend", 10, time.Minute)),
		WithBaseKey("test"),
	)
	require.NoError(t, err)

	// Test getting backend
	retrievedBackend := limiter.GetBackend()
	assert.Equal(t, backend, retrievedBackend)

	// Test getting config
	config := limiter.GetConfig()
	assert.Equal(t, "test", config.BaseKey)
	assert.Equal(t, backend, config.Storage)
	assert.Equal(t, "fixed_window", config.PrimaryConfig.Name())

	// Test closing
	err = limiter.Close()
	assert.NoError(t, err)
}

func TestRateLimiter_MixedStrategyTypes(t *testing.T) {
	tests := []struct {
		name      string
		primary   strategies.StrategyConfig
		secondary strategies.StrategyConfig
		option    Option
	}{
		{
			name:    "Fixed Window only",
			primary: strategies.NewFixedWindowConfig("test", 10, time.Minute),
			option:  WithPrimaryStrategy(strategies.NewFixedWindowConfig("test", 10, time.Minute)),
		},
		{
			name:    "Token Bucket only",
			primary: strategies.TokenBucketConfig{Key: "test", BurstSize: 10, RefillRate: 1.0},
			option:  WithPrimaryStrategy(strategies.TokenBucketConfig{Key: "test", BurstSize: 10, RefillRate: 1.0}),
		},
		{
			name:    "Leaky Bucket only",
			primary: strategies.LeakyBucketConfig{Key: "test", Capacity: 10, LeakRate: 1.0},
			option:  WithPrimaryStrategy(strategies.LeakyBucketConfig{Key: "test", Capacity: 10, LeakRate: 1.0}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := memory.New()

			limiter, err := New(
				WithBackend(backend),
				tt.option,
				WithBaseKey("test:mixed"),
			)
			require.NoError(t, err)
			defer limiter.Close()

			// Should allow at least one request
			allowed, err := limiter.Allow(WithContext(context.Background()))
			require.NoError(t, err)
			assert.True(t, allowed)
		})
	}
}

func TestRateLimiter_DualStrategy(t *testing.T) {
	backend := memory.New()

	// Test dual strategy: Fixed Window (primary) + Token Bucket (secondary)
	limiter, err := New(
		WithBackend(backend),
		WithPrimaryStrategy(strategies.NewFixedWindowConfig("test:dual", 10, time.Minute)),
		WithSecondaryStrategy(strategies.TokenBucketConfig{
			BurstSize:  5,
			RefillRate: 0.5,
		}),
		WithBaseKey("test"),
	)
	require.NoError(t, err)
	defer limiter.Close()

	// Test burst behavior - should be limited by secondary (5 burst)
	for i := range 5 {
		allowed, err := limiter.Allow(WithContext(context.Background()))
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// 6th request should be denied by secondary strategy
	allowed, err := limiter.Allow(WithContext(context.Background()))
	require.NoError(t, err)
	assert.False(t, allowed, "Request 6 should be denied by secondary strategy")

	// Get stats to verify both strategies are working
	stats, err := limiter.GetStats(WithContext(context.Background()))
	require.NoError(t, err)
	require.Len(t, stats, 2)
	assert.Contains(t, stats, "primary_default")
	assert.Contains(t, stats, "secondary_default")
}

func TestRateLimiter_DualStrategy_WithResults(t *testing.T) {
	backend := memory.New()

	limiter, err := New(
		WithBackend(backend),
		WithPrimaryStrategy(strategies.NewFixedWindowConfig("test:dual:results", 20, time.Minute)),
		WithSecondaryStrategy(strategies.TokenBucketConfig{
			BurstSize:  3,
			RefillRate: 1.0,
		}),
		WithBaseKey("test"),
	)
	require.NoError(t, err)
	defer limiter.Close()

	// Test with results
	var stats1 map[string]strategies.Result
	allowed, err := limiter.Allow(WithResult(&stats1))
	require.NoError(t, err)
	assert.True(t, allowed)
	require.Len(t, stats1, 2)
	assert.Contains(t, stats1, "primary_default")
	assert.Contains(t, stats1, "secondary_default")

	var stats2 map[string]strategies.Result
	allowed, err = limiter.Allow(WithResult(&stats2))
	require.NoError(t, err)
	assert.True(t, allowed)
	require.Len(t, stats2, 2)
}

func TestDualStrategy_QuotaConsumption(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mem := memory.New()

		// Create dual strategy limiter: Fixed Window (primary) + Token Bucket (secondary)
		limiter, err := New(
			WithPrimaryStrategy(strategies.NewFixedWindowConfig("api:test", 3, time.Minute)),
			WithSecondaryStrategy(strategies.TokenBucketConfig{BurstSize: 1, RefillRate: 1.0}), // Very small burst (1), refill 1/sec
			WithBackend(mem),
		)
		require.NoError(t, err)
		require.NotNil(t, limiter)
		defer limiter.Close()

		ctx := t.Context()
		// Get initial stats
		stats, err := limiter.GetStats(WithContext(ctx))
		require.NoError(t, err)
		assert.Equal(t, 3, stats["primary_default"].Remaining)

		// Phase 1: Use up the token bucket burst capacity (1 request)
		allowed, err := limiter.Allow(WithContext(ctx))
		require.NoError(t, err)
		assert.True(t, allowed, "First request should be allowed")

		// Get initial stats
		stats, err = limiter.GetStats(WithContext(ctx))
		require.NoError(t, err)
		assert.Equal(t, 2, stats["primary_default"].Remaining, "Should have 2 remaining quota")

		// Phase 2: Request should be denied by secondary strategy, but primary quota preserved
		allowed, err = limiter.Allow(WithContext(ctx))
		require.NoError(t, err)
		assert.False(t, allowed, "Second request should be denied by secondary strategy")

		// Phase 3: Verify primary quota was NOT consumed
		stats, err = limiter.GetStats(WithContext(ctx))
		require.NoError(t, err)
		assert.True(t, stats["primary_default"].Allowed, "Primary strategy should still allow")
		assert.Equal(t, 2, stats["primary_default"].Remaining, "Should have 2 remaining quota")

		// The last `Allow` was denied by secondary
		assert.False(t, stats["secondary_default"].Allowed, "Secondary strategy should deny")
	})
}
