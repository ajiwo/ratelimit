package strategies

import (
	"context"
	"encoding/json"
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
	// Type assert to LeakyBucketConfig
	leakyConfig, ok := config.(LeakyBucketConfig)
	if !ok {
		return false, fmt.Errorf("LeakyBucket strategy requires LeakyBucketConfig")
	}

	// Get per-key lock to prevent concurrent access to the same bucket
	lock := l.getLock(leakyConfig.Key)
	lock.Lock()
	defer lock.Unlock()

	capacity := float64(leakyConfig.Capacity)
	leakRate := leakyConfig.LeakRate

	now := time.Now()

	// Get current bucket state
	data, err := l.storage.Get(ctx, leakyConfig.Key)
	if err != nil {
		return false, fmt.Errorf("failed to get bucket state: %w", err)
	}

	var bucket LeakyBucket
	if data == "" {
		// Initialize new bucket
		bucket = LeakyBucket{
			Requests: 0,
			LastLeak: now,
			Capacity: capacity,
			LeakRate: leakRate,
		}
	} else {
		// Parse existing bucket state
		err := json.Unmarshal([]byte(data), &bucket)
		if err != nil {
			return false, fmt.Errorf("failed to parse bucket state: %w", err)
		}

		// Leak requests based on elapsed time
		elapsed := now.Sub(bucket.LastLeak)
		requestsToLeak := float64(elapsed.Nanoseconds()) * bucket.LeakRate / 1e9
		bucket.Requests = max(bucket.Requests-requestsToLeak, 0)
		bucket.LastLeak = now
	}

	// Check if bucket would be full after adding request
	if bucket.Requests+1 > bucket.Capacity {
		// Save updated bucket state even when denying request
		// We still need to save the leaked state
		bucketData, err := json.Marshal(bucket)
		if err != nil {
			return false, fmt.Errorf("failed to marshal bucket state: %w", err)
		}

		// Use a reasonable expiration time (based on capacity and leak rate)
		// Ensure minimum expiration of 1 second
		expirationSeconds := float64(leakyConfig.Capacity) / leakyConfig.LeakRate * 2
		if expirationSeconds < 1 {
			expirationSeconds = 1
		}
		expiration := time.Duration(expirationSeconds) * time.Second

		// Save the updated bucket state
		err = l.storage.Set(ctx, leakyConfig.Key, string(bucketData), expiration)
		if err != nil {
			return false, fmt.Errorf("failed to save bucket state: %w", err)
		}

		return false, nil
	}

	// Add request to bucket
	bucket.Requests += 1

	// Save updated bucket state
	bucketData, err := json.Marshal(bucket)
	if err != nil {
		return false, fmt.Errorf("failed to marshal bucket state: %w", err)
	}

	// Use a reasonable expiration time (based on capacity and leak rate)
	// Ensure minimum expiration of 1 second
	expirationSeconds := float64(leakyConfig.Capacity) / leakyConfig.LeakRate * 2
	if expirationSeconds < 1 {
		expirationSeconds = 1
	}
	expiration := time.Duration(expirationSeconds) * time.Second

	// Save the updated bucket state
	err = l.storage.Set(ctx, leakyConfig.Key, string(bucketData), expiration)
	if err != nil {
		return false, fmt.Errorf("failed to save bucket state: %w", err)
	}

	return true, nil
}

// GetResult returns detailed statistics for the current bucket state
func (l *LeakyBucketStrategy) GetResult(ctx context.Context, config any) (Result, error) {
	// Type assert to LeakyBucketConfig
	leakyConfig, ok := config.(LeakyBucketConfig)
	if !ok {
		return Result{}, fmt.Errorf("LeakyBucket strategy requires LeakyBucketConfig")
	}

	// Get per-key lock to prevent concurrent access to the same bucket
	lock := l.getLock(leakyConfig.Key)
	lock.Lock()
	defer lock.Unlock()

	now := time.Now()

	// Get current bucket state
	data, err := l.storage.Get(ctx, leakyConfig.Key)
	if err != nil {
		return Result{}, fmt.Errorf("failed to get bucket state: %w", err)
	}

	var bucket LeakyBucket
	if data == "" {
		// No existing bucket, return default state
		return Result{
			Allowed:   true,
			Remaining: leakyConfig.Capacity,
			Reset:     now, // Leaky buckets don't have a reset time, they continuously leak
		}, nil
	}

	// Parse existing bucket state
	err = json.Unmarshal([]byte(data), &bucket)
	if err != nil {
		return Result{}, fmt.Errorf("failed to parse bucket state: %w", err)
	}

	// Leak requests based on time elapsed
	timeElapsed := now.Sub(bucket.LastLeak).Seconds()
	requestsToLeak := timeElapsed * bucket.LeakRate
	bucket.Requests = max(0, bucket.Requests-requestsToLeak)
	bucket.LastLeak = now

	// Calculate remaining capacity
	remaining := max(leakyConfig.Capacity-int(bucket.Requests), 0)

	return Result{
		Allowed:   remaining > 0,
		Remaining: remaining,
		Reset:     now, // Leaky buckets don't have a reset time
	}, nil
}

// Reset resets the leaky bucket counter for the given key
func (l *LeakyBucketStrategy) Reset(ctx context.Context, config any) error {
	// Type assert to LeakyBucketConfig
	leakyConfig, ok := config.(LeakyBucketConfig)
	if !ok {
		return fmt.Errorf("LeakyBucket strategy requires LeakyBucketConfig")
	}

	// Get per-key lock to prevent concurrent access to the same bucket
	lock := l.getLock(leakyConfig.Key)
	lock.Lock()
	defer lock.Unlock()

	// Delete the key from storage to reset the bucket
	return l.storage.Delete(ctx, leakyConfig.Key)
}
