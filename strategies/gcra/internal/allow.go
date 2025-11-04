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
	// For internal use: indicates if state was updated (only meaningful in TryUpdate mode)
	stateUpdated bool
}

type parameter struct {
	burst            int
	emissionInterval time.Duration
	key              string
	limit            time.Duration
	maxRetries       int
	now              time.Time
	rate             float64
	storage          backends.Backend
}

// Allow provides a unified implementation for both Allow and Peek operations
// mode determines whether to perform read-only inspection or actual quota consumption
func Allow(
	ctx context.Context,
	storage backends.Backend,
	config Config,
	mode AllowMode,

) (Result, error) {
	if ctx.Err() != nil {
		return Result{}, NewContextCanceledError(ctx.Err())
	}

	maxRetries := config.MaxRetries()
	if maxRetries <= 0 {
		maxRetries = strategies.DefaultMaxRetries
	}

	now := time.Now()
	emissionInterval := time.Duration(1e9/config.GetRate()) * time.Nanosecond
	limit := time.Duration(float64(config.GetBurst()) * float64(emissionInterval))

	p := &parameter{
		burst:            config.GetBurst(),
		emissionInterval: emissionInterval,
		key:              config.GetKey(),
		limit:            limit,
		maxRetries:       maxRetries,
		now:              now,
		rate:             config.GetRate(),
		storage:          storage,
	}

	// Read-only mode: just get current state and calculate results
	if mode == ReadOnly {
		return p.allowReadOnly(ctx)
	}

	// Try-and-update mode: attempt to consume quota with retries
	return p.consumeQuota(ctx)
}

// allowReadOnly implements read-only mode
func (p *parameter) allowReadOnly(ctx context.Context) (Result, error) {
	// Get current state
	data, err := p.storage.Get(ctx, p.key)
	if err != nil {
		return Result{}, NewStateRetrievalError(err)
	}

	if data == "" {
		// No existing state, fresh start
		return Result{
			Allowed:      true,
			Remaining:    p.burst,
			Reset:        p.now.Add(p.limit),
			stateUpdated: false,
		}, nil
	}

	// Parse existing state
	state, ok := decodeState(data)
	if !ok {
		return Result{}, ErrStateParsing
	}

	// Calculate remaining requests based on current TAT
	remaining := p.calculateRemaining(state.TAT)

	// Calculate reset time (when next request will be allowed)
	resetTime := state.TAT.Add(p.limit)

	return Result{
		Allowed:      remaining > 0,
		Remaining:    remaining,
		Reset:        resetTime,
		stateUpdated: false,
	}, nil
}

// consumeQuota consumes quota using atomic CheckAndSet with retries
func (p *parameter) consumeQuota(ctx context.Context) (Result, error) {
	// Try atomic CheckAndSet operations first
	for attempt := range p.maxRetries {
		// Check if context is canceled or timed out
		if ctx.Err() != nil {
			return Result{}, NewContextCanceledError(ctx.Err())
		}

		// Get current state
		data, err := p.storage.Get(ctx, p.key)
		if err != nil {
			return Result{}, NewStateRetrievalError(err)
		}

		var state GCRA
		var oldValue string
		if data == "" {
			// Initialize new state
			state = GCRA{
				TAT: p.now,
			}
			oldValue = "" // Key doesn't exist
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
		newTAT := p.now
		if state.TAT.After(p.now) {
			newTAT = state.TAT
		}
		newTAT = newTAT.Add(p.emissionInterval)

		// Check if request is allowed
		allowed := newTAT.Sub(p.now) <= p.limit

		if allowed {
			beforeCAS := time.Now()

			// Update state with new TAT
			state.TAT = newTAT

			// Calculate remaining requests
			remaining := p.calculateRemaining(state.TAT)

			// Save updated state
			newValue := encodeState(state)
			expiration := strategies.CalcExpiration(p.burst, p.rate)

			success, err := p.storage.CheckAndSet(ctx, p.key, oldValue, newValue, expiration)
			if err != nil {
				return Result{}, NewStateSaveError(err)
			}

			if success {
				// Atomic update succeeded
				return Result{
					Allowed:      true,
					Remaining:    remaining,
					Reset:        state.TAT.Add(p.limit),
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
			// Request denied
			remaining := 0
			resetTime := newTAT

			// Update the state even when denying to ensure proper TAT progression
			// Only if this was a new state (rare case)
			if oldValue == "" {
				stateData := encodeState(state)
				expiration := strategies.CalcExpiration(p.burst, p.rate)
				_, err := p.storage.CheckAndSet(ctx, p.key, oldValue, stateData, expiration)
				if err != nil {
					return Result{}, NewStateSaveError(err)
				}
			}

			return Result{
				Allowed:      false,
				Remaining:    remaining,
				Reset:        resetTime,
				stateUpdated: oldValue == "",
			}, nil
		}
	}

	// CheckAndSet failed after maxRetries attempts
	return Result{}, NewStateUpdateError(p.maxRetries)
}

// calculateRemaining calculates the number of remaining requests based on current state
func (p *parameter) calculateRemaining(tat time.Time) int {
	if p.now.After(tat) {
		// We're ahead of schedule, full burst available
		return p.burst
	}

	// Calculate how far behind we are
	behind := tat.Sub(p.now)
	if behind >= p.limit {
		// We're too far behind, no requests available
		return 0
	}

	// Calculate remaining burst based on how far behind we are
	remainingBurst := int((p.limit - behind) / p.emissionInterval)
	if remainingBurst > p.burst {
		return p.burst
	}
	return remainingBurst
}
