package strategies

import (
	"context"
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
	return false, nil
}

// GetResult returns detailed statistics for the current window state
func (f *FixedWindowStrategy) GetResult(ctx context.Context, config any) (Result, error) {
	return Result{}, nil
}

// Reset resets the rate limit counter for the given key
func (f *FixedWindowStrategy) Reset(ctx context.Context, config any) error {
	return nil
}
