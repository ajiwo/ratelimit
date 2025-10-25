package internal

import (
	"strconv"
	"strings"
	"time"

	"github.com/ajiwo/ratelimit/strategies"
)

// LeakyBucket represents the state of a leaky bucket
type LeakyBucket struct {
	Requests float64   `json:"requests"`  // Current number of requests in the bucket
	LastLeak time.Time `json:"last_leak"` // Last time we leaked requests
}

// encodeState serializes LeakyBucket into a compact ASCII format:
// v2|requests|lastleak_unix_nano
func encodeState(b LeakyBucket) string {
	sb := &strings.Builder{}
	sb.Grow(2 + 1 + 24 + 1 + 20)
	sb.WriteString("v2|")
	sb.WriteString(strconv.FormatFloat(b.Requests, 'g', -1, 64))
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatInt(b.LastLeak.UnixNano(), 10))
	return sb.String()
}

// parseStateFields parses the fields from a leaky bucket string representation
func parseStateFields(data string) (float64, int64, bool) {
	// Parse requests (first field)
	pos1 := 0
	for pos1 < len(data) && data[pos1] != '|' {
		pos1++
	}
	if pos1 == len(data) {
		return 0, 0, false
	}

	req, err1 := strconv.ParseFloat(data[:pos1], 64)
	if err1 != nil {
		return 0, 0, false
	}

	// Parse last leak (second field)
	last, err2 := strconv.ParseInt(data[pos1+1:], 10, 64)
	if err2 != nil {
		return 0, 0, false
	}

	return req, last, true
}

// decodeState deserializes from compact format; returns ok=false if not compact.
func decodeState(s string) (LeakyBucket, bool) {
	if !strategies.CheckV2Header(s) {
		return LeakyBucket{}, false
	}

	data := s[3:] // Skip "v2|"

	req, last, ok := parseStateFields(data)
	if !ok {
		return LeakyBucket{}, false
	}

	return LeakyBucket{
		Requests: req,
		LastLeak: time.Unix(0, last),
	}, true
}
