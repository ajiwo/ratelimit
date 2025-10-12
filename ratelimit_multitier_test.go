package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiter_FixedWindow_MultiTier(t *testing.T) {
	backend := memory.New()

	// Create a multi-tier fixed window configuration
	config := strategies.FixedWindowConfig{
		Key: "multitier:test",
		Tiers: map[string]strategies.FixedWindowTier{
			"burst": {
				Limit:  10, // Allow 10 requests per minute
				Window: time.Minute,
			},
			"sustained": {
				Limit:  100, // Allow 100 requests per hour
				Window: time.Hour,
			},
		},
	}

	limiter, err := New(
		WithBackend(backend),
		WithPrimaryStrategy(config),
		WithBaseKey("test"),
	)
	require.NoError(t, err)
	defer limiter.Close()

	// Should allow first 10 requests (burst tier limit)
	for i := range 10 {
		allowed, err := limiter.Allow(WithContext(context.Background()))
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// 11th request should be denied by burst tier
	allowed, err := limiter.Allow(WithContext(context.Background()))
	require.NoError(t, err)
	assert.False(t, allowed, "Request 11 should be denied by burst tier")

	// Get stats to verify both tiers are working
	stats, err := limiter.GetStats(WithContext(context.Background()))
	require.NoError(t, err)
	require.Len(t, stats, 2) // Both tiers should return results

	// Check burst tier (should be exhausted)
	burstResult := stats["primary_burst"]
	assert.False(t, burstResult.Allowed, "Burst tier should be exhausted")
	assert.Equal(t, 0, burstResult.Remaining, "Burst tier should have 0 remaining")

	// Check sustained tier (should have remaining quota)
	sustainedResult := stats["primary_sustained"]
	assert.True(t, sustainedResult.Allowed, "Sustained tier should still allow")
	assert.Equal(t, 90, sustainedResult.Remaining, "Sustained tier should have 90 remaining")
}

func TestRateLimiter_FixedWindow_BackwardCompatibility(t *testing.T) {
	backend := memory.New()

	// Test the NewFixedWindowConfig helper function
	limiter, err := New(
		WithBackend(backend),
		WithPrimaryStrategy(strategies.NewFixedWindowConfig("test:compat", 5, time.Minute)),
		WithBaseKey("test"),
	)
	require.NoError(t, err)
	defer limiter.Close()

	// Should allow first 5 requests
	for i := range 5 {
		allowed, err := limiter.Allow(WithContext(context.Background()))
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// 6th request should be denied
	allowed, err := limiter.Allow(WithContext(context.Background()))
	require.NoError(t, err)
	assert.False(t, allowed, "Request 6 should be denied")

	// Get stats to verify single tier works
	stats, err := limiter.GetStats(WithContext(context.Background()))
	require.NoError(t, err)
	require.Len(t, stats, 1) // Only one tier

	// Check default tier
	defaultResult := stats["primary_default"]
	assert.False(t, defaultResult.Allowed, "Default tier should be exhausted")
	assert.Equal(t, 0, defaultResult.Remaining, "Default tier should have 0 remaining")
}
