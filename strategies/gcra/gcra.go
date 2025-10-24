package gcra

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

// GCRA represents the state for GCRA rate limiting
type GCRA struct {
	TAT time.Time `json:"tat"` // Theoretical Arrival Time
}

// builderPool reduces allocations in string operations for GCRA strategy
var builderPool = sync.Pool{
	New: func() any {
		return &strings.Builder{}
	},
}

// Strategy implements the GCRA rate limiting algorithm
type Strategy struct {
	storage backends.Backend
}

// New creates a new GCRA strategy
func New(storage backends.Backend) *Strategy {
	return &Strategy{
		storage: storage,
	}
}

// Allow checks if a request is allowed and returns detailed statistics
func (g *Strategy) Allow(ctx context.Context, config strategies.StrategyConfig) (map[string]strategies.Result, error) {
	gcraConfig, ok := config.(Config)
	if !ok {
		return nil, fmt.Errorf("GCRA strategy requires GCRAConfig")
	}

	now := time.Now()
	emissionInterval := time.Duration(1e9/gcraConfig.Rate) * time.Nanosecond
	limit := time.Duration(float64(gcraConfig.Burst) * float64(emissionInterval))

	// Try atomic CheckAndSet operations first
	for attempt := range strategies.CheckAndSetRetries {
		// Check if context is cancelled or timed out
		if ctx.Err() != nil {
			return nil, fmt.Errorf("context cancelled or timed out: %w", ctx.Err())
		}

		// Get current state
		data, err := g.storage.Get(ctx, gcraConfig.Key)
		if err != nil {
			return nil, fmt.Errorf("failed to get GCRA state: %w", err)
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
			if s, ok := decodeGCRAState(data); ok {
				state = s
			} else {
				return nil, fmt.Errorf("failed to parse GCRA state: invalid encoding")
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
			remaining := g.calculateRemaining(now, state.TAT, emissionInterval, limit, gcraConfig.Burst)

			// Save updated state
			newValue := encodeGCRAState(state)
			expiration := strategies.CalcExpiration(gcraConfig.Burst, gcraConfig.Rate)

			success, err := g.storage.CheckAndSet(ctx, gcraConfig.Key, oldValue, newValue, expiration)
			if err != nil {
				return nil, fmt.Errorf("failed to save GCRA state: %w", err)
			}

			if success {
				// Atomic update succeeded
				return map[string]strategies.Result{
					"default": {
						Allowed:   true,
						Remaining: remaining,
						Reset:     state.TAT.Add(limit),
					},
				}, nil
			}

			// If CheckAndSet failed, retry if we haven't exhausted attempts
			if attempt < strategies.CheckAndSetRetries-1 {
				time.Sleep(time.Duration(3*(attempt+1)) * time.Microsecond)
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
				stateData := encodeGCRAState(state)
				expiration := strategies.CalcExpiration(gcraConfig.Burst, gcraConfig.Rate)
				_, err := g.storage.CheckAndSet(ctx, gcraConfig.Key, oldValue, stateData, expiration)
				if err != nil {
					return nil, fmt.Errorf("failed to initialize GCRA state: %w", err)
				}
			}

			return map[string]strategies.Result{
				"default": {
					Allowed:   false,
					Remaining: remaining,
					Reset:     resetTime,
				},
			}, nil
		}
	}

	// CheckAndSet failed after checkAndSetRetries attempts
	return nil, fmt.Errorf("failed to update GCRA state after %d attempts due to concurrent access", strategies.CheckAndSetRetries)
}

// GetResult returns detailed statistics for the current GCRA state
func (g *Strategy) GetResult(ctx context.Context, config strategies.StrategyConfig) (map[string]strategies.Result, error) {
	gcraConfig, ok := config.(Config)
	if !ok {
		return nil, fmt.Errorf("GCRA strategy requires GCRAConfig")
	}

	now := time.Now()
	emissionInterval := time.Duration(1e9/gcraConfig.Rate) * time.Nanosecond
	limit := time.Duration(float64(gcraConfig.Burst) * float64(emissionInterval))

	// Get current state
	data, err := g.storage.Get(ctx, gcraConfig.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to get GCRA state: %w", err)
	}

	if data == "" {
		// No existing state, fresh start
		return map[string]strategies.Result{
			"default": {
				Allowed:   true,
				Remaining: gcraConfig.Burst,
				Reset:     now.Add(limit),
			},
		}, nil
	}

	// Parse existing state
	state, ok := decodeGCRAState(data)
	if !ok {
		return nil, fmt.Errorf("failed to parse GCRA state: invalid encoding")
	}

	// Calculate remaining requests based on current TAT
	remaining := g.calculateRemaining(now, state.TAT, emissionInterval, limit, gcraConfig.Burst)

	// Calculate reset time (when next request will be allowed)
	resetTime := state.TAT.Add(limit)

	return map[string]strategies.Result{
		"default": {
			Allowed:   remaining > 0,
			Remaining: remaining,
			Reset:     resetTime,
		},
	}, nil
}

// Reset resets the GCRA state for the given key
func (g *Strategy) Reset(ctx context.Context, config strategies.StrategyConfig) error {
	gcraConfig, ok := config.(Config)
	if !ok {
		return fmt.Errorf("GCRA strategy requires GCRAConfig")
	}

	return g.storage.Delete(ctx, gcraConfig.Key)
}

// encodeGCRAState serializes GCRAState into a compact ASCII format:
// v2|tat_unix_nano
func encodeGCRAState(s GCRA) string {
	sb := builderPool.Get().(*strings.Builder)
	defer func() {
		sb.Reset()
		builderPool.Put(sb)
	}()

	sb.Grow(2 + 1 + 20) // rough capacity
	sb.WriteString("v2|")
	sb.WriteString(strconv.FormatInt(s.TAT.UnixNano(), 10))
	return sb.String()
}

// decodeGCRAState deserializes from compact format; returns ok=false if not compact.
func decodeGCRAState(s string) (GCRA, bool) {
	if !strategies.CheckV2Header(s) {
		return GCRA{}, false
	}

	data := s[3:] // Skip "v2|"

	tat, ok := parseGCRAStateFields(data)
	if !ok {
		return GCRA{}, false
	}

	return GCRA{
		TAT: time.Unix(0, tat),
	}, true
}

// parseGCRAStateFields parses the fields from a GCRA state string representation
func parseGCRAStateFields(data string) (int64, bool) {
	// Parse TAT (only field)
	tat, err := strconv.ParseInt(data, 10, 64)
	if err != nil {
		return 0, false
	}

	return tat, true
}

// calculateRemaining calculates the number of remaining requests based on current state
func (g *Strategy) calculateRemaining(now, tat time.Time, emissionInterval, limit time.Duration, burst int) int {
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
