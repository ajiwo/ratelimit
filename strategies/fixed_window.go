package strategies

import (
	"context"
	"fmt"
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

// FixedWindowStrategy implements the fixed window rate limiting algorithm
type FixedWindowStrategy struct {
	windowDuration time.Duration // Duration of each fixed window
}

// NewFixedWindowStrategy creates a new fixed window strategy
func NewFixedWindowStrategy(config StrategyConfig) (*FixedWindowStrategy, error) {
	windowDuration := config.WindowDuration
	if windowDuration == 0 {
		windowDuration = time.Minute // Default: 1 minute windows
	}

	return &FixedWindowStrategy{
		windowDuration: windowDuration,
	}, nil
}

// Allow checks if a request should be allowed using the fixed window algorithm
func (fw *FixedWindowStrategy) Allow(ctx context.Context, backend backends.Backend, key string, limit int64, window time.Duration) (*StrategyResult, error) {
	now := time.Now()

	// Use the passed window duration, or fall back to the configured duration
	windowDuration := window
	if windowDuration == 0 {
		windowDuration = fw.windowDuration
	}

	// Calculate current window start time
	windowStart := fw.calculateWindowStart(now, windowDuration)

	// Use atomic check-and-increment to avoid race conditions
	// Only increment if within the limit
	count, incremented, err := backend.Increment(ctx, key, windowDuration, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to check and increment window count: %w", err)
	}

	// Calculate window end time
	windowEnd := windowStart.Add(windowDuration)

	// Request is allowed if we successfully incremented
	allowed := incremented

	// Calculate remaining requests
	remaining := max(limit-count, 0)

	// Calculate retry after (time until next window)
	var retryAfter time.Duration
	if !allowed {
		retryAfter = max(windowEnd.Sub(now), 0)
	}

	return &StrategyResult{
		Allowed:       allowed,
		Remaining:     remaining,
		ResetTime:     windowEnd,
		RetryAfter:    retryAfter,
		TotalRequests: count,
		WindowStart:   windowStart,
	}, nil
}

// GetStatus returns the current status of the fixed window
func (fw *FixedWindowStrategy) GetStatus(ctx context.Context, backend backends.Backend, key string, limit int64, window time.Duration) (*StrategyStatus, error) {
	now := time.Now()

	// Use the passed window duration, or fall back to the configured duration
	windowDuration := window
	if windowDuration == 0 {
		windowDuration = fw.windowDuration
	}

	// Calculate current window start time
	windowStart := fw.calculateWindowStart(now, windowDuration)

	// Get current window data
	data, err := backend.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get window state: %w", err)
	}

	// If no data, return fresh window status
	if data == nil {
		return &StrategyStatus{
			Limit:          limit,
			Remaining:      limit,
			ResetTime:      windowStart.Add(windowDuration),
			WindowStart:    windowStart,
			TotalRequests:  0,
			WindowDuration: windowDuration,
		}, nil
	}

	// Check if the current window has expired
	if now.Sub(data.WindowStart) >= windowDuration {
		// Window has expired, return fresh window status
		return &StrategyStatus{
			Limit:          limit,
			Remaining:      limit,
			ResetTime:      windowStart.Add(windowDuration),
			WindowStart:    windowStart,
			TotalRequests:  0,
			WindowDuration: windowDuration,
		}, nil
	}

	// Calculate remaining requests
	remaining := max(limit-data.Count, 0)

	return &StrategyStatus{
		Limit:          limit,
		Remaining:      remaining,
		ResetTime:      data.WindowStart.Add(windowDuration),
		WindowStart:    data.WindowStart,
		TotalRequests:  data.Count,
		WindowDuration: windowDuration,
	}, nil
}

// Reset clears the fixed window state for the given key
func (fw *FixedWindowStrategy) Reset(ctx context.Context, backend backends.Backend, key string) error {
	return backend.Delete(ctx, key)
}

// Name returns the name of the strategy
func (fw *FixedWindowStrategy) Name() string {
	return "fixed_window"
}

// calculateWindowStart calculates the start time of the current window
func (fw *FixedWindowStrategy) calculateWindowStart(now time.Time, windowDuration time.Duration) time.Time {
	// Truncate to window boundary
	// For example, if window is 1 minute, truncate to the minute boundary
	return now.Truncate(windowDuration)
}

// GetWindowInfo returns detailed information about the fixed window configuration
func (fw *FixedWindowStrategy) GetWindowInfo() FixedWindowInfo {
	return FixedWindowInfo{
		WindowDuration: fw.windowDuration,
	}
}

// FixedWindowInfo holds information about fixed window configuration
type FixedWindowInfo struct {
	WindowDuration time.Duration
}

// SetWindowDuration updates the window duration (use with caution in production)
func (fw *FixedWindowStrategy) SetWindowDuration(duration time.Duration) {
	if duration > 0 {
		fw.windowDuration = duration
	}
}

// CalculateOptimalWindowDuration calculates optimal window duration for given requirements
func CalculateOptimalWindowDuration(requestsPerMinute int64, desiredGranularity time.Duration) time.Duration {
	if requestsPerMinute <= 0 {
		return time.Minute
	}

	// For low rates, use longer windows for efficiency
	if requestsPerMinute <= 10 {
		return time.Minute
	}

	// For medium rates, use shorter windows for better granularity
	if requestsPerMinute <= 60 {
		if desiredGranularity > 0 && desiredGranularity < time.Minute {
			return desiredGranularity
		}
		return 30 * time.Second
	}

	// For high rates, use very short windows
	if desiredGranularity > 0 && desiredGranularity < 30*time.Second {
		return desiredGranularity
	}
	return 15 * time.Second
}

// GetCurrentWindow returns information about the current window
func (fw *FixedWindowStrategy) GetCurrentWindow(now time.Time, window time.Duration) WindowInfo {
	// Use the passed window duration, or fall back to the configured duration
	windowDuration := window
	if windowDuration == 0 {
		windowDuration = fw.windowDuration
	}

	windowStart := fw.calculateWindowStart(now, windowDuration)
	windowEnd := windowStart.Add(windowDuration)

	return WindowInfo{
		Start:    windowStart,
		End:      windowEnd,
		Duration: windowDuration,
		Progress: now.Sub(windowStart),
	}
}

// WindowInfo holds information about a specific window
type WindowInfo struct {
	Start    time.Time
	End      time.Time
	Duration time.Duration
	Progress time.Duration
}
