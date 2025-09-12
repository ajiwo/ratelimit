package strategies

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/stretchr/testify/assert"
)

func setupFixedWindowTest(t *testing.T) (context.Context, *FixedWindowStrategy, backends.Backend) {
	ctx := t.Context()
	backend, err := backends.NewMemoryBackend(backends.BackendConfig{})
	assert.NoError(t, err)

	strategy, err := NewFixedWindowStrategy(StrategyConfig{
		WindowDuration: time.Minute,
	})
	assert.NoError(t, err)

	t.Cleanup(func() {
		backend.Close()
	})

	return ctx, strategy, backend
}

func TestFixedWindow_Allow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, strategy, backend := setupFixedWindowTest(t)
		key := "fw_allow_test"

		// Allow 10 requests
		for i := range 10 {
			res, err := strategy.Allow(ctx, backend, key, 10, time.Minute)
			assert.NoError(t, err)
			assert.True(t, res.Allowed)
			assert.Equal(t, int64(10-i-1), res.Remaining)
		}

		// 11th request should be denied
		res, err := strategy.Allow(ctx, backend, key, 10, time.Minute)
		assert.NoError(t, err)
		assert.False(t, res.Allowed)
		assert.Equal(t, int64(0), res.Remaining)
		assert.Greater(t, res.RetryAfter, time.Duration(0))
	})
}

func TestFixedWindow_WindowReset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, strategy, backend := setupFixedWindowTest(t)
		key := "fw_reset_test"
		limit := int64(5)
		window := time.Second * 2

		// Exhaust the limit
		for range 5 {
			res, err := strategy.Allow(ctx, backend, key, limit, window)
			assert.NoError(t, err)
			assert.True(t, res.Allowed)
		}

		// One more should be denied
		res, err := strategy.Allow(ctx, backend, key, limit, window)
		assert.NoError(t, err)
		assert.False(t, res.Allowed)

		// Wait for the window to pass
		time.Sleep(window)

		// The next request should be allowed
		res, err = strategy.Allow(ctx, backend, key, limit, window)
		assert.NoError(t, err)
		assert.True(t, res.Allowed)
		assert.Equal(t, limit-1, res.Remaining)
	})
}

func TestFixedWindow_GetStatus(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, strategy, backend := setupFixedWindowTest(t)
		key := "fw_status_test"
		limit := int64(10)
		window := time.Minute

		status, err := strategy.GetStatus(ctx, backend, key, limit, window)
		assert.NoError(t, err)
		assert.Equal(t, limit, status.Remaining)
		assert.False(t, status.Remaining == 0)

		// Use 3 requests
		for range 3 {
			_, err := strategy.Allow(ctx, backend, key, limit, window)
			assert.NoError(t, err)
		}

		status, err = strategy.GetStatus(ctx, backend, key, limit, window)
		assert.NoError(t, err)
		assert.Equal(t, limit-3, status.Remaining)
		assert.False(t, status.Remaining == 0)

		// Use all remaining requests
		for range 7 {
			_, err := strategy.Allow(ctx, backend, key, limit, window)
			assert.NoError(t, err)
		}

		status, err = strategy.GetStatus(ctx, backend, key, limit, window)
		assert.NoError(t, err)
		assert.Equal(t, int64(0), status.Remaining)
		assert.True(t, status.Remaining == 0)
	})
}

func TestFixedWindow_Reset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, strategy, backend := setupFixedWindowTest(t)
		key := "fw_manual_reset_test"
		limit := int64(5)
		window := time.Minute

		// Use all requests
		for range 5 {
			_, err := strategy.Allow(ctx, backend, key, limit, window)
			assert.NoError(t, err)
		}

		// Check it's limited
		res, err := strategy.Allow(ctx, backend, key, limit, window)
		assert.NoError(t, err)
		assert.False(t, res.Allowed)

		// Reset the key
		err = strategy.Reset(ctx, backend, key)
		assert.NoError(t, err)

		// Check that it's allowed again
		res, err = strategy.Allow(ctx, backend, key, limit, window)
		assert.NoError(t, err)
		assert.True(t, res.Allowed)
		assert.Equal(t, limit-1, res.Remaining)
	})
}

