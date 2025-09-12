package strategies

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

// TokenBucketStrategy implements the token bucket rate limiting algorithm
type TokenBucketStrategy struct {
	refillRate   time.Duration // How often to refill tokens
	refillAmount int64         // Number of tokens to add per refill
	bucketSize   int64         // Maximum number of tokens in bucket
}

// NewTokenBucketStrategy creates a new token bucket strategy
func NewTokenBucketStrategy(config StrategyConfig) (*TokenBucketStrategy, error) {
	refillRate := config.RefillRate
	if refillRate == 0 {
		refillRate = time.Minute // Default: refill every minute
	}

	refillAmount := config.RefillAmount
	if refillAmount == 0 {
		refillAmount = 1 // Default: add 1 token per refill
	}

	bucketSize := config.BucketSize
	if bucketSize == 0 {
		bucketSize = 1 // Default: bucket size of 1
	}

	return &TokenBucketStrategy{
		refillRate:   refillRate,
		refillAmount: refillAmount,
		bucketSize:   bucketSize,
	}, nil
}

// Allow checks if a request should be allowed using the token bucket algorithm
func (tb *TokenBucketStrategy) Allow(ctx context.Context, backend backends.Backend, key string, limit int64, window time.Duration) (*StrategyResult, error) {
	now := time.Now()

	// Determine effective bucket size (use configured size or fall back to limit)
	effectiveBucketSize := tb.bucketSize
	if effectiveBucketSize <= 1 && limit > 1 {
		effectiveBucketSize = limit
	}

	// Use atomic check-and-consume-token to avoid race conditions
	currentTokens, consumed, totalRequests, err := backend.ConsumeToken(
		ctx, key, effectiveBucketSize, tb.refillRate, tb.refillAmount, window)
	if err != nil {
		return nil, fmt.Errorf("failed to check and consume token: %w", err)
	}

	// Request is allowed if we successfully consumed a token
	allowed := consumed

	// Calculate when the bucket will have tokens again
	var resetTime time.Time
	var retryAfter time.Duration

	if !allowed {
		// Calculate when next token will be available
		timeToNextToken := tb.calculateTimeToNextToken(currentTokens)
		resetTime = now.Add(timeToNextToken)
		retryAfter = timeToNextToken
	} else {
		// If we have tokens, reset time is when bucket will be full
		tokensNeeded := float64(effectiveBucketSize) - currentTokens
		timeToFull := time.Duration(tokensNeeded/float64(tb.refillAmount)) * tb.refillRate
		resetTime = now.Add(timeToFull)
		retryAfter = 0
	}

	return &StrategyResult{
		Allowed:       allowed,
		Remaining:     int64(math.Floor(currentTokens)),
		ResetTime:     resetTime,
		RetryAfter:    retryAfter,
		TotalRequests: totalRequests,
		WindowStart:   now.Add(-window), // Rolling window
	}, nil
}

// GetStatus returns the current status of the token bucket
func (tb *TokenBucketStrategy) GetStatus(ctx context.Context, backend backends.Backend, key string, limit int64, window time.Duration) (*StrategyStatus, error) {
	now := time.Now()

	// Determine effective bucket size (use configured size or fall back to limit)
	effectiveBucketSize := tb.bucketSize
	if effectiveBucketSize <= 1 && limit > 1 {
		effectiveBucketSize = limit
	}

	// Get current bucket state
	data, err := backend.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket state: %w", err)
	}

	// If bucket doesn't exist, return full bucket status
	if data == nil {
		return &StrategyStatus{
			Limit:          effectiveBucketSize,
			Remaining:      effectiveBucketSize,
			ResetTime:      now.Add(window),
			WindowStart:    now.Add(-window),
			TotalRequests:  0,
			WindowDuration: window,
		}, nil
	}

	// Calculate current tokens
	tokensToAdd := tb.calculateTokensToAdd(data.LastRefill, now)
	currentTokens := math.Min(data.Tokens+tokensToAdd, float64(effectiveBucketSize))

	// Calculate reset time (when bucket will be full)
	tokensNeeded := float64(effectiveBucketSize) - currentTokens
	timeToFull := time.Duration(tokensNeeded/float64(tb.refillAmount)) * tb.refillRate
	resetTime := now.Add(timeToFull)

	return &StrategyStatus{
		Limit:          effectiveBucketSize,
		Remaining:      int64(math.Floor(currentTokens)),
		ResetTime:      resetTime,
		WindowStart:    data.WindowStart,
		TotalRequests:  data.Count,
		WindowDuration: window,
	}, nil
}

