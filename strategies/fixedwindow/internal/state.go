package internal

import (
	"strconv"
	"time"

	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/utils/builderpool"
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
	oldValue string
	allowed  bool
}

// buildQuotaKey builds a quota-specific key
func buildQuotaKey(baseKey, quotaName string) string {
	sb := builderpool.Get()
	defer func() {
		builderpool.Put(sb)
	}()
	sb.WriteString(baseKey)
	sb.WriteByte(':')
	sb.WriteString(quotaName)
	return sb.String()
}

// encodeState serializes FixedWindow into a compact ASCII format:
// v2|count|start_unix_nano
func encodeState(w FixedWindow) string {
	sb := builderpool.Get()
	defer func() {
		builderpool.Put(sb)
	}()

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
