package strategies

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

// LeakyBucket represents the state of a leaky bucket
type LeakyBucket struct {
	Requests float64   `json:"requests"`  // Current number of requests in the bucket
	LastLeak time.Time `json:"last_leak"` // Last time we leaked requests
	Capacity float64   `json:"capacity"`  // Maximum requests the bucket can hold
	LeakRate float64   `json:"leak_rate"` // Requests to leak per second
}

// LeakyBucketStrategy implements the leaky bucket rate limiting algorithm
type LeakyBucketStrategy struct {
	storage backends.Backend
	mu      sync.Map // per-key locks
}

// NewLeakyBucket creates a new leaky bucket strategy
func NewLeakyBucket(storage backends.Backend) *LeakyBucketStrategy {
	return &LeakyBucketStrategy{
		storage: storage,
		mu:      sync.Map{},
	}
}

// getLock returns a mutex for the given key
func (l *LeakyBucketStrategy) getLock(key string) *sync.Mutex {
	actual, _ := l.mu.LoadOrStore(key, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// Allow checks if a request is allowed based on leaky bucket algorithm
func (l *LeakyBucketStrategy) Allow(ctx context.Context, config any) (bool, error) {
	_ = l.getLock("")
	return false, fmt.Errorf("not implemented")
}

// GetResult returns detailed statistics for the current bucket state
func (l *LeakyBucketStrategy) GetResult(ctx context.Context, config any) (Result, error) {
	return Result{}, fmt.Errorf("not implemented")
}

// Reset resets the leaky bucket counter for the given key
func (l *LeakyBucketStrategy) Reset(ctx context.Context, config any) error {
	return fmt.Errorf("not implemented")
}
