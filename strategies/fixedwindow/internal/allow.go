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
	// For internal use: indicates if state was updated (only meaningful in TryUpdate mode)
	stateUpdated bool
}

// Allow provides a unified implementation for both Allow and Peek operations
// mode determines whether to perform read-only inspection or actual quota consumption
func Allow(ctx context.Context, storage backends.Backend, config Config, mode AllowMode) (map[string]Result, error) {
	now := time.Now()
	quotas := config.GetQuotas()

	// Read-only mode: just get current state and calculate results
	if mode == ReadOnly {
		return allowReadOnly(ctx, storage, config, now, quotas)
	}

	// Try-and-update mode: attempt to consume quota with retries
	return allowTryAndUpdate(ctx, storage, config, now, quotas)
}

// allowReadOnly implements read-only mode
func allowReadOnly(ctx context.Context, storage backends.Backend, config Config, now time.Time, quotas map[string]Quota) (map[string]Result, error) {
	results := make(map[string]Result, len(quotas))

	// Process each quota
	for quotaName, quota := range quotas {
		// Create quota-specific key
		quotaKey := buildQuotaKey(config.GetKey(), quotaName)

		// Get current window state for this quota
		data, err := storage.Get(ctx, quotaKey)
		if err != nil {
			return nil, NewStateRetrievalError(quotaName, err)
		}

		var window FixedWindow
		if data == "" {
			// No existing window, return default state
			results[quotaName] = Result{
				Allowed:      true,
				Remaining:    quota.Limit,
				Reset:        now.Add(quota.Window),
				stateUpdated: false,
			}
			continue
		}

		// Parse existing window state
		if w, ok := decodeState(data); ok {
			window = w
		} else {
			return nil, NewStateParsingError(quotaName)
		}

		// Check if current window has expired
		if now.Sub(window.Start) >= quota.Window {
			// Window has expired, return fresh state
			results[quotaName] = Result{
				Allowed:      true,
				Remaining:    quota.Limit,
				Reset:        now.Add(quota.Window),
				stateUpdated: false,
			}
			continue
		}

		// Calculate remaining requests and reset time
		remaining := max(quota.Limit-window.Count, 0)
		resetTime := window.Start.Add(quota.Window)

		results[quotaName] = Result{
			Allowed:      remaining > 0,
			Remaining:    remaining,
			Reset:        resetTime,
			stateUpdated: false,
		}
	}

	return results, nil
}

// allowTryAndUpdate implements try-and-update mode with retries
func allowTryAndUpdate(ctx context.Context, storage backends.Backend, config Config, now time.Time, quotas map[string]Quota) (map[string]Result, error) {
	// Check if this is a multi-quota configuration
	isMultiQuota := len(quotas) > 1

	if isMultiQuota {
		// Multi-quota: Use two-phase commit for proper atomicity
		return allowMultiQuota(ctx, storage, config, now, quotas)
	}
	// Single-quota: Use original optimized approach
	return allowSingleQuota(ctx, storage, config, now, quotas)
}

