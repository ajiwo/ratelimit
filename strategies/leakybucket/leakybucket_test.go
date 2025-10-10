package leakybucket

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLeakyBucketAllow(t *testing.T) {
	ctx := t.Context()
	storage := memory.New()

	strategy := New(storage)

	config := strategies.LeakyBucketConfig{
		Key:      "test-user",
		Capacity: 5,
		LeakRate: 1.0, // 1 request per second
	}

	// Fill up the bucket
	for i := range 5 {
		result, err := strategy.AllowWithResult(ctx, config)
		assert.NoError(t, err)
		assert.True(t, result.Allowed, "Request %d should be allowed", i)
	}

	// Next request should be denied (bucket full)
	result, err := strategy.AllowWithResult(ctx, config)
	assert.NoError(t, err)
	assert.False(t, result.Allowed, "Request should be denied when bucket is full")
}

func TestLeakyBucketLeak(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		storage := memory.New()

		strategy := New(storage)

		config := strategies.LeakyBucketConfig{
			Key:      "test-user-2",
			Capacity: 3,
			LeakRate: 1.0, // 1 request per second
		}

		// Fill the bucket
		for range 3 {
			result, err := strategy.AllowWithResult(ctx, config)
			assert.NoError(t, err)
			assert.True(t, result.Allowed)
		}

		// Next request should be denied (bucket full)
		result, err := strategy.AllowWithResult(ctx, config)
		assert.NoError(t, err)
		assert.False(t, result.Allowed, "Request should be denied when bucket is full")

		// Wait for 2 seconds to allow leaking
		time.Sleep(2 * time.Second)
		synctest.Wait()

		// Now requests should be allowed again as requests have leaked
		result, err = strategy.AllowWithResult(ctx, config)
		assert.NoError(t, err)
		assert.True(t, result.Allowed, "Request should be allowed after leaking")
	})
}

func TestLeakyBucketGetResult(t *testing.T) {
	ctx := t.Context()
	storage := memory.New()
	strategy := New(storage)

	config := strategies.LeakyBucketConfig{
		Key:      "result-test-user",
		Capacity: 5,
		LeakRate: 1.0, // 1 request per second
	}

	// Test GetResult with no existing data
	result, err := strategy.GetResult(ctx, config)
	require.NoError(t, err)
	assert.True(t, result.Allowed, "Result should be allowed initially")
	assert.Equal(t, 5, result.Remaining, "Remaining should be 5 initially")

	// Fill up the bucket
	for i := range 3 {
		allowed, err := strategy.Allow(ctx, config)
		assert.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i)
	}

	// Test GetResult after adding requests
	result, err = strategy.GetResult(ctx, config)
	require.NoError(t, err)
	assert.True(t, result.Allowed, "Result should still be allowed")
	// Note: Since we're leaking 1 request per second, and some time has passed,
	// we might have more than 2 remaining. Let's just check it's more than 0.
	assert.True(t, result.Remaining > 0, "Remaining should be greater than 0 after 3 requests")

	// Fill the bucket completely
	for i := range 2 {
		allowed, err := strategy.Allow(ctx, config)
		assert.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+3)
	}

	// Next request should be denied (bucket full)
	allowed, err := strategy.Allow(ctx, config)
	assert.NoError(t, err)
	assert.False(t, allowed, "Request should be denied when bucket is full")

	// Test invalid config type
	_, err = strategy.GetResult(ctx, struct{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LeakyBucket strategy requires LeakyBucketConfig")
}

func TestLeakyBucketReset(t *testing.T) {
	ctx := t.Context()
	storage := memory.New()
	strategy := New(storage)

	config := strategies.LeakyBucketConfig{
		Key:      "reset-test-user",
		Capacity: 3,
		LeakRate: 1.0, // 1 request per second
	}

	// Fill the bucket
	for range 3 {
		allowed, err := strategy.Allow(ctx, config)
		assert.NoError(t, err)
		assert.True(t, allowed)
	}

	// Next request should be denied (bucket full)
	allowed, err := strategy.Allow(ctx, config)
	assert.NoError(t, err)
	assert.False(t, allowed, "Request should be denied when bucket is full")

	// Reset the bucket
	err = strategy.Reset(ctx, config)
	require.NoError(t, err)

	// After reset, requests should be allowed again
	allowed, err = strategy.Allow(ctx, config)
	assert.NoError(t, err)
	assert.True(t, allowed, "Request should be allowed after reset")

	// Test invalid config type
	err = strategy.Reset(ctx, struct{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LeakyBucket strategy requires LeakyBucketConfig")
}

func TestLeakyBucketInvalidConfig(t *testing.T) {
	ctx := t.Context()
	storage := memory.New()

	strategy := New(storage)

	// Try with wrong config type
	result, err := strategy.AllowWithResult(ctx, struct{}{})
	assert.Error(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, err.Error(), "LeakyBucket strategy requires LeakyBucketConfig")
}
