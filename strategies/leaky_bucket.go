package strategies

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

// LeakyBucket represents the state of a leaky bucket
type LeakyBucket struct {
	Requests float64   `json:"requests"`  // Current number of requests in the bucket
	LastLeak time.Time `json:"last_leak"` // Last time we leaked requests
	Capacity float64   `json:"capacity"`  // Maximum requests the bucket can hold
	LeakRate float64   `json:"leak_rate"` // Requests to leak per second
}

// LeakyBucketStrategy implements the leaky bucket rate limiting algorithm
type LeakyBucketStrategy struct {
	BaseStrategy
}

// NewLeakyBucket creates a new leaky bucket strategy
func NewLeakyBucket(storage backends.Backend) *LeakyBucketStrategy {
	return &LeakyBucketStrategy{
		BaseStrategy: BaseStrategy{
			storage: storage,
		},
	}
}

// Allow checks if a request is allowed based on leaky bucket algorithm
//
// Deprecated: Use AllowWithResult instead. Allow will be removed in a future release.
func (l *LeakyBucketStrategy) Allow(ctx context.Context, config any) (bool, error) {
	result, err := l.AllowWithResult(ctx, config)
	return result.Allowed, err
}

// GetResult returns detailed statistics for the current bucket state
func (l *LeakyBucketStrategy) GetResult(ctx context.Context, config any) (Result, error) {
	// Type assert to LeakyBucketConfig
	leakyConfig, ok := config.(LeakyBucketConfig)
	if !ok {
		return Result{}, fmt.Errorf("LeakyBucket strategy requires LeakyBucketConfig")
	}

	// Get per-key lock to prevent concurrent access to the same bucket
	lock := l.getLock(leakyConfig.Key)
	lock.Lock()
	defer lock.Unlock()

	now := time.Now()

	// Get current bucket state
	data, err := l.storage.Get(ctx, leakyConfig.Key)
	if err != nil {
		return Result{}, fmt.Errorf("failed to get bucket state: %w", err)
	}

	var bucket LeakyBucket
	if data == "" {
		// No existing bucket, return default state
		return Result{
			Allowed:   true,
			Remaining: leakyConfig.Capacity,
			Reset:     now, // Leaky buckets don't have a reset time, they continuously leak
		}, nil
	}

	// Parse existing bucket state (compact format only)
	if b, ok := decodeLeakyBucket(data); ok {
		bucket = b
	} else {
		return Result{}, fmt.Errorf("failed to parse bucket state: invalid encoding")
	}

	// Leak requests based on time elapsed
	timeElapsed := now.Sub(bucket.LastLeak).Seconds()
	requestsToLeak := timeElapsed * bucket.LeakRate
	bucket.Requests = max(0, bucket.Requests-requestsToLeak)
	bucket.LastLeak = now

	// Calculate remaining capacity
	remaining := max(leakyConfig.Capacity-int(bucket.Requests), 0)

	return Result{
		Allowed:   remaining > 0,
		Remaining: remaining,
		Reset:     now, // Leaky buckets don't have a reset time
	}, nil
}

// Reset resets the leaky bucket counter for the given key
func (l *LeakyBucketStrategy) Reset(ctx context.Context, config any) error {
	// Type assert to LeakyBucketConfig
	leakyConfig, ok := config.(LeakyBucketConfig)
	if !ok {
		return fmt.Errorf("LeakyBucket strategy requires LeakyBucketConfig")
	}

	// Get per-key lock to prevent concurrent access to the same bucket
	lock := l.getLock(leakyConfig.Key)
	lock.Lock()
	defer lock.Unlock()

	// Delete the key from storage to reset the bucket
	return l.storage.Delete(ctx, leakyConfig.Key)
}

// encodeLeakyBucket serializes LeakyBucket into a compact ASCII format:
// v1|requests|lastleak_unix_nano|capacity|leak_rate
func encodeLeakyBucket(b LeakyBucket) string {
	var sb strings.Builder
	sb.Grow(2 + 1 + 24 + 1 + 20 + 1 + 24 + 1 + 24)
	sb.WriteString("v1|")
	sb.WriteString(strconv.FormatFloat(b.Requests, 'g', -1, 64))
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatInt(b.LastLeak.UnixNano(), 10))
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatFloat(b.Capacity, 'g', -1, 64))
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatFloat(b.LeakRate, 'g', -1, 64))
	return sb.String()
}

// parseLeakyBucketFields parses the fields from a leaky bucket string representation
func parseLeakyBucketFields(data string) (float64, int64, float64, float64, bool) {
	// Parse requests (first field)
	pos1 := 0
	for pos1 < len(data) && data[pos1] != '|' {
		pos1++
	}
	if pos1 == len(data) {
		return 0, 0, 0, 0, false
	}

	req, err1 := strconv.ParseFloat(data[:pos1], 64)
	if err1 != nil {
		return 0, 0, 0, 0, false
	}

	// Parse last leak (second field)
	pos2 := pos1 + 1
	for pos2 < len(data) && data[pos2] != '|' {
		pos2++
	}
	if pos2 == len(data) {
		return 0, 0, 0, 0, false
	}

	last, err2 := strconv.ParseInt(data[pos1+1:pos2], 10, 64)
	if err2 != nil {
		return 0, 0, 0, 0, false
	}

	// Parse capacity (third field)
	pos3 := pos2 + 1
	for pos3 < len(data) && data[pos3] != '|' {
		pos3++
	}
	if pos3 == len(data) {
		return 0, 0, 0, 0, false
	}

	capf, err3 := strconv.ParseFloat(data[pos2+1:pos3], 64)
	if err3 != nil {
		return 0, 0, 0, 0, false
	}

	// Parse leak rate (fourth field)
	lrate, err4 := strconv.ParseFloat(data[pos3+1:], 64)
	if err4 != nil {
		return 0, 0, 0, 0, false
	}

	return req, last, capf, lrate, true
}

