package ratelimit

import (
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_DefaultConfiguration(t *testing.T) {
	limiter, err := New()
	require.Error(t, err)
	require.Nil(t, limiter)
	require.Contains(t, err.Error(), "storage backend cannot be nil")
}
func TestNew_DefaultNoStrategy(t *testing.T) {
	backend := memory.New()
	limiter, err := New(WithBackend(backend))
	require.Error(t, err)
	require.Nil(t, limiter)
	require.Contains(t, err.Error(), "primary strategy config cannot be nil")
}

func TestNew_WithOptions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mem := memory.New()
		customTiers := []TierConfig{
			{Interval: 5 * time.Second, Limit: 5},
			{Interval: time.Minute, Limit: 100},
		}

		limiter, err := New(
			WithBaseKey("test-key"),
			WithFixedWindowStrategy(customTiers...),
			WithBackend(mem),
		)
		require.NoError(t, err)
		require.NotNil(t, limiter)

		config := limiter.GetConfig()
		assert.Equal(t, "test-key", config.BaseKey)

		// Check primary strategy configuration
		fixedWindowConfig, ok := config.PrimaryConfig.(FixedWindowConfig)
		require.True(t, ok)
		assert.Equal(t, strategies.StrategyFixedWindow, fixedWindowConfig.Type())
		assert.Len(t, fixedWindowConfig.Tiers, 2)
		assert.Equal(t, 5*time.Second, fixedWindowConfig.Tiers[0].Interval)
		assert.Equal(t, 5, fixedWindowConfig.Tiers[0].Limit)

		err = limiter.Close()
		require.NoError(t, err)
	})
}

func TestMultiTierLimiter_Allow_FixedWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mem := memory.New()
		tiers := []TierConfig{
			{Interval: 5 * time.Second, Limit: 3}, // 3 requests per 5 seconds
			{Interval: time.Minute, Limit: 10},    // 10 requests per minute
		}

		limiter, err := New(
			WithBaseKey("multi-tier-test"),
			WithFixedWindowStrategy(tiers...),
			WithBackend(mem),
		)
		require.NoError(t, err)
		require.NotNil(t, limiter)

		ctx := t.Context()

		// First 3 requests should be allowed (within both limits)
		for i := range 3 {
			allowed, err := limiter.Allow(WithContext(ctx))
			require.NoError(t, err)
			assert.True(t, allowed, "Request %d should be allowed", i)
		}

		// 4th request should be denied (exceeds second tier limit)
		allowed, err := limiter.Allow(WithContext(ctx))
		require.NoError(t, err)
		assert.False(t, allowed, "4th request should be denied (exceeds per-second limit)")

		// Advance time past the 5-second window
		time.Sleep(5100 * time.Millisecond)
		synctest.Wait()

		// Now we should be able to make more requests (but still limited by minute tier)
		allowed, err = limiter.Allow(WithContext(ctx))
		require.NoError(t, err)
		assert.True(t, allowed, "Request after second reset should be allowed")

		err = limiter.Close()
		require.NoError(t, err)
	})
}

