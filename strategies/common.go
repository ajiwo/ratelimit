package strategies

import (
	"time"
)

const (
	// DefaultMaxRetries is the default maximum number of retry attempts for CheckAndSet operations
	DefaultMaxRetries = 30
	MaxRetries        = 16383
)

// CheckV2Header validates that the string starts with "v2|"
func CheckV2Header(s string) bool {
	return len(s) >= 3 && s[0] == 'v' && s[1] == '2' && s[2] == '|'
}

// CalcExpiration calculates an appropriate expiration time for storage operations
// based on capacity and rate, with a minimum of 1 second
// it currently is used by leaky/token bucket strategy
func CalcExpiration(capacity int, rate float64) time.Duration {
	expirationSeconds := float64(capacity) / rate * 2
	if expirationSeconds < 1 {
		expirationSeconds = 1
	}
	return time.Duration(expirationSeconds) * time.Second
}

// NextDelay calculates the next delay.
// It produces a sawtooth-like pattern of exponential backoff for constant feedback.
// In practice, feedback is random, measured from the time before and after of the
// last failed CheckAndSet operation.
func NextDelay(attempt int, feedback time.Duration) time.Duration {

	// Clamp attempt
	attempt = min(max(attempt, 0), MaxRetries)

	// Clamp feedback duration
	feedback = min(max(feedback, 10*time.Millisecond), 10*time.Second)

	// Calculate shift amount (capped exponential growth)
	shift := attempt % 16

	mult := time.Duration(attempt + 1)
	delay := (feedback * mult) << shift

	return delay
}
