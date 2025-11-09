package internal

import (
	"context"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/utils"
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

type parameter struct {
	capacity   int
	key        string
	leakRate   float64
	maxRetries int
	now        time.Time
	storage    backends.Backend
}

// Allow provides a unified implementation for both Allow and Peek operations
// mode determines whether to perform read-only inspection or actual quota consumption
func Allow(
	ctx context.Context,
	storage backends.Backend,
	config Config,
	mode AllowMode,

) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, NewContextCanceledError(err)
	}

	maxRetries := config.MaxRetries()
	if maxRetries <= 0 {
		maxRetries = strategies.DefaultMaxRetries
	}

	p := &parameter{
		storage:    storage,
		key:        config.GetKey(),
		now:        time.Now(),
		leakRate:   config.GetLeakRate(),
		capacity:   config.GetCapacity(),
		maxRetries: maxRetries,
	}

	// Read-only mode: just get current state and calculate results
	if mode == ReadOnly {
		return p.allowReadOnly(ctx)
	}

	// Try-and-update mode: attempt to consume quota with retries
	return p.allowTryAndUpdate(ctx)
}

// allowReadOnly implements read-only mode
func (p *parameter) allowReadOnly(ctx context.Context) (Result, error) {
	// Get current state
	data, err := p.storage.Get(ctx, p.key)
	if err != nil {
		return Result{}, NewStateRetrievalError(err)
	}

	var bucket LeakyBucket
	if data == "" {
		// No existing bucket, return default state
		return Result{
			Allowed:      true,
			Remaining:    p.capacity,
			Reset:        p.now, // Leaky buckets don't have a reset time, they continuously leak
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
	timeElapsed := p.now.Sub(bucket.LastLeak).Seconds()
	requestsToLeak := timeElapsed * p.leakRate
	bucket.Requests = max(0.0, bucket.Requests-requestsToLeak)
	bucket.LastLeak = p.now

	// Calculate remaining capacity
	remaining := max(p.capacity-int(bucket.Requests), 0)

	return Result{
		Allowed:      remaining > 0,
		Remaining:    remaining,
		Reset:        p.now, // Leaky buckets don't have a reset time
		stateUpdated: false,
	}, nil
}

// allowTryAndUpdate implements try-and-update mode with retries
func (p *parameter) allowTryAndUpdate(ctx context.Context) (Result, error) {
	// Try atomic CheckAndSet operations first
	for attempt := range p.maxRetries {
		// Check if context is canceled or timed out
		if err := ctx.Err(); err != nil {
			return Result{}, NewContextCanceledError(err)
		}

		// Get current bucket state
		data, err := p.storage.Get(ctx, p.key)
		if err != nil {
			return Result{}, NewStateRetrievalError(err)
		}

		var bucket LeakyBucket
		var oldValue string
		if data == "" {
			// Initialize new bucket
			bucket = LeakyBucket{
				Requests: 0,
				LastLeak: p.now,
			}
			oldValue = "" // Key doesn't exist
		} else {
			// Parse existing bucket state
			if b, ok := decodeState(data); ok {
				bucket = b
			} else {
				return Result{}, ErrStateParsing
			}
			oldValue = data

			// Leak requests based on elapsed time
			elapsed := p.now.Sub(bucket.LastLeak)
			requestsToLeak := float64(elapsed.Nanoseconds()) * p.leakRate / 1e9
			bucket.Requests = max(0.0, bucket.Requests-requestsToLeak)
			bucket.LastLeak = p.now
		}

		// Calculate if request is allowed
		allowed := bucket.Requests+1 <= float64(p.capacity)

		if allowed {
			beforeCAS := time.Now()

			// Add request to bucket
			bucket.Requests += 1.0

			// Calculate remaining capacity after adding request
			remaining := max(p.capacity-int(bucket.Requests), 0)

			// Save updated bucket state
			newValue := encodeState(bucket)
			expiration := strategies.CalcExpiration(p.capacity, p.leakRate)

			success, err := p.storage.CheckAndSet(ctx, p.key, oldValue, newValue, expiration)
			if err != nil {
				return Result{}, NewStateSaveError(err)
			}

			if success {
				// Atomic update succeeded
				return Result{
					Allowed:      true,
					Remaining:    remaining,
					Reset:        p.now, // When allowed, no specific reset needed
					stateUpdated: true,
				}, nil
			}

			feedback := time.Since(beforeCAS)
			delay := strategies.NextDelay(attempt, feedback)

			// If CheckAndSet failed, retry if we haven't exhausted attempts
			if attempt < p.maxRetries-1 {
				if err := utils.SleepOrWait(ctx, delay, 500*time.Millisecond); err != nil {
					return Result{}, NewContextCanceledError(err)
				}
				continue
			}
			break
		} else {
			// Request denied, return current remaining capacity
			remaining := max(p.capacity-int(bucket.Requests), 0)

			return Result{
				Allowed:      false,
				Remaining:    remaining,
				Reset:        calculateResetTime(p.now, bucket, p.capacity, p.leakRate),
				stateUpdated: oldValue == "",
			}, nil
		}
	}

	// CheckAndSet failed after checkAndSetRetries attempts
	return Result{}, ErrConcurrentAccess
}

// calculateResetTime calculates when the bucket will have capacity for another request
func calculateResetTime(
	now time.Time,
	bucket LeakyBucket,
	capacity int,
	leakRate float64,

) time.Time {
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
