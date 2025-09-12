package strategies

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/stretchr/testify/assert"
)

func setupTokenBucketTest(t *testing.T) (context.Context, *TokenBucketStrategy, backends.Backend) {
	ctx := t.Context()
	backend, err := backends.NewMemoryBackend(backends.BackendConfig{})
	assert.NoError(t, err)

	strategy, err := NewTokenBucketStrategy(StrategyConfig{
		BucketSize:   20,
		RefillRate:   time.Second,
		RefillAmount: 10,
	})
	assert.NoError(t, err)

	t.Cleanup(func() {
		backend.Close()
	})

	return ctx, strategy, backend
}

func TestTokenBucket_Allow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, strategy, backend := setupTokenBucketTest(t)
		key := "tb_allow_test"

		// Consume all 20 tokens (burst)
		for i := 0; i < int(strategy.bucketSize); i++ {
			res, err := strategy.Allow(ctx, backend, key, strategy.bucketSize, time.Minute)
			assert.NoError(t, err)
			assert.True(t, res.Allowed)
		}

		// 21st request should be denied as bucket is empty
		res, err := strategy.Allow(ctx, backend, key, strategy.bucketSize, time.Minute)
		assert.NoError(t, err)
		assert.False(t, res.Allowed)
		assert.Equal(t, int64(0), res.Remaining)
		assert.Greater(t, res.RetryAfter, time.Duration(0))
	})
}

func TestTokenBucket_Refill(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, strategy, backend := setupTokenBucketTest(t)
		key := "tb_refill_test"

		// Consume all 20 tokens
		for i := 0; i < int(strategy.bucketSize); i++ {
			_, err := strategy.Allow(ctx, backend, key, strategy.bucketSize, time.Minute)
			assert.NoError(t, err)
		}

		// Check that it's limited
		res, err := strategy.Allow(ctx, backend, key, strategy.bucketSize, time.Minute)
		assert.NoError(t, err)
		assert.False(t, res.Allowed)

		// Wait for a refill cycle (refills 10 tokens)
		time.Sleep(strategy.refillRate)

		// Now 10 requests should be allowed
		for i := 0; i < int(strategy.refillAmount); i++ {
			res, err := strategy.Allow(ctx, backend, key, strategy.bucketSize, time.Minute)
			assert.NoError(t, err)
			assert.True(t, res.Allowed)
		}

		// The next one should be denied
		res, err = strategy.Allow(ctx, backend, key, strategy.bucketSize, time.Minute)
		assert.NoError(t, err)
		assert.False(t, res.Allowed)
	})
}

func TestTokenBucket_GetStatus(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, strategy, backend := setupTokenBucketTest(t)
		key := "tb_status_test"

		status, err := strategy.GetStatus(ctx, backend, key, strategy.bucketSize, time.Minute)
		assert.NoError(t, err)
		assert.Equal(t, strategy.bucketSize, status.Remaining)
		assert.False(t, status.Remaining == 0)

		// Consume 5 tokens
		for range 5 {
			_, err := strategy.Allow(ctx, backend, key, strategy.bucketSize, time.Minute)
			assert.NoError(t, err)
		}

		status, err = strategy.GetStatus(ctx, backend, key, strategy.bucketSize, time.Minute)
		assert.NoError(t, err)
		assert.Equal(t, strategy.bucketSize-5, status.Remaining)
		assert.False(t, status.Remaining == 0)
	})
}

func TestTokenBucket_Reset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, strategy, backend := setupTokenBucketTest(t)
		key := "tb_manual_reset_test"

		// Consume all tokens
		for i := 0; i < int(strategy.bucketSize); i++ {
			_, err := strategy.Allow(ctx, backend, key, strategy.bucketSize, time.Minute)
			assert.NoError(t, err)
		}

		// Check it's limited
		res, err := strategy.Allow(ctx, backend, key, strategy.bucketSize, time.Minute)
		assert.NoError(t, err)
		assert.False(t, res.Allowed)

		// Reset the key
		err = strategy.Reset(ctx, backend, key)
		assert.NoError(t, err)

		// Check that the bucket is full again
		status, err := strategy.GetStatus(ctx, backend, key, strategy.bucketSize, time.Minute)
		assert.NoError(t, err)
		assert.Equal(t, strategy.bucketSize, status.Remaining)

		res, err = strategy.Allow(ctx, backend, key, strategy.bucketSize, time.Minute)
		assert.NoError(t, err)
		assert.True(t, res.Allowed)
	})
}

