package internal

import (
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/ajiwo/ratelimit/utils/builderpool"
)

// FixedWindow represents the state of a fixed window
type FixedWindow struct {
	Count int       `json:"count"` // Current request count in the window
	Start time.Time `json:"start"` // Window start time
}

// encodeState serializes multiple quotas into a combined ASCII format:
// v2|N|quotaName1|count1|startUnixNano1|...|quotaNameN|countN|startUnixNanoN
func encodeState(quotaStates map[string]FixedWindow) string {
	if len(quotaStates) == 0 {
		return ""
	}

	// Get quota names in deterministic order (lexical ascending)
	quotaNames := slices.Sorted(maps.Keys(quotaStates))

	sb := builderpool.Get()
	defer func() {
		builderpool.Put(sb)
	}()

	sb.WriteString("v2|")
	sb.WriteString(strconv.Itoa(len(quotaNames)))

	for _, name := range quotaNames {
		window := quotaStates[name]
		sb.WriteByte('|')
		sb.WriteString(name)
		sb.WriteByte('|')
		sb.WriteString(strconv.Itoa(window.Count))
		sb.WriteByte('|')
		sb.WriteString(strconv.FormatInt(window.Start.UnixNano(), 10))
	}

	return sb.String()
}

// findPipeSeparator finds the next pipe separator in data and returns its position
func findPipeSeparator(data string) (int, bool) {
	pos := 0
	for pos < len(data) && data[pos] != '|' {
		pos++
	}
	return pos, pos < len(data)
}

// parseQuotaCount parses the quota count from the beginning of data
func parseQuotaCount(data string) (int, string, bool) {
	pos, found := findPipeSeparator(data)
	if !found {
		return 0, "", false
	}

	n, err := strconv.Atoi(data[:pos])
	if err != nil || n <= 0 || n > MaxQuota {
		return 0, "", false
	}

	return n, data[pos+1:], true
}

// parseQuotaField parses a single field (name or count) and returns the value and remaining data
func parseQuotaField(data string) (string, string, bool) {
	pos, found := findPipeSeparator(data)
	if !found {
		return "", "", false
	}

	return data[:pos], data[pos+1:], true
}

// parseStartTime parses the start time field for a quota
func parseStartTime(data string, isLastQuota bool) (int64, string, bool) {
	if isLastQuota {
		startNS, err := strconv.ParseInt(data, 10, 64)
		if err != nil {
			return 0, "", false
		}
		return startNS, "", true
	}

	pos, found := findPipeSeparator(data)
	if !found {
		return 0, "", false
	}

	startNS, err := strconv.ParseInt(data[:pos], 10, 64)
	if err != nil {
		return 0, "", false
	}

	return startNS, data[pos+1:], true
}

func decodeState(s string) (map[string]FixedWindow, bool) {
	if len(s) < 3 || s[:3] != "v2|" {
		return nil, false
	}

	data := s[3:] // Skip "v2|"

	// Parse number of quotas
	n, data, ok := parseQuotaCount(data)
	if !ok {
		return nil, false
	}

	result := make(map[string]FixedWindow, n)

	// Parse each quota
	for i := range n {
		if len(data) == 0 {
			return nil, false
		}

		// Parse quota name
		name, remainingData, ok := parseQuotaField(data)
		if !ok {
			return nil, false
		}

		// Parse count
		countStr, remainingData, ok := parseQuotaField(remainingData)
		if !ok {
			return nil, false
		}

		count, err := strconv.Atoi(countStr)
		if err != nil || count < 0 {
			return nil, false
		}

		// Parse start time
		isLastQuota := i == n-1
		startNS, remainingData, ok := parseStartTime(remainingData, isLastQuota)
		if !ok {
			return nil, false
		}

		result[name] = FixedWindow{
			Count: count,
			Start: time.Unix(0, startNS),
		}
		data = remainingData
	}

	return result, true
}

// computeMaxResetTTL calculates the TTL as the maximum reset time across all quotas minus now
// with a minimum of 1 second (non-configurable)
func computeMaxResetTTL(quotaStates map[string]FixedWindow, quotas map[string]Quota, now time.Time) time.Duration {
	var maxReset time.Time

	// Find the latest reset time across all quotas
	for name, window := range quotaStates {
		if quota, exists := quotas[name]; exists {
			resetTime := window.Start.Add(quota.Window)
			if resetTime.After(maxReset) {
				maxReset = resetTime
			}
		}
	}

	if maxReset.IsZero() {
		return 1 * time.Second // Default minimum
	}

	ttl := maxReset.Sub(now)
	if ttl < 1*time.Second {
		return 1 * time.Second
	}

	return ttl
}
