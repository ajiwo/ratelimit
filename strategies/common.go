package strategies

import (
	"time"
)

const (
	// DefaultMaxRetries is the default maximum number of retry attempts for CheckAndSet operations
	DefaultMaxRetries = 300
	MaxRetries        = 65_535
)

// CheckV2Header validates that the string starts with "v2|"
func CheckV2Header(s string) bool {
	return len(s) >= 3 && s[0] == 'v' && s[1] == '2' && s[2] == '|'
}

// CalcExpiration calculates an appropriate expiration time for storage operations
// based on capacity and rate, with a minimum of 1 second
// it currenty is used by leaky/token bucket strategy
func CalcExpiration(capacity int, rate float64) time.Duration {
	expirationSeconds := float64(capacity) / rate * 2
	if expirationSeconds < 1 {
		expirationSeconds = 1
	}
	return time.Duration(expirationSeconds) * time.Second
}