func TestFixedWindow_Name(t *testing.T) {
	_, strategy, _ := setupFixedWindowTest(t)
	assert.Equal(t, "fixed_window", strategy.Name())
}

func TestFixedWindow_GetWindowInfo(t *testing.T) {
	_, strategy, _ := setupFixedWindowTest(t)
	info := strategy.GetWindowInfo()
	assert.Equal(t, time.Minute, info.WindowDuration)
}

func TestFixedWindow_SetWindowDuration(t *testing.T) {
	_, strategy, _ := setupFixedWindowTest(t)

	// Test setting a new duration
	newDuration := 30 * time.Second
	strategy.SetWindowDuration(newDuration)
	info := strategy.GetWindowInfo()
	assert.Equal(t, newDuration, info.WindowDuration)

	// Test that setting zero duration doesn't change it
	strategy.SetWindowDuration(0)
	info = strategy.GetWindowInfo()
	assert.Equal(t, newDuration, info.WindowDuration)
}

func TestFixedWindow_CalculateOptimalWindowDuration(t *testing.T) {
	// Test with low rate
	duration := CalculateOptimalWindowDuration(5, 0)
	assert.Equal(t, time.Minute, duration)

	// Test with medium rate and no desired granularity
	duration = CalculateOptimalWindowDuration(30, 0)
	assert.Equal(t, 30*time.Second, duration)

	// Test with medium rate and desired granularity
	duration = CalculateOptimalWindowDuration(30, 10*time.Second)
	assert.Equal(t, 10*time.Second, duration)

	// Test with high rate and no desired granularity
	duration = CalculateOptimalWindowDuration(100, 0)
	assert.Equal(t, 15*time.Second, duration)

	// Test with high rate and desired granularity
	duration = CalculateOptimalWindowDuration(100, 5*time.Second)
	assert.Equal(t, 5*time.Second, duration)

	// Test with invalid input
	duration = CalculateOptimalWindowDuration(0, 0)
	assert.Equal(t, time.Minute, duration)
}

func TestFixedWindow_GetCurrentWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		now := time.Now()
		_, strategy, _ := setupFixedWindowTest(t)

		// Test with default window duration
		windowInfo := strategy.GetCurrentWindow(now, 0)
		expectedStart := now.Truncate(time.Minute)
		assert.Equal(t, expectedStart, windowInfo.Start)
		assert.Equal(t, expectedStart.Add(time.Minute), windowInfo.End)
		assert.Equal(t, time.Minute, windowInfo.Duration)
		assert.Equal(t, now.Sub(expectedStart), windowInfo.Progress)

		// Test with custom window duration
		customWindow := 30 * time.Second
		windowInfo = strategy.GetCurrentWindow(now, customWindow)
		expectedStart = now.Truncate(customWindow)
		assert.Equal(t, expectedStart, windowInfo.Start)
		assert.Equal(t, expectedStart.Add(customWindow), windowInfo.End)
		assert.Equal(t, customWindow, windowInfo.Duration)
		assert.Equal(t, now.Sub(expectedStart), windowInfo.Progress)
	})
}

func TestFixedWindow_NewFixedWindowStrategy(t *testing.T) {
	// Test with default configuration (zero window duration)
	strategy, err := NewFixedWindowStrategy(StrategyConfig{})
	assert.NoError(t, err)
	assert.Equal(t, time.Minute, strategy.windowDuration)

	// Test with custom window duration
	customDuration := 30 * time.Second
	strategy, err = NewFixedWindowStrategy(StrategyConfig{
		WindowDuration: customDuration,
	})
	assert.NoError(t, err)
	assert.Equal(t, customDuration, strategy.windowDuration)
}