// allowSingleQuota handles single-quota configurations with the original optimized approach
func allowSingleQuota(ctx context.Context, storage backends.Backend, config Config, now time.Time, quotas map[string]Quota) (map[string]Result, error) {
	results := make(map[string]Result)

	// Get the single quota (there should be exactly one)
	var quotaName string
	var quota Quota
	for name, q := range quotas {
		quotaName = name
		quota = q
		break
	}

	// Create quota-specific key
	quotaKey := buildQuotaKey(config.GetKey(), quotaName)

	// Try atomic CheckAndSet operations
	for attempt := range strategies.CheckAndSetRetries {
		// Check if context is cancelled or timed out
		if ctx.Err() != nil {
			return nil, NewContextCancelledError(ctx.Err())
		}

		// Get current window state for this quota
		data, err := storage.Get(ctx, quotaKey)
		if err != nil {
			return nil, NewStateRetrievalError(quotaName, err)
		}

		var window FixedWindow
		var oldValue any
		if data == "" {
			// Initialize new window
			window = FixedWindow{
				Count: 0,
				Start: now,
			}
			oldValue = nil // Key doesn't exist
		} else {
			// Parse existing window state
			if w, ok := decodeState(data); ok {
				window = w
			} else {
				return nil, NewStateParsingError(quotaName)
			}

			// Check if current window has expired
			if now.Sub(window.Start) >= quota.Window {
				// Start new window
				window.Count = 0
				window.Start = now
			}
			oldValue = data
		}

		// Determine if request is allowed based on current count
		allowed := window.Count < quota.Limit
		resetTime := window.Start.Add(quota.Window)

		if allowed {
			// Increment request count
			window.Count++

			// Calculate remaining after increment
			remaining := max(quota.Limit-window.Count, 0)

			// Save updated window state
			newValue := encodeState(window)

			// Use CheckAndSet for atomic update
			success, err := storage.CheckAndSet(ctx, quotaKey, oldValue, newValue, quota.Window)
			if err != nil {
				return nil, NewStateSaveError(quotaName, err)
			}

			if success {
				// Atomic update succeeded
				results[quotaName] = Result{
					Allowed:      true,
					Remaining:    remaining,
					Reset:        resetTime,
					stateUpdated: true,
				}
				return results, nil
			}

			// If CheckAndSet failed, retry if we haven't exhausted attempts
			if attempt < strategies.CheckAndSetRetries-1 {
				time.Sleep((19 * time.Nanosecond) << time.Duration(attempt))
				continue
			}
			return nil, NewStateUpdateError(quotaName, strategies.CheckAndSetRetries)
		}

		// Request was denied, return original remaining count
		remaining := max(quota.Limit-window.Count, 0)

		// For denied requests, only update if window expired
		if now.Sub(window.Start) >= quota.Window {
			// Window expired, reset it
			window.Count = 0
			window.Start = now

			newValue := encodeState(window)
			_, err := storage.CheckAndSet(ctx, quotaKey, oldValue, newValue, quota.Window)
			if err != nil {
				return nil, NewResetExpiredWindowError(quotaName, err)
			}
		}

		results[quotaName] = Result{
			Allowed:      false,
			Remaining:    remaining,
			Reset:        resetTime,
			stateUpdated: oldValue == nil,
		}
		return results, nil
	}

	return nil, ErrConcurrentAccess
}

