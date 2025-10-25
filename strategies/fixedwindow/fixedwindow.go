package fixedwindow

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

// FixedWindow represents the state of a fixed window
type FixedWindow struct {
	Count int       `json:"count"` // Current request count in the window
	Start time.Time `json:"start"` // Window start time
}

// quotaState represents the state of a single quota during processing
type quotaState struct {
	name     string
	quota    Quota
	key      string
	window   FixedWindow
	oldValue any
	allowed  bool
}

// keyBuilderPool reduces allocations in key construction for fixed window strategy
var keyBuilderPool = sync.Pool{
	New: func() any {
		return &strings.Builder{}
	},
}

// buildQuotaKey builds a quota-specific key using pooled string builder for efficiency
func buildQuotaKey(baseKey, quotaName string) string {
	keyBuilder := keyBuilderPool.Get().(*strings.Builder)
	defer func() {
		keyBuilder.Reset()
		keyBuilderPool.Put(keyBuilder)
	}()

	keyBuilder.Grow(len(baseKey) + len(quotaName) + 1)
	keyBuilder.WriteString(baseKey)
	keyBuilder.WriteByte(':')
	keyBuilder.WriteString(quotaName)
	return keyBuilder.String()
}

// Strategy implements the fixed window rate limiting algorithm
type Strategy struct {
	storage backends.Backend
}

// New creates a new fixed window strategy
func New(storage backends.Backend) *Strategy {
	return &Strategy{
		storage: storage,
	}
}

// Peek inspects current state without consuming quota
func (f *Strategy) Peek(ctx context.Context, config strategies.StrategyConfig) (map[string]strategies.Result, error) {
	// Type assert to FixedWindowConfig
	fixedConfig, ok := config.(Config)
	if !ok {
		return nil, fmt.Errorf("FixedWindow strategy requires FixedWindowConfig")
	}

	now := time.Now()
	results := make(map[string]strategies.Result, len(fixedConfig.Quotas))

	// Process each quota
	for quotaName, quota := range fixedConfig.Quotas {
		// Create quota-specific key
		quotaKey := buildQuotaKey(fixedConfig.Key, quotaName)

		// Get current window state for this quota
		data, err := f.storage.Get(ctx, quotaKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get window state for quota '%s': %w", quotaName, err)
		}

		var window FixedWindow
		if data == "" {
			// No existing window, return default state
			results[quotaName] = strategies.Result{
				Allowed:   true,
				Remaining: quota.Limit,
				Reset:     now.Add(quota.Window),
			}
			continue
		}

		// Parse existing window state (compact format only)
		if w, ok := decodeFixedWindow(data); ok {
			window = w
		} else {
			return nil, fmt.Errorf("failed to parse window state for quota '%s': invalid encoding", quotaName)
		}

		// Check if current window has expired
		if now.Sub(window.Start) >= quota.Window {
			// Window has expired, return fresh state
			results[quotaName] = strategies.Result{
				Allowed:   true,
				Remaining: quota.Limit,
				Reset:     now.Add(quota.Window),
			}
			continue
		}

		// Calculate remaining requests and reset time
		remaining := max(quota.Limit-window.Count, 0)
		resetTime := window.Start.Add(quota.Window)

		results[quotaName] = strategies.Result{
			Allowed:   remaining > 0,
			Remaining: remaining,
			Reset:     resetTime,
		}
	}

	return results, nil
}

