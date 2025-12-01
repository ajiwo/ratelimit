package tests

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ajiwo/ratelimit"
	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/fixedwindow"
	"github.com/ajiwo/ratelimit/strategies/gcra"
	"github.com/ajiwo/ratelimit/strategies/leakybucket"
	"github.com/ajiwo/ratelimit/strategies/tokenbucket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DualStrategyConfig defines a test configuration for dual rate limiting strategies
type DualStrategyConfig struct {
	name              string
	primaryStrategy   strategies.Config
	secondaryStrategy strategies.Config
	testType          string // "basic", "multiQuota", "concurrent", "peek", "reset", "error", "differentUsers"
}

// createDualLimiter creates a new rate limiter with dual strategies
func createDualLimiter(t *testing.T, backend backends.Backend, config DualStrategyConfig, keyPrefix string) *ratelimit.RateLimiter {
	key := fmt.Sprintf("%s-%s-%d", keyPrefix, config.name, time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(11),
		ratelimit.WithPrimaryStrategy(config.primaryStrategy),
		ratelimit.WithSecondaryStrategy(config.secondaryStrategy),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	t.Cleanup(func() {
		_ = limiter.Close()
	})

	return limiter
}

// dualStrategyConfigs defines all dual strategy configurations to test
var dualStrategyConfigs = []DualStrategyConfig{
	{
		name: "FixedWindow1Quota_LeakyBucket",
		primaryStrategy: fixedwindow.NewConfig().
			SetKey("user").
			AddQuota("default", 5, 10*time.Second).
			Build(),
		secondaryStrategy: leakybucket.Config{
			Burst: 3,
			Rate:  0.5, // 0.5 requests per second
		},
		testType: "basic",
	},
	{
		name: "FixedWindow3Quota_TokenBucket",
		primaryStrategy: fixedwindow.NewConfig().
			SetKey("user").
			AddQuota("minute", 5, time.Minute).  // 5 requests per minute
			AddQuota("hour", 100, time.Hour).    // 100 requests per hour
			AddQuota("day", 1000, 24*time.Hour). // 1000 requests per day
			Build(),
		secondaryStrategy: tokenbucket.Config{
			Burst: 5,   // max burst tokens
			Rate:  2.0, // 2 tokens per second
		},
		testType: "multiQuota",
	},
	{
		name: "FixedWindow_Concurrent",
		primaryStrategy: fixedwindow.NewConfig().
			SetKey("user").
			AddQuota("default", 10, 5*time.Second).
			Build(),
		secondaryStrategy: tokenbucket.Config{
			Burst: 5,
			Rate:  1.0,
		},
		testType: "concurrent",
	},
	{
		name: "FixedWindow_Peek",
		primaryStrategy: fixedwindow.NewConfig().
			SetKey("user").
			AddQuota("default", 3, 10*time.Second).
			Build(),
		secondaryStrategy: tokenbucket.Config{
			Burst: 2,
			Rate:  0.1,
		},
		testType: "peek",
	},
	{
		name: "FixedWindow_Reset",
		primaryStrategy: fixedwindow.NewConfig().
			SetKey("user").
			AddQuota("default", 2, 10*time.Second).
			Build(),
		secondaryStrategy: tokenbucket.Config{
			Burst: 2,
			Rate:  0.1,
		},
		testType: "reset",
	},
	{
		name: "FixedWindow_DifferentUsers",
		primaryStrategy: fixedwindow.NewConfig().
			AddQuota("default", 2, 10*time.Second).
			Build(),
		secondaryStrategy: tokenbucket.Config{
			Burst: 1,
			Rate:  0.1,
		},
		testType: "differentUsers",
	},
	{
		name: "FixedWindow3Quota_GCRA",
		primaryStrategy: fixedwindow.NewConfig().
			SetKey("user").
			AddQuota("minute", 5, time.Minute).  // 5 requests per minute
			AddQuota("hour", 100, time.Hour).    // 100 requests per hour
			AddQuota("day", 1000, 24*time.Hour). // 100 requests per hour
			Build(),
		secondaryStrategy: gcra.Config{
			Rate:  5.0,
			Burst: 5,
		},
		testType: "multiQuota",
	},
}

// testDualStrategyBackend runs a single test case for a backend and dual strategy configuration
func testDualStrategyBackend(t *testing.T, backendName string, config DualStrategyConfig) {
	backend := UseBackend(t, backendName)
	limiter := createDualLimiter(t, backend, config, backendName)

	switch config.testType {
	case "basic":
		testBasicDualStrategy(t, limiter)
	case "multiQuota":
		testMultiQuotaDualStrategy(t, limiter)
	case "concurrent":
		testConcurrentDualStrategy(t, limiter, backendName)
	case "peek":
		testPeekDualStrategy(t, limiter)
	case "reset":
		testResetDualStrategy(t, limiter)
	case "differentUsers":
		testDifferentUsersDualStrategy(t, limiter)
	}
}

// testBasicDualStrategy tests basic dual strategy functionality
func testBasicDualStrategy(t *testing.T, limiter *ratelimit.RateLimiter) {
	ctx := t.Context()
	userID := "testuser"
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

// testMultiQuotaDualStrategy tests dual strategy with multiple quotas
func testMultiQuotaDualStrategy(t *testing.T, limiter *ratelimit.RateLimiter) {
	ctx := t.Context()
	userID := "testuser"
	var results strategies.Results

	// First request should be allowed
	allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
	require.NoError(t, err)
	assert.True(t, allowed)

	// Check that we get all primary results plus secondary
	assert.Contains(t, results, "primary_minute", "Should have primary_minute result")
	assert.Contains(t, results, "primary_hour", "Should have primary_hour result")
	assert.Contains(t, results, "primary_day", "Should have primary_day result")
	assert.Contains(t, results, "secondary_default", "Should have secondary_default result")

	// Verify all quotas were consumed
	assert.Equal(t, 4, results["primary_minute"].Remaining, "minute quota should be consumed")
	assert.Equal(t, 99, results["primary_hour"].Remaining, "hour quota should be consumed")
	assert.Equal(t, 999, results["primary_day"].Remaining, "day quota should be consumed")
	assert.Equal(t, 4, results["secondary_default"].Remaining, "secondary tokens should be consumed")

	t.Logf("Initial request: minute=%d/10, hour=%d/100, day=%d/1000, secondary=%d/5",
		10-results["primary_minute"].Remaining,
		100-results["primary_hour"].Remaining,
		1000-results["primary_day"].Remaining,
		5-results["secondary_default"].Remaining)

	// Consume remaining minute quota
	for i := range 4 {
		allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID})
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+2)
	}

	// Next request should be denied (minute quota exhausted)
	allowed, err = limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
	require.NoError(t, err)
	assert.False(t, allowed, "Should be denied when minute quota exhausted")
	assert.Equal(t, 0, results["primary_minute"].Remaining, "Minute quota should be exhausted")
}

