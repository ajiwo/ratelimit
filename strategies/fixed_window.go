package strategies

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

// FixedWindow represents the state of a fixed window
type FixedWindow struct {
	Count    int           `json:"count"`    // Current request count in the window
	Start    time.Time     `json:"start"`    // Window start time
	Duration time.Duration `json:"duration"` // Window duration
}

// FixedWindowStrategy implements the fixed window rate limiting algorithm
type FixedWindowStrategy struct {
	storage backends.Backend
	mu      sync.Map // per-key locks
}

// NewFixedWindow creates a new fixed window strategy
func NewFixedWindow(storage backends.Backend) *FixedWindowStrategy {
	return &FixedWindowStrategy{
		storage: storage,
		mu:      sync.Map{},
	}
}

// getLock returns a mutex for the given key
func (f *FixedWindowStrategy) getLock(key string) *sync.Mutex {
	actual, _ := f.mu.LoadOrStore(key, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// Allow checks if a request is allowed based on fixed window algorithm
func (f *FixedWindowStrategy) Allow(ctx context.Context, config any) (bool, error) {
	// Type assert to FixedWindowConfig
	fixedConfig, ok := config.(FixedWindowConfig)
	if !ok {
		return false, fmt.Errorf("FixedWindow strategy requires FixedWindowConfig")
	}

	// Get per-key lock to prevent concurrent access to the same window
	lock := f.getLock(fixedConfig.Key)
	lock.Lock()
	defer lock.Unlock()

	now := time.Now()

	// Get current window state
	data, err := f.storage.Get(ctx, fixedConfig.Key)
	if err != nil {
		return false, fmt.Errorf("failed to get window state: %w", err)
	}

	var window FixedWindow
	if data == "" {
		// Initialize new window
		window = FixedWindow{
			Count:    0,
			Start:    now,
			Duration: fixedConfig.Window,
		}
	} else {
		// Parse existing window state
		err := json.Unmarshal([]byte(data), &window)
		if err != nil {
			return false, fmt.Errorf("failed to parse window state: %w", err)
		}

		// Check if current window has expired
		if now.Sub(window.Start) >= window.Duration {
			// Start new window
			window.Count = 0
			window.Start = now
			window.Duration = fixedConfig.Window
		}
	}

	// Check if limit has been reached
	if window.Count >= fixedConfig.Limit {
		// Save window state even when denying request
		windowData, err := json.Marshal(window)
		if err != nil {
			return false, fmt.Errorf("failed to marshal window state: %w", err)
		}

		// Save the updated window state with expiration set to window duration
		err = f.storage.Set(ctx, fixedConfig.Key, string(windowData), window.Duration)
		if err != nil {
			return false, fmt.Errorf("failed to save window state: %w", err)
		}

		return false, nil
	}

	// Increment request count
	window.Count += 1

	// Save updated window state
	windowData, err := json.Marshal(window)
	if err != nil {
		return false, fmt.Errorf("failed to marshal window state: %w", err)
	}

	// Save the updated window state with expiration set to window duration
	err = f.storage.Set(ctx, fixedConfig.Key, string(windowData), window.Duration)
	if err != nil {
		return false, fmt.Errorf("failed to save window state: %w", err)
	}

	return true, nil
}

// GetResult returns detailed statistics for the current window state
func (f *FixedWindowStrategy) GetResult(ctx context.Context, config any) (Result, error) {
	// Type assert to FixedWindowConfig
	fixedConfig, ok := config.(FixedWindowConfig)
	if !ok {
		return Result{}, fmt.Errorf("FixedWindow strategy requires FixedWindowConfig")
	}

	// Get per-key lock to prevent concurrent access to the same window
	lock := f.getLock(fixedConfig.Key)
	lock.Lock()
	defer lock.Unlock()

	now := time.Now()

	// Get current window state
	data, err := f.storage.Get(ctx, fixedConfig.Key)
	if err != nil {
		return Result{}, fmt.Errorf("failed to get window state: %w", err)
	}

	var window FixedWindow
	if data == "" {
		// No existing window, return default state
		return Result{
			Allowed:   true,
			Remaining: fixedConfig.Limit,
			Reset:     now.Add(fixedConfig.Window),
		}, nil
	}

	// Parse existing window state
	err = json.Unmarshal([]byte(data), &window)
	if err != nil {
		return Result{}, fmt.Errorf("failed to parse window state: %w", err)
	}

	// Check if current window has expired
	if now.Sub(window.Start) >= window.Duration {
		// Window has expired, return fresh state
		return Result{
			Allowed:   true,
			Remaining: fixedConfig.Limit,
			Reset:     now.Add(fixedConfig.Window),
		}, nil
	}

	// Calculate remaining requests and reset time
	remaining := max(fixedConfig.Limit-window.Count, 0)

	resetTime := window.Start.Add(window.Duration)

	return Result{
		Allowed:   remaining > 0,
		Remaining: remaining,
		Reset:     resetTime,
	}, nil
}

// Reset resets the rate limit counter for the given key
func (f *FixedWindowStrategy) Reset(ctx context.Context, config any) error {
	// Type assert to FixedWindowConfig
	fixedConfig, ok := config.(FixedWindowConfig)
	if !ok {
		return fmt.Errorf("FixedWindow strategy requires FixedWindowConfig")
	}

	// Get per-key lock to prevent concurrent access to the same window
	lock := f.getLock(fixedConfig.Key)
	lock.Lock()
	defer lock.Unlock()

	// Delete the key from storage to reset the counter
	return f.storage.Delete(ctx, fixedConfig.Key)
}