// Reset resets the rate limit counter for the given key
func (f *Strategy) Reset(ctx context.Context, config strategies.StrategyConfig) error {
	// Type assert to FixedWindowConfig
	fixedConfig, ok := config.(Config)
	if !ok {
		return fmt.Errorf("FixedWindow strategy requires FixedWindowConfig")
	}

	// Delete all quota keys from storage
	var lastErr error
	for quotaName := range fixedConfig.Quotas {
		quotaKey := buildQuotaKey(fixedConfig.Key, quotaName)
		if err := f.storage.Delete(ctx, quotaKey); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// encodeFixedWindow serializes FixedWindow into a compact ASCII format:
// v2|count|start_unix_nano
func encodeFixedWindow(w FixedWindow) string {
	var b strings.Builder
	b.Grow(2 + 1 + 20 + 1 + 20) // rough capacity
	b.WriteString("v2|")
	b.WriteString(strconv.Itoa(w.Count))
	b.WriteByte('|')
	b.WriteString(strconv.FormatInt(w.Start.UnixNano(), 10))
	return b.String()
}

// parseFixedWindowFields parses the fields from a fixed window string representation
func parseFixedWindowFields(data string) (int, int64, bool) {
	// Parse count (first field)
	pos1 := 0
	for pos1 < len(data) && data[pos1] != '|' {
		pos1++
	}
	if pos1 == len(data) {
		return 0, 0, false
	}

	count, err1 := strconv.Atoi(data[:pos1])
	if err1 != nil {
		return 0, 0, false
	}

	// Parse start time (second field)
	startNS, err2 := strconv.ParseInt(data[pos1+1:], 10, 64)
	if err2 != nil {
		return 0, 0, false
	}

	return count, startNS, true
}

// decodeFixedWindow deserializes from compact format; returns ok=false if not compact.
func decodeFixedWindow(s string) (FixedWindow, bool) {
	if !strategies.CheckV2Header(s) {
		return FixedWindow{}, false
	}

	data := s[3:] // Skip "v2|"

	count, startNS, ok := parseFixedWindowFields(data)
	if !ok {
		return FixedWindow{}, false
	}

	return FixedWindow{
		Count: count,
		Start: time.Unix(0, startNS),
	}, true
}

// Allow checks if a request is allowed and returns detailed statistics
func (f *Strategy) Allow(ctx context.Context, config strategies.StrategyConfig) (map[string]strategies.Result, error) {
	// Type assert to FixedWindowConfig
	fixedConfig, ok := config.(Config)
	if !ok {
		return nil, fmt.Errorf("FixedWindow strategy requires FixedWindowConfig")
	}

	now := time.Now()

	// Check if this is a multi-quota configuration
	isMultiQuota := len(fixedConfig.Quotas) > 1

	if isMultiQuota {
		// Multi-quota: Use two-phase commit for proper atomicity
		return f.allowMultiQuota(ctx, fixedConfig, now)
	} else {
		// Single-quota: Use original optimized approach
		return f.allowSingleQuota(ctx, fixedConfig, now)
	}
}

// allowMultiQuota handles multi-quota configurations with two-phase commit
func (f *Strategy) allowMultiQuota(ctx context.Context, fixedConfig Config, now time.Time) (map[string]strategies.Result, error) {
	results := make(map[string]strategies.Result, len(fixedConfig.Quotas))

	// Phase 1: Check all quotas without consuming quota
	quotaStates, err := f.getQuotaStates(ctx, fixedConfig, now)
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
			result, err := f.consumeQuota(ctx, state, now)
			if err != nil {
				return nil, err
			}
			results[state.name] = result
		} else {
			// At least one quota denied, so don't consume quota from any quota
			result, err := f.getDenyResult(ctx, state, now)
			if err != nil {
				return nil, err
			}
			results[state.name] = result
		}
	}

	// If some quotas weren't processed because an earlier quota denied,
	// we need to initialize their results
	if !allAllowed {
		err = f.initializeUnprocessedQuotas(ctx, fixedConfig, now, results)
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// getQuotaStates gets the current state for all quotas without consuming quota
func (f *Strategy) getQuotaStates(ctx context.Context, fixedConfig Config, now time.Time) ([]quotaState, error) {
	var quotaStates []quotaState
	quotaStates = make([]quotaState, 0, len(fixedConfig.Quotas))

	for quotaName, quota := range fixedConfig.Quotas {
		var keyBuilder strings.Builder
		keyBuilder.Grow(len(fixedConfig.Key) + len(quotaName) + 1)
		keyBuilder.WriteString(fixedConfig.Key)
		keyBuilder.WriteByte(':')
		keyBuilder.WriteString(quotaName)
		quotaKey := keyBuilder.String()

		// Get current window state for this quota
		data, err := f.storage.Get(ctx, quotaKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get window state for quota '%s': %w", quotaName, err)
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
			// Parse existing window state (compact format only)
			if w, ok := decodeFixedWindow(data); ok {
				window = w
			} else {
				return nil, fmt.Errorf("failed to parse window state for quota '%s': invalid encoding", quotaName)
			}

			// Check if current window has expired
			if now.Sub(window.Start) >= quota.Window {
				// Start new window - reset count but keep same start time structure
				window.Count = 0
				window.Start = now
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

// consumeQuota consumes quota for a quota using atomic CheckAndSet
func (f *Strategy) consumeQuota(ctx context.Context, state quotaState, now time.Time) (strategies.Result, error) {
	for attempt := range strategies.CheckAndSetRetries {
		// Check if context is cancelled or timed out
		if ctx.Err() != nil {
			return strategies.Result{}, fmt.Errorf("context cancelled or timed out: %w", ctx.Err())
		}

		// Increment request count
		updatedWindow := state.window
		updatedWindow.Count += 1

		// Calculate remaining after increment
		remaining := max(state.quota.Limit-updatedWindow.Count, 0)
		resetTime := updatedWindow.Start.Add(state.quota.Window)

		// Save updated window state in compact format
		newValue := encodeFixedWindow(updatedWindow)

		// Use CheckAndSet for atomic update
		success, err := f.storage.CheckAndSet(ctx, state.key, state.oldValue, newValue, state.quota.Window)
		if err != nil {
			return strategies.Result{}, fmt.Errorf("failed to save window state for quota '%s': %w", state.name, err)
		}

		if success {
			// Atomic update succeeded
			return strategies.Result{
				Allowed:   true,
				Remaining: remaining,
				Reset:     resetTime,
			}, nil
		}

		// If CheckAndSet failed, retry if we haven't exhausted attempts
		if attempt < strategies.CheckAndSetRetries-1 {
			// Re-read the current state and retry
			time.Sleep((19 * time.Nanosecond) << (time.Duration(attempt)))
			data, err := f.storage.Get(ctx, state.key)
			if err != nil {
				return strategies.Result{}, fmt.Errorf("failed to re-read window state for quota '%s': %w", state.name, err)
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
				if w, ok := decodeFixedWindow(data); ok {
					updatedWindow = w
					// Check if window expired during retry
					if now.Sub(updatedWindow.Start) >= state.quota.Window {
						updatedWindow.Count = 0
						updatedWindow.Start = now
					}
				} else {
					return strategies.Result{}, fmt.Errorf("failed to parse window state for quota '%s' during retry: invalid encoding", state.name)
				}
				state.oldValue = data
			}
			state.window = updatedWindow
			continue
		}
		return strategies.Result{}, fmt.Errorf("failed to update window state for quota '%s' after %d attempts due to concurrent access", state.name, strategies.CheckAndSetRetries)
	}
	return strategies.Result{}, nil // This shouldn't happen due to the loop logic
}

// getDenyResult returns results for a quota when the request is denied
func (f *Strategy) getDenyResult(ctx context.Context, state quotaState, now time.Time) (strategies.Result, error) {
	remaining := max(state.quota.Limit-state.window.Count, 0)
	resetTime := state.window.Start.Add(state.quota.Window)

	result := strategies.Result{
		Allowed:   state.allowed,
		Remaining: remaining,
		Reset:     resetTime,
	}

	// For denied requests, handle window reset if expired
	if !state.allowed && now.Sub(state.window.Start) >= state.quota.Window {
		// Window expired, reset it
		resetWindow := FixedWindow{
			Count: 0,
			Start: now,
		}
		newValue := encodeFixedWindow(resetWindow)
		_, err := f.storage.CheckAndSet(ctx, state.key, state.oldValue, newValue, state.quota.Window)
		if err != nil {
			return strategies.Result{}, fmt.Errorf("failed to reset expired window for quota '%s': %w", state.name, err)
		}
	}

	return result, nil
}

// initializeUnprocessedQuotas initializes results for quotas that weren't processed
func (f *Strategy) initializeUnprocessedQuotas(ctx context.Context, fixedConfig Config, now time.Time, results map[string]strategies.Result) error {
	for quotaName, quota := range fixedConfig.Quotas {
		if _, exists := results[quotaName]; !exists {
			// This quota wasn't processed, initialize it without consuming quota
			var keyBuilder strings.Builder
			keyBuilder.Grow(len(fixedConfig.Key) + len(quotaName) + 1)
			keyBuilder.WriteString(fixedConfig.Key)
			keyBuilder.WriteByte(':')
			keyBuilder.WriteString(quotaName)
			quotaKey := keyBuilder.String()
			data, err := f.storage.Get(ctx, quotaKey)
			if err != nil {
				return fmt.Errorf("failed to get window state for quota '%s': %w", quotaName, err)
			}

			if data == "" {
				// Initialize this quota without consuming quota
				results[quotaName] = strategies.Result{
					Allowed:   true,
					Remaining: quota.Limit,
					Reset:     now.Add(quota.Window),
				}
			} else {
				// Get current state without modifying
				if w, ok := decodeFixedWindow(data); ok {
					if now.Sub(w.Start) >= quota.Window {
						// Window expired
						results[quotaName] = strategies.Result{
							Allowed:   true,
							Remaining: quota.Limit,
							Reset:     now.Add(quota.Window),
						}
					} else {
						remaining := max(quota.Limit-w.Count, 0)
						results[quotaName] = strategies.Result{
							Allowed:   remaining > 0,
							Remaining: remaining,
							Reset:     w.Start.Add(quota.Window),
						}
					}
				} else {
					return fmt.Errorf("failed to parse window state for quota '%s': invalid encoding", quotaName)
				}
			}
		}
	}
	return nil
}

// allowSingleQuota handles single-quota configurations with the original optimized approach
func (f *Strategy) allowSingleQuota(ctx context.Context, fixedConfig Config, now time.Time) (map[string]strategies.Result, error) {
	results := make(map[string]strategies.Result)

	// Get the single quota (there should be exactly one)
	var quotaName string
	var quota Quota
	for name, t := range fixedConfig.Quotas {
		quotaName = name
		quota = t
		break
	}

	// Create quota-specific key
	quotaKey := buildQuotaKey(fixedConfig.Key, quotaName)

	// Try atomic CheckAndSet operations first
	var allowed bool
	var remaining int
	var resetTime time.Time

	for attempt := range strategies.CheckAndSetRetries {
		// Check if context is cancelled or timed out
		if ctx.Err() != nil {
			return nil, fmt.Errorf("context cancelled or timed out: %w", ctx.Err())
		}

		// Get current window state for this quota
		data, err := f.storage.Get(ctx, quotaKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get window state for quota '%s': %w", quotaName, err)
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
			// Parse existing window state (compact format only)
			if w, ok := decodeFixedWindow(data); ok {
				window = w
			} else {
				return nil, fmt.Errorf("failed to parse window state for quota '%s': invalid encoding", quotaName)
			}

			// Check if current window has expired
			if now.Sub(window.Start) >= quota.Window {
				// Start new window - reset count but keep same start time structure
				window.Count = 0
				window.Start = now
			}
			oldValue = data
		}

		// Determine if request is allowed based on current count
		allowed = window.Count < quota.Limit
		resetTime = window.Start.Add(quota.Window)

		if allowed {
			// Increment request count
			window.Count += 1

			// Calculate remaining after increment (subtract 1 for the request we just processed)
			remaining = max(quota.Limit-window.Count, 0)

			// Save updated window state in compact format
			newValue := encodeFixedWindow(window)

			// Use CheckAndSet for atomic update
			success, err := f.storage.CheckAndSet(ctx, quotaKey, oldValue, newValue, quota.Window)
			if err != nil {
				return nil, fmt.Errorf("failed to save window state for quota '%s': %w", quotaName, err)
			}

			if success {
				// Atomic update succeeded
				break
			}
			// If CheckAndSet failed, retry if we haven't exhausted attempts
			if attempt < strategies.CheckAndSetRetries-1 {
				time.Sleep((19 * time.Nanosecond) << (time.Duration(attempt)))
				continue
			}
			return nil, fmt.Errorf("failed to update window state for quota '%s' after %d attempts due to concurrent access", quotaName, strategies.CheckAndSetRetries)
		} else {
			// Request was denied, return original remaining count
			remaining = max(quota.Limit-window.Count, 0)

			// For denied requests, we don't need to update the count
			// Only update if window expired to ensure proper reset handling
			if now.Sub(window.Start) >= quota.Window {
				// Window expired, reset it
				window.Count = 0
				window.Start = now

				newValue := encodeFixedWindow(window)
				_, err := f.storage.CheckAndSet(ctx, quotaKey, oldValue, newValue, quota.Window)
				if err != nil {
					return nil, fmt.Errorf("failed to reset expired window for quota '%s': %w", quotaName, err)
				}
			}
			break
		}
	}

	// Store result for this quota
	results[quotaName] = strategies.Result{
		Allowed:   allowed,
		Remaining: remaining,
		Reset:     resetTime,
	}

	return results, nil
}