// allowMultiQuota handles multi-quota configurations with two-phase commit
func allowMultiQuota(ctx context.Context, storage backends.Backend, config Config, now time.Time, quotas map[string]Quota) (map[string]Result, error) {
	results := make(map[string]Result, len(quotas))

	// Phase 1: Check all quotas without consuming quota
	quotaStates, err := getQuotaStates(ctx, storage, config, now, quotas)
	if err != nil {
		return nil, err
	}

	// Check if all quotas allow the request
	allAllowed := true
	for _, state := range quotaStates {
		if !state.allowed {
			allAllowed = false
			break
		}
	}

	// Phase 2: Generate results and optionally consume quota
	for _, state := range quotaStates {
		if allAllowed {
			// All quotas allowed, so consume quota from this quota
			result, err := consumeQuota(ctx, storage, state, now)
			if err != nil {
				return nil, err
			}
			results[state.name] = result
		} else {
			// At least one quota denied, so don't consume quota from any quota
			result, err := getDenyResult(ctx, storage, state, now)
			if err != nil {
				return nil, err
			}
			results[state.name] = result
		}
	}

	// If some quotas weren't processed because an earlier quota denied,
	// we need to initialize their results
	if !allAllowed {
		err = initializeUnprocessedQuotas(ctx, storage, config, now, quotas, results)
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// consumeQuota consumes quota for a quota using atomic CheckAndSet
func consumeQuota(ctx context.Context, storage backends.Backend, state quotaState, now time.Time) (Result, error) {
	for attempt := range strategies.CheckAndSetRetries {
		// Check if context is cancelled or timed out
		if ctx.Err() != nil {
			return Result{}, NewContextCancelledError(ctx.Err())
		}

		// Increment request count
		updatedWindow := state.window
		updatedWindow.Count++

		// Calculate remaining after increment
		remaining := max(state.quota.Limit-updatedWindow.Count, 0)
		resetTime := updatedWindow.Start.Add(state.quota.Window)

		// Save updated window state
		newValue := encodeState(updatedWindow)

		// Use CheckAndSet for atomic update
		success, err := storage.CheckAndSet(ctx, state.key, state.oldValue, newValue, state.quota.Window)
		if err != nil {
			return Result{}, NewStateSaveError(state.name, err)
		}

		if success {
			// Atomic update succeeded
			return Result{
				Allowed:      true,
				Remaining:    remaining,
				Reset:        resetTime,
				stateUpdated: true,
			}, nil
		}

		// If CheckAndSet failed, retry if we haven't exhausted attempts
		if attempt < strategies.CheckAndSetRetries-1 {
			// Re-read the current state and retry
			time.Sleep((19 * time.Nanosecond) << time.Duration(attempt))
			data, err := storage.Get(ctx, state.key)
			if err != nil {
				return Result{}, NewStateRetrievalError(state.name, err)
			}

			if data == "" {
				// Window disappeared, start fresh
				updatedWindow = FixedWindow{
					Count: 0,
					Start: now,
				}
				state.oldValue = nil
			} else {
				// Parse updated state
				if w, ok := decodeState(data); ok {
					updatedWindow = w
					// Check if window expired during retry
					if now.Sub(updatedWindow.Start) >= state.quota.Window {
						updatedWindow.Count = 0
						updatedWindow.Start = now
					}
				} else {
					return Result{}, NewStateParsingError(state.name)
				}
				state.oldValue = data
			}
			state.window = updatedWindow
			continue
		}
		return Result{}, NewStateUpdateError(state.name, strategies.CheckAndSetRetries)
	}
	return Result{}, nil // This shouldn't happen due to the loop logic
}

// getDenyResult returns results for a quota when the request is denied
func getDenyResult(ctx context.Context, storage backends.Backend, state quotaState, now time.Time) (Result, error) {
	remaining := max(state.quota.Limit-state.window.Count, 0)
	resetTime := state.window.Start.Add(state.quota.Window)

	result := Result{
		Allowed:      state.allowed,
		Remaining:    remaining,
		Reset:        resetTime,
		stateUpdated: false,
	}

	// For denied requests, handle window reset if expired
	if !state.allowed && now.Sub(state.window.Start) >= state.quota.Window {
		// Window expired, reset it
		resetWindow := FixedWindow{
			Count: 0,
			Start: now,
		}
		newValue := encodeState(resetWindow)
		_, err := storage.CheckAndSet(ctx, state.key, state.oldValue, newValue, state.quota.Window)
		if err != nil {
			return Result{}, NewResetExpiredWindowError(state.name, err)
		}
		result.stateUpdated = true
	}

	return result, nil
}

// initializeUnprocessedQuotas initializes results for quotas that weren't processed
func initializeUnprocessedQuotas(ctx context.Context, storage backends.Backend, config Config, now time.Time, quotas map[string]Quota, results map[string]Result) error {
	for quotaName, quota := range quotas {
		if _, exists := results[quotaName]; !exists {
			// This quota wasn't processed, initialize it without consuming quota
			quotaKey := buildQuotaKey(config.GetKey(), quotaName)
			data, err := storage.Get(ctx, quotaKey)
			if err != nil {
				return NewStateRetrievalError(quotaName, err)
			}

			if data == "" {
				// Initialize this quota without consuming quota
				results[quotaName] = Result{
					Allowed:      true,
					Remaining:    quota.Limit,
					Reset:        now.Add(quota.Window),
					stateUpdated: false,
				}
			} else {
				// Get current state without modifying
				if w, ok := decodeState(data); ok {
					if now.Sub(w.Start) >= quota.Window {
						// Window expired
						results[quotaName] = Result{
							Allowed:      true,
							Remaining:    quota.Limit,
							Reset:        now.Add(quota.Window),
							stateUpdated: false,
						}
					} else {
						remaining := max(quota.Limit-w.Count, 0)
						results[quotaName] = Result{
							Allowed:      remaining > 0,
							Remaining:    remaining,
							Reset:        w.Start.Add(quota.Window),
							stateUpdated: false,
						}
					}
				} else {
					return NewStateParsingError(quotaName)
				}
			}
		}
	}
	return nil
}
