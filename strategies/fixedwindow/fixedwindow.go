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

// TierConfig defines a single tier in multi-tier rate limiting
type TierConfig struct {
	Interval time.Duration // Time window (1 minute, 1 hour, 1 day, etc.)
	Limit    int           // Number of requests allowed in this interval
	Name     string        // Optional custom name for the tier (e.g., "login_attempts")
}

// MultiTierConfig defines configuration for multi-tier fixed window rate limiting
type MultiTierConfig struct {
	BaseKey    string       // Base key for rate limiting
	DynamicKey string       // Dynamic key (e.g., user ID)
	Tiers      []TierConfig // Rate limit tiers
}

// TierResult represents the result for a single tier
type TierResult struct {
	Allowed   bool      // Whether this tier allowed the request
	Remaining int       // Remaining requests in this tier
	Reset     time.Time // When this tier resets
	Total     int       // Total limit for this tier
}

// MultiTierResult represents the result for all tiers
type MultiTierResult struct {
	Results map[string]TierResult // Results for each tier by name
	Allowed bool                  // Overall decision (all tiers must allow)
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
func (f *Strategy) GetResult(ctx context.Context, config any) (strategies.Result, error) {
	// Type assert to FixedWindowConfig
	fixedConfig, ok := config.(strategies.FixedWindowConfig)
	if !ok {
		return strategies.Result{}, fmt.Errorf("FixedWindow strategy requires FixedWindowConfig")
	}

	now := time.Now()

	// Get current window state
	data, err := f.storage.Get(ctx, fixedConfig.Key)
	if err != nil {
		return strategies.Result{}, fmt.Errorf("failed to get window state: %w", err)
	}

	var window FixedWindow
	if data == "" {
		// No existing window, return default state
		return strategies.Result{
			Allowed:   true,
			Remaining: fixedConfig.Limit,
			Reset:     now.Add(fixedConfig.Window),
		}, nil
	}

	// Parse existing window state (compact format only)
	if w, ok := decodeFixedWindow(data); ok {
		window = w
	} else {
		return strategies.Result{}, fmt.Errorf("failed to parse window state: invalid encoding")
	}

	// Check if current window has expired
	if now.Sub(window.Start) >= window.Duration {
		// Window has expired, return fresh state
		return strategies.Result{
			Allowed:   true,
			Remaining: fixedConfig.Limit,
			Reset:     now.Add(fixedConfig.Window),
		}, nil
	}

	// Calculate remaining requests and reset time
	remaining := max(fixedConfig.Limit-window.Count, 0)

	resetTime := window.Start.Add(window.Duration)

	return strategies.Result{
		Allowed:   remaining > 0,
		Remaining: remaining,
		Reset:     resetTime,
	}, nil
}

// Reset resets the rate limit counter for the given key
func (f *Strategy) Reset(ctx context.Context, config any) error {
	// Type assert to FixedWindowConfig
	fixedConfig, ok := config.(strategies.FixedWindowConfig)
	if !ok {
		return fmt.Errorf("FixedWindow strategy requires FixedWindowConfig")
	}

	// Delete the key from storage to reset the counter
	return f.storage.Delete(ctx, fixedConfig.Key)
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
func (f *Strategy) Allow(ctx context.Context, config any) (strategies.Result, error) {
	// Type assert to FixedWindowConfig
	fixedConfig, ok := config.(strategies.FixedWindowConfig)
	if !ok {
		return strategies.Result{}, fmt.Errorf("FixedWindow strategy requires FixedWindowConfig")
	}

	now := time.Now()

	// Try atomic CheckAndSet operations first
	for attempt := range strategies.CheckAndSetRetries {
		// Check if context is cancelled or timed out
		if ctx.Err() != nil {
			return strategies.Result{}, fmt.Errorf("context cancelled or timed out: %w", ctx.Err())
		}

		// Get current window state
		data, err := f.storage.Get(ctx, fixedConfig.Key)
		if err != nil {
			return strategies.Result{}, fmt.Errorf("failed to get window state: %w", err)
		}

		var window FixedWindow
		var oldValue any
		if data == "" {
			// Initialize new window
			window = FixedWindow{
				Count:    0,
				Start:    now,
				Duration: fixedConfig.Window,
			}
			oldValue = nil // Key doesn't exist
		} else {
			// Parse existing window state (compact format only)
			if w, ok := decodeFixedWindow(data); ok {
				window = w
			} else {
				return strategies.Result{}, fmt.Errorf("failed to parse window state: invalid encoding")
			}

			// Check if current window has expired
			if now.Sub(window.Start) >= window.Duration {
				// Start new window - reset count but keep same start time structure
				window.Count = 0
				window.Start = now
				window.Duration = fixedConfig.Window
			}
			oldValue = data
		}

		// Determine if request is allowed based on current count
		allowed := window.Count < fixedConfig.Limit

		// Calculate reset time
		resetTime := window.Start.Add(window.Duration)

		if allowed {
			// Increment request count
			window.Count += 1

			// Calculate remaining after increment (subtract 1 for the request we just processed)
			remaining := max(fixedConfig.Limit-window.Count, 0)

			// Save updated window state in compact format
			newValue := encodeFixedWindow(window)

			// Use CheckAndSet for atomic update
			success, err := f.storage.CheckAndSet(ctx, fixedConfig.Key, oldValue, newValue, window.Duration)
			if err != nil {
				return strategies.Result{}, fmt.Errorf("failed to save window state: %w", err)
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
				time.Sleep(time.Duration(3*(attempt+1)) * time.Microsecond)
				continue
			}
			break
		} else {
			// Request was denied, return original remaining count
			remaining := max(fixedConfig.Limit-window.Count, 0)

			// For denied requests, we don't need to update the count
			// Only update if window expired to ensure proper reset handling
			if now.Sub(window.Start) >= window.Duration {
				// Window expired, reset it
				window.Count = 0
				window.Start = now
				window.Duration = fixedConfig.Window

				newValue := encodeFixedWindow(window)
				_, err := f.storage.CheckAndSet(ctx, fixedConfig.Key, oldValue, newValue, window.Duration)
				if err != nil {
					return strategies.Result{}, fmt.Errorf("failed to reset expired window: %w", err)
				}
			}

			return strategies.Result{
				Allowed:   false,
				Remaining: remaining,
				Reset:     resetTime,
			}, nil
		}
	}

	// CheckAndSet failed after checkAndSetRetries attempts
	return strategies.Result{}, fmt.Errorf("failed to update window state after %d attempts due to concurrent access", strategies.CheckAndSetRetries)
}

// getTierName generates a tier name based on interval or uses custom name
func getTierName(tier TierConfig) string {
	if tier.Name != "" {
		return tier.Name
	}

	// Generate name based on interval - match the original ratelimit package behavior
	switch tier.Interval {
	case time.Minute:
		return "minute"
	case time.Hour:
		return "hour"
	case 24 * time.Hour:
		return "day"
	case 7 * 24 * time.Hour:
		return "week"
	case 30 * 24 * time.Hour:
		return "month"
	default:
		// For custom intervals, use duration string
		return tier.Interval.String()
	}
}

// buildTierKey builds the storage key for a specific tier
func buildTierKey(baseKey, dynamicKey, tierName string) string {
	var keyBuilder strings.Builder
	keyBuilder.Grow(len(baseKey) + len(dynamicKey) + len(tierName) + 2) // +2 for the colons
	keyBuilder.WriteString(baseKey)
	keyBuilder.WriteByte(':')
	keyBuilder.WriteString(dynamicKey)
	keyBuilder.WriteByte(':')
	keyBuilder.WriteString(tierName)
	return keyBuilder.String()
}

// AllowMulti checks if a request is allowed across all configured tiers
func (f *Strategy) AllowMulti(ctx context.Context, config MultiTierConfig) (MultiTierResult, error) {
	results := make(map[string]TierResult)
	overallAllowed := true

	// Process each tier
	for _, tier := range config.Tiers {
		tierName := getTierName(tier)
		key := buildTierKey(config.BaseKey, config.DynamicKey, tierName)

		// Create single-tier config
		singleConfig := strategies.FixedWindowConfig{
			Key:    key,
			Limit:  tier.Limit,
			Window: tier.Interval,
		}

		// Check this tier
		result, err := f.Allow(ctx, singleConfig)
		if err != nil {
			return MultiTierResult{}, fmt.Errorf("tier %s failed: %w", tierName, err)
		}

		// Convert to tier result
		tierResult := TierResult{
			Allowed:   result.Allowed,
			Remaining: result.Remaining,
			Reset:     result.Reset,
			Total:     tier.Limit,
		}
		results[tierName] = tierResult

		// Overall decision: all tiers must allow
		if !result.Allowed {
			overallAllowed = false
		}
	}

	return MultiTierResult{
		Results: results,
		Allowed: overallAllowed,
	}, nil
}

// GetResultMulti returns detailed statistics for all tiers without consuming quota
func (f *Strategy) GetResultMulti(ctx context.Context, config MultiTierConfig) (MultiTierResult, error) {
	results := make(map[string]TierResult)
	overallAllowed := true

	// Process each tier
	for _, tier := range config.Tiers {
		tierName := getTierName(tier)
		key := buildTierKey(config.BaseKey, config.DynamicKey, tierName)

		// Create single-tier config
		singleConfig := strategies.FixedWindowConfig{
			Key:    key,
			Limit:  tier.Limit,
			Window: tier.Interval,
		}

		// Get stats for this tier
		result, err := f.GetResult(ctx, singleConfig)
		if err != nil {
			return MultiTierResult{}, fmt.Errorf("tier %s failed: %w", tierName, err)
		}

		// Convert to tier result
		tierResult := TierResult{
			Allowed:   result.Allowed,
			Remaining: result.Remaining,
			Reset:     result.Reset,
			Total:     tier.Limit,
		}
		results[tierName] = tierResult

		// Overall decision: all tiers must allow
		if !result.Allowed {
			overallAllowed = false
		}
	}

	return MultiTierResult{
		Results: results,
		Allowed: overallAllowed,
	}, nil
}

// ResetMulti resets rate limit counters for all tiers
func (f *Strategy) ResetMulti(ctx context.Context, config MultiTierConfig) error {
	var lastErr error

	// Reset each tier
	for _, tier := range config.Tiers {
		tierName := getTierName(tier)
		key := buildTierKey(config.BaseKey, config.DynamicKey, tierName)

		// Create single-tier config
		singleConfig := strategies.FixedWindowConfig{
			Key:    key,
			Limit:  tier.Limit,
			Window: tier.Interval,
		}

		// Reset this tier
		if err := f.Reset(ctx, singleConfig); err != nil {
			lastErr = fmt.Errorf("tier %s reset failed: %w", tierName, err)
		}
	}

	return lastErr
}
