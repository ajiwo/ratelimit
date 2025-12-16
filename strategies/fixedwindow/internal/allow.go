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
	key        string
	maxRetries int
	now        time.Time
	quotas     []Quota
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
	if err := ctx.Err(); err != nil {
		return nil, NewContextCanceledError(err)
	}

	maxRetries := config.GetMaxRetries()
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

	// Get current combined state
	quotaStates, _, err := p.getAndParseState(ctx)
	if err != nil {
		return nil, err
	}

	// Create a map for quick lookup of quota states by name
	quotaStateMap := make(map[string]FixedWindow, len(quotaStates))
	for _, state := range quotaStates {
		quotaStateMap[state.Name] = state
	}

	// Normalize per-quota windows in-memory and calculate results
	for _, quota := range p.quotas {
		name := quota.Name
		var window FixedWindow
		if existingState, exists := quotaStateMap[name]; exists {
			window = existingState
			// Check if current window has expired
			if p.now.Sub(window.Start) >= quota.Window {
				// Window has expired, use fresh state
				window = FixedWindow{
					Name:  name,
					Count: 0,
					Start: p.now,
				}
			}
		} else {
			// This quota doesn't exist in state yet, initialize it
			window = FixedWindow{
				Name:  name,
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
		if err := ctx.Err(); err != nil {
			return nil, NewContextCanceledError(err)
		}

		// Get and parse state
		quotaStates, oldValue, err := p.getAndParseState(ctx)
		if err != nil {
			return nil, err
		}

		beforeCAS := time.Now()

		// Normalize per-quota windows in-memory
		normalizedStates := p.normalizeWindows(quotaStates)

		// Evaluate allow for all quotas
		allAllowed := p.areAllQuotasAllowed(normalizedStates)

		if !allAllowed {
			return p.calculateResults(normalizedStates), nil
		}

		// All quotas allowed, increment counts
		incrementedStates := p.incrementAllQuotas(normalizedStates)

		// Use CheckAndSet for atomic update
		newValue := encodeState(incrementedStates)
		newTTL := computeMaxResetTTL(incrementedStates, p.quotas, p.now)
		success, err := p.storage.CheckAndSet(ctx, p.key, oldValue, newValue, newTTL)
		if err != nil {
			return nil, err
		}

		if success {
			// Atomic update succeeded - update results with final remaining counts
			finalResults := p.calculateFinalResults(incrementedStates)
			return finalResults, nil
		}

		feedback := time.Since(beforeCAS)
		delay := strategies.NextDelay(attempt, feedback)

		// If CheckAndSet failed, retry if we haven't exhausted attempts
		if attempt < p.maxRetries-1 {
			if err := utils.SleepOrWait(ctx, delay, 500*time.Millisecond); err != nil {
				return nil, NewContextCanceledError(err)
			}
			continue
		}
		return nil, NewStateUpdateError(p.maxRetries)
	}

	return nil, ErrConcurrentAccess
}

// getAndParseState retrieves and parses the current state from storage
func (p *parameter) getAndParseState(ctx context.Context) ([]FixedWindow, string, error) {
	data, err := p.storage.Get(ctx, p.key)
	if err != nil {
		return nil, "", NewStateRetrievalError(err)
	}

	var quotaStates []FixedWindow
	var oldValue string
	if data == "" {
		// Initialize new combined state from config - maintain order
		quotaStates = make([]FixedWindow, 0, len(p.quotas))
		for _, quota := range p.quotas {
			name := quota.Name
			quotaStates = append(quotaStates, FixedWindow{
				Name:  name,
				Count: 0,
				Start: p.now,
			})
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
func (p *parameter) normalizeWindows(quotaStates []FixedWindow) []FixedWindow {
	// Create a map for quick lookup of existing quota states
	stateMap := make(map[string]FixedWindow, len(quotaStates))
	for _, state := range quotaStates {
		stateMap[state.Name] = state
	}

	normalizedStates := make([]FixedWindow, 0, len(p.quotas))
	for _, quota := range p.quotas {
		name := quota.Name
		window := stateMap[name]
		// Check if current window has expired
		if p.now.Sub(window.Start) >= quota.Window {
			// Start new window
			window.Count = 0
			window.Start = p.now
		}
		normalizedStates = append(normalizedStates, window)
	}
	return normalizedStates
}

// areAllQuotasAllowed checks if all quotas are allowed (have capacity)
func (p *parameter) areAllQuotasAllowed(normalizedStates []FixedWindow) bool {
	// Create a map for quick lookup of normalized states
	stateMap := make(map[string]FixedWindow, len(normalizedStates))
	for _, state := range normalizedStates {
		stateMap[state.Name] = state
	}

	for _, quota := range p.quotas {
		name := quota.Name
		window := stateMap[name]
		if window.Count >= quota.Limit {
			return false
		}
	}
	return true
}

// calculateResults calculates the results for all quotas based on current state
func (p *parameter) calculateResults(normalizedStates []FixedWindow) map[string]Result {
	// Create a map for quick lookup of normalized states
	stateMap := make(map[string]FixedWindow, len(normalizedStates))
	for _, state := range normalizedStates {
		stateMap[state.Name] = state
	}

	tempResults := make(map[string]Result, len(p.quotas))
	for _, quota := range p.quotas {
		name := quota.Name
		window := stateMap[name]
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

// incrementAllQuotas increments the count for all quotas
func (p *parameter) incrementAllQuotas(normalizedStates []FixedWindow) []FixedWindow {
	// Create a copy and increment all quotas
	incrementedStates := make([]FixedWindow, len(normalizedStates))
	for i, window := range normalizedStates {
		incrementedStates[i] = FixedWindow{
			Name:  window.Name,
			Count: window.Count + 1,
			Start: window.Start,
		}
	}
	return incrementedStates
}

// calculateFinalResults calculates the final results after state update
func (p *parameter) calculateFinalResults(normalizedStates []FixedWindow) map[string]Result {
	// Create a map for quick lookup of normalized states
	stateMap := make(map[string]FixedWindow, len(normalizedStates))
	for _, state := range normalizedStates {
		stateMap[state.Name] = state
	}

	finalResults := make(map[string]Result, len(p.quotas))
	for _, quota := range p.quotas {
		name := quota.Name
		window := stateMap[name]
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
