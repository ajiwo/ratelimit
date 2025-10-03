package strategies

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

// TokenBucket represents the state of a token bucket
type TokenBucket struct {
	Tokens     float64   `json:"tokens"`
	LastRefill time.Time `json:"last_refill"`
	Capacity   float64   `json:"capacity"`
	RefillRate float64   `json:"refill_rate"` // tokens per second
}

// TokenBucketStrategy implements the token bucket rate limiting algorithm
type TokenBucketStrategy struct {
	BaseStrategy
}

func NewTokenBucket(storage backends.Backend) *TokenBucketStrategy {
	return &TokenBucketStrategy{
		BaseStrategy: BaseStrategy{
			storage: storage,
		},
	}
}

// Allow checks if a request is allowed based on token bucket algorithm
//
// Deprecated: Use AllowWithResult instead. Allow will be removed in a future release.
func (t *TokenBucketStrategy) Allow(ctx context.Context, config any) (bool, error) {
	result, err := t.AllowWithResult(ctx, config)
	return result.Allowed, err
}

// GetResult returns detailed statistics for the current bucket state
func (t *TokenBucketStrategy) GetResult(ctx context.Context, config any) (Result, error) {
	// Type assert to TokenBucketConfig
	tokenConfig, ok := config.(TokenBucketConfig)
	if !ok {
		return Result{}, fmt.Errorf("TokenBucket strategy requires TokenBucketConfig")
	}

	// Get per-key lock to prevent concurrent access to the same bucket
	lock := t.getLock(tokenConfig.Key)
	lock.Lock()
	defer lock.Unlock()

	now := time.Now()

	// Get current bucket state
	data, err := t.storage.Get(ctx, tokenConfig.Key)
	if err != nil {
		return Result{}, fmt.Errorf("failed to get bucket state: %w", err)
	}

	var bucket TokenBucket
	if data == "" {
		// No existing bucket, return default state
		return Result{
			Allowed:   true,
			Remaining: tokenConfig.BurstSize,
			Reset:     now, // Token buckets don't have a reset time, they continuously refill
		}, nil
	}

	// Parse existing bucket state (compact format only)
	if b, ok := decodeTokenBucket(data); ok {
		bucket = b
	} else {
		return Result{}, fmt.Errorf("failed to parse bucket state: invalid encoding")
	}

	// Refill tokens based on time elapsed
	timeElapsed := now.Sub(bucket.LastRefill).Seconds()
	tokensToAdd := timeElapsed * bucket.RefillRate
	bucket.Tokens = math.Min(bucket.Tokens+tokensToAdd, bucket.Capacity)
	bucket.LastRefill = now

	// Calculate remaining requests (floor of available tokens)
	remaining := max(int(bucket.Tokens), 0)

	return Result{
		Allowed:   remaining > 0,
		Remaining: remaining,
		Reset:     now, // Token buckets don't have a reset time
	}, nil
}

// Reset resets the token bucket counter for the given key
func (t *TokenBucketStrategy) Reset(ctx context.Context, config any) error {
	// Type assert to TokenBucketConfig
	tokenConfig, ok := config.(TokenBucketConfig)
	if !ok {
		return fmt.Errorf("TokenBucket strategy requires TokenBucketConfig")
	}

	// Get per-key lock to prevent concurrent access to the same bucket
	lock := t.getLock(tokenConfig.Key)
	lock.Lock()
	defer lock.Unlock()

	// Delete the key from storage to reset the bucket
	return t.storage.Delete(ctx, tokenConfig.Key)
}

// encodeTokenBucket serializes TokenBucket into a compact ASCII format:
// v1|tokens|lastrefill_unix_nano|capacity|refill_rate
func encodeTokenBucket(b TokenBucket) string {
	var sb strings.Builder
	// rough capacity
	sb.Grow(2 + 1 + 24 + 1 + 20 + 1 + 24 + 1 + 24)
	sb.WriteString("v1|")
	sb.WriteString(strconv.FormatFloat(b.Tokens, 'g', -1, 64))
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatInt(b.LastRefill.UnixNano(), 10))
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatFloat(b.Capacity, 'g', -1, 64))
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatFloat(b.RefillRate, 'g', -1, 64))
	return sb.String()
}