func TestTokenBucket_Name(t *testing.T) {
	_, strategy, _ := setupTokenBucketTest(t)
	assert.Equal(t, "token_bucket", strategy.Name())
}

func TestTokenBucket_GetBucketInfo(t *testing.T) {
	_, strategy, _ := setupTokenBucketTest(t)
	info := strategy.GetBucketInfo()
	assert.Equal(t, time.Second, info.RefillRate)
	assert.Equal(t, int64(10), info.RefillAmount)
	assert.Equal(t, int64(20), info.BucketSize)
}

func TestTokenBucket_SetRefillRate(t *testing.T) {
	_, strategy, _ := setupTokenBucketTest(t)

	// Test setting a new refill rate
	newRate := 30 * time.Second
	strategy.SetRefillRate(newRate)
	info := strategy.GetBucketInfo()
	assert.Equal(t, newRate, info.RefillRate)

	// Test that setting zero rate doesn't change it
	strategy.SetRefillRate(0)
	info = strategy.GetBucketInfo()
	assert.Equal(t, newRate, info.RefillRate)
}

func TestTokenBucket_SetRefillAmount(t *testing.T) {
	_, strategy, _ := setupTokenBucketTest(t)

	// Test setting a new refill amount
	newAmount := int64(5)
	strategy.SetRefillAmount(newAmount)
	info := strategy.GetBucketInfo()
	assert.Equal(t, newAmount, info.RefillAmount)

	// Test that setting zero amount doesn't change it
	strategy.SetRefillAmount(0)
	info = strategy.GetBucketInfo()
	assert.Equal(t, newAmount, info.RefillAmount)
}

func TestTokenBucket_CalculateOptimalRefillRate(t *testing.T) {
	// Test with low rate
	rate, amount := CalculateOptimalRefillRate(5, 10)
	assert.Equal(t, time.Minute/5, rate)
	assert.Equal(t, int64(1), amount)

	// Test with medium rate
	rate, amount = CalculateOptimalRefillRate(30, 20)
	assert.Equal(t, time.Minute/12, rate) // Max 12 refills per minute
	assert.Equal(t, int64(2), amount)     // 30/12 = 2.5, rounded down to 2

	// Test with high rate
	rate, amount = CalculateOptimalRefillRate(100, 50)
	assert.Equal(t, time.Minute/12, rate) // Max 12 refills per minute
	assert.Equal(t, int64(8), amount)     // 100/12 = 8.33, rounded down to 8

	// Test with invalid input
	rate, amount = CalculateOptimalRefillRate(0, 10)
	assert.Equal(t, time.Minute, rate)
	assert.Equal(t, int64(1), amount)
}

func TestTokenBucket_NewTokenBucketStrategy(t *testing.T) {
	// Test with default configuration (zero values)
	strategy, err := NewTokenBucketStrategy(StrategyConfig{})
	assert.NoError(t, err)
	assert.Equal(t, time.Minute, strategy.refillRate)
	assert.Equal(t, int64(1), strategy.refillAmount)
	assert.Equal(t, int64(1), strategy.bucketSize)

	// Test with custom configuration
	strategy, err = NewTokenBucketStrategy(StrategyConfig{
		RefillRate:   30 * time.Second,
		RefillAmount: 5,
		BucketSize:   20,
	})
	assert.NoError(t, err)
	assert.Equal(t, 30*time.Second, strategy.refillRate)
	assert.Equal(t, int64(5), strategy.refillAmount)
	assert.Equal(t, int64(20), strategy.bucketSize)
}

func TestTokenBucket_CalculateTimeToNextToken(t *testing.T) {
	_, strategy, _ := setupTokenBucketTest(t)

	// Test when we already have tokens
	duration := strategy.calculateTimeToNextToken(1.5)
	assert.Equal(t, time.Duration(0), duration)

	// Test when we have no tokens
	duration = strategy.calculateTimeToNextToken(0.0)
	expected := time.Duration(float64(strategy.refillRate) / float64(strategy.refillAmount))
	assert.Equal(t, expected, duration)

	// Test when we have partial tokens
	duration = strategy.calculateTimeToNextToken(0.5)
	fractionNeeded := 0.5 / float64(strategy.refillAmount)
	expected = time.Duration(fractionNeeded * float64(strategy.refillRate))
	assert.Equal(t, expected, duration)
}
