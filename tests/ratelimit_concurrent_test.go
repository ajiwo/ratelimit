package tests

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
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

var (
	numGoroutines   = getNumGoroutines()
	maxRetries      = numGoroutines / 2
	expectedAllowed = 1 + rand.IntN(numGoroutines) // #nosec: G404
	expectedDenied  = numGoroutines - expectedAllowed
)

// isCI returns true if running in a CI environment
func isCI() bool {
	return os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != ""
}

// getNumGoroutines returns the number of goroutines to use for concurrent tests
// In CI environment, use fewer goroutines to avoid overwhelming the runner
func getNumGoroutines() int {
	if isCI() {
		return 16 // Reduced concurrency for CI environment
	}
	return 32
}

// StrategyConfig defines a test configuration for a rate limiting strategy
type StrategyConfig struct {
	name     string
	strategy strategies.Config
}

// TestResult holds the results of a concurrent test
type TestResult struct {
	allowedCount int
	deniedCount  int
	errCount     int
}

// strategyConfigs defines all strategy configurations to test
var strategyConfigs = []StrategyConfig{
	{
		name: "FixedWindow",
		strategy: fixedwindow.Config{
			Key: "test",
			Quotas: map[string]fixedwindow.Quota{
				"default": {
					Limit:  expectedAllowed,
					Window: 5 * time.Second,
				},
				"hourly": {
					Limit:  10 * expectedAllowed,
					Window: 5 * time.Second,
				},
			},
		},
	},
	{
		name: "LeakyBucket",
		strategy: leakybucket.Config{
			Burst: expectedAllowed,
			Rate:  0.1,
		},
	},
	{
		name: "TokenBucket",
		strategy: tokenbucket.Config{
			Burst: expectedAllowed,
			Rate:  0.1,
		},
	},
	{
		name: "GCRA",
		strategy: gcra.Config{
			Burst: expectedAllowed,
			Rate:  0.1,
		},
	},
}

// createLimiter creates a new rate limiter with the given configuration
func createLimiter(t *testing.T, backend backends.Backend, config StrategyConfig, keyPrefix string) *ratelimit.RateLimiter {
	key := fmt.Sprintf("%s-%s-%d", keyPrefix, config.name, time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(maxRetries),
		ratelimit.WithPrimaryStrategy(config.strategy),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	t.Cleanup(func() {
		_ = limiter.Close()
	})

	return limiter
}

// runConcurrentTest executes the concurrent test for a given limiter
func runConcurrentTest(t *testing.T, limiter *ratelimit.RateLimiter, wantResult bool) TestResult {
	ctx := t.Context()

	// Determine channel type based on wantResult
	if wantResult {
		return runConcurrentTestWithResult(t, ctx, limiter)
	}
	return runConcurrentTestWithoutResult(t, ctx, limiter)
}

// runConcurrentTestWithoutResult runs test using simple boolean return
func runConcurrentTestWithoutResult(t *testing.T, ctx context.Context, limiter *ratelimit.RateLimiter) TestResult {
	results := make(chan bool, numGoroutines)
	errors := make(chan error, numGoroutines)

	// Launch concurrent goroutines
	for range numGoroutines {
		go func() {
			allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{})
			if err != nil {
				errors <- err
				return
			}
			results <- allowed
		}()
	}

	return collectResults(t, results, errors)
}

// runConcurrentTestWithResult runs test using detailed Result return
func runConcurrentTestWithResult(t *testing.T, ctx context.Context, limiter *ratelimit.RateLimiter) TestResult {
	results := make(chan strategies.Result, numGoroutines)
	errors := make(chan error, numGoroutines)

	// Launch concurrent goroutines
	for range numGoroutines {
		go func() {
			var res strategies.Results
			_, err := limiter.Allow(ctx, ratelimit.AccessOptions{Result: &res})
			if err != nil {
				errors <- err
				return
			}
			results <- res["default"]
		}()
	}

	return collectResultsWithStrategyResult(t, results, errors)
}

// collectResults collects results from boolean channels
func collectResults(t *testing.T, results <-chan bool, errors <-chan error) TestResult {
	var result TestResult

	for range numGoroutines {
		select {
		case allowed := <-results:
			if allowed {
				result.allowedCount++
			} else {
				result.deniedCount++
			}
		case err := <-errors:
			result.errCount++
			t.Logf("Unexpected error: %v", err)
		}
	}

	return result
}

// collectResultsWithStrategyResult collects results from strategy.Result channels
func collectResultsWithStrategyResult(t *testing.T, results <-chan strategies.Result, errors <-chan error) TestResult {
	var result TestResult

	for range numGoroutines {
		select {
		case res := <-results:
			if res.Allowed {
				result.allowedCount++
			} else {
				result.deniedCount++
			}
		case err := <-errors:
			result.errCount++
			t.Logf("Unexpected error: %v", err)
		}
	}

	return result
}

// assertTestResults validates the test results meet expectations
func assertTestResults(t *testing.T, result TestResult, testName string) {
	assert.Equal(t, expectedAllowed, result.allowedCount,
		"%s: Exactly %d requests should be allowed", testName, expectedAllowed)
	assert.Equal(t, expectedDenied, result.deniedCount,
		"%s: Exactly %d requests should be denied", testName, expectedDenied)
	assert.Equal(t, 0, result.errCount,
		"%s: No errors should occur", testName)
}

// testBackendStrategy runs a single test case for a backend and strategy combination
func testBackendStrategy(t *testing.T, backendName string, config StrategyConfig, wantResult bool) {
	backend := UseBackend(t, backendName)
	limiter := createLimiter(t, backend, config, backendName)
	result := runConcurrentTest(t, limiter, wantResult)

	testName := fmt.Sprintf("%s_%s", config.name, backendName)
	if wantResult {
		testName += "_WithResult"
	}
	assertTestResults(t, result, testName)
}

// TestConcurrentAccess_Basic tests concurrent access for all strategy and backend combinations
func TestConcurrentAccess_Basic(t *testing.T) {
	backends := []string{"memory", "postgres", "redis"}

	for _, config := range strategyConfigs {
		for _, backend := range backends {
			if isCI() {
				time.Sleep(100 * time.Millisecond)
			}
			t.Run(fmt.Sprintf("%s_%s", config.name, backend), func(t *testing.T) {
				testBackendStrategy(t, backend, config, false)
			})
		}
	}
}

// TestConcurrentAccess_WithResult tests concurrent access with detailed results
func TestConcurrentAccess_WithResult(t *testing.T) {
	backends := []string{"memory", "postgres", "redis"}

	for _, config := range strategyConfigs {
		for _, backend := range backends {
			if isCI() {
				time.Sleep(100 * time.Millisecond)
			}
			t.Run(fmt.Sprintf("%s_%s_WithResult", config.name, backend), func(t *testing.T) {
				testBackendStrategy(t, backend, config, true)
			})
		}
	}
}