// Reset clears the token bucket state for the given key
func (tb *TokenBucketStrategy) Reset(ctx context.Context, backend backends.Backend, key string) error {
	return backend.Delete(ctx, key)
}

// Name returns the name of the strategy
func (tb *TokenBucketStrategy) Name() string {
	return "token_bucket"
}

// calculateTokensToAdd calculates how many tokens to add based on elapsed time
func (tb *TokenBucketStrategy) calculateTokensToAdd(lastRefill, now time.Time) float64 {
	elapsed := now.Sub(lastRefill)

	// Calculate how many refill periods have passed
	refillPeriods := float64(elapsed) / float64(tb.refillRate)

	// Calculate tokens to add
	tokensToAdd := refillPeriods * float64(tb.refillAmount)

	return tokensToAdd
}

// calculateTimeToNextToken calculates when the next token will be available
func (tb *TokenBucketStrategy) calculateTimeToNextToken(currentTokens float64) time.Duration {
	if currentTokens >= 1.0 {
		return 0 // Already have tokens
	}

	// Calculate fraction of refill period needed for next token
	tokensNeeded := 1.0 - currentTokens
	fractionNeeded := tokensNeeded / float64(tb.refillAmount)

	return time.Duration(fractionNeeded * float64(tb.refillRate))
}

// GetBucketInfo returns detailed information about the token bucket configuration
func (tb *TokenBucketStrategy) GetBucketInfo() TokenBucketInfo {
	return TokenBucketInfo{
		RefillRate:   tb.refillRate,
		RefillAmount: tb.refillAmount,
		BucketSize:   tb.bucketSize,
	}
}

// TokenBucketInfo holds information about token bucket configuration
type TokenBucketInfo struct {
	RefillRate   time.Duration
	RefillAmount int64
	BucketSize   int64
}

// SetRefillRate updates the refill rate (use with caution in production)
func (tb *TokenBucketStrategy) SetRefillRate(rate time.Duration) {
	if rate > 0 {
		tb.refillRate = rate
	}
}

// SetRefillAmount updates the refill amount (use with caution in production)
func (tb *TokenBucketStrategy) SetRefillAmount(amount int64) {
	if amount > 0 {
		tb.refillAmount = amount
	}
}

// CalculateOptimalRefillRate calculates optimal refill rate for given requirements
func CalculateOptimalRefillRate(requestsPerMinute int64, burstSize int64) (time.Duration, int64) {
	// For smooth operation, refill more frequently with smaller amounts
	// This provides better burst handling and more accurate rate limiting

	if requestsPerMinute <= 0 {
		return time.Minute, 1
	}

	// Consider burst size when determining refill strategy
	// Aim for refills every few seconds for responsive rate limiting
	maxRefillsPerMinute := int64(12) // Maximum 12 refills per minute (every 5 seconds)

	// Calculate target refills per minute based on requests per minute and burst size
	targetRefillsPerMinute := min(requestsPerMinute, maxRefillsPerMinute)
	refillRate := time.Minute / time.Duration(targetRefillsPerMinute)
	refillAmount := requestsPerMinute / targetRefillsPerMinute

	// Ensure we refill at least 1 token per period
	if refillAmount == 0 {
		refillAmount = 1
		refillRate = time.Minute / time.Duration(requestsPerMinute)
	}

	return refillRate, refillAmount
}
