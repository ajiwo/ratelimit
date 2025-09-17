package strategies

import (
	"context"
	"sync"
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

// TokenBucket represents the state of a token bucket
type TokenBucket struct {
	Tokens     float64   `json:"tokens"`
	LastRefill time.Time `json:"last_refill"`
	Capacity   float64   `json:"capacity"`
	RefillRate float64   `json:"refill_rate"` // tokens per second
}

// TokenBucket implements the token bucket rate limiting algorithm
type TokenBucketStrategy struct {
	storage backends.Backend
	mu      sync.Map // per-key locks
}

func NewTokenBucket(storage backends.Backend) *TokenBucketStrategy {
	return &TokenBucketStrategy{
		storage: storage,
		mu:      sync.Map{},
	}
}

// getLock returns a mutex for the given key
func (t *TokenBucketStrategy) getLock(key string) *sync.Mutex {
	actual, _ := t.mu.LoadOrStore(key, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// Allow checks if a request is allowed based on token bucket algorithm
func (t *TokenBucketStrategy) Allow(ctx context.Context, config any) (bool, error) {
	return false, nil
}

// GetResult returns detailed statistics for the current bucket state
func (t *TokenBucketStrategy) GetResult(ctx context.Context, config any) (Result, error) {
	return Result{}, nil
}

// Reset resets the token bucket counter for the given key
func (t *TokenBucketStrategy) Reset(ctx context.Context, config any) error {
	return nil
}
