package strategies

import (
	"time"
)

const (
	// CheckAndSetRetries is the maximum number of retry attempts for CheckAndSet operations
	CheckAndSetRetries = 3000
)

// CheckV1Header validates that the string starts with "v1|"
func CheckV1Header(s string) bool {
	return len(s) >= 3 && s[0] == 'v' && s[1] == '1' && s[2] == '|'
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
