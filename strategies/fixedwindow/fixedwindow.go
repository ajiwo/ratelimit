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
	results := make(map[string]strategies.Result)

	// Process each tier - all tiers must allow for the request to be allowed
	for tierName, tier := range fixedConfig.Tiers {
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

		// If this tier denied the request, we can stop processing further tiers
		if !allowed {
			// We still need to ensure other tiers are properly initialized/handled
			// but we don't consume quota from them
			for otherTierName, otherTier := range fixedConfig.Tiers {
				if otherTierName == tierName {
					continue // Already processed this tier
				}

				otherTierKey := fixedConfig.Key + ":" + otherTierName
				data, err := f.storage.Get(ctx, otherTierKey)
				if err != nil {
					return nil, fmt.Errorf("failed to get window state for tier '%s': %w", otherTierName, err)
				}

				if data == "" {
					// Initialize this tier without consuming quota
					results[otherTierName] = strategies.Result{
						Allowed:   true,
						Remaining: otherTier.Limit,
						Reset:     now.Add(otherTier.Window),
					}
				} else {
					// Get current state without modifying
					if w, ok := decodeFixedWindow(data); ok {
						if now.Sub(w.Start) >= w.Duration {
							// Window expired
							results[otherTierName] = strategies.Result{
								Allowed:   true,
								Remaining: otherTier.Limit,
								Reset:     now.Add(otherTier.Window),
							}
						} else {
							remaining := max(otherTier.Limit-w.Count, 0)
							results[otherTierName] = strategies.Result{
								Allowed:   remaining > 0,
								Remaining: remaining,
								Reset:     w.Start.Add(w.Duration),
							}
						}
					} else {
						return nil, fmt.Errorf("failed to parse window state for tier '%s': invalid encoding", otherTierName)
					}
				}
			}
			return results, nil
		}
	}

	return results, nil
}
