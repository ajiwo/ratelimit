package internal

import (
	"strconv"
	"time"

	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/utils/builderpool"
)

// GCRA represents the state for GCRA rate limiting
type GCRA struct {
	TAT time.Time `json:"tat"` // Theoretical Arrival Time
}

// encodeState serializes GCRAState into a compact ASCII format:
// v2|tat_unix_nano
func encodeState(s GCRA) string {
	sb := builderpool.Get()
	defer func() {
		builderpool.Put(sb)
	}()

	sb.WriteString("v2|")
	sb.WriteString(strconv.FormatInt(s.TAT.UnixNano(), 10))
	return sb.String()
}

// decodeState deserializes from compact format; returns ok=false if not compact.
func decodeState(s string) (GCRA, bool) {
	if !strategies.CheckV2Header(s) {
		return GCRA{}, false
	}

	data := s[3:] // Skip "v2|"

	tat, ok := parseStateFields(data)
	if !ok {
		return GCRA{}, false
	}

	return GCRA{
		TAT: time.Unix(0, tat),
	}, true
}

// parseStateFields parses the fields from a GCRA state string representation
func parseStateFields(data string) (int64, bool) {
	// Parse TAT (only field)
	tat, err := strconv.ParseInt(data, 10, 64)
	if err != nil {
		return 0, false
	}

	return tat, true
}