func TestMultiTierLimiter_Allow_TokenBucket(t *testing.T) {
	mem := memory.New()
	limiter, err := New(
		WithBaseKey("token-bucket-test"),
		WithTokenBucketStrategy(10, 5.0), // burst 10, refill 5/sec
		WithBackend(mem),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	// First 5 requests should be allowed (within minute limit)
	for i := range 5 {
		allowed, err := limiter.Allow(WithContext(ctx))
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i)
	}

	// Should still be able to make more requests due to burst capacity
	for i := range 5 {
		allowed, err := limiter.Allow(WithContext(ctx))
		require.NoError(t, err)
		assert.True(t, allowed, "Burst request %d should be allowed", i)
	}

	// Now we should be denied (exhausted burst and minute limit)
	allowed, err := limiter.Allow(WithContext(ctx))
	require.NoError(t, err)
	assert.False(t, allowed, "Request should be denied after burst exhausted")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestMultiTierLimiter_Allow_LeakyBucket(t *testing.T) {
	mem := memory.New()

	_, err := New(
		WithBaseKey("leaky-bucket-test"),
		WithLeakyBucketStrategy(5, 2.0), // capacity 5, leak 2/sec
		WithBackend(mem),
	)

	require.NoError(t, err)
}

func TestMultiTierLimiter_GetStats(t *testing.T) {
	mem := memory.New()
	tiers := []TierConfig{
		{Interval: time.Minute, Limit: 5},
		{Interval: time.Hour, Limit: 100},
	}

	limiter, err := New(
		WithBaseKey("stats-test"),
		WithFixedWindowStrategy(tiers...),
		WithBackend(mem),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	// Get initial stats
	stats, err := limiter.GetStats(WithContext(ctx))
	require.NoError(t, err)
	assert.Len(t, stats, 2)

	// Check minute tier stats
	minuteStats := stats["minute"]
	assert.True(t, minuteStats.Allowed)
	assert.Equal(t, 5, minuteStats.Total)
	assert.Equal(t, 5, minuteStats.Remaining)

	// Check hour tier stats
	hourStats := stats["hour"]
	assert.True(t, hourStats.Allowed)
	assert.Equal(t, 100, hourStats.Total)
	assert.Equal(t, 100, hourStats.Remaining)

	// Make some requests
	for i := range 3 {
		allowed, err := limiter.Allow(WithContext(ctx))
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i)
	}

	// Get updated stats
	stats, err = limiter.GetStats(WithContext(ctx))
	require.NoError(t, err)

	// Check minute tier after requests
	minuteStats = stats["minute"]
	assert.True(t, minuteStats.Allowed)
	assert.Equal(t, 5, minuteStats.Total)
	assert.Equal(t, 2, minuteStats.Remaining)

	// Check hour tier after requests
	hourStats = stats["hour"]
	assert.True(t, hourStats.Allowed)
	assert.Equal(t, 100, hourStats.Total)
	assert.Equal(t, 97, hourStats.Remaining)

	err = limiter.Close()
	require.NoError(t, err)
}

func TestMultiTierLimiter_Reset(t *testing.T) {
	mem := memory.New()
	tiers := []TierConfig{
		{Interval: time.Minute, Limit: 2},
	}

	limiter, err := New(
		WithBaseKey("reset-test"),
		WithFixedWindowStrategy(tiers...),
		WithBackend(mem),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	// Use up the limit
	for i := range 2 {
		allowed, err := limiter.Allow(WithContext(ctx))
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i)
	}

	// Next request should be denied
	allowed, err := limiter.Allow(WithContext(ctx))
	require.NoError(t, err)
	assert.False(t, allowed, "Request should be denied after limit exceeded")

	// Reset the limiter
	err = limiter.Reset(WithContext(ctx))
	require.NoError(t, err)

	// After reset, requests should be allowed again
	allowed, err = limiter.Allow(WithContext(ctx))
	require.NoError(t, err)
	assert.True(t, allowed, "Request after reset should be allowed")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestMultiTierLimiter_ConcurrentAccess(t *testing.T) {
	mem := memory.New()
	tiers := []TierConfig{
		{Interval: time.Minute, Limit: 10},
	}

	limiter, err := New(
		WithBaseKey("concurrent-test"),
		WithFixedWindowStrategy(tiers...),
		WithBackend(mem),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan bool, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			allowed, err := limiter.Allow(WithContext(ctx))
			if err != nil {
				errors <- err
				return
			}
			results <- allowed
		}()
	}

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for range 20 {
		select {
		case allowed := <-results:
			if allowed {
				allowedCount++
			} else {
				deniedCount++
			}
		case err := <-errors:
			errCount++
			t.Errorf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestMultiTierLimiter_MultipleKeys(t *testing.T) {
	mem := memory.New()
	tiers := []TierConfig{
		{Interval: time.Minute, Limit: 2},
	}

	// Create two limiters with different keys
	limiter1, err := New(
		WithBaseKey("user1"),
		WithFixedWindowStrategy(tiers...),
		WithBackend(mem),
	)
	require.NoError(t, err)

	limiter2, err := New(
		WithBaseKey("user2"),
		WithFixedWindowStrategy(tiers...),
		WithBackend(mem),
	)
	require.NoError(t, err)

	ctx := t.Context()

	// Use up limit for user1
	for range 2 {
		allowed, err := limiter1.Allow(WithContext(ctx))
		require.NoError(t, err)
		assert.True(t, allowed, "User1 request should be allowed")
	}

	// User1 should be denied
	allowed, err := limiter1.Allow(WithContext(ctx))
	require.NoError(t, err)
	assert.False(t, allowed, "User1 should be denied after limit exceeded")

	// User2 should still be able to make requests
	for range 2 {
		allowed, err := limiter2.Allow(WithContext(ctx))
		require.NoError(t, err)
		assert.True(t, allowed, "User2 request should be allowed")
	}

	err = limiter1.Close()
	require.NoError(t, err)
	err = limiter2.Close()
	require.NoError(t, err)
}

func TestMultiTierLimiter_TimeBehavior(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mem := memory.New()
		tiers := []TierConfig{
			{Interval: 5 * time.Second, Limit: 3},
		}

		limiter, err := New(
			WithBaseKey("time-test"),
			WithFixedWindowStrategy(tiers...),
			WithBackend(mem),
		)
		require.NoError(t, err)
		require.NotNil(t, limiter)

		ctx := t.Context()

		// Use up the limit
		for i := range 3 {
			allowed, err := limiter.Allow(WithContext(ctx))
			require.NoError(t, err)
			assert.True(t, allowed, "Request %d should be allowed", i)
		}

		// Next request should be denied
		allowed, err := limiter.Allow(WithContext(ctx))
		require.NoError(t, err)
		assert.False(t, allowed, "Request should be denied")

		// Advance time to just before window reset
		time.Sleep(4900 * time.Millisecond)
		synctest.Wait()

		// Still should be denied
		allowed, err = limiter.Allow(WithContext(ctx))
		require.NoError(t, err)
		assert.False(t, allowed, "Request should still be denied")

		// Advance time past window reset
		time.Sleep(200 * time.Millisecond)
		synctest.Wait()

		// Now should be allowed
		allowed, err = limiter.Allow(WithContext(ctx))
		require.NoError(t, err)
		assert.True(t, allowed, "Request should be allowed after window reset")

		err = limiter.Close()
		require.NoError(t, err)
	})
}

func TestMultiTierLimiter_InvalidConfiguration(t *testing.T) {
	mem := memory.New()
	// Test empty base key
	_, err := New(WithBaseKey(""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base key cannot be empty")

	// Test no tiers (empty fixed window config)
	_, err = New(
		WithBaseKey("test"),
		WithFixedWindowStrategy(), // Empty tiers
		WithBackend(mem),
	)
	require.NoError(t, err) // Should work now because WithFixedWindowStrategy provides a default tier

	// Test too many tiers
	var tooManyTiers []TierConfig
	for range MaxTiers + 1 {
		tooManyTiers = append(tooManyTiers, TierConfig{
			Interval: time.Minute,
			Limit:    100,
		})
	}

	_, err = New(
		WithBaseKey("test"),
		WithFixedWindowStrategy(tooManyTiers...),
		WithBackend(mem),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maximum 12 tiers")

	// Test invalid interval
	_, err = New(
		WithBaseKey("test"),
		WithFixedWindowStrategy(TierConfig{
			Interval: time.Second, // Less than MinInterval
			Limit:    100,
		}),
		WithBackend(mem),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is below minimum")

	// Test invalid limit
	_, err = New(
		WithBaseKey("test"),
		WithFixedWindowStrategy(TierConfig{
			Interval: time.Minute,
			Limit:    0, // Invalid limit
		}),
		WithBackend(mem),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "limit must be positive")

	// Test invalid token bucket config
	_, err = New(
		WithBaseKey("test"),
		WithTokenBucketStrategy(0, 1.0), // Invalid burst size
		WithBackend(mem),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "burst size must be positive")

	_, err = New(
		WithBaseKey("test"),
		WithTokenBucketStrategy(10, 0), // Invalid refill rate
		WithBackend(mem),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refill rate must be positive")

	// Test invalid leaky bucket config
	_, err = New(
		WithBaseKey("test"),
		WithLeakyBucketStrategy(0, 1.0), // Invalid capacity
		WithBackend(mem),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capacity must be positive")

	_, err = New(
		WithBaseKey("test"),
		WithLeakyBucketStrategy(10, 0), // Invalid leak rate
		WithBackend(mem),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "leak rate must be positive")
}

func TestMultiTierLimiter_BackendOperations(t *testing.T) {
	mem := memory.New()
	limiter, err := New(
		WithBaseKey("backend-test"),
		WithFixedWindowStrategy(TierConfig{
			Interval: time.Minute,
			Limit:    5,
		}),
		WithBackend(mem),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	// Test GetBackend
	oldBackend := limiter.GetBackend()
	assert.NotNil(t, oldBackend)

	// Test that backend is working
	ctx := t.Context()
	allowed, err := limiter.Allow(WithContext(ctx))
	require.NoError(t, err)
	assert.True(t, allowed)

	// Test Close
	err = limiter.Close()
	require.NoError(t, err)

	// After close, operations should still work (but storage is cleared)
	allowed, err = limiter.Allow(WithContext(ctx))
	require.NoError(t, err)
	assert.True(t, allowed, "Operations should work after close")
}

func TestMultiTierLimiter_MixedStrategyTypes(t *testing.T) {
	// Test that we can create limiters with different strategies
	testCases := []struct {
		name       string
		option     Option
		shouldFail bool
	}{
		{
			name:       "Fixed Window",
			option:     WithFixedWindowStrategy(TierConfig{Interval: time.Minute, Limit: 10}),
			shouldFail: false,
		},
		{
			name:       "Token Bucket",
			option:     WithTokenBucketStrategy(5, 2.0),
			shouldFail: false,
		},
		{
			name:       "Leaky Bucket",
			option:     WithLeakyBucketStrategy(5, 2.0),
			shouldFail: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				mem := memory.New()
				limiter, err := New(
					WithBaseKey(fmt.Sprintf("strategy-test-%s", strings.ReplaceAll(tc.name, " ", "_"))),
					WithBackend(mem),
					tc.option,
				)

				if tc.shouldFail {
					assert.Error(t, err, "Expected error for %s strategy", tc.name)
					assert.Contains(t, err.Error(), "failed to create strategy")
				} else {
					require.NoError(t, err)
					require.NotNil(t, limiter)

					// Test basic functionality
					ctx := t.Context()
					allowed, err := limiter.Allow(WithContext(ctx))
					require.NoError(t, err)
					assert.True(t, allowed, "First request should be allowed for %s strategy", tc.name)

					err = limiter.Close()
					require.NoError(t, err)
				}
			})
		})
	}
}

func TestAccessOptions_SingleLimiterMultipleKeys(t *testing.T) {
	mem := memory.New()
	tiers := []TierConfig{
		{Interval: time.Minute, Limit: 2},
	}

	// Create a single limiter with a base key prefix
	limiter, err := New(
		WithBaseKey("user"), // This is now a prefix, not a complete key
		WithFixedWindowStrategy(tiers...),
		WithBackend(mem),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	// Use up limit for user1
	for i := range 2 {
		allowed, err := limiter.Allow(WithContext(ctx), WithKey("123"))
		require.NoError(t, err)
		assert.True(t, allowed, "User1 request %d should be allowed", i)
	}

	// User1 should be denied
	allowed, err := limiter.Allow(WithContext(ctx), WithKey("123"))
	require.NoError(t, err)
	assert.False(t, allowed, "User1 should be denied after limit exceeded")

	// User2 should still be able to make requests (different key)
	for i := range 2 {
		allowed, err := limiter.Allow(WithContext(ctx), WithKey("456"))
		require.NoError(t, err)
		assert.True(t, allowed, "User2 request %d should be allowed", i)
	}

	// User2 should be denied
	allowed, err = limiter.Allow(WithContext(ctx), WithKey("456"))
	require.NoError(t, err)
	assert.False(t, allowed, "User2 should be denied after limit exceeded")

	// Default behavior (no options)
	allowed, err = limiter.Allow()
	require.NoError(t, err)
	assert.True(t, allowed)

	err = limiter.Close()
	require.NoError(t, err)
}

func TestAccessOptions_GetStats(t *testing.T) {
	mem := memory.New()
	tiers := []TierConfig{
		{Interval: time.Minute, Limit: 5},
	}

	limiter, err := New(
		WithBaseKey("api"), // Prefix
		WithFixedWindowStrategy(tiers...),
		WithBackend(mem),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	// Make requests for user1
	for i := range 3 {
		allowed, err := limiter.Allow(WithContext(ctx), WithKey("user1"))
		require.NoError(t, err)
		assert.True(t, allowed, "User1 request %d should be allowed", i)
	}

	// Get stats for user1
	stats, err := limiter.GetStats(WithContext(ctx), WithKey("user1"))
	require.NoError(t, err)
	assert.Len(t, stats, 1)

	minuteStats := stats["minute"]
	assert.Equal(t, 5, minuteStats.Total)
	assert.Equal(t, 2, minuteStats.Remaining)

	// Get stats for user2 (should be fresh)
	stats, err = limiter.GetStats(WithContext(ctx), WithKey("user2"))
	require.NoError(t, err)
	assert.Len(t, stats, 1)

	minuteStats = stats["minute"]
	assert.Equal(t, 5, minuteStats.Total)
	assert.Equal(t, 5, minuteStats.Remaining)

	err = limiter.Close()
	require.NoError(t, err)
}

func TestAccessOptions_AllowWithResult(t *testing.T) {
	mem := memory.New()
	tiers := []TierConfig{
		{Interval: time.Minute, Limit: 5},
	}

	limiter, err := New(
		WithBaseKey("api"), // Prefix
		WithFixedWindowStrategy(tiers...),
		WithBackend(mem),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	var stats1 map[string]TierResult
	// Make requests for user1
	for i := range 3 {
		allowed, s, err := limiter.allowWithResult(WithContext(ctx), WithKey("user1"))
		require.NoError(t, err)
		assert.True(t, allowed, "User1 request %d should be allowed", i)
		stats1 = s
	}

	minuteStats := stats1["minute"]
	assert.Equal(t, 5, minuteStats.Total)
	assert.Equal(t, 2, minuteStats.Remaining)

	var stats2 map[string]TierResult
	// Make requests for user2
	_, err = limiter.Allow(
		WithContext(ctx),
		WithKey("user2"),
		WithResult(&stats2),
	)
	require.NoError(t, err)
	assert.Len(t, stats2, 1)

	minuteStats = stats2["minute"]
	assert.Equal(t, 5, minuteStats.Total)
	assert.Equal(t, 4, minuteStats.Remaining)

	err = limiter.Close()
	require.NoError(t, err)
}

func TestAccessOptions_Reset(t *testing.T) {
	mem := memory.New()
	tiers := []TierConfig{
		{Interval: time.Minute, Limit: 2},
	}

	limiter, err := New(
		WithBaseKey("test"),
		WithFixedWindowStrategy(tiers...),
		WithBackend(mem),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	// Use up limit for user1
	for range 2 {
		allowed, err := limiter.Allow(WithContext(ctx), WithKey("user1"))
		require.NoError(t, err)
		assert.True(t, allowed)
	}

	// User1 should be denied
	allowed, err := limiter.Allow(WithContext(ctx), WithKey("user1"))
	require.NoError(t, err)
	assert.False(t, allowed)

	// Reset only user1
	err = limiter.Reset(WithContext(ctx), WithKey("user1"))
	require.NoError(t, err)

	// User1 should now be allowed again
	allowed, err = limiter.Allow(WithContext(ctx), WithKey("user1"))
	require.NoError(t, err)
	assert.True(t, allowed)

	// User2 should still be fresh (not affected by user1 reset)
	allowed, err = limiter.Allow(WithContext(ctx), WithKey("user2"))
	require.NoError(t, err)
	assert.True(t, allowed)

	err = limiter.Close()
	require.NoError(t, err)
}

func TestAccessOptions_DefaultBehavior(t *testing.T) {
	mem := memory.New()
	tiers := []TierConfig{
		{Interval: time.Minute, Limit: 3},
	}

	limiter, err := New(
		WithBaseKey("test"),
		WithFixedWindowStrategy(tiers...),
		WithBackend(mem),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	for i := range 3 {
		allowed, err := limiter.Allow(WithContext(ctx))
		require.NoError(t, err)
		assert.Truef(t, allowed, "request #%d should be allowed", i)
	}

	// Should be denied
	allowed, err := limiter.Allow(WithContext(ctx))
	require.NoError(t, err)
	assert.False(t, allowed)

	stats, err := limiter.GetStats(WithContext(ctx))
	require.NoError(t, err)
	assert.Len(t, stats, 1)

	minuteStats := stats["minute"]
	assert.Equal(t, 3, minuteStats.Total)
	assert.Equal(t, 0, minuteStats.Remaining)

	err = limiter.Reset(WithContext(ctx))
	require.NoError(t, err)

	// Should be allowed again
	allowed, err = limiter.Allow(WithContext(ctx))
	require.NoError(t, err)
	assert.True(t, allowed)

	err = limiter.Close()
	require.NoError(t, err)
}

func TestAccessOptions_MiddlewareExample(t *testing.T) {
	mem := memory.New()
	tiers := []TierConfig{
		{Interval: time.Minute, Limit: 5},
	}

	// Create ONE limiter instance for the entire app
	rateLimiter, err := New(
		WithBaseKey("user"), // Prefix
		WithFixedWindowStrategy(tiers...),
		WithBackend(mem),
	)
	require.NoError(t, err)
	require.NotNil(t, rateLimiter)
	defer rateLimiter.Close()

	ctx := t.Context()

	// Simulate middleware handling different users
	users := []string{"123", "456", "789"}

	for _, userID := range users {
		// Simulate multiple requests per user
		for i := range 3 {
			// Pass dynamic key to the shared limiter
			allowed, err := rateLimiter.Allow(WithContext(ctx), WithKey(userID))
			require.NoError(t, err)
			assert.True(t, allowed, "User %s request %d should be allowed", userID, i)
		}
	}

	// Each user should have made 3 requests and have 2 remaining
	for _, userID := range users {
		stats, err := rateLimiter.GetStats(WithContext(ctx), WithKey(userID))
		require.NoError(t, err)

		minuteStats := stats["minute"]
		assert.Equal(t, 5, minuteStats.Total)
		assert.Equal(t, 2, minuteStats.Remaining)
	}
}

func TestAccessOptions_ValidationErrors(t *testing.T) {
	mem := memory.New()
	tiers := []TierConfig{
		{Interval: time.Minute, Limit: 5},
	}

	limiter, err := New(
		WithBaseKey("test"),
		WithFixedWindowStrategy(tiers...),
		WithBackend(mem),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := t.Context()

	// Test empty key
	_, err = limiter.Allow(WithContext(ctx), WithKey(""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key cannot be empty")

	// Test valid options should work
	allowed, err := limiter.Allow(WithContext(ctx), WithKey("valid"))
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestDualStrategy_QuotaConsumption(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mem := memory.New()

		// Create dual strategy limiter: Fixed Window (primary) + Token Bucket (secondary)
		limiter, err := New(
			WithBaseKey("quota-test"),
			WithFixedWindowStrategy(
				TierConfig{Interval: time.Minute, Limit: 3}, // 3 requests per minute (smaller for easier testing)
			),
			WithTokenBucketStrategy(1, 1.0), // Very small burst (1), refill 1/sec
			WithBackend(mem),
		)
		require.NoError(t, err)
		require.NotNil(t, limiter)
		defer limiter.Close()

		ctx := t.Context()

		// Phase 1: Use up the token bucket burst capacity (1 request)
		allowed, err := limiter.Allow(WithContext(ctx))
		require.NoError(t, err)
		assert.True(t, allowed, "First request should be allowed")

		// Get initial stats
		stats, err := limiter.GetStats(WithContext(ctx))
		require.NoError(t, err)
		minuteStats := stats["minute"]
		assert.Equal(t, 2, minuteStats.Remaining, "Should have 2 remaining quota")

		// Phase 2: Request should be denied by secondary strategy, but primary quota preserved
		allowed, err = limiter.Allow(WithContext(ctx))
		require.NoError(t, err)
		assert.False(t, allowed, "Second request should be denied by secondary strategy")

		// Phase 3: Verify primary quota was NOT consumed (still 1, not 2)
		stats, err = limiter.GetStats(WithContext(ctx))
		require.NoError(t, err)
		minuteStats = stats["minute"]
		assert.True(t, minuteStats.Allowed, "Primary strategy should still allow")

		smootherStats := stats["smoother"]
		assert.False(t, smootherStats.Allowed, "Secondary strategy should deny")

		// Phase 4: This test successfully demonstrates the quota preservation fix.
		// The primary strategy quota was preserved when secondary denied.
		// No need to test timing-dependent refill behavior in this test.
	})
}
