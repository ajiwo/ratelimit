package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/ajiwo/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)


func TestFixedWindow_ConcurrentAccessMemory(t *testing.T) {
	backend := UseBackend(t, "memory")
	key := fmt.Sprintf("fwm-%d", time.Now().UnixNano())
	tiers := []ratelimit.TierConfig{
		{Interval: ratelimit.MinInterval, Limit: 10},
	}

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithFixedWindowStrategy(tiers...),
		ratelimit.WithBackend(backend),
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
			allowed, err := limiter.Allow(ratelimit.WithContext(ctx))
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

func TestFixedWindow_ConcurrentAccessPostgres(t *testing.T) {
	backend := UseBackend(t, "postgres")
	key := fmt.Sprintf("fwp-%d", time.Now().UnixNano())
	tiers := []ratelimit.TierConfig{
		{Interval: ratelimit.MinInterval, Limit: 10},
	}

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithFixedWindowStrategy(tiers...),
		ratelimit.WithBackend(backend),
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
			allowed, err := limiter.Allow(ratelimit.WithContext(ctx))
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
	tiers := []ratelimit.TierConfig{
		{Interval: ratelimit.MinInterval, Limit: 10},
	}

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithFixedWindowStrategy(tiers...),
		ratelimit.WithBackend(backend),
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
			allowed, err := limiter.Allow(ratelimit.WithContext(ctx))
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
		ratelimit.WithLeakyBucketStrategy(10, 0.1),
		ratelimit.WithBackend(backend),
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
			allowed, err := limiter.Allow(ratelimit.WithContext(ctx))
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
		ratelimit.WithLeakyBucketStrategy(10, 0.1),
		ratelimit.WithBackend(backend),
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
			allowed, err := limiter.Allow(ratelimit.WithContext(ctx))
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
		ratelimit.WithLeakyBucketStrategy(10, 0.1),
		ratelimit.WithBackend(backend),
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
			allowed, err := limiter.Allow(ratelimit.WithContext(ctx))
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
		ratelimit.WithTokenBucketStrategy(10, 0.1),
		ratelimit.WithBackend(backend),
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
			allowed, err := limiter.Allow(ratelimit.WithContext(ctx))
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
		ratelimit.WithTokenBucketStrategy(10, 0.1),
		ratelimit.WithBackend(backend),
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
			allowed, err := limiter.Allow(ratelimit.WithContext(ctx))
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
		ratelimit.WithTokenBucketStrategy(10, 0.1),
		ratelimit.WithBackend(backend),
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
			allowed, err := limiter.Allow(ratelimit.WithContext(ctx))
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
	tiers := []ratelimit.TierConfig{
		{Interval: ratelimit.MinInterval, Limit: 10},
	}

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithFixedWindowStrategy(tiers...),
		ratelimit.WithBackend(backend),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan ratelimit.TierResult, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			var res map[string]ratelimit.TierResult
			_, err := limiter.Allow(
				ratelimit.WithContext(ctx),
				ratelimit.WithResult(&res),
			)
			if err != nil {
				errors <- err
				return
			}
			results <- res["5s"]
		}()
	}

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for i := range 20 {
		select {
		case result := <-results:
			if result.Allowed {
				allowedCount++
				assert.Equal(t, allowedCount, result.Used, "Used should be %d for request %d", allowedCount, i+1)
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
	tiers := []ratelimit.TierConfig{
		{Interval: ratelimit.MinInterval, Limit: 10},
	}

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithFixedWindowStrategy(tiers...),
		ratelimit.WithBackend(backend),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan ratelimit.TierResult, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			var res map[string]ratelimit.TierResult
			_, err := limiter.Allow(
				ratelimit.WithContext(ctx),
				ratelimit.WithResult(&res),
			)
			if err != nil {
				errors <- err
				return
			}
			results <- res["5s"]
		}()
	}

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for i := range 20 {
		select {
		case result := <-results:
			if result.Allowed {
				allowedCount++
				assert.Equal(t, allowedCount, result.Used, "Used should be %d for request %d", allowedCount, i+1)
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
	tiers := []ratelimit.TierConfig{
		{Interval: ratelimit.MinInterval, Limit: 10},
	}

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithFixedWindowStrategy(tiers...),
		ratelimit.WithBackend(backend),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan ratelimit.TierResult, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			var res map[string]ratelimit.TierResult
			_, err := limiter.Allow(
				ratelimit.WithContext(ctx),
				ratelimit.WithResult(&res),
			)
			if err != nil {
				errors <- err
				return
			}
			results <- res["5s"]
		}()
	}

	// Collect results
	var allowedCount, deniedCount int
	var errCount int

	for i := range 20 {
		select {
		case result := <-results:
			if result.Allowed {
				allowedCount++
				assert.Equal(t, allowedCount, result.Used, "Used should be %d for request %d", allowedCount, i+1)
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
		ratelimit.WithLeakyBucketStrategy(10, 0.1),
		ratelimit.WithBackend(backend),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan ratelimit.TierResult, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			var res map[string]ratelimit.TierResult
			_, err := limiter.Allow(
				ratelimit.WithContext(ctx),
				ratelimit.WithResult(&res),
			)
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
		ratelimit.WithLeakyBucketStrategy(10, 0.1),
		ratelimit.WithBackend(backend),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan ratelimit.TierResult, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			var res map[string]ratelimit.TierResult
			_, err := limiter.Allow(
				ratelimit.WithContext(ctx),
				ratelimit.WithResult(&res),
			)
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
		ratelimit.WithLeakyBucketStrategy(10, 0.1),
		ratelimit.WithBackend(backend),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan ratelimit.TierResult, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			var res map[string]ratelimit.TierResult
			_, err := limiter.Allow(
				ratelimit.WithContext(ctx),
				ratelimit.WithResult(&res),
			)
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
		ratelimit.WithTokenBucketStrategy(10, 0.1),
		ratelimit.WithBackend(backend),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan ratelimit.TierResult, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			var res map[string]ratelimit.TierResult
			_, err := limiter.Allow(
				ratelimit.WithContext(ctx),
				ratelimit.WithResult(&res),
			)
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
		ratelimit.WithTokenBucketStrategy(10, 0.1),
		ratelimit.WithBackend(backend),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan ratelimit.TierResult, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			var res map[string]ratelimit.TierResult
			_, err := limiter.Allow(
				ratelimit.WithContext(ctx),
				ratelimit.WithResult(&res),
			)
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
		ratelimit.WithTokenBucketStrategy(10, 0.1),
		ratelimit.WithBackend(backend),
	)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	ctx := t.Context()

	// Start multiple goroutines making requests concurrently
	results := make(chan ratelimit.TierResult, 20)
	errors := make(chan error, 20)

	// Launch 20 goroutines
	for range 20 {
		go func() {
			var res map[string]ratelimit.TierResult
			_, err := limiter.Allow(
				ratelimit.WithContext(ctx),
				ratelimit.WithResult(&res),
			)
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
