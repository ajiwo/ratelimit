package internal

import (
	"strconv"
	"time"

	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/utils/builderpool"
)

// FixedWindow represents the state of a fixed window
type FixedWindow struct {
	Name  string    `json:"name"`  // Quota name
	Count int       `json:"count"` // Current request count in the window
	Start time.Time `json:"start"` // Window start time
}

// encodeState serializes multiple quotas into a combined ASCII format:
// 23|N|quotaName1|count1|startUnixNano1|...|quotaNameN|countN|startUnixNanoN
func encodeState(quotaStates []FixedWindow) string {
	count := len(quotaStates)
	if count == 0 {
		return ""
	}

	sb := builderpool.Get()
	defer builderpool.Put(sb)

	sb.WriteString("23|")
	sb.WriteString(strconv.Itoa(count))

	for _, window := range quotaStates {
		sb.WriteByte('|')
		sb.WriteString(window.Name)
		sb.WriteByte('|')
		sb.WriteString(strconv.Itoa(window.Count))
		sb.WriteByte('|')
		sb.WriteString(strconv.FormatInt(window.Start.UnixNano(), 10))
	}

	return sb.String()
}

// findPipeSeparator finds the next pipe separator in data and returns its position
func findPipeSeparator(data string) (int, bool) {
	dataLen := len(data)
	pos := 0
	for pos < dataLen && data[pos] != '|' {
		pos++
	}
	return pos, pos < dataLen
}

// parseQuotaCount parses the quota count from the beginning of data
func parseQuotaCount(data string) (int, string, bool) {
	// Since N is max 8 (single digit), we can check first char directly
	if data[0] < '1' || data[0] > '8' {
		return 0, "", false
	}

	n := int(data[0] - '0')
	return n, data[2:], true
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

func decodeState(s string) ([]FixedWindow, bool) {
	// example minimal valid state:
	// "23|1|a|1|0"
	if len(s) < 10 || s[:3] != "23|" || s[4:5] != "|" {
		return nil, false
	}

	data := s[3:] // Skip "23|"

	// Parse number of quotas
	n, data, ok := parseQuotaCount(data)
	if !ok {
		return nil, false
	}

	result := make([]FixedWindow, 0, n)

	// Parse each quota
	for i := range n {
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

		result = append(result, FixedWindow{
			Name:  name,
			Count: count,
			Start: time.Unix(0, startNS),
		})
		data = remainingData
	}

	return result, true
}

// findQuotaByName finds a quota by name in the quotas slice
func findQuotaByName(name string, quotas []Quota) (Quota, bool) {
	for _, q := range quotas {
		if q.Name == name {
			return q, true
		}
	}
	return Quota{}, false
}

// computeMaxResetTTL calculates the TTL as the maximum reset time across all quotas minus now
// with a minimum of 1 second (non-configurable)
func computeMaxResetTTL(quotaStates []FixedWindow, quotas []Quota, now time.Time) time.Duration {
	var maxReset time.Time

	// Find the latest reset time across all quotas
	for _, window := range quotaStates {
		if quota, exists := findQuotaByName(window.Name, quotas); exists {
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

	return ttl * strategies.TTLFactor
}
