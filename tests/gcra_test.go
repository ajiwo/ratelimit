package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/ajiwo/ratelimit"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/gcra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGCRA_ConcurrentAccessMemory(t *testing.T) {
	backend := UseBackend(t, "memory")
	key := fmt.Sprintf("gm-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithPrimaryStrategy(gcra.Config{Burst: 10, Rate: 0.1}),
		ratelimit.WithBackend(backend),
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

func TestGCRA_ConcurrentAccessPostgres(t *testing.T) {
	backend := UseBackend(t, "postgres")
	key := fmt.Sprintf("gp-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithPrimaryStrategy(gcra.Config{Burst: 10, Rate: 0.1}),
		ratelimit.WithBackend(backend),
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

func TestGCRA_ConcurrentAccessRedis(t *testing.T) {
	backend := UseBackend(t, "redis")
	key := fmt.Sprintf("gr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithPrimaryStrategy(gcra.Config{Burst: 10, Rate: 0.1}),
		ratelimit.WithBackend(backend),
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

func TestGCRA_ConcurrentAccessMemoryWithResult(t *testing.T) {
	backend := UseBackend(t, "memory")
	key := fmt.Sprintf("gcra-mwr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithPrimaryStrategy(gcra.Config{Burst: 10, Rate: 0.1}),
		ratelimit.WithBackend(backend),
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
			var res map[string]strategies.Result
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

func TestGCRA_ConcurrentAccessPostgresWithResult(t *testing.T) {
	backend := UseBackend(t, "postgres")
	key := fmt.Sprintf("gcra-pwr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithPrimaryStrategy(gcra.Config{Burst: 10, Rate: 0.1}),
		ratelimit.WithBackend(backend),
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
			var res map[string]strategies.Result
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

func TestGCRA_ConcurrentAccessRedisWithResult(t *testing.T) {
	backend := UseBackend(t, "redis")
	key := fmt.Sprintf("gcra-rwr-%d", time.Now().UnixNano())

	limiter, err := ratelimit.New(
		ratelimit.WithBaseKey(key),
		ratelimit.WithPrimaryStrategy(gcra.Config{Burst: 10, Rate: 0.1}),
		ratelimit.WithBackend(backend),
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
			var res map[string]strategies.Result
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
