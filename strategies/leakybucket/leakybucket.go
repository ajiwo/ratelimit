package leakybucket

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

// LeakyBucket represents the state of a leaky bucket
type LeakyBucket struct {
	Requests float64   `json:"requests"`  // Current number of requests in the bucket
	LastLeak time.Time `json:"last_leak"` // Last time we leaked requests
}

// builderPool reduces allocations in string operations for leaky bucket strategy
var builderPool = sync.Pool{
	New: func() any {
		return &strings.Builder{}
	},
}

// Strategy implements the leaky bucket rate limiting algorithm
type Strategy struct {
	storage backends.Backend
}

// New creates a new leaky bucket strategy
func New(storage backends.Backend) *Strategy {
	return &Strategy{
		storage: storage,
	}
}

// GetResult returns detailed statistics for the current bucket state
func (l *Strategy) GetResult(ctx context.Context, config strategies.StrategyConfig) (map[string]strategies.Result, error) {
	// Type assert to LeakyBucketConfig
	leakyConfig, ok := config.(Config)
	if !ok {
		return nil, fmt.Errorf("LeakyBucket strategy requires LeakyBucketConfig")
	}

	now := time.Now()

	// Get current bucket state
	data, err := l.storage.Get(ctx, leakyConfig.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket state: %w", err)
	}

	var bucket LeakyBucket
	if data == "" {
		// No existing bucket, return default state
		return map[string]strategies.Result{
			"default": {
				Allowed:   true,
				Remaining: leakyConfig.Capacity,
				Reset:     now, // Leaky buckets don't have a reset time, they continuously leak
			},
		}, nil
	}

	// Parse existing bucket state (compact format only)
	if b, ok := decodeLeakyBucket(data); ok {
		bucket = b
	} else {
		return nil, fmt.Errorf("failed to parse bucket state: invalid encoding")
	}

	// Leak requests based on time elapsed using config values
	timeElapsed := now.Sub(bucket.LastLeak).Seconds()
	requestsToLeak := timeElapsed * leakyConfig.LeakRate
	bucket.Requests = max(0, bucket.Requests-requestsToLeak)
	bucket.LastLeak = now

	// Calculate remaining capacity
	remaining := max(leakyConfig.Capacity-int(bucket.Requests), 0)

	return map[string]strategies.Result{
		"default": {
			Allowed:   remaining > 0,
			Remaining: remaining,
			Reset:     now, // Leaky buckets don't have a reset time
		},
	}, nil
}

// Reset resets the leaky bucket counter for the given key
func (l *Strategy) Reset(ctx context.Context, config strategies.StrategyConfig) error {
	// Type assert to LeakyBucketConfig
	leakyConfig, ok := config.(Config)
	if !ok {
		return fmt.Errorf("LeakyBucket strategy requires LeakyBucketConfig")
	}

	// Delete the key from storage to reset the bucket
	return l.storage.Delete(ctx, leakyConfig.Key)
}

// encodeLeakyBucket serializes LeakyBucket into a compact ASCII format:
// v2|requests|lastleak_unix_nano
func encodeLeakyBucket(b LeakyBucket) string {
	sb := builderPool.Get().(*strings.Builder)
	defer func() {
		sb.Reset()
		builderPool.Put(sb)
	}()

	sb.Grow(2 + 1 + 24 + 1 + 20)
	sb.WriteString("v2|")
	sb.WriteString(strconv.FormatFloat(b.Requests, 'g', -1, 64))
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatInt(b.LastLeak.UnixNano(), 10))
	return sb.String()
}

