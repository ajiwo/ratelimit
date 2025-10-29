package internal

import (
	"strconv"
	"time"

	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/utils/builderpool"
)

type TokenBucket struct {
	Tokens     float64   `json:"tokens"`
	LastRefill time.Time `json:"last_refill"`
}

func encodeState(b TokenBucket) string {
	sb := builderpool.Get()
	defer func() {
		builderpool.Put(sb)
	}()

	sb.WriteString("v2|")
	sb.WriteString(strconv.FormatFloat(b.Tokens, 'g', -1, 64))
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatInt(b.LastRefill.UnixNano(), 10))
	return sb.String()
}

func decodeState(s string) (TokenBucket, bool) {
	if !strategies.CheckV2Header(s) {
		return TokenBucket{}, false
	}

	data := s[3:]

	tokens, last, ok := parseStateFields(data)
	if !ok {
		return TokenBucket{}, false
	}

	return TokenBucket{
		Tokens:     tokens,
		LastRefill: time.Unix(0, last),
	}, true
}

func parseStateFields(data string) (float64, int64, bool) {
	pos1 := 0
	for pos1 < len(data) && data[pos1] != '|' {
		pos1++
	}
	if pos1 == len(data) {
		return 0, 0, false
	}

	tokens, err1 := strconv.ParseFloat(data[:pos1], 64)
	if err1 != nil {
		return 0, 0, false
	}

	last, err2 := strconv.ParseInt(data[pos1+1:], 10, 64)
	if err2 != nil {
		return 0, 0, false
	}

	return tokens, last, true
}
