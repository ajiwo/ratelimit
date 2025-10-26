package tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ajiwo/ratelimit"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/fixedwindow"
	"github.com/ajiwo/ratelimit/strategies/leakybucket"
	"github.com/ajiwo/ratelimit/strategies/tokenbucket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDualStrategy_FixedWindow1Quota_LeakyBucket tests Fixed Window (1 quota) + Leaky Bucket secondary
func TestDualStrategy_FixedWindow1Quota_LeakyBucket_Memory(t *testing.T) {
	backend := UseBackend(t, "memory")
	key := fmt.Sprintf("ds-fw1-lb-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBackend(backend),
		ratelimit.WithBaseKey(key),
		// Primary: hard limits with 1 quota
		ratelimit.WithPrimaryStrategy(
			fixedwindow.NewConfig().
				SetKey("user").
				AddQuota("default", 5, 10*time.Second).
				Build(),
		),
		// Secondary: smoothing with leaky bucket
		ratelimit.WithSecondaryStrategy(leakybucket.Config{
			Capacity: 3,
			LeakRate: 0.5, // 0.5 requests per second
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := t.Context()
	userID := "testuser"

	// Test that both strategies work together
	var results strategies.Results

	// First few requests should be allowed (both strategies allow)
	for i := range 3 {
		allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
		require.NoError(t, err, "Request %d should not error", i+1)
		assert.True(t, allowed, "Request %d should be allowed", i+1)

		// Check that we get both primary and secondary results
		assert.Contains(t, results, "primary_default", "Should have primary_default result")
		assert.Contains(t, results, "secondary_default", "Should have secondary_default result")

		t.Logf("Request %d: allowed=%v, primary_remaining=%d, secondary_remaining=%d",
			i+1, allowed, results["primary_default"].Remaining, results["secondary_default"].Remaining)
	}

	// Eventually should be limited by one of the strategies
	deniedCount := 0
	for i := 3; i < 10; i++ {
		allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
		require.NoError(t, err, "Request %d should not error", i+1)
		if !allowed {
			deniedCount++
			t.Logf("Request %d: denied (primary_remaining=%d, secondary_remaining=%d)",
				i+1, results["primary_default"].Remaining, results["secondary_default"].Remaining)
		} else {
			t.Logf("Request %d: allowed (primary_remaining=%d, secondary_remaining=%d)",
				i+1, results["primary_default"].Remaining, results["secondary_default"].Remaining)
		}
	}

	assert.Greater(t, deniedCount, 0, "Some requests should be denied")
}

// TestDualStrategy_FixedWindow1Quota_LeakyBucket_Postgres tests the same with Postgres backend
func TestDualStrategy_FixedWindow1Quota_LeakyBucket_Postgres(t *testing.T) {
	backend := UseBackend(t, "postgres")
	key := fmt.Sprintf("ds-fw1-lb-pg-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBackend(backend),
		ratelimit.WithBaseKey(key),
		ratelimit.WithPrimaryStrategy(
			fixedwindow.NewConfig().
				SetKey("user").
				AddQuota("default", 5, 10*time.Second).
				Build(),
		),
		ratelimit.WithSecondaryStrategy(leakybucket.Config{
			Capacity: 3,
			LeakRate: 0.5,
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()
	userID := "testuser"

	// Test basic functionality
	var results strategies.Results
	allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Contains(t, results, "primary_default")
	assert.Contains(t, results, "secondary_default")
}

// TestDualStrategy_FixedWindow1Quota_LeakyBucket_Redis tests the same with Redis backend
func TestDualStrategy_FixedWindow1Quota_LeakyBucket_Redis(t *testing.T) {
	backend := UseBackend(t, "redis")
	key := fmt.Sprintf("ds-fw1-lb-redis-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBackend(backend),
		ratelimit.WithBaseKey(key),
		ratelimit.WithPrimaryStrategy(
			fixedwindow.NewConfig().
				SetKey("user").
				AddQuota("default", 5, 10*time.Second).
				Build(),
		),
		ratelimit.WithSecondaryStrategy(leakybucket.Config{
			Capacity: 3,
			LeakRate: 0.5,
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()
	userID := "testuser"

	// Test basic functionality
	var results strategies.Results
	allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Contains(t, results, "primary_default")
	assert.Contains(t, results, "secondary_default")
}

// TestDualStrategy_FixedWindow3Quota_TokenBucket tests Fixed Window (3 quotas) + Token Bucket secondary
func TestDualStrategy_FixedWindow3Quota_TokenBucket_Memory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		backend := UseBackend(t, "memory")
		key := fmt.Sprintf("ds-fw3-tb-%d", time.Now().UnixNano())

		limiter, err := ratelimit.New(
			ratelimit.WithBackend(backend),
			ratelimit.WithBaseKey(key),
			// Primary: hard limits with 3 quotas
			ratelimit.WithPrimaryStrategy(
				fixedwindow.NewConfig().
					SetKey("user").
					AddQuota("requests", 10, time.Minute).    // 10 requests per minute
					AddQuota("bandwidth", 1000, time.Minute). // 1000 units per minute
					AddQuota("connections", 5, time.Minute).  // 5 connections per minute
					Build(),
			),
			// Secondary: smoothing with token bucket
			ratelimit.WithSecondaryStrategy(tokenbucket.Config{
				BurstSize:  5,   // max burst tokens
				RefillRate: 2.0, // 2 tokens per second
			}),
		)
		require.NoError(t, err)
		require.NotNil(t, limiter)
		defer limiter.Close()

		ctx := t.Context()
		userID := "testuser"

		// Test that all 3 primary quotas work
		var results strategies.Results

		// First request should be allowed
		allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
		require.NoError(t, err)
		assert.True(t, allowed)

		// Check that we get all primary results plus secondary
		assert.Contains(t, results, "primary_requests", "Should have primary_requests result")
		assert.Contains(t, results, "primary_bandwidth", "Should have primary_bandwidth result")
		assert.Contains(t, results, "primary_connections", "Should have primary_connections result")
		assert.Contains(t, results, "secondary_default", "Should have secondary_default result")

		// Verify all quotas were consumed
		assert.Equal(t, 9, results["primary_requests"].Remaining, "requests quota should be consumed")
		assert.Equal(t, 999, results["primary_bandwidth"].Remaining, "bandwidth quota should be consumed")
		assert.Equal(t, 4, results["primary_connections"].Remaining, "connections quota should be consumed")
		assert.Equal(t, 4, results["secondary_default"].Remaining, "secondary tokens should be consumed")

		t.Logf("Initial request: requests=%d/10, bandwidth=%d/1000, connections=%d/5, secondary=%d/5",
			10-results["primary_requests"].Remaining,
			1000-results["primary_bandwidth"].Remaining,
			5-results["primary_connections"].Remaining,
			5-results["secondary_default"].Remaining)

		// Consume remaining connection quota (should be limited by connections)
		for i := range 4 {
			allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID})
			require.NoError(t, err)
			assert.True(t, allowed, "Request %d should be allowed", i+2)
		}

		// Next request should be denied (connections exhausted)
		allowed, err = limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
		require.NoError(t, err)
		assert.False(t, allowed, "Should be denied when connections quota exhausted")
		assert.Equal(t, 0, results["primary_connections"].Remaining, "Connections should be exhausted")
	})
}

// TestDualStrategy_FixedWindow3Quota_TokenBucket_Postgres tests the same with Postgres backend
func TestDualStrategy_FixedWindow3Quota_TokenBucket_Postgres(t *testing.T) {
	backend := UseBackend(t, "postgres")
	key := fmt.Sprintf("ds-fw3-tb-pg-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBackend(backend),
		ratelimit.WithBaseKey(key),
		ratelimit.WithPrimaryStrategy(
			fixedwindow.NewConfig().
				SetKey("user").
				AddQuota("requests", 10, time.Minute).
				AddQuota("bandwidth", 1000, time.Minute).
				AddQuota("connections", 5, time.Minute).
				Build(),
		),
		ratelimit.WithSecondaryStrategy(tokenbucket.Config{
			BurstSize:  5,
			RefillRate: 2.0,
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()
	userID := "testuser"

	// Test that all 3 quotas work
	var results strategies.Results
	allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
	require.NoError(t, err)
	assert.True(t, allowed)

	assert.Contains(t, results, "primary_requests")
	assert.Contains(t, results, "primary_bandwidth")
	assert.Contains(t, results, "primary_connections")
	assert.Contains(t, results, "secondary_default")
}

// TestDualStrategy_FixedWindow3Quota_TokenBucket_Redis tests the same with Redis backend
func TestDualStrategy_FixedWindow3Quota_TokenBucket_Redis(t *testing.T) {
	backend := UseBackend(t, "redis")
	key := fmt.Sprintf("ds-fw3-tb-redis-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBackend(backend),
		ratelimit.WithBaseKey(key),
		ratelimit.WithPrimaryStrategy(
			fixedwindow.NewConfig().
				SetKey("user").
				AddQuota("requests", 10, time.Minute).
				AddQuota("bandwidth", 1000, time.Minute).
				AddQuota("connections", 5, time.Minute).
				Build(),
		),
		ratelimit.WithSecondaryStrategy(tokenbucket.Config{
			BurstSize:  5,
			RefillRate: 2.0,
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()
	userID := "testuser"

	// Test that all 3 quotas work
	var results strategies.Results
	allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
	require.NoError(t, err)
	assert.True(t, allowed)

	assert.Contains(t, results, "primary_requests")
	assert.Contains(t, results, "primary_bandwidth")
	assert.Contains(t, results, "primary_connections")
	assert.Contains(t, results, "secondary_default")
}

// TestDualStrategy_ConcurrentAccess tests concurrent access with dual strategies
func TestDualStrategy_ConcurrentAccess_Memory(t *testing.T) {
	backend := UseBackend(t, "memory")
	key := fmt.Sprintf("ds-concurrent-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBackend(backend),
		ratelimit.WithBaseKey(key),
		ratelimit.WithMaxRetries(11),
		ratelimit.WithPrimaryStrategy(
			fixedwindow.NewConfig().
				SetKey("user").
				AddQuota("default", 10, 5*time.Second).
				Build(),
		),
		ratelimit.WithSecondaryStrategy(tokenbucket.Config{
			BurstSize:  5,
			RefillRate: 1.0,
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Start multiple goroutines making requests concurrently
	const numGoroutines = 20
	const requestsPerGoroutine = 5

	results := make(chan bool, numGoroutines*requestsPerGoroutine)
	errors := make(chan error, numGoroutines*requestsPerGoroutine)

	var wg sync.WaitGroup

	// Launch goroutines
	for range numGoroutines {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for range requestsPerGoroutine {
				userID := fmt.Sprintf("user%d", goroutineID%5) // 5 different users
				allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID})
				if err != nil {
					errors <- err
					return
				}
				results <- allowed
			}
		}(len(results))
	}

	wg.Wait()
	close(results)
	close(errors)

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for allowed := range results {
		if allowed {
			allowedCount++
		} else {
			deniedCount++
		}
	}

	for err := range errors {
		errCount++
		t.Logf("Unexpected error: %v", err)
	}

	totalRequests := numGoroutines * requestsPerGoroutine
	assert.Equal(t, totalRequests, allowedCount+deniedCount, "All requests should be accounted for")
	assert.Equal(t, 0, errCount, "No errors should occur")

	// Some requests should be allowed, some denied (due to rate limiting)
	assert.Greater(t, allowedCount, 0, "Some requests should be allowed")
	assert.Greater(t, deniedCount, 0, "Some requests should be denied")

	t.Logf("Concurrent test: %d allowed, %d denied out of %d total requests",
		allowedCount, deniedCount, totalRequests)
}

// TestDualStrategy_ConcurrentAccess_Postgres tests concurrent access with Postgres
func TestDualStrategy_ConcurrentAccess_Postgres(t *testing.T) {
	backend := UseBackend(t, "postgres")
	key := fmt.Sprintf("ds-concurrent-pg-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBackend(backend),
		ratelimit.WithBaseKey(key),
		ratelimit.WithMaxRetries(11),
		ratelimit.WithPrimaryStrategy(
			fixedwindow.NewConfig().
				SetKey("user").
				AddQuota("default", 10, 5*time.Second).
				Build(),
		),
		ratelimit.WithSecondaryStrategy(tokenbucket.Config{
			BurstSize:  5,
			RefillRate: 1.0,
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	// Test with fewer concurrent requests for Postgres to avoid overwhelming it
	const numGoroutines = 10
	const requestsPerGoroutine = 3

	results := make(chan bool, numGoroutines*requestsPerGoroutine)
	errors := make(chan error, numGoroutines*requestsPerGoroutine)

	var wg sync.WaitGroup

	for range numGoroutines {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for range requestsPerGoroutine {
				userID := fmt.Sprintf("user%d", goroutineID%3)
				allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID})
				if err != nil {
					errors <- err
					return
				}
				results <- allowed
			}
		}(len(results))
	}

	wg.Wait()
	close(results)
	close(errors)

	var allowedCount, deniedCount, errCount int
	for allowed := range results {
		if allowed {
			allowedCount++
		} else {
			deniedCount++
		}
	}
	for range errors {
		errCount++
	}

	assert.Equal(t, 0, errCount, "No errors should occur")
	assert.Greater(t, allowedCount, 0, "Some requests should be allowed")
}

// TestDualStrategy_ConcurrentAccess_Redis tests concurrent access with Redis
func TestDualStrategy_ConcurrentAccess_Redis(t *testing.T) {
	backend := UseBackend(t, "redis")
	key := fmt.Sprintf("ds-concurrent-redis-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBackend(backend),
		ratelimit.WithBaseKey(key),
		ratelimit.WithMaxRetries(11),
		ratelimit.WithPrimaryStrategy(
			fixedwindow.NewConfig().
				SetKey("user").
				AddQuota("default", 10, 5*time.Second).
				Build(),
		),
		ratelimit.WithSecondaryStrategy(tokenbucket.Config{
			BurstSize:  5,
			RefillRate: 1.0,
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()

	const numGoroutines = 10
	const requestsPerGoroutine = 3

	results := make(chan bool, numGoroutines*requestsPerGoroutine)
	errors := make(chan error, numGoroutines*requestsPerGoroutine)

	var wg sync.WaitGroup

	for range numGoroutines {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for range requestsPerGoroutine {
				userID := fmt.Sprintf("user%d", goroutineID%3)
				allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID})
				if err != nil {
					errors <- err
					return
				}
				results <- allowed
			}
		}(len(results))
	}

	wg.Wait()
	close(results)
	close(errors)

	var allowedCount, deniedCount, errCount int
	for allowed := range results {
		if allowed {
			allowedCount++
		} else {
			deniedCount++
		}
	}
	for range errors {
		errCount++
	}

	assert.Equal(t, 0, errCount, "No errors should occur")
	assert.Greater(t, allowedCount, 0, "Some requests should be allowed")
}

// TestDualStrategy_PeekBehavior tests Peek vs Allow behavior with dual strategies
func TestDualStrategy_PeekBehavior(t *testing.T) {
	backend := UseBackend(t, "memory")
	key := fmt.Sprintf("ds-peek-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBackend(backend),
		ratelimit.WithBaseKey(key),
		ratelimit.WithPrimaryStrategy(
			fixedwindow.NewConfig().
				SetKey("user").
				AddQuota("default", 3, 10*time.Second).
				Build(),
		),
		ratelimit.WithSecondaryStrategy(tokenbucket.Config{
			BurstSize:  2,
			RefillRate: 0.1,
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()
	userID := "testuser"

	// Initial peek should show full capacity
	var results strategies.Results
	allowed, err := limiter.Peek(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
	require.NoError(t, err)
	assert.True(t, allowed, "Peek should show request as allowed")
	assert.Equal(t, 3, results["primary_default"].Remaining, "Primary should have full capacity")
	assert.Equal(t, 2, results["secondary_default"].Remaining, "Secondary should have full capacity")

	// Peek again should show same state (no consumption)
	allowed, err = limiter.Peek(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
	require.NoError(t, err)
	assert.True(t, allowed, "Peek should still show request as allowed")
	assert.Equal(t, 3, results["primary_default"].Remaining, "Primary should still have full capacity")
	assert.Equal(t, 2, results["secondary_default"].Remaining, "Secondary should still have full capacity")

	// Allow should consume quota
	allowed, err = limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
	require.NoError(t, err)
	assert.True(t, allowed, "Allow should succeed")
	assert.Equal(t, 2, results["primary_default"].Remaining, "Primary should have 2 remaining")
	assert.Equal(t, 1, results["secondary_default"].Remaining, "Secondary should have 1 remaining")

	// Peek should reflect consumed state
	allowed, err = limiter.Peek(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
	require.NoError(t, err)
	assert.True(t, allowed, "Peek should show request as allowed")
	assert.Equal(t, 2, results["primary_default"].Remaining, "Peek should reflect consumed primary state")
	assert.Equal(t, 1, results["secondary_default"].Remaining, "Peek should reflect consumed secondary state")
}

// TestDualStrategy_ResetBehavior tests reset functionality with dual strategies
func TestDualStrategy_ResetBehavior(t *testing.T) {
	backend := UseBackend(t, "memory")
	key := fmt.Sprintf("ds-reset-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBackend(backend),
		ratelimit.WithBaseKey(key),
		ratelimit.WithPrimaryStrategy(
			fixedwindow.NewConfig().
				SetKey("user").
				AddQuota("default", 2, 10*time.Second).
				Build(),
		),
		ratelimit.WithSecondaryStrategy(tokenbucket.Config{
			BurstSize:  2,
			RefillRate: 0.1,
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := context.Background()
	userID := "testuser"

	// Consume all quota
	var results strategies.Results
	for i := range 2 {
		allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID})
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// Should be denied now (secondary exhausted)
	allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID})
	require.NoError(t, err)
	assert.False(t, allowed, "Should be denied after consuming all quota")

	// Reset should clear the state
	err = limiter.Reset(ctx, ratelimit.AccessOptions{Key: userID})
	require.NoError(t, err, "Reset should not error")

	// Should now be allowed again with full capacity
	allowed, err = limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
	require.NoError(t, err)
	assert.True(t, allowed, "Should be allowed after reset")
	assert.Equal(t, 1, results["primary_default"].Remaining, "Primary should have 1 remaining after reset")
	assert.Equal(t, 1, results["secondary_default"].Remaining, "Secondary should have 1 remaining after reset")
}

// TestDualStrategy_ErrorHandling tests error scenarios with dual strategies
func TestDualStrategy_ErrorHandling(t *testing.T) {
	backend := UseBackend(t, "memory")

	// Test invalid configuration - secondary strategy must support CapSecondary
	_, err := ratelimit.New(
		ratelimit.WithBackend(backend),
		ratelimit.WithBaseKey("test"),
		ratelimit.WithPrimaryStrategy(
			fixedwindow.NewConfig().
				AddQuota("default", 5, time.Second).
				Build(),
		),
		// Try to use Fixed Window as secondary (doesn't support CapSecondary)
		ratelimit.WithSecondaryStrategy(
			fixedwindow.NewConfig().
				AddQuota("default", 3, time.Second).
				Build(),
		),
	)
	assert.Error(t, err, "Should error when secondary strategy doesn't support CapSecondary")
	assert.Contains(t, err.Error(), "secondary", "Error should mention secondary strategy")
}

// TestDualStrategy_DifferentUsers tests that dual strategies work independently for different users
func TestDualStrategy_DifferentUsers(t *testing.T) {
	backend := UseBackend(t, "memory")
	key := fmt.Sprintf("ds-users-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBackend(backend),
		ratelimit.WithBaseKey(key),
		ratelimit.WithPrimaryStrategy(
			fixedwindow.NewConfig().
				AddQuota("default", 2, 10*time.Second).
				Build(),
		),
		ratelimit.WithSecondaryStrategy(tokenbucket.Config{
			BurstSize:  1,
			RefillRate: 0.1,
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer limiter.Close()

	ctx := t.Context()

	// User1 should be limited independently of User2
	user1 := "user1"
	user2 := "user2"

	// User1 consumes quota - only 1 request should be allowed due to token bucket burst size of 1
	allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: user1})
	require.NoError(t, err)
	assert.True(t, allowed, "User1 first request should be allowed")

	// User1 should now be denied (token bucket burst exhausted)
	allowed, err = limiter.Allow(ctx, ratelimit.AccessOptions{Key: user1})
	require.NoError(t, err)
	assert.False(t, allowed, "User1 should be denied after consuming token bucket burst")

	// User2 should still be allowed (different key)
	allowed, err = limiter.Allow(ctx, ratelimit.AccessOptions{Key: user2})
	require.NoError(t, err)
	assert.True(t, allowed, "User2 should be allowed (independent of User1)")
}
