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

// allowReadOnly implements read-only mode using combined state
func (p *parameter) allowReadOnly(ctx context.Context) (map[string]Result, error) {
	results := make(map[string]Result, len(p.quotas))

	// Use the base key for combined state (not quota-specific)
	combinedKey := p.key

	// Get current combined state
	data, err := p.storage.Get(ctx, combinedKey)
	if err != nil {
		return nil, NewStateRetrievalError(err)
	}

	var quotaStates map[string]FixedWindow
	if data == "" {
		// No existing state, initialize fresh state
		quotaStates = make(map[string]FixedWindow, len(p.quotas))
		for name := range p.quotas {
			quotaStates[name] = FixedWindow{
				Count: 0,
				Start: p.now,
			}
		}
	} else {
		if states, ok := decodeState(data); ok {
			quotaStates = states
		} else {
			return nil, NewStateParsingError()
		}
	}

	// Normalize per-quota windows in-memory and calculate results
	for name, quota := range p.quotas {
		var window FixedWindow
		if existingState, exists := quotaStates[name]; exists {
			window = existingState
			// Check if current window has expired
			if p.now.Sub(window.Start) >= quota.Window {
				// Window has expired, use fresh state
				window = FixedWindow{
					Count: 0,
					Start: p.now,
				}
			}
		} else {
			// This quota doesn't exist in state yet, initialize it
			window = FixedWindow{
				Count: 0,
				Start: p.now,
			}
		}

		// Calculate remaining requests and reset time
		remaining := max(quota.Limit-window.Count, 0)
		resetTime := window.Start.Add(quota.Window)

		results[name] = Result{
			Allowed:      remaining > 0,
			Remaining:    remaining,
			Reset:        resetTime,
			stateUpdated: false,
		}
	}

	return results, nil
}

// allowTryAndUpdate implements try-and-update mode with retries using combined state
func (p *parameter) allowTryAndUpdate(ctx context.Context) (map[string]Result, error) {
	// Try atomic CheckAndSet operations with combined state
	for attempt := range p.maxRetries {
		// Check if context is canceled or timed out
		if ctx.Err() != nil {
			return nil, NewContextCanceledError(ctx.Err())
		}

		// Get and parse state
		quotaStates, oldValue, err := p.getAndParseState(ctx)
		if err != nil {
			return nil, err
		}

		// Normalize per-quota windows in-memory
		normalizedStates := p.normalizeWindows(quotaStates)

		// Evaluate allow for all quotas
		allAllowed := p.areAllQuotasAllowed(normalizedStates)

		// Calculate results for all quotas
		tempResults := p.calculateResults(normalizedStates)

		if !allAllowed {
			// At least one quota denied, refresh expired windows via CAS (best-effort) and return deny
			return p.handleDeniedQuota(ctx, normalizedStates, oldValue, tempResults)
		}

		// All quotas allowed, increment counts
		p.incrementAllQuotas(normalizedStates)

		// Use CheckAndSet for atomic update
		newValue := encodeState(normalizedStates)
		newTTL := computeMaxResetTTL(normalizedStates, p.quotas, p.now)
		beforeCAS := time.Now()
		success, err := p.storage.CheckAndSet(ctx, p.key, oldValue, newValue, newTTL)
		if err != nil {
			return nil, err
		}

		if success {
			// Atomic update succeeded - update results with final remaining counts
			finalResults := p.calculateFinalResults(normalizedStates)
			return finalResults, nil
		}

		baseDelay := min(max(time.Since(beforeCAS), 32*time.Nanosecond), 32*time.Millisecond)

		// If CheckAndSet failed, retry if we haven't exhausted attempts
		if attempt < p.maxRetries-1 {
			time.Sleep(baseDelay << (attempt % 16))
			continue
		}
		return nil, NewStateUpdateError(p.maxRetries)
	}

	return nil, ErrConcurrentAccess
}