// parseTokenBucketFields parses the fields from a token bucket string representation
func parseTokenBucketFields(data string) (float64, int64, float64, float64, bool) {
	// Parse tokens (first field)
	pos1 := 0
	for pos1 < len(data) && data[pos1] != '|' {
		pos1++
	}
	if pos1 == len(data) {
		return 0, 0, 0, 0, false
	}

	tokens, err1 := strconv.ParseFloat(data[:pos1], 64)
	if err1 != nil {
		return 0, 0, 0, 0, false
	}

	// Parse last refill (second field)
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

	// Parse refill rate (fourth field)
	rrate, err4 := strconv.ParseFloat(data[pos3+1:], 64)
	if err4 != nil {
		return 0, 0, 0, 0, false
	}

	return tokens, last, capf, rrate, true
}

// decodeTokenBucket deserializes from compact format; returns ok=false if not compact.
func decodeTokenBucket(s string) (TokenBucket, bool) {
	if !checkV1Header(s) {
		return TokenBucket{}, false
	}

	data := s[3:] // Skip "v1|"

	tokens, last, capf, rrate, ok := parseTokenBucketFields(data)
	if !ok {
		return TokenBucket{}, false
	}

	return TokenBucket{
		Tokens:     tokens,
		LastRefill: time.Unix(0, last),
		Capacity:   capf,
		RefillRate: rrate,
	}, true
}

// Cleanup removes stale locks
func (t *TokenBucketStrategy) Cleanup(maxAge time.Duration) {
	t.CleanupLocks(maxAge)
}

// allowGMS checks if a request is allowed using the Get-Modify-Set pattern with locks.
// This is the fallback method when CheckAndSet atomic operations fail after multiple retries.
func (t *TokenBucketStrategy) allowGMS(ctx context.Context, config any) (Result, error) {
	// Type assert to TokenBucketConfig
	tokenConfig, ok := config.(TokenBucketConfig)
	if !ok {
		return Result{}, fmt.Errorf("TokenBucket strategy requires TokenBucketConfig")
	}

	// Get per-key lock to prevent concurrent access to the same bucket
	lock := t.getLock(tokenConfig.Key)
	lock.Lock()
	defer lock.Unlock()

	capacity := float64(tokenConfig.BurstSize)
	refillRate := tokenConfig.RefillRate

	now := time.Now()

	// Get current bucket state
	data, err := t.storage.Get(ctx, tokenConfig.Key)
	if err != nil {
		return Result{}, fmt.Errorf("failed to get bucket state: %w", err)
	}

	var bucket TokenBucket
	if data == "" {
		// Initialize new bucket
		bucket = TokenBucket{
			Tokens:     capacity,
			LastRefill: now,
			Capacity:   capacity,
			RefillRate: refillRate,
		}
	} else {
		// Parse existing bucket state (compact format only)
		if b, ok := decodeTokenBucket(data); ok {
			bucket = b
		} else {
			return Result{}, fmt.Errorf("failed to parse bucket state: invalid encoding")
		}

		// Refill tokens based on elapsed time
		elapsed := now.Sub(bucket.LastRefill)
		tokensToAdd := float64(elapsed.Nanoseconds()) * bucket.RefillRate / 1e9
		bucket.Tokens = math.Min(bucket.Tokens+tokensToAdd, bucket.Capacity)
		bucket.LastRefill = now
	}

	// Determine if request is allowed based on available tokens
	allowed := math.Floor(bucket.Tokens) >= 1.0

	// Calculate remaining tokens
	remaining := max(int(bucket.Tokens), 0)

	// Save updated bucket state in compact format
	bucketData := encodeTokenBucket(bucket)
	expiration := calcExpiration(tokenConfig.BurstSize, tokenConfig.RefillRate)

	// Save the bucket state
	err = t.storage.Set(ctx, tokenConfig.Key, bucketData, expiration)
	if err != nil {
		return Result{}, fmt.Errorf("failed to save bucket state: %w", err)
	}

	// Calculate reset time - when bucket will have at least one full token
	var resetTime time.Time
	if allowed {
		// When allowed, no specific reset needed, use current time
		resetTime = now
	} else {
		// When denied, calculate when bucket will have at least 1 token
		resetTime = calculateTBResetTime(now, bucket)
	}

	return Result{
		Allowed:   allowed,
		Remaining: remaining,
		Reset:     resetTime,
	}, nil
}

