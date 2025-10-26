package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/ajiwo/ratelimit"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/fixedwindow"
	"github.com/ajiwo/ratelimit/strategies/gcra"
	"github.com/ajiwo/ratelimit/strategies/leakybucket"
	"github.com/ajiwo/ratelimit/strategies/tokenbucket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixedWindow_ConcurrentAccessMemory(t *testing.T) {
	backend := UseBackend(t, "memory")
	key := fmt.Sprintf("fwm-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithBaseKey(key),
		ratelimit.WithPrimaryStrategy(fixedwindow.Config{
			Key: "test",
			Quotas: map[string]fixedwindow.Quota{
				"default": {
					Limit:  10,
					Window: 5 * time.Second,
				},
			},
		}),
		// Slow down, don't spend all 10 allowances at once
		ratelimit.WithSecondaryStrategy(tokenbucket.Config{
			BurstSize:  5,
			RefillRate: 500.0, // fast enough to refil after the burst
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan bool, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{})
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
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 5, allowedCount, "Exactly 5 requests should be allowed")
	assert.Equal(t, 15, deniedCount, "Exactly 15 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestFixedWindow_ConcurrentAccessPostgres(t *testing.T) {
	backend := UseBackend(t, "postgres")
	key := fmt.Sprintf("fwp-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(fixedwindow.Config{
			Key: "test",
			Quotas: map[string]fixedwindow.Quota{
				"default": {
					Limit:  10,
					Window: 5 * time.Second,
				},
			},
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan bool, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{})
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
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestFixedWindow_ConcurrentAccessRedis(t *testing.T) {
	backend := UseBackend(t, "redis")
	key := fmt.Sprintf("fwr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(fixedwindow.Config{
			Key: "test",
			Quotas: map[string]fixedwindow.Quota{
				"default": {
					Limit:  10,
					Window: 5 * time.Second,
				},
			},
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan bool, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{})
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
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestLeakyBucket_ConcurrentAccessMemory(t *testing.T) {
	backend := UseBackend(t, "memory")
	key := fmt.Sprintf("lbm-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(leakybucket.Config{Capacity: 10, LeakRate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan bool, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{})
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
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestLeakyBucket_ConcurrentAccessPostgres(t *testing.T) {
	backend := UseBackend(t, "postgres")
	key := fmt.Sprintf("lbp-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(leakybucket.Config{Capacity: 10, LeakRate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan bool, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{})
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
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestLeakyBucket_ConcurrentAccessRedis(t *testing.T) {
	backend := UseBackend(t, "redis")
	key := fmt.Sprintf("lbr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(leakybucket.Config{Capacity: 10, LeakRate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan bool, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{})
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
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestTokenBucket_ConcurrentAccessMemory(t *testing.T) {
	backend := UseBackend(t, "memory")
	key := fmt.Sprintf("tbm-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(tokenbucket.Config{BurstSize: 10, RefillRate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan bool, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{})
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
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestTokenBucket_ConcurrentAccessPostgres(t *testing.T) {
	backend := UseBackend(t, "postgres")
	key := fmt.Sprintf("tbp-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(tokenbucket.Config{BurstSize: 10, RefillRate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan bool, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{})
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
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestTokenBucket_ConcurrentAccessRedis(t *testing.T) {
	backend := UseBackend(t, "redis")
	key := fmt.Sprintf("tbr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(tokenbucket.Config{BurstSize: 10, RefillRate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan bool, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{})
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
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestFixedWindow_ConcurrentAccessMemoryWithResult(t *testing.T) {
	backend := UseBackend(t, "memory")
	key := fmt.Sprintf("fw-mwr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(fixedwindow.Config{
			Key: "test",
			Quotas: map[string]fixedwindow.Quota{
				"default": {
					Limit:  10,
					Window: 5 * time.Second,
				},
			},
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan strategies.Result, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
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

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for range 20 {
		select {
		case result := <-results:
			if result.Allowed {
				allowedCount++
			} else {
				deniedCount++
			}
		case err := <-errors:
			errCount++
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestFixedWindow_ConcurrentAccessPostgresWithResult(t *testing.T) {
	backend := UseBackend(t, "postgres")
	key := fmt.Sprintf("fw-pwr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(fixedwindow.Config{
			Key: "test",
			Quotas: map[string]fixedwindow.Quota{
				"default": {
					Limit:  10,
					Window: 5 * time.Second,
				},
			},
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan strategies.Result, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
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

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for range 20 {
		select {
		case result := <-results:
			if result.Allowed {
				allowedCount++
			} else {
				deniedCount++
			}
		case err := <-errors:
			errCount++
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestFixedWindow_ConcurrentAccessRedisWithResult(t *testing.T) {
	backend := UseBackend(t, "redis")
	key := fmt.Sprintf("fw-rwr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(fixedwindow.Config{
			Key: "test",
			Quotas: map[string]fixedwindow.Quota{
				"default": {
					Limit:  10,
					Window: 5 * time.Second,
				},
			},
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan strategies.Result, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
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

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for range 20 {
		select {
		case result := <-results:
			if result.Allowed {
				allowedCount++
			} else {
				deniedCount++
			}
		case err := <-errors:
			errCount++
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestLeakyBucket_ConcurrentAccessMemoryWithResult(t *testing.T) {
	backend := UseBackend(t, "memory")
	key := fmt.Sprintf("lb-mwr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(leakybucket.Config{Capacity: 10, LeakRate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan strategies.Result, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
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

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for range 20 {
		select {
		case result := <-results:
			if result.Allowed {
				allowedCount++
			} else {
				deniedCount++
			}
		case err := <-errors:
			errCount++
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestLeakyBucket_ConcurrentAccessPostgresWithResult(t *testing.T) {
	backend := UseBackend(t, "postgres")
	key := fmt.Sprintf("lb-pwr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(leakybucket.Config{Capacity: 10, LeakRate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan strategies.Result, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
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

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for range 20 {
		select {
		case result := <-results:
			if result.Allowed {
				allowedCount++
			} else {
				deniedCount++
			}
		case err := <-errors:
			errCount++
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestLeakyBucket_ConcurrentAccessRedisWithResult(t *testing.T) {
	backend := UseBackend(t, "redis")
	key := fmt.Sprintf("lb-rwr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(leakybucket.Config{Capacity: 10, LeakRate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan strategies.Result, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
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

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for range 20 {
		select {
		case result := <-results:
			if result.Allowed {
				allowedCount++
			} else {
				deniedCount++
			}
		case err := <-errors:
			errCount++
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestTokenBucket_ConcurrentAccessMemoryWithResult(t *testing.T) {
	backend := UseBackend(t, "memory")
	key := fmt.Sprintf("tb-mwr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(tokenbucket.Config{BurstSize: 10, RefillRate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan strategies.Result, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
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

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for range 20 {
		select {
		case result := <-results:
			if result.Allowed {
				allowedCount++
			} else {
				deniedCount++
			}
		case err := <-errors:
			errCount++
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestTokenBucket_ConcurrentAccessPostgresWithResult(t *testing.T) {
	backend := UseBackend(t, "postgres")
	key := fmt.Sprintf("tb-pwr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(tokenbucket.Config{BurstSize: 10, RefillRate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan strategies.Result, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
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

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for range 20 {
		select {
		case result := <-results:
			if result.Allowed {
				allowedCount++
			} else {
				deniedCount++
			}
		case err := <-errors:
			errCount++
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestTokenBucket_ConcurrentAccessRedisWithResult(t *testing.T) {
	backend := UseBackend(t, "redis")
	key := fmt.Sprintf("tb-rwr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(tokenbucket.Config{BurstSize: 10, RefillRate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan strategies.Result, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
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

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for range 20 {
		select {
		case result := <-results:
			if result.Allowed {
				allowedCount++
			} else {
				deniedCount++
			}
		case err := <-errors:
			errCount++
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestGCRA_ConcurrentAccessMemory(t *testing.T) {
	backend := UseBackend(t, "memory")
	key := fmt.Sprintf("gm-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(gcra.Config{Burst: 10, Rate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan bool, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{})
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
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestGCRA_ConcurrentAccessPostgres(t *testing.T) {
	backend := UseBackend(t, "postgres")
	key := fmt.Sprintf("gp-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(gcra.Config{Burst: 10, Rate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan bool, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{})
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
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestGCRA_ConcurrentAccessRedis(t *testing.T) {
	backend := UseBackend(t, "redis")
	key := fmt.Sprintf("gr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(gcra.Config{Burst: 10, Rate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan bool, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{})
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
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestGCRA_ConcurrentAccessMemoryWithResult(t *testing.T) {
	backend := UseBackend(t, "memory")
	key := fmt.Sprintf("gcra-mwr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(gcra.Config{Burst: 10, Rate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan strategies.Result, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
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

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for range 20 {
		select {
		case result := <-results:
			if result.Allowed {
				allowedCount++
			} else {
				deniedCount++
			}
		case err := <-errors:
			errCount++
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestGCRA_ConcurrentAccessPostgresWithResult(t *testing.T) {
	backend := UseBackend(t, "postgres")
	key := fmt.Sprintf("gcra-pwr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(gcra.Config{Burst: 10, Rate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan strategies.Result, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
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

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for range 20 {
		select {
		case result := <-results:
			if result.Allowed {
				allowedCount++
			} else {
				deniedCount++
			}
		case err := <-errors:
			errCount++
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}

func TestGCRA_ConcurrentAccessRedisWithResult(t *testing.T) {
	backend := UseBackend(t, "redis")
	key := fmt.Sprintf("gcra-rwr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithBackend(backend),
		ratelimit.WithMaxRetries(21),
		ratelimit.WithPrimaryStrategy(gcra.Config{Burst: 10, Rate: 0.1}),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)
	defer func() {
		_ = limiter.Close()
	}()

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan strategies.Result, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
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

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for range 20 {
		select {
		case result := <-results:
			if result.Allowed {
				allowedCount++
			} else {
				deniedCount++
			}
		case err := <-errors:
			errCount++
			t.Logf("Unexpected error: %v", err)
		}
	}

	// Should have exactly 10 allowed and 10 denied
	assert.Equal(t, 10, allowedCount, "Exactly 10 requests should be allowed")
	assert.Equal(t, 10, deniedCount, "Exactly 10 requests should be denied")
	assert.Equal(t, 0, errCount, "No errors should occur")

	err = limiter.Close()
	require.NoError(t, err)
}
