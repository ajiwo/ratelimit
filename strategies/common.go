package strategies

import (
	"math/rand/v2"
	"time"
)

const (
	// DefaultMaxRetries is the default maximum number of retry attempts for CheckAndSet operations
	DefaultMaxRetries = 30
	MaxRetries        = 9390
	TTLFactor         = 5
)

// CalcExpiration calculates an appropriate expiration time for storage operations
// based on capacity and rate, with a minimum of 1 second
// it's currently used by gcra, leaky bucket, and token bucket strategy
func CalcExpiration(capacity int, rate float64) time.Duration {
	expirationSeconds := (float64(capacity) / rate) * TTLFactor
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
	// Clamp feedback duration to prevent very short delays that could overwhelm the system
	// The 30ns lower bound reduces randomness for sub-30ns feedback values but prevents
	// system overload from rapid retries
	feedback = min(max(feedback, 30*time.Nanosecond), 10*time.Second)

	// Calculate shift amount (capped exponential growth)
	shift := attempt % 8

	// Calculate delay with linear multiplier and exponential shift
	mult := time.Duration(attempt + 1)
	delay := (feedback * mult) << shift

	half := delay >> 1
	// #nosec: G404 non security context
	jitter := time.Duration(rand.Int64N(int64(half)))
	// fmt.Printf("attempt=%d feedback=%v delay=%v half=%v jitter=%v\n", attempt, feedback, delay, half, jitter)

	return half + jitter
}