// calculateTBResetTime calculates when the bucket will have at least one full token
func calculateTBResetTime(now time.Time, bucket TokenBucket) time.Time {
	if bucket.Tokens >= 1.0 {
		// Already has tokens, no reset needed
		return now
	}

	// Calculate time to refill to at least 1 token
	tokensNeeded := 1.0 - bucket.Tokens
	if tokensNeeded <= 0 {
		return now
	}

	// time = tokensNeeded / refillRate (convert from float seconds to time.Duration)
	timeToRefillSeconds := tokensNeeded / bucket.RefillRate
	return now.Add(time.Duration(timeToRefillSeconds * float64(time.Second)))
}

// AllowWithResult checks if a request is allowed and returns detailed statistics
func (t *TokenBucketStrategy) AllowWithResult(ctx context.Context, config any) (Result, error) {
	// Type assert to TokenBucketConfig
	tokenConfig, ok := config.(TokenBucketConfig)
	if !ok {
		return Result{}, fmt.Errorf("TokenBucket strategy requires TokenBucketConfig")
	}

	capacity := float64(tokenConfig.BurstSize)
	refillRate := tokenConfig.RefillRate

	now := time.Now()

	// Try atomic CheckAndSet operations first
	for attempt := range checkAndSetRetries {
		// Get current bucket state
		data, err := t.storage.Get(ctx, tokenConfig.Key)
		if err != nil {
			return Result{}, fmt.Errorf("failed to get bucket state: %w", err)
		}

		var bucket TokenBucket
		var oldValue any
		if data == "" {
			// Initialize new bucket
			bucket = TokenBucket{
				Tokens:     capacity,
				LastRefill: now,
				Capacity:   capacity,
				RefillRate: refillRate,
			}
			oldValue = nil // Key doesn't exist
		} else {
			// Parse existing bucket state (compact format only)
			if b, ok := decodeTokenBucket(data); ok {
				bucket = b
			} else {
				return Result{}, fmt.Errorf("failed to parse bucket state: invalid encoding")
			}
			oldValue = data // Current value for CheckAndSet

			// Refill tokens based on elapsed time
			elapsed := now.Sub(bucket.LastRefill)
			tokensToAdd := float64(elapsed.Nanoseconds()) * bucket.RefillRate / 1e9
			bucket.Tokens = math.Min(bucket.Tokens+tokensToAdd, bucket.Capacity)
			bucket.LastRefill = now
		}

		// Determine if request is allowed based on available tokens
		allowed := math.Floor(bucket.Tokens) >= 1.0

		if allowed {
			// Consume one token
			bucket.Tokens -= 1.0

			// Calculate remaining tokens after consumption
			remaining := max(int(bucket.Tokens), 0)

			// Save updated bucket state in compact format
			newValue := encodeTokenBucket(bucket)

			// Use a reasonable expiration time (based on burst size and refill rate)
			expiration := calcExpiration(tokenConfig.BurstSize, tokenConfig.RefillRate)

			// Use CheckAndSet for atomic update
			success, err := t.storage.CheckAndSet(ctx, tokenConfig.Key, oldValue, newValue, expiration)
			if err != nil {
				return Result{}, fmt.Errorf("failed to save bucket state: %w", err)
			}

			if success {
				// Atomic update succeeded
				return Result{
					Allowed:   true,
					Remaining: remaining,
					Reset:     now, // When allowed, no specific reset needed
				}, nil
			}
			// If CheckAndSet failed, retry if we haven't exhausted attempts
			if attempt < checkAndSetRetries-1 {
				continue
			}
			// Exhausted attempts, fall back to lock-based approach
			break
		} else {
			// Request denied, return current remaining count
			remaining := max(int(bucket.Tokens), 0)

			// Save the current bucket state (even when denying, to handle token refills)
			bucketData := encodeTokenBucket(bucket)
			expiration := calcExpiration(tokenConfig.BurstSize, tokenConfig.RefillRate)

			// If this was a new bucket (rare case), set it
			if oldValue == nil {
				_, err := t.storage.CheckAndSet(ctx, tokenConfig.Key, oldValue, bucketData, expiration)
				if err != nil {
					return Result{}, fmt.Errorf("failed to initialize bucket state: %w", err)
				}
			}

			return Result{
				Allowed:   false,
				Remaining: remaining,
				Reset:     calculateTBResetTime(now, bucket),
			}, nil
		}
	}

	// CheckAndSet failed after checkAndSetRetries attempts, fall back to lock-based approach
	return t.allowGMS(ctx, config)
}
