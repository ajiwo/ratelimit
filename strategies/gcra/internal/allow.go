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
	emissionInterval := time.Duration(1e9/config.GetRate()) * time.Nanosecond
	limit := time.Duration(float64(config.GetBurst()) * float64(emissionInterval))

	// Read-only mode: just get current state and calculate results
	if mode == ReadOnly {
		return allowReadOnly(ctx, storage, config, now, emissionInterval, limit)
	}

	// Try-and-update mode: attempt to consume quota with retries
	return allowTryAndUpdate(ctx, storage, config, now, emissionInterval, limit)
}

// allowReadOnly implements read-only mode
func allowReadOnly(ctx context.Context, storage backends.Backend, gcraConfig Config, now time.Time, emissionInterval, limit time.Duration) (Result, error) {
	// Get current state
	data, err := storage.Get(ctx, gcraConfig.GetKey())
	if err != nil {
		return Result{}, NewStateRetrievalError(err)
	}

	if data == "" {
		// No existing state, fresh start
		return Result{
			Allowed:      true,
			Remaining:    gcraConfig.GetBurst(),
			Reset:        now.Add(limit),
			stateUpdated: false,
		}, nil
	}

	// Parse existing state
	state, ok := decodeState(data)
	if !ok {
		return Result{}, ErrStateParsing
	}

	// Calculate remaining requests based on current TAT
	remaining := calculateRemaining(now, state.TAT, emissionInterval, limit, gcraConfig.GetBurst())

	// Calculate reset time (when next request will be allowed)
	resetTime := state.TAT.Add(limit)

	return Result{
		Allowed:      remaining > 0,
		Remaining:    remaining,
		Reset:        resetTime,
		stateUpdated: false,
	}, nil
}

// allowTryAndUpdate implements try-and-update mode with retries
func allowTryAndUpdate(ctx context.Context, storage backends.Backend, gcraConfig Config, now time.Time, emissionInterval, limit time.Duration) (Result, error) {
	maxRetries := gcraConfig.MaxRetries()
	if maxRetries <= 0 {
		maxRetries = strategies.DefaultMaxRetries
	}

	// Try atomic CheckAndSet operations first
	for attempt := range maxRetries {
		// Check if context is canceled or timed out
		if ctx.Err() != nil {
			return Result{}, NewContextCanceledError(ctx.Err())
		}

		// Get current state
		data, err := storage.Get(ctx, gcraConfig.GetKey())
		if err != nil {
			return Result{}, NewStateRetrievalError(err)
		}

		var state GCRA
		var oldValue any
		if data == "" {
			// Initialize new state
			state = GCRA{
				TAT: now,
			}
			oldValue = nil // Key doesn't exist
		} else {
			// Parse existing state
			if s, ok := decodeState(data); ok {
				state = s
			} else {
				return Result{}, ErrStateParsing
			}
			oldValue = data
		}

		// Calculate new TAT
		newTAT := now
		if state.TAT.After(now) {
			newTAT = state.TAT
		}
		newTAT = newTAT.Add(emissionInterval)

		// Check if request is allowed
		allowed := newTAT.Sub(now) <= limit

		if allowed {
			// Update state with new TAT
			state.TAT = newTAT

			// Calculate remaining requests
			remaining := calculateRemaining(now, state.TAT, emissionInterval, limit, gcraConfig.GetBurst())

			// Save updated state
			newValue := encodeState(state)
			expiration := strategies.CalcExpiration(gcraConfig.GetBurst(), gcraConfig.GetRate())

			success, err := storage.CheckAndSet(ctx, gcraConfig.GetKey(), oldValue, newValue, expiration)
			if err != nil {
				return Result{}, NewStateSaveError(err)
			}

			if success {
				// Atomic update succeeded
				return Result{
					Allowed:      true,
					Remaining:    remaining,
					Reset:        state.TAT.Add(limit),
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
			// Request denied
			remaining := 0
			resetTime := newTAT

			// Update the state even when denying to ensure proper TAT progression
			// Only if this was a new state (rare case)
			if oldValue == nil {
				stateData := encodeState(state)
				expiration := strategies.CalcExpiration(gcraConfig.GetBurst(), gcraConfig.GetRate())
				_, err := storage.CheckAndSet(ctx, gcraConfig.GetKey(), oldValue, stateData, expiration)
				if err != nil {
					return Result{}, NewStateSaveError(err)
				}
			}

			return Result{
				Allowed:      false,
				Remaining:    remaining,
				Reset:        resetTime,
				stateUpdated: oldValue == nil,
			}, nil
		}
	}

	// CheckAndSet failed after checkAndSetRetries attempts
	return Result{}, ErrConcurrentAccess
}

// calculateRemaining calculates the number of remaining requests based on current state
func calculateRemaining(now, tat time.Time, emissionInterval, limit time.Duration, burst int) int {
	if now.After(tat) {
		// We're ahead of schedule, full burst available
		return burst
	}

	// Calculate how far behind we are
	behind := tat.Sub(now)
	if behind >= limit {
		// We're too far behind, no requests available
		return 0
	}

	// Calculate remaining burst based on how far behind we are
	remainingBurst := int((limit - behind) / emissionInterval)
	if remainingBurst > burst {
		return burst
	}
	return remainingBurst
}
