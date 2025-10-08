package strategies

import (
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

const (
	// checkAndSetRetries is the maximum number of retry attempts for CheckAndSet operations
	checkAndSetRetries = 3000
)

// checkV1Header validates that the string starts with "v1|"
func checkV1Header(s string) bool {
	return len(s) >= 3 && s[0] == 'v' && s[1] == '1' && s[2] == '|'
}

// BaseStrategy provides common functionality for all rate limiting strategies
type BaseStrategy struct {
	storage backends.Backend
}

// calcExpiration calculates an appropriate expiration time for storage operations
// based on capacity and rate, with a minimum of 1 second
func calcExpiration(capacity int, rate float64) time.Duration {
	expirationSeconds := float64(capacity) / rate * 2
	if expirationSeconds < 1 {
		expirationSeconds = 1
	}
	return time.Duration(expirationSeconds) * time.Second
}
