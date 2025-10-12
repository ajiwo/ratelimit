package fixedwindow

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

// FixedWindow represents the state of a fixed window
type FixedWindow struct {
	Count    int           `json:"count"`    // Current request count in the window
	Start    time.Time     `json:"start"`    // Window start time
	Duration time.Duration `json:"duration"` // Window duration
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

// GetResult returns detailed statistics for the current window state
func (f *Strategy) GetResult(ctx context.Context, config any) (map[string]strategies.Result, error) {
	// Type assert to FixedWindowConfig
	fixedConfig, ok := config.(strategies.FixedWindowConfig)
	if !ok {
		return nil, fmt.Errorf("FixedWindow strategy requires FixedWindowConfig")
	}

	now := time.Now()
	results := make(map[string]strategies.Result)

	// Process each tier
	for tierName, tier := range fixedConfig.Tiers {
		// Create tier-specific key
		tierKey := fixedConfig.Key + ":" + tierName

		// Get current window state for this tier
		data, err := f.storage.Get(ctx, tierKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get window state for tier '%s': %w", tierName, err)
		}

		var window FixedWindow
		if data == "" {
			// No existing window, return default state
			results[tierName] = strategies.Result{
				Allowed:   true,
				Remaining: tier.Limit,
				Reset:     now.Add(tier.Window),
			}
			continue
		}

		// Parse existing window state (compact format only)
		if w, ok := decodeFixedWindow(data); ok {
			window = w
		} else {
			return nil, fmt.Errorf("failed to parse window state for tier '%s': invalid encoding", tierName)
		}

		// Check if current window has expired
		if now.Sub(window.Start) >= window.Duration {
			// Window has expired, return fresh state
			results[tierName] = strategies.Result{
				Allowed:   true,
				Remaining: tier.Limit,
				Reset:     now.Add(tier.Window),
			}
			continue
		}

		// Calculate remaining requests and reset time
		remaining := max(tier.Limit-window.Count, 0)
		resetTime := window.Start.Add(window.Duration)

		results[tierName] = strategies.Result{
			Allowed:   remaining > 0,
			Remaining: remaining,
			Reset:     resetTime,
		}
	}

	return results, nil
}

// Reset resets the rate limit counter for the given key
func (f *Strategy) Reset(ctx context.Context, config any) error {
	// Type assert to FixedWindowConfig
	fixedConfig, ok := config.(strategies.FixedWindowConfig)
	if !ok {
		return fmt.Errorf("FixedWindow strategy requires FixedWindowConfig")
	}

	// Delete all tier keys from storage
	var lastErr error
	for tierName := range fixedConfig.Tiers {
		tierKey := fixedConfig.Key + ":" + tierName
		if err := f.storage.Delete(ctx, tierKey); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// encodeFixedWindow serializes FixedWindow into a compact ASCII format:
// v1|count|start_unix_nano|duration_nano
func encodeFixedWindow(w FixedWindow) string {
	var b strings.Builder
	b.Grow(2 + 1 + 20 + 1 + 20 + 1 + 20) // rough capacity
	b.WriteString("v1|")
	b.WriteString(strconv.Itoa(w.Count))
	b.WriteByte('|')
	b.WriteString(strconv.FormatInt(w.Start.UnixNano(), 10))
	b.WriteByte('|')
	b.WriteString(strconv.FormatInt(int64(w.Duration), 10))
	return b.String()
}

// parseFixedWindowFields parses the fields from a fixed window string representation
func parseFixedWindowFields(data string) (int, int64, int64, bool) {
	// Parse count (first field)
	pos1 := 0
	for pos1 < len(data) && data[pos1] != '|' {
		pos1++
	}
	if pos1 == len(data) {
		return 0, 0, 0, false
	}

	count, err1 := strconv.Atoi(data[:pos1])
	if err1 != nil {
		return 0, 0, 0, false
	}

	// Parse start time (second field)
	pos2 := pos1 + 1
	for pos2 < len(data) && data[pos2] != '|' {
		pos2++
	}
	if pos2 == len(data) {
		return 0, 0, 0, false
	}

	startNS, err2 := strconv.ParseInt(data[pos1+1:pos2], 10, 64)
	if err2 != nil {
		return 0, 0, 0, false
	}

	// Parse duration (third field)
	durNS, err3 := strconv.ParseInt(data[pos2+1:], 10, 64)
	if err3 != nil {
		return 0, 0, 0, false
	}

	return count, startNS, durNS, true
}

// decodeFixedWindow deserializes from compact format; returns ok=false if not compact.
func decodeFixedWindow(s string) (FixedWindow, bool) {
	if !strategies.CheckV1Header(s) {
		return FixedWindow{}, false
	}

	data := s[3:] // Skip "v1|"

	count, startNS, durNS, ok := parseFixedWindowFields(data)
	if !ok {
		return FixedWindow{}, false
	}

	return FixedWindow{
		Count:    count,
		Start:    time.Unix(0, startNS),
		Duration: time.Duration(durNS),
	}, true
}

// Allow checks if a request is allowed and returns detailed statistics
func (f *Strategy) Allow(ctx context.Context, config any) (map[string]strategies.Result, error) {
	// Type assert to FixedWindowConfig
	fixedConfig, ok := config.(strategies.FixedWindowConfig)
	if !ok {
		return nil, fmt.Errorf("FixedWindow strategy requires FixedWindowConfig")
	}

	now := time.Now()

	// Check if this is a multi-tier configuration
	isMultiTier := len(fixedConfig.Tiers) > 1

	if isMultiTier {
		// Multi-tier: Use two-phase commit for proper atomicity
		return f.allowMultiTier(ctx, fixedConfig, now)
	} else {
		// Single-tier: Use original optimized approach
		return f.allowSingleTier(ctx, fixedConfig, now)
	}
}

// allowMultiTier handles multi-tier configurations with two-phase commit
func (f *Strategy) allowMultiTier(ctx context.Context, fixedConfig strategies.FixedWindowConfig, now time.Time) (map[string]strategies.Result, error) {
	results := make(map[string]strategies.Result)

	// Phase 1: Check all tiers without consuming quota
	type tierState struct {
		name     string
		tier     strategies.FixedWindowTier
		key      string
		window   FixedWindow
		oldValue any
		allowed  bool
	}

	var tierStates []tierState

	for tierName, tier := range fixedConfig.Tiers {
		tierKey := fixedConfig.Key + ":" + tierName

		// Get current window state for this tier
		data, err := f.storage.Get(ctx, tierKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get window state for tier '%s': %w", tierName, err)
		}

		var window FixedWindow
		var oldValue any
		if data == "" {
			// Initialize new window
			window = FixedWindow{
				Count:    0,
				Start:    now,
				Duration: tier.Window,
			}
			oldValue = nil // Key doesn't exist
		} else {
			// Parse existing window state (compact format only)
			if w, ok := decodeFixedWindow(data); ok {
				window = w
			} else {
				return nil, fmt.Errorf("failed to parse window state for tier '%s': invalid encoding", tierName)
			}

			// Check if current window has expired
			if now.Sub(window.Start) >= window.Duration {
				// Start new window - reset count but keep same start time structure
				window.Count = 0
				window.Start = now
				window.Duration = tier.Window
			}
			oldValue = data
		}

		// Determine if request is allowed based on current count
		allowed := window.Count < tier.Limit

		tierStates = append(tierStates, tierState{
			name:     tierName,
			tier:     tier,
			key:      tierKey,
			window:   window,
			oldValue: oldValue,
			allowed:  allowed,
		})

		// If this tier doesn't allow, we can stop checking others
		if !allowed {
			break
		}
	}

	// Check if all tiers allow the request
	allAllowed := true
	for _, state := range tierStates {
		if !state.allowed {
			allAllowed = false
			break
		}
	}

	// Phase 2: Generate results and optionally consume quota
	for _, state := range tierStates {
		if allAllowed {
			// All tiers allowed, so consume quota from this tier using atomic CheckAndSet
			for attempt := range strategies.CheckAndSetRetries {
				// Check if context is cancelled or timed out
				if ctx.Err() != nil {
					return nil, fmt.Errorf("context cancelled or timed out: %w", ctx.Err())
				}

				// Increment request count
				updatedWindow := state.window
				updatedWindow.Count += 1

				// Calculate remaining after increment
				remaining := max(state.tier.Limit-updatedWindow.Count, 0)
				resetTime := updatedWindow.Start.Add(updatedWindow.Duration)

				// Save updated window state in compact format
				newValue := encodeFixedWindow(updatedWindow)

				// Use CheckAndSet for atomic update
				success, err := f.storage.CheckAndSet(ctx, state.key, state.oldValue, newValue, updatedWindow.Duration)
				if err != nil {
					return nil, fmt.Errorf("failed to save window state for tier '%s': %w", state.name, err)
				}

				if success {
					// Atomic update succeeded
					results[state.name] = strategies.Result{
						Allowed:   true,
						Remaining: remaining,
						Reset:     resetTime,
					}
					break
				}

				// If CheckAndSet failed, retry if we haven't exhausted attempts
				if attempt < strategies.CheckAndSetRetries-1 {
					// Re-read the current state and retry
					time.Sleep(time.Duration(3*(attempt+1)) * time.Microsecond)
					data, err := f.storage.Get(ctx, state.key)
					if err != nil {
						return nil, fmt.Errorf("failed to re-read window state for tier '%s': %w", state.name, err)
					}

					if data == "" {
						// Window disappeared, start fresh
						updatedWindow = FixedWindow{
							Count:    0,
							Start:    now,
							Duration: state.tier.Window,
						}
						state.oldValue = nil
					} else {
						// Parse updated state
						if w, ok := decodeFixedWindow(data); ok {
							updatedWindow = w
							// Check if window expired during retry
							if now.Sub(updatedWindow.Start) >= updatedWindow.Duration {
								updatedWindow.Count = 0
								updatedWindow.Start = now
								updatedWindow.Duration = state.tier.Window
							}
						} else {
							return nil, fmt.Errorf("failed to parse window state for tier '%s' during retry: invalid encoding", state.name)
						}
						state.oldValue = data
					}
					state.window = updatedWindow
					continue
				}
				return nil, fmt.Errorf("failed to update window state for tier '%s' after %d attempts due to concurrent access", state.name, strategies.CheckAndSetRetries)
			}
		} else {
			// At least one tier denied, so don't consume quota from any tier
			// Just return the current state without modification
			remaining := max(state.tier.Limit-state.window.Count, 0)
			resetTime := state.window.Start.Add(state.window.Duration)

			results[state.name] = strategies.Result{
				Allowed:   state.allowed,
				Remaining: remaining,
				Reset:     resetTime,
			}

			// For denied requests, handle window reset if expired
			if !state.allowed && now.Sub(state.window.Start) >= state.window.Duration {
				// Window expired, reset it
				resetWindow := FixedWindow{
					Count:    0,
					Start:    now,
					Duration: state.tier.Window,
				}
				newValue := encodeFixedWindow(resetWindow)
				_, err := f.storage.CheckAndSet(ctx, state.key, state.oldValue, newValue, resetWindow.Duration)
				if err != nil {
					return nil, fmt.Errorf("failed to reset expired window for tier '%s': %w", state.name, err)
				}
			}
		}
	}

	// If some tiers weren't processed because an earlier tier denied,
	// we need to initialize their results
	if !allAllowed {
		for tierName, tier := range fixedConfig.Tiers {
			if _, exists := results[tierName]; !exists {
				// This tier wasn't processed, initialize it without consuming quota
				tierKey := fixedConfig.Key + ":" + tierName
				data, err := f.storage.Get(ctx, tierKey)
				if err != nil {
					return nil, fmt.Errorf("failed to get window state for tier '%s': %w", tierName, err)
				}

				if data == "" {
					// Initialize this tier without consuming quota
					results[tierName] = strategies.Result{
						Allowed:   true,
						Remaining: tier.Limit,
						Reset:     now.Add(tier.Window),
					}
				} else {
					// Get current state without modifying
					if w, ok := decodeFixedWindow(data); ok {
						if now.Sub(w.Start) >= w.Duration {
							// Window expired
							results[tierName] = strategies.Result{
								Allowed:   true,
								Remaining: tier.Limit,
								Reset:     now.Add(tier.Window),
							}
						} else {
							remaining := max(tier.Limit-w.Count, 0)
							results[tierName] = strategies.Result{
								Allowed:   remaining > 0,
								Remaining: remaining,
								Reset:     w.Start.Add(w.Duration),
							}
						}
					} else {
						return nil, fmt.Errorf("failed to parse window state for tier '%s': invalid encoding", tierName)
					}
				}
			}
		}
	}

	return results, nil
}

// allowSingleTier handles single-tier configurations with the original optimized approach
func (f *Strategy) allowSingleTier(ctx context.Context, fixedConfig strategies.FixedWindowConfig, now time.Time) (map[string]strategies.Result, error) {
	results := make(map[string]strategies.Result)

	// Get the single tier (there should be exactly one)
	var tierName string
	var tier strategies.FixedWindowTier
	for name, t := range fixedConfig.Tiers {
		tierName = name
		tier = t
		break
	}

	// Create tier-specific key
	tierKey := fixedConfig.Key + ":" + tierName

	// Try atomic CheckAndSet operations first
	var allowed bool
	var remaining int
	var resetTime time.Time

	for attempt := range strategies.CheckAndSetRetries {
		// Check if context is cancelled or timed out
		if ctx.Err() != nil {
			return nil, fmt.Errorf("context cancelled or timed out: %w", ctx.Err())
		}

		// Get current window state for this tier
		data, err := f.storage.Get(ctx, tierKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get window state for tier '%s': %w", tierName, err)
		}

		var window FixedWindow
		var oldValue any
		if data == "" {
			// Initialize new window
			window = FixedWindow{
				Count:    0,
				Start:    now,
				Duration: tier.Window,
			}
			oldValue = nil // Key doesn't exist
		} else {
			// Parse existing window state (compact format only)
			if w, ok := decodeFixedWindow(data); ok {
				window = w
			} else {
				return nil, fmt.Errorf("failed to parse window state for tier '%s': invalid encoding", tierName)
			}

			// Check if current window has expired
			if now.Sub(window.Start) >= window.Duration {
				// Start new window - reset count but keep same start time structure
				window.Count = 0
				window.Start = now
				window.Duration = tier.Window
			}
			oldValue = data
		}

		// Determine if request is allowed based on current count
		allowed = window.Count < tier.Limit
		resetTime = window.Start.Add(window.Duration)

		if allowed {
			// Increment request count
			window.Count += 1

			// Calculate remaining after increment (subtract 1 for the request we just processed)
			remaining = max(tier.Limit-window.Count, 0)

			// Save updated window state in compact format
			newValue := encodeFixedWindow(window)

			// Use CheckAndSet for atomic update
			success, err := f.storage.CheckAndSet(ctx, tierKey, oldValue, newValue, window.Duration)
			if err != nil {
				return nil, fmt.Errorf("failed to save window state for tier '%s': %w", tierName, err)
			}

			if success {
				// Atomic update succeeded
				break
			}
			// If CheckAndSet failed, retry if we haven't exhausted attempts
			if attempt < strategies.CheckAndSetRetries-1 {
				time.Sleep(time.Duration(3*(attempt+1)) * time.Microsecond)
				continue
			}
			return nil, fmt.Errorf("failed to update window state for tier '%s' after %d attempts due to concurrent access", tierName, strategies.CheckAndSetRetries)
		} else {
			// Request was denied, return original remaining count
			remaining = max(tier.Limit-window.Count, 0)

			// For denied requests, we don't need to update the count
			// Only update if window expired to ensure proper reset handling
			if now.Sub(window.Start) >= window.Duration {
				// Window expired, reset it
				window.Count = 0
				window.Start = now
				window.Duration = tier.Window

				newValue := encodeFixedWindow(window)
				_, err := f.storage.CheckAndSet(ctx, tierKey, oldValue, newValue, window.Duration)
				if err != nil {
					return nil, fmt.Errorf("failed to reset expired window for tier '%s': %w", tierName, err)
				}
			}
			break
		}
	}

	// Store result for this tier
	results[tierName] = strategies.Result{
		Allowed:   allowed,
		Remaining: remaining,
		Reset:     resetTime,
	}

	return results, nil
}