// getAndParseState retrieves and parses the current state from storage
func (p *parameter) getAndParseState(ctx context.Context) (map[string]FixedWindow, string, error) {
	data, err := p.storage.Get(ctx, p.key)
	if err != nil {
		return nil, "", NewStateRetrievalError(err)
	}

	var quotaStates map[string]FixedWindow
	var oldValue string
	if data == "" {
		// Initialize new combined state from config
		quotaStates = make(map[string]FixedWindow, len(p.quotas))
		for name := range p.quotas {
			quotaStates[name] = FixedWindow{
				Count: 0,
				Start: p.now,
			}
		}
		oldValue = "" // Key doesn't exist
	} else {
		if states, ok := decodeState(data); ok {
			quotaStates = states
		} else {
			return nil, "", NewStateParsingError()
		}
		oldValue = data
	}
	return quotaStates, oldValue, nil
}

// normalizeWindows normalizes per-quota windows in-memory
func (p *parameter) normalizeWindows(quotaStates map[string]FixedWindow) map[string]FixedWindow {
	normalizedStates := make(map[string]FixedWindow, len(p.quotas))
	for name, quota := range p.quotas {
		var window FixedWindow
		if existingState, exists := quotaStates[name]; exists {
			window = existingState
			// Check if current window has expired
			if p.now.Sub(window.Start) >= quota.Window {
				// Start new window
				window.Count = 0
				window.Start = p.now
			}
		} else {
			// This quota doesn't exist in state yet, initialize it
			window = FixedWindow{
				Count: 0,
				Start: p.now,
			}
		}
		normalizedStates[name] = window
	}
	return normalizedStates
}

// areAllQuotasAllowed checks if all quotas are allowed (have capacity)
func (p *parameter) areAllQuotasAllowed(normalizedStates map[string]FixedWindow) bool {
	for name, quota := range p.quotas {
		window := normalizedStates[name]
		if window.Count >= quota.Limit {
			return false
		}
	}
	return true
}

// calculateResults calculates the results for all quotas based on current state
func (p *parameter) calculateResults(normalizedStates map[string]FixedWindow) map[string]Result {
	tempResults := make(map[string]Result, len(p.quotas))
	for name, quota := range p.quotas {
		window := normalizedStates[name]
		allowed := window.Count < quota.Limit
		remaining := max(quota.Limit-window.Count, 0)
		resetTime := window.Start.Add(quota.Window)

		tempResults[name] = Result{
			Allowed:      allowed,
			Remaining:    remaining,
			Reset:        resetTime,
			stateUpdated: false,
		}
	}
	return tempResults
}

// handleDeniedQuota handles the case when at least one quota is denied
func (p *parameter) handleDeniedQuota(
	ctx context.Context,
	normalizedStates map[string]FixedWindow,
	oldValue string,
	tempResults map[string]Result,

) (map[string]Result, error) {
	newValue := encodeState(normalizedStates)
	ttl := computeMaxResetTTL(normalizedStates, p.quotas, p.now)

	// Try to update to refresh expired windows, but don't retry on failure
	if oldValue != newValue {
		_, _ = p.storage.CheckAndSet(ctx, p.key, oldValue, newValue, ttl)
	}

	return tempResults, nil
}

// incrementAllQuotas increments the count for all quotas
func (p *parameter) incrementAllQuotas(normalizedStates map[string]FixedWindow) {
	for name := range normalizedStates {
		window := normalizedStates[name]
		window.Count++
		normalizedStates[name] = window
	}
}

// calculateFinalResults calculates the final results after state update
func (p *parameter) calculateFinalResults(normalizedStates map[string]FixedWindow) map[string]Result {
	finalResults := make(map[string]Result, len(p.quotas))
	for name, quota := range p.quotas {
		window := normalizedStates[name]
		remaining := max(quota.Limit-window.Count, 0)
		finalResults[name] = Result{
			Allowed:      true,
			Remaining:    remaining,
			Reset:        window.Start.Add(quota.Window),
			stateUpdated: true,
		}
	}
	return finalResults
}