// parseLeakyBucketFields parses the fields from a leaky bucket string representation
func parseLeakyBucketFields(data string) (float64, int64, bool) {
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

// decodeLeakyBucket deserializes from compact format; returns ok=false if not compact.
func decodeLeakyBucket(s string) (LeakyBucket, bool) {
	if !strategies.CheckV2Header(s) {
		return LeakyBucket{}, false
	}

	data := s[3:] // Skip "v2|"

	req, last, ok := parseLeakyBucketFields(data)
	if !ok {
		return LeakyBucket{}, false
	}

	return LeakyBucket{
		Requests: req,
		LastLeak: time.Unix(0, last),
	}, true
}

// calculateLBResetTime calculates when the bucket will have capacity for another request
func calculateLBResetTime(now time.Time, bucket LeakyBucket, capacity int, leakRate float64) time.Time {
	if bucket.Requests < float64(capacity) {
		// Already has capacity, no reset needed
		return now
	}

	// Calculate time to leak (bucket.Requests - capacity + 1) requests
	requestsToLeak := bucket.Requests - float64(capacity) + 1
	if requestsToLeak <= 0 {
		return now
	}

	// time = requests / leakRate (convert from float seconds to time.Duration)
	timeToLeakSeconds := requestsToLeak / leakRate
	return now.Add(time.Duration(timeToLeakSeconds * float64(time.Second)))
}

// Allow checks if a request is allowed and returns detailed statistics
func (l *Strategy) Allow(ctx context.Context, config strategies.StrategyConfig) (map[string]strategies.Result, error) {
	// Type assert to LeakyBucketConfig
	leakyConfig, ok := config.(Config)
	if !ok {
		return nil, fmt.Errorf("LeakyBucket strategy requires LeakyBucketConfig")
	}

	leakRate := leakyConfig.LeakRate

	now := time.Now()

	// Try atomic CheckAndSet operations first
	for attempt := range strategies.CheckAndSetRetries {
		// Check if context is cancelled or timed out
		if ctx.Err() != nil {
			return nil, fmt.Errorf("context cancelled or timed out: %w", ctx.Err())
		}

		// Get current bucket state
		data, err := l.storage.Get(ctx, leakyConfig.Key)
		if err != nil {
			return nil, fmt.Errorf("failed to get bucket state: %w", err)
		}

		var bucket LeakyBucket
		var oldValue any
		if data == "" {
			// Initialize new bucket
			bucket = LeakyBucket{
				Requests: 0,
				LastLeak: now,
			}
			oldValue = nil // Key doesn't exist
		} else {
			// Parse existing bucket state (compact format only)
			if b, ok := decodeLeakyBucket(data); ok {
				bucket = b
			} else {
				return nil, fmt.Errorf("failed to parse bucket state: invalid encoding")
			}
			oldValue = data // Current value for CheckAndSet

			// Leak requests based on elapsed time using config values
			elapsed := now.Sub(bucket.LastLeak)
			requestsToLeak := float64(elapsed.Nanoseconds()) * leakRate / 1e9
			bucket.Requests = max(bucket.Requests-requestsToLeak, 0)
			bucket.LastLeak = now
		}

		// Calculate if request is allowed
		allowed := bucket.Requests+1 <= float64(leakyConfig.Capacity)

		if allowed {
			// Add request to bucket
			bucket.Requests += 1

			// Calculate remaining capacity after adding request
			remaining := max(leakyConfig.Capacity-int(bucket.Requests), 0)

			// Save updated bucket state in compact format
			newValue := encodeLeakyBucket(bucket)

			// Use a reasonable expiration time (based on capacity and leak rate)
			expiration := strategies.CalcExpiration(leakyConfig.Capacity, leakyConfig.LeakRate)

			// Use CheckAndSet for atomic update
			success, err := l.storage.CheckAndSet(ctx, leakyConfig.Key, oldValue, newValue, expiration)
			if err != nil {
				return nil, fmt.Errorf("failed to save bucket state: %w", err)
			}

			if success {
				// Atomic update succeeded
				return map[string]strategies.Result{
					"default": {
						Allowed:   true,
						Remaining: remaining,
						Reset:     now, // When allowed, no specific reset needed
					},
				}, nil
			}
			// If CheckAndSet failed, retry if we haven't exhausted attempts
			if attempt < strategies.CheckAndSetRetries-1 {
				time.Sleep(time.Duration(3*(attempt+1)) * time.Microsecond)
				continue
			}
			break
		} else {
			// Request denied, return current remaining capacity
			remaining := max(leakyConfig.Capacity-int(bucket.Requests), 0)

			// Save the current bucket state (even when denying, to handle leaks)
			bucketData := encodeLeakyBucket(bucket)
			expiration := strategies.CalcExpiration(leakyConfig.Capacity, leakyConfig.LeakRate)

			// If this was a new bucket (rare case), set it
			if oldValue == nil {
				_, err := l.storage.CheckAndSet(ctx, leakyConfig.Key, oldValue, bucketData, expiration)
				if err != nil {
					return nil, fmt.Errorf("failed to initialize bucket state: %w", err)
				}
			}

			return map[string]strategies.Result{
				"default": {
					Allowed:   false,
					Remaining: remaining,
					Reset:     calculateLBResetTime(now, bucket, leakyConfig.Capacity, leakRate),
				},
			}, nil
		}
	}

	// CheckAndSet failed after checkAndSetRetries attempts
	return nil, fmt.Errorf("failed to update bucket state after %d attempts due to concurrent access", strategies.CheckAndSetRetries)
}