// runConcurrentDualTest runs concurrent test with specified parameters
func runConcurrentDualTest(t *testing.T, limiter *ratelimit.RateLimiter, numGoroutines, requestsPerGoroutine, userDivisor int) {
	ctx := t.Context()

	results := make(chan bool, numGoroutines*requestsPerGoroutine)
	errors := make(chan error, numGoroutines*requestsPerGoroutine)

	var wg sync.WaitGroup

	// Launch goroutines
	for range numGoroutines {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for range requestsPerGoroutine {
				userID := fmt.Sprintf("user%d", goroutineID%userDivisor)
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

	totalRequests := numGoroutines * requestsPerGoroutine
	assert.Equal(t, totalRequests, allowedCount+deniedCount, "All requests should be accounted for")
	assert.Equal(t, 0, errCount, "No errors should occur")
	assert.Greater(t, allowedCount, 0, "Some requests should be allowed")
	assert.Greater(t, deniedCount, 0, "Some requests should be denied")

	t.Logf("Concurrent test: %d allowed, %d denied out of %d total requests",
		allowedCount, deniedCount, totalRequests)
}

// testConcurrentDualStrategy tests concurrent access with dual strategies
func testConcurrentDualStrategy(t *testing.T, limiter *ratelimit.RateLimiter, backendName string) {
	// Adjust concurrent parameters based on backend
	switch backendName {
	case "memory":
		runConcurrentDualTest(t, limiter, 20, 5, 5)
	case "postgres", "redis":
		runConcurrentDualTest(t, limiter, 16, 3, 3)
	}
}

// testPeekDualStrategy tests Peek vs Allow behavior with dual strategies
func testPeekDualStrategy(t *testing.T, limiter *ratelimit.RateLimiter) {
	ctx := t.Context()
	userID := "testuser"
	var results strategies.Results

	// Initial peek should show full capacity
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

// testResetDualStrategy tests reset functionality with dual strategies
func testResetDualStrategy(t *testing.T, limiter *ratelimit.RateLimiter) {
	ctx := t.Context()
	userID := "testuser"

	// Consume all quota
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
	var results strategies.Results
	allowed, err = limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
	require.NoError(t, err)
	assert.True(t, allowed, "Should be allowed after reset")
	assert.Equal(t, 1, results["primary_default"].Remaining, "Primary should have 1 remaining after reset")
	assert.Equal(t, 1, results["secondary_default"].Remaining, "Secondary should have 1 remaining after reset")
}

// testDifferentUsersDualStrategy tests that dual strategies work independently for different users
func testDifferentUsersDualStrategy(t *testing.T, limiter *ratelimit.RateLimiter) {
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

// TestDualStrategy_Basic tests basic dual strategy functionality across all backends
func TestDualStrategy_Basic(t *testing.T) {
	backends := []string{"memory", "postgres", "redis"}
	config := dualStrategyConfigs[0] // FixedWindow1Quota_LeakyBucket

	for _, backend := range backends {
		if isCI() {
			time.Sleep(100 * time.Millisecond)
		}
		t.Run(fmt.Sprintf("%s_%s", config.name, backend), func(t *testing.T) {
			testDualStrategyBackend(t, backend, config)
		})
	}
}

// TestDualStrategy_MultiQuota tests dual strategy with multiple quotas across all backends
func TestDualStrategy_MultiQuota(t *testing.T) {
	backends := []string{"memory", "postgres", "redis"}
	configs := []DualStrategyConfig{
		dualStrategyConfigs[1], // FixedWindow3Quota_TokenBucket
		dualStrategyConfigs[6], // FixedWindow3Quota_GCRA
	}

	for _, backend := range backends {
		for _, config := range configs {
			if isCI() {
				time.Sleep(100 * time.Millisecond)
			}
			t.Run(fmt.Sprintf("%s_%s", config.name, backend), func(t *testing.T) {
				testDualStrategyBackend(t, backend, config)
			})
		}
	}
}

// TestDualStrategy_ConcurrentAccess tests concurrent access across all backends
func TestDualStrategy_ConcurrentAccess(t *testing.T) {
	backends := []string{"memory", "postgres", "redis"}
	config := dualStrategyConfigs[2] // FixedWindow_Concurrent

	for _, backend := range backends {
		if isCI() {
			time.Sleep(100 * time.Millisecond)
		}
		t.Run(fmt.Sprintf("%s_%s", config.name, backend), func(t *testing.T) {
			testDualStrategyBackend(t, backend, config)
		})
	}
}

// TestDualStrategy_PeekBehavior tests Peek vs Allow behavior
func TestDualStrategy_PeekBehavior(t *testing.T) {
	config := dualStrategyConfigs[3] // FixedWindow_Peek
	t.Run(config.name, func(t *testing.T) {
		testDualStrategyBackend(t, "memory", config)
	})
}

// TestDualStrategy_ResetBehavior tests reset functionality
func TestDualStrategy_ResetBehavior(t *testing.T) {
	config := dualStrategyConfigs[4] // FixedWindow_Reset
	t.Run(config.name, func(t *testing.T) {
		testDualStrategyBackend(t, "memory", config)
	})
}

// TestDualStrategy_DifferentUsers tests that dual strategies work independently for different users
func TestDualStrategy_DifferentUsers(t *testing.T) {
	config := dualStrategyConfigs[5] // FixedWindow_DifferentUsers
	t.Run(config.name, func(t *testing.T) {
		testDualStrategyBackend(t, "memory", config)
	})
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
