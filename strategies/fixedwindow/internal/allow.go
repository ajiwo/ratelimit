package internal

import (
	"context"
	"maps"
	"slices"
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

type parameter struct {
	key        string
	maxRetries int
	now        time.Time
	quotas     map[string]Quota
	storage    backends.Backend
}

// Allow provides a unified implementation for both Allow and Peek operations
// mode determines whether to perform read-only inspection or actual quota consumption
func Allow(
	ctx context.Context,
	storage backends.Backend,
	config Config,
	mode AllowMode,

) (map[string]Result, error) {
	if ctx.Err() != nil {
		return nil, NewContextCanceledError(ctx.Err())
	}

	maxRetries := config.MaxRetries()
	if maxRetries <= 0 {
		maxRetries = strategies.DefaultMaxRetries
	}

	p := &parameter{
		storage:    storage,
		key:        config.GetKey(),
		now:        time.Now(),
		quotas:     config.GetQuotas(),
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
func (p *parameter) allowReadOnly(ctx context.Context) (map[string]Result, error) {
	results := make(map[string]Result, len(p.quotas))

	// Process each quota
	for quotaName, quota := range p.quotas {
		// Create quota-specific key
		quotaKey := buildQuotaKey(p.key, quotaName)

		// Get current window state for this quota
		data, err := p.storage.Get(ctx, quotaKey)
		if err != nil {
			return nil, NewStateRetrievalError(quotaName, err)
		}

		var window FixedWindow
		if data == "" {
			// No existing window, return default state
			results[quotaName] = Result{
				Allowed:      true,
				Remaining:    quota.Limit,
				Reset:        p.now.Add(quota.Window),
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
		if p.now.Sub(window.Start) >= quota.Window {
			// Window has expired, return fresh state
			results[quotaName] = Result{
				Allowed:      true,
				Remaining:    quota.Limit,
				Reset:        p.now.Add(quota.Window),
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
func (p *parameter) allowTryAndUpdate(ctx context.Context) (map[string]Result, error) {
	// Check if this is a multi-quota configuration
	isMultiQuota := len(p.quotas) > 1

	if isMultiQuota {
		// Multi-quota: Use two-phase commit for proper atomicity
		return p.allowMultiQuota(ctx)
	}
	// Single-quota: Use original optimized approach
	return p.allowSingleQuota(ctx)
}

// allowSingleQuota handles single-quota configurations with the original optimized approach
func (p *parameter) allowSingleQuota(ctx context.Context) (map[string]Result, error) {
	results := make(map[string]Result)

	// Get the single quota (there should be exactly one)
	quotaName := slices.Sorted(maps.Keys(p.quotas))[0]
	quota := p.quotas[quotaName]

	// Create quota-specific key
	quotaKey := buildQuotaKey(p.key, quotaName)

	// Try atomic CheckAndSet operations
	for attempt := range p.maxRetries {
		// Check if context is canceled or timed out
		if ctx.Err() != nil {
			return nil, NewContextCanceledError(ctx.Err())
		}

		// Get current window state for this quota
		data, err := p.storage.Get(ctx, quotaKey)
		if err != nil {
			return nil, NewStateRetrievalError(quotaName, err)
		}

		var window FixedWindow
		var oldValue string
		if data == "" {
			// Initialize new window
			window = FixedWindow{
				Count: 0,
				Start: p.now,
			}
			oldValue = "" // Key doesn't exist
		} else {
			// Parse existing window state
			if w, ok := decodeState(data); ok {
				window = w
			} else {
				return nil, NewStateParsingError(quotaName)
			}

			// Check if current window has expired
			if p.now.Sub(window.Start) >= quota.Window {
				// Start new window
				window.Count = 0
				window.Start = p.now
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
			success, err := p.storage.CheckAndSet(ctx, quotaKey, oldValue, newValue, quota.Window)
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
			if attempt < p.maxRetries-1 {
				time.Sleep((19 * time.Nanosecond) << time.Duration(attempt))
				continue
			}
			return nil, NewStateUpdateError(quotaName, p.maxRetries)
		}

		// Request was denied, return original remaining count
		remaining := max(quota.Limit-window.Count, 0)

		// For denied requests, only update if window expired
		if p.now.Sub(window.Start) >= quota.Window {
			// Window expired, reset it
			window.Count = 0
			window.Start = p.now

			newValue := encodeState(window)
			_, err := p.storage.CheckAndSet(ctx, quotaKey, oldValue, newValue, quota.Window)
			if err != nil {
				return nil, NewResetExpiredWindowError(quotaName, err)
			}
		}

		results[quotaName] = Result{
			Allowed:      false,
			Remaining:    remaining,
			Reset:        resetTime,
			stateUpdated: oldValue == "",
		}
		return results, nil
	}

	return nil, ErrConcurrentAccess
}

// allowMultiQuota handles multi-quota configurations with two-phase commit
func (p *parameter) allowMultiQuota(ctx context.Context) (map[string]Result, error) {
	results := make(map[string]Result, len(p.quotas))

	// Phase 1: Check all quotas without consuming quota
	quotaStates, err := p.getQuotaStates(ctx)
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
			result, err := p.consumeQuota(ctx, state)
			if err != nil {
				return nil, err
			}
			results[state.name] = result
		} else {
			// At least one quota denied, so don't consume quota from any quota
			result, err := p.getDenyResult(ctx, state)
			if err != nil {
				return nil, err
			}
			results[state.name] = result
		}
	}

	// If some quotas weren't processed because an earlier quota denied,
	// we need to initialize their results
	if !allAllowed {
		err = p.initializeUnprocessedQuotas(ctx, results)
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// consumeQuota consumes quota for a quota using atomic CheckAndSet
func (p *parameter) consumeQuota(ctx context.Context, state quotaState) (Result, error) {
	for attempt := range p.maxRetries {
		// Check if context is canceled or timed out
		if ctx.Err() != nil {
			return Result{}, NewContextCanceledError(ctx.Err())
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
		success, err := p.storage.CheckAndSet(ctx, state.key, state.oldValue, newValue, state.quota.Window)
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
		if attempt < p.maxRetries-1 {
			// Re-read the current state and retry
			time.Sleep((19 * time.Nanosecond) << time.Duration(attempt))
			data, err := p.storage.Get(ctx, state.key)
			if err != nil {
				return Result{}, NewStateRetrievalError(state.name, err)
			}

			if data == "" {
				// Window disappeared, start fresh
				updatedWindow = FixedWindow{
					Count: 0,
					Start: p.now,
				}
				state.oldValue = ""
			} else {
				// Parse updated state
				if w, ok := decodeState(data); ok {
					updatedWindow = w
					// Check if window expired during retry
					if p.now.Sub(updatedWindow.Start) >= state.quota.Window {
						updatedWindow.Count = 0
						updatedWindow.Start = p.now
					}
				} else {
					return Result{}, NewStateParsingError(state.name)
				}
				state.oldValue = data
			}
			state.window = updatedWindow
			continue
		}
		return Result{}, NewStateUpdateError(state.name, p.maxRetries)
	}
	return Result{}, nil // This shouldn't happen due to the loop logic
}

// getDenyResult returns results for a quota when the request is denied
func (p *parameter) getDenyResult(ctx context.Context, state quotaState) (Result, error) {
	remaining := max(state.quota.Limit-state.window.Count, 0)
	resetTime := state.window.Start.Add(state.quota.Window)

	result := Result{
		Allowed:      state.allowed,
		Remaining:    remaining,
		Reset:        resetTime,
		stateUpdated: false,
	}

	// For denied requests, handle window reset if expired
	if !state.allowed && p.now.Sub(state.window.Start) >= state.quota.Window {
		// Window expired, reset it
		resetWindow := FixedWindow{
			Count: 0,
			Start: p.now,
		}
		newValue := encodeState(resetWindow)
		_, err := p.storage.CheckAndSet(ctx, state.key, state.oldValue, newValue, state.quota.Window)
		if err != nil {
			return Result{}, NewResetExpiredWindowError(state.name, err)
		}
		result.stateUpdated = true
	}

	return result, nil
}

// initializeUnprocessedQuotas initializes results for quotas that weren't processed
func (p *parameter) initializeUnprocessedQuotas(ctx context.Context, results map[string]Result) error {
	for quotaName, quota := range p.quotas {
		if _, exists := results[quotaName]; !exists {
			// This quota wasn't processed, initialize it without consuming quota
			quotaKey := buildQuotaKey(p.key, quotaName)
			data, err := p.storage.Get(ctx, quotaKey)
			if err != nil {
				return NewStateRetrievalError(quotaName, err)
			}

			if data == "" {
				// Initialize this quota without consuming quota
				results[quotaName] = Result{
					Allowed:      true,
					Remaining:    quota.Limit,
					Reset:        p.now.Add(quota.Window),
					stateUpdated: false,
				}
			} else {
				// Get current state without modifying
				if w, ok := decodeState(data); ok {
					if p.now.Sub(w.Start) >= quota.Window {
						// Window expired
						results[quotaName] = Result{
							Allowed:      true,
							Remaining:    quota.Limit,
							Reset:        p.now.Add(quota.Window),
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

// getQuotaStates gets the current state for all quotas without consuming quota
func (p *parameter) getQuotaStates(ctx context.Context) ([]quotaState, error) {
	quotaStates := make([]quotaState, 0, len(p.quotas))

	for quotaName, quota := range p.quotas {
		quotaKey := buildQuotaKey(p.key, quotaName)

		// Get current window state for this quota
		data, err := p.storage.Get(ctx, quotaKey)
		if err != nil {
			return nil, NewStateRetrievalError(quotaName, err)
		}

		var window FixedWindow
		var oldValue string
		if data == "" {
			// Initialize new window
			window = FixedWindow{
				Count: 0,
				Start: p.now,
			}
			oldValue = "" // Key doesn't exist
		} else {
			// Parse existing window state
			if w, ok := decodeState(data); ok {
				window = w
			} else {
				return nil, NewStateParsingError(quotaName)
			}

			// Check if current window has expired
			if p.now.Sub(window.Start) >= quota.Window {
				// Start new window
				window.Count = 0
				window.Start = p.now
			}
			oldValue = data
		}

		// Determine if request is allowed based on current count
		allowed := window.Count < quota.Limit

		quotaStates = append(quotaStates, quotaState{
			name:     quotaName,
			quota:    quota,
			key:      quotaKey,
			window:   window,
			oldValue: oldValue,
			allowed:  allowed,
		})

		// If this quota doesn't allow, we can stop checking others
		if !allowed {
			break
		}
	}

	return quotaStates, nil
}
