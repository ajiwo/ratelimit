package internal

import (
	"context"
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

// builderPool reduces allocations in string operations for fixed window strategy
var builderPool = sync.Pool{
	New: func() any {
		return &strings.Builder{}
	},
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

// buildQuotaKey builds a quota-specific key
func buildQuotaKey(baseKey, quotaName string) string {
	sb := builderPool.Get().(*strings.Builder)
	defer func() {
		sb.Reset()
		builderPool.Put(sb)
	}()
	sb.Grow(len(baseKey) + len(quotaName) + 1)
	sb.WriteString(baseKey)
	sb.WriteByte(':')
	sb.WriteString(quotaName)
	return sb.String()
}

// encodeState serializes FixedWindow into a compact ASCII format:
// v2|count|start_unix_nano
func encodeState(w FixedWindow) string {
	sb := builderPool.Get().(*strings.Builder)
	defer func() {
		sb.Reset()
		builderPool.Put(sb)
	}()

	sb.Grow(2 + 1 + 20 + 1 + 20) // rough capacity
	sb.WriteString("v2|")
	sb.WriteString(strconv.Itoa(w.Count))
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatInt(w.Start.UnixNano(), 10))
	return sb.String()
}

// decodeState deserializes from compact format; returns ok=false if not compact.
func decodeState(s string) (FixedWindow, bool) {
	if !strategies.CheckV2Header(s) {
		return FixedWindow{}, false
	}

	data := s[3:] // Skip "v2|"

	count, startNS, ok := parseStateFields(data)
	if !ok {
		return FixedWindow{}, false
	}

	return FixedWindow{
		Count: count,
		Start: time.Unix(0, startNS),
	}, true
}

// parseStateFields parses the fields from a fixed window string representation
func parseStateFields(data string) (int, int64, bool) {
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

// getQuotaStates gets the current state for all quotas without consuming quota
func getQuotaStates(ctx context.Context, storage backends.Backend, config Config, now time.Time, quotas map[string]Quota) ([]quotaState, error) {
	quotaStates := make([]quotaState, 0, len(quotas))

	for quotaName, quota := range quotas {
		quotaKey := buildQuotaKey(config.GetKey(), quotaName)

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
