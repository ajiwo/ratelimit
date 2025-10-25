package tokenbucket

import (
	"context"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

// TokenBucket represents the state of a token bucket
type TokenBucket struct {
	Tokens     float64   `json:"tokens"`
	LastRefill time.Time `json:"last_refill"`
}

// builderPool reduces allocations in string operations for token bucket strategy
var builderPool = sync.Pool{
	New: func() any {
		return &strings.Builder{}
	},
}

// Strategy implements the token bucket rate limiting algorithm
type Strategy struct {
	storage backends.Backend
}

func New(storage backends.Backend) *Strategy {
	return &Strategy{
		storage: storage,
	}
}

// Peek inspects current state without consuming quota
func (t *Strategy) Peek(ctx context.Context, config strategies.StrategyConfig) (map[string]strategies.Result, error) {
	// // Type assert to TokenBucketConfig
	tokenConfig, ok := config.(Config)
	if !ok {
		return nil, ErrInvalidConfig
	}

	now := time.Now()

	// Get current bucket state
	data, err := t.storage.Get(ctx, tokenConfig.Key)
	if err != nil {
		return nil, NewStateRetrievalError(err)
	}

	var bucket TokenBucket
	if data == "" {
		// No existing bucket, return default state
		return map[string]strategies.Result{
			"default": {
				Allowed:   true,
				Remaining: tokenConfig.BurstSize,
				Reset:     now, // Token buckets don't have a reset time, they continuously refill
			},
		}, nil
	}

	// Parse existing bucket state (compact format only)
	if b, ok := decodeTokenBucket(data); ok {
		bucket = b
	} else {
		return nil, ErrStateParsing
	}

	// Refill tokens based on time elapsed using config values
	timeElapsed := now.Sub(bucket.LastRefill).Seconds()
	tokensToAdd := timeElapsed * tokenConfig.RefillRate
	bucket.Tokens = math.Min(bucket.Tokens+tokensToAdd, float64(tokenConfig.BurstSize))
	bucket.LastRefill = now

	// Calculate remaining requests (floor of available tokens)
	remaining := max(int(bucket.Tokens), 0)

	return map[string]strategies.Result{
		"default": {
			Allowed:   remaining > 0,
			Remaining: remaining,
			Reset:     now, // Token buckets don't have a reset time
		},
	}, nil
}

// Reset resets the token bucket counter for the given key
func (t *Strategy) Reset(ctx context.Context, config strategies.StrategyConfig) error {
	// Type assert to TokenBucketConfig
	tokenConfig, ok := config.(Config)
	if !ok {
		return ErrInvalidConfig
	}

	// Delete the key from storage to reset the bucket
	return t.storage.Delete(ctx, tokenConfig.Key)
}

// encodeTokenBucket serializes TokenBucket into a compact ASCII format:
// v2|tokens|lastrefill_unix_nano
func encodeTokenBucket(b TokenBucket) string {
	sb := builderPool.Get().(*strings.Builder)
	defer func() {
		sb.Reset()
		builderPool.Put(sb)
	}()

	// rough capacity
	sb.Grow(2 + 1 + 24 + 1 + 20)
	sb.WriteString("v2|")
	sb.WriteString(strconv.FormatFloat(b.Tokens, 'g', -1, 64))
	sb.WriteByte('|')
	sb.WriteString(strconv.FormatInt(b.LastRefill.UnixNano(), 10))
	return sb.String()
}

// parseTokenBucketFields parses the fields from a token bucket string representation
func parseTokenBucketFields(data string) (float64, int64, bool) {
	// Parse tokens (first field)
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

	// Parse last refill (second field)
	last, err2 := strconv.ParseInt(data[pos1+1:], 10, 64)
	if err2 != nil {
		return 0, 0, false
	}

	return tokens, last, true
}

// decodeTokenBucket deserializes from compact format; returns ok=false if not compact.
func decodeTokenBucket(s string) (TokenBucket, bool) {
	if !strategies.CheckV2Header(s) {
		return TokenBucket{}, false
	}

	data := s[3:] // Skip "v2|"

	tokens, last, ok := parseTokenBucketFields(data)
	if !ok {
		return TokenBucket{}, false
	}

	return TokenBucket{
		Tokens:     tokens,
		LastRefill: time.Unix(0, last),
	}, true
}

// calculateTBResetTime calculates when the bucket will have at least one full token
func calculateTBResetTime(now time.Time, bucket TokenBucket, refillRate float64) time.Time {
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
	timeToRefillSeconds := tokensNeeded / refillRate
	return now.Add(time.Duration(timeToRefillSeconds * float64(time.Second)))
}

// Allow checks if a request is allowed and returns detailed statistics
func (t *Strategy) Allow(ctx context.Context, config strategies.StrategyConfig) (map[string]strategies.Result, error) {
	// // Type assert to TokenBucketConfig
	tokenConfig, ok := config.(Config)
	if !ok {
		return nil, ErrInvalidConfig
	}

	capacity := float64(tokenConfig.BurstSize)
	refillRate := tokenConfig.RefillRate

	now := time.Now()

	// Try atomic CheckAndSet operations first
	for attempt := range strategies.CheckAndSetRetries {
		// Check if context is cancelled or timed out
		if ctx.Err() != nil {
			return nil, NewContextCancelledError(ctx.Err())
		}

		// Get current bucket state
		data, err := t.storage.Get(ctx, tokenConfig.Key)
		if err != nil {
			return nil, NewStateRetrievalError(err)
		}

		var bucket TokenBucket
		var oldValue any
		if data == "" {
			// Initialize new bucket
			bucket = TokenBucket{
				Tokens:     capacity,
				LastRefill: now,
			}
			oldValue = nil // Key doesn't exist
		} else {
			// Parse existing bucket state (compact format only)
			if b, ok := decodeTokenBucket(data); ok {
				bucket = b
			} else {
				return nil, ErrStateParsing
			}
			oldValue = data // Current value for CheckAndSet

			// Refill tokens based on elapsed time using config values
			elapsed := now.Sub(bucket.LastRefill)
			tokensToAdd := float64(elapsed.Nanoseconds()) * refillRate / 1e9
			bucket.Tokens = math.Min(bucket.Tokens+tokensToAdd, capacity)
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
			expiration := strategies.CalcExpiration(tokenConfig.BurstSize, tokenConfig.RefillRate)

			// Use CheckAndSet for atomic update
			success, err := t.storage.CheckAndSet(ctx, tokenConfig.Key, oldValue, newValue, expiration)
			if err != nil {
				return nil, NewStateSaveError(err)
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
				time.Sleep((19 * time.Nanosecond) << (time.Duration(attempt)))
				continue
			}
			break
		} else {
			// Request denied, return current remaining count
			remaining := max(int(bucket.Tokens), 0)

			// Save the current bucket state (even when denying, to handle token refills)
			bucketData := encodeTokenBucket(bucket)
			expiration := strategies.CalcExpiration(tokenConfig.BurstSize, tokenConfig.RefillRate)

			// If this was a new bucket (rare case), set it
			if oldValue == nil {
				_, err := t.storage.CheckAndSet(ctx, tokenConfig.Key, oldValue, bucketData, expiration)
				if err != nil {
					return nil, NewStateSaveError(err)
				}
			}

			return map[string]strategies.Result{
				"default": {
					Allowed:   false,
					Remaining: remaining,
					Reset:     calculateTBResetTime(now, bucket, refillRate),
				},
			}, nil
		}
	}

	// CheckAndSet failed after checkAndSetRetries attempts
	return nil, ErrConcurrentAccess
}
