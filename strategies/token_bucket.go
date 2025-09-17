package strategies

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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
	// Type assert to TokenBucketConfig
	tokenConfig, ok := config.(TokenBucketConfig)
	if !ok {
		return false, fmt.Errorf("TokenBucket strategy requires TokenBucketConfig")
	}

	// Get per-key lock to prevent concurrent access to the same bucket
	lock := t.getLock(tokenConfig.Key)
	lock.Lock()
	defer lock.Unlock()

	capacity := float64(tokenConfig.BurstSize)
	refillRate := tokenConfig.RefillRate

	now := time.Now()

	// Get current bucket state
	data, err := t.storage.Get(ctx, tokenConfig.Key)
	if err != nil {
		return false, fmt.Errorf("failed to get bucket state: %w", err)
	}

	var bucket TokenBucket
	if data == "" {
		// Initialize new bucket
		bucket = TokenBucket{
			Tokens:     capacity,
			LastRefill: now,
			Capacity:   capacity,
			RefillRate: refillRate,
		}
	} else {
		// Parse existing bucket state
		err := json.Unmarshal([]byte(data), &bucket)
		if err != nil {
			return false, fmt.Errorf("failed to parse bucket state: %w", err)
		}

		// Refill tokens based on elapsed time
		elapsed := now.Sub(bucket.LastRefill)
		tokensToAdd := float64(elapsed.Nanoseconds()) * bucket.RefillRate / 1e9
		bucket.Tokens = math.Min(bucket.Tokens+tokensToAdd, bucket.Capacity)
		bucket.LastRefill = now
	}

	// Check if we have enough tokens (use floor to avoid fractional tokens)
	if math.Floor(bucket.Tokens) < 1 {
		return false, nil
	}

	// Consume one token
	bucket.Tokens -= 1

	// Save updated bucket state
	bucketData, err := json.Marshal(bucket)
	if err != nil {
		return false, fmt.Errorf("failed to marshal bucket state: %w", err)
	}

	// Use a reasonable expiration time (based on burst size and refill rate)
	// Ensure minimum expiration of 1 second
	expirationSeconds := float64(tokenConfig.BurstSize) / tokenConfig.RefillRate * 2
	if expirationSeconds < 1 {
		expirationSeconds = 1
	}
	expiration := time.Duration(expirationSeconds) * time.Second

	// Save the updated bucket state
	err = t.storage.Set(ctx, tokenConfig.Key, string(bucketData), expiration)
	if err != nil {
		return false, fmt.Errorf("failed to save bucket state: %w", err)
	}

	return true, nil
}

// GetResult returns detailed statistics for the current bucket state
func (t *TokenBucketStrategy) GetResult(ctx context.Context, config any) (Result, error) {
	// Type assert to TokenBucketConfig
	tokenConfig, ok := config.(TokenBucketConfig)
	if !ok {
		return Result{}, fmt.Errorf("TokenBucket strategy requires TokenBucketConfig")
	}

	// Get per-key lock to prevent concurrent access to the same bucket
	lock := t.getLock(tokenConfig.Key)
	lock.Lock()
	defer lock.Unlock()

	now := time.Now()

	// Get current bucket state
	data, err := t.storage.Get(ctx, tokenConfig.Key)
	if err != nil {
		return Result{}, fmt.Errorf("failed to get bucket state: %w", err)
	}

	var bucket TokenBucket
	if data == "" {
		// No existing bucket, return default state
		return Result{
			Allowed:   true,
			Remaining: tokenConfig.BurstSize,
			Reset:     now, // Token buckets don't have a reset time, they continuously refill
		}, nil
	}

	// Parse existing bucket state
	err = json.Unmarshal([]byte(data), &bucket)
	if err != nil {
		return Result{}, fmt.Errorf("failed to parse bucket state: %w", err)
	}

	// Refill tokens based on time elapsed
	timeElapsed := now.Sub(bucket.LastRefill).Seconds()
	tokensToAdd := timeElapsed * bucket.RefillRate
	bucket.Tokens = math.Min(bucket.Tokens+tokensToAdd, bucket.Capacity)
	bucket.LastRefill = now

	// Calculate remaining requests (floor of available tokens)
	remaining := max(int(bucket.Tokens), 0)

	return Result{
		Allowed:   remaining > 0,
		Remaining: remaining,
		Reset:     now, // Token buckets don't have a reset time
	}, nil
}

// Reset resets the token bucket counter for the given key
func (t *TokenBucketStrategy) Reset(ctx context.Context, config any) error {
	// Type assert to TokenBucketConfig
	tokenConfig, ok := config.(TokenBucketConfig)
	if !ok {
		return fmt.Errorf("TokenBucket strategy requires TokenBucketConfig")
	}

	// Get per-key lock to prevent concurrent access to the same bucket
	lock := t.getLock(tokenConfig.Key)
	lock.Lock()
	defer lock.Unlock()

	// Delete the key from storage to reset the bucket
	return t.storage.Delete(ctx, tokenConfig.Key)
}