// decodeLeakyBucket deserializes from compact format; returns ok=false if not compact.
func decodeLeakyBucket(s string) (LeakyBucket, bool) {
	if !checkV1Header(s) {
		return LeakyBucket{}, false
	}

	data := s[3:] // Skip "v1|"

	req, last, capf, lrate, ok := parseLeakyBucketFields(data)
	if !ok {
		return LeakyBucket{}, false
	}

	return LeakyBucket{
		Requests: req,
		LastLeak: time.Unix(0, last),
		Capacity: capf,
		LeakRate: lrate,
	}, true
}

// Cleanup removes stale locks
func (l *LeakyBucketStrategy) Cleanup(maxAge time.Duration) {
	l.CleanupLocks(maxAge)
}

// calculateLBResetTime calculates when the bucket will have capacity for another request
func calculateLBResetTime(now time.Time, bucket LeakyBucket, capacity int) time.Time {
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
	timeToLeakSeconds := requestsToLeak / bucket.LeakRate
	return now.Add(time.Duration(timeToLeakSeconds * float64(time.Second)))
}

// AllowWithResult checks if a request is allowed and returns detailed statistics
func (l *LeakyBucketStrategy) AllowWithResult(ctx context.Context, config any) (Result, error) {
	// Type assert to LeakyBucketConfig
	leakyConfig, ok := config.(LeakyBucketConfig)
	if !ok {
		return Result{}, fmt.Errorf("LeakyBucket strategy requires LeakyBucketConfig")
	}

	// Get per-key lock to prevent concurrent access to the same bucket
	lock := l.getLock(leakyConfig.Key)
	lock.Lock()
	defer lock.Unlock()

	capacity := float64(leakyConfig.Capacity)
	leakRate := leakyConfig.LeakRate

	now := time.Now()

	// Get current bucket state
	data, err := l.storage.Get(ctx, leakyConfig.Key)
	if err != nil {
		return Result{}, fmt.Errorf("failed to get bucket state: %w", err)
	}

	var bucket LeakyBucket
	if data == "" {
		// Initialize new bucket
		bucket = LeakyBucket{
			Requests: 0,
			LastLeak: now,
			Capacity: capacity,
			LeakRate: leakRate,
		}
	} else {
		// Parse existing bucket state (compact format only)
		if b, ok := decodeLeakyBucket(data); ok {
			bucket = b
		} else {
			return Result{}, fmt.Errorf("failed to parse bucket state: invalid encoding")
		}

		// Leak requests based on elapsed time
		elapsed := now.Sub(bucket.LastLeak)
		requestsToLeak := float64(elapsed.Nanoseconds()) * bucket.LeakRate / 1e9
		bucket.Requests = max(bucket.Requests-requestsToLeak, 0)
		bucket.LastLeak = now
	}

	// Calculate if request is allowed using the same logic as Allow method
	allowed := bucket.Requests+1 <= float64(leakyConfig.Capacity)

	// Calculate remaining capacity (before adding request if allowed)
	remaining := max(leakyConfig.Capacity-int(bucket.Requests), 0)

	// Only add request to bucket if allowed
	if allowed {
		// Add request to bucket
		bucket.Requests += 1

		// Save updated bucket state in compact format
		bucketData := encodeLeakyBucket(bucket)

		// Use a reasonable expiration time (based on capacity and leak rate)
		expiration := calcExpiration(leakyConfig.Capacity, leakyConfig.LeakRate)

		// Save the updated bucket state
		err = l.storage.Set(ctx, leakyConfig.Key, bucketData, expiration)
		if err != nil {
			return Result{}, fmt.Errorf("failed to save bucket state: %w", err)
		}

		// Update remaining after adding the request
		remaining = max(leakyConfig.Capacity-int(bucket.Requests), 0)
	} else {
		// Save updated bucket state even when denying request (persist leaked state)
		bucketData := encodeLeakyBucket(bucket)

		// Use a reasonable expiration time (based on capacity and leak rate)
		expiration := calcExpiration(leakyConfig.Capacity, leakyConfig.LeakRate)

		// Save the updated bucket state
		err = l.storage.Set(ctx, leakyConfig.Key, bucketData, expiration)
		if err != nil {
			return Result{}, fmt.Errorf("failed to save bucket state: %w", err)
		}
	}

	// Calculate reset time - when bucket will have capacity for at least one more request
	var resetTime time.Time
	if allowed {
		// When allowed, no specific reset needed, use current time
		resetTime = now
	} else {
		// When denied, calculate when bucket will leak enough to allow a request
		resetTime = calculateLBResetTime(now, bucket, leakyConfig.Capacity)
	}

	return Result{
		Allowed:   allowed,
		Remaining: remaining,
		Reset:     resetTime,
	}, nil
}
