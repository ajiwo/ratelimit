package internal

import (
	"context"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

// AllowMode represents the operation mode for `Allow`
type AllowMode int

const (
	// ReadOnly only inspects current state without modifications
	ReadOnly AllowMode = iota
	// TryUpdate attempts to consume quota with retry logic
	TryUpdate
)

// Result contains the result of Allow operation
type Result struct {
	Allowed   bool
	Remaining int
	Reset     time.Time
	// For internal use: indicates if state was updated (only meaningful in tryAndUpdateMode)
	stateUpdated bool
}

// Allow provides a unified implementation for both Allow and Peek operations
// mode determines whether to perform read-only inspection or actual quota consumption
func Allow(ctx context.Context, storage backends.Backend, config Config, mode AllowMode) (Result, error) {
	if ctx.Err() != nil {
		return Result{}, NewContextCanceledError(ctx.Err())
	}

	now := time.Now()
	leakRate := config.GetLeakRate()
	capacity := config.GetCapacity()

	// Read-only mode: just get current state and calculate results
	if mode == ReadOnly {
		return allowReadOnly(ctx, storage, config, now, leakRate, capacity)
	}

	// Try-and-update mode: attempt to consume quota with retries
	return allowTryAndUpdate(ctx, storage, config, now, leakRate, capacity)
}

// allowReadOnly implements read-only mode
func allowReadOnly(ctx context.Context, storage backends.Backend, lbConfig Config, now time.Time, leakRate float64, capacity int) (Result, error) {
	// Get current state
	data, err := storage.Get(ctx, lbConfig.GetKey())
	if err != nil {
		return Result{}, NewStateRetrievalError(err)
	}

	var bucket LeakyBucket
	if data == "" {
		// No existing bucket, return default state
		return Result{
			Allowed:      true,
			Remaining:    capacity,
			Reset:        now, // Leaky buckets don't have a reset time, they continuously leak
			stateUpdated: false,
		}, nil
	}

	// Parse existing bucket state
	if b, ok := decodeState(data); ok {
		bucket = b
	} else {
		return Result{}, ErrStateParsing
	}

	// Leak requests based on time elapsed
	timeElapsed := now.Sub(bucket.LastLeak).Seconds()
	requestsToLeak := timeElapsed * leakRate
	bucket.Requests = max(0.0, bucket.Requests-requestsToLeak)
	bucket.LastLeak = now

	// Calculate remaining capacity
	remaining := max(capacity-int(bucket.Requests), 0)

	return Result{
		Allowed:      remaining > 0,
		Remaining:    remaining,
		Reset:        now, // Leaky buckets don't have a reset time
		stateUpdated: false,
	}, nil
}

// allowTryAndUpdate implements try-and-update mode with retries
func allowTryAndUpdate(ctx context.Context, storage backends.Backend, lbConfig Config, now time.Time, leakRate float64, capacity int) (Result, error) {
	maxRetries := lbConfig.MaxRetries()
	if maxRetries <= 0 {
		maxRetries = strategies.DefaultMaxRetries
	}

	// Try atomic CheckAndSet operations first
	for attempt := range maxRetries {
		// Check if context is canceled or timed out
		if ctx.Err() != nil {
			return Result{}, NewContextCanceledError(ctx.Err())
		}

		// Get current bucket state
		data, err := storage.Get(ctx, lbConfig.GetKey())
		if err != nil {
			return Result{}, NewStateRetrievalError(err)
		}

		var bucket LeakyBucket
		var oldValue any
		if data == "" {
			// Initialize new bucket
			bucket = LeakyBucket{
				Requests: 0,
				LastLeak: now,
			}
			oldValue = nil // Key doesn't exist
		} else {
			// Parse existing bucket state
			if b, ok := decodeState(data); ok {
				bucket = b
			} else {
				return Result{}, ErrStateParsing
			}
			oldValue = data

			// Leak requests based on elapsed time
			elapsed := now.Sub(bucket.LastLeak)
			requestsToLeak := float64(elapsed.Nanoseconds()) * leakRate / 1e9
			bucket.Requests = max(0.0, bucket.Requests-requestsToLeak)
			bucket.LastLeak = now
		}

		// Calculate if request is allowed
		allowed := bucket.Requests+1 <= float64(capacity)

		if allowed {
			// Add request to bucket
			bucket.Requests += 1.0

			// Calculate remaining capacity after adding request
			remaining := max(capacity-int(bucket.Requests), 0)

			// Save updated bucket state
			newValue := encodeState(bucket)
			expiration := strategies.CalcExpiration(capacity, leakRate)

			success, err := storage.CheckAndSet(ctx, lbConfig.GetKey(), oldValue, newValue, expiration)
			if err != nil {
				return Result{}, NewStateSaveError(err)
			}

			if success {
				// Atomic update succeeded
				return Result{
					Allowed:      true,
					Remaining:    remaining,
					Reset:        now, // When allowed, no specific reset needed
					stateUpdated: true,
				}, nil
			}

			// If CheckAndSet failed, retry if we haven't exhausted attempts
			if attempt < maxRetries-1 {
				time.Sleep((19 * time.Nanosecond) << (time.Duration(attempt)))
				continue
			}
			break
		} else {
			// Request denied, return current remaining capacity
			remaining := max(capacity-int(bucket.Requests), 0)

			// Save the current bucket state (even when denying, to handle leaks)
			bucketData := encodeState(bucket)
			expiration := strategies.CalcExpiration(capacity, leakRate)

			// If this was a new bucket (rare case), set it
			if oldValue == nil {
				_, err := storage.CheckAndSet(ctx, lbConfig.GetKey(), oldValue, bucketData, expiration)
				if err != nil {
					return Result{}, NewStateSaveError(err)
				}
			}

			return Result{
				Allowed:      false,
				Remaining:    remaining,
				Reset:        calculateResetTime(now, bucket, capacity, leakRate),
				stateUpdated: oldValue == nil,
			}, nil
		}
	}

	// CheckAndSet failed after checkAndSetRetries attempts
	return Result{}, ErrConcurrentAccess
}

// calculateResetTime calculates when the bucket will have capacity for another request
func calculateResetTime(now time.Time, bucket LeakyBucket, capacity int, leakRate float64) time.Time {
	if bucket.Requests < float64(capacity) {
		// Already has capacity, no reset needed
		return now
	}

	// Calculate time to leak (bucket.Requests - capacity + 1) requests
	requestsToLeak := bucket.Requests - float64(capacity) + 1
	if requestsToLeak <= 0 {
		return now
	}

	// time = requests / leakRate (convert from float seconds to time.Duration)
	timeToLeakSeconds := requestsToLeak / leakRate
	return now.Add(time.Duration(timeToLeakSeconds * float64(time.Second)))
}
