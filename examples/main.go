package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ajiwo/ratelimit"
	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

func main() {
	// Example 1: Token Bucket with Memory Backend
	fmt.Println("=== Token Bucket with Memory Backend ===")
	if err := tokenBucketExample(); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	// Example 2: Fixed Window with Memory Backend
	fmt.Println("\n=== Fixed Window with Memory Backend ===")
	if err := fixedWindowExample(); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	// Example 3: Token Bucket with Redis Backend
	fmt.Println("\n=== Token Bucket with Redis Backend ===")
	if err := tokenBucketRedisExample(); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	// Example 4: Get rate limit status
	fmt.Println("\n=== Getting Rate Limit Status ===")
	if err := statusExample(); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	// Example 5: Custom cleanup interval
	fmt.Println("\n=== Custom Cleanup Interval ===")
	if err := customCleanupExample(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

func tokenBucketExample() error {
	// Create memory backend
	backend := backends.NewMemoryStorage()

	// Create token bucket strategy
	strategy := strategies.NewTokenBucket(backend)

	// Test rate limiting
	userID := "user:token_bucket:123"
	config := strategies.TokenBucketConfig{
		RateLimitConfig: strategies.RateLimitConfig{
			Key:   userID,
			Limit: 20,
		},
		BurstSize:  20,
		RefillRate: 5.0, // 5 tokens per second
	}

	for i := range 25 {
		allowed, err := strategy.Allow(context.Background(), config)
		if err != nil {
			return err
		}

		result, err := strategy.GetResult(context.Background(), config)
		if err != nil {
			return err
		}

		if allowed {
			fmt.Printf("Request %d: ALLOWED (remaining: %d)\n", i+1, result.Remaining)
		} else {
			fmt.Printf("Request %d: BLOCKED\n", i+1)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

func fixedWindowExample() error {
	// Create memory backend
	backend := backends.NewMemoryStorage()

	// Create fixed window strategy
	strategy := strategies.NewFixedWindow(backend)

	// Test rate limiting
	userID := "user:fixed_window:456"
	config := strategies.FixedWindowConfig{
		RateLimitConfig: strategies.RateLimitConfig{
			Key:   userID,
			Limit: 10,
		},
		Window: 10 * time.Second, // Short window for demo
	}

	for i := range 15 {
		allowed, err := strategy.Allow(context.Background(), config)
		if err != nil {
			return err
		}

		result, err := strategy.GetResult(context.Background(), config)
		if err != nil {
			return err
		}

		if allowed {
			fmt.Printf("Request %d: ALLOWED (remaining: %d)\n", i+1, result.Remaining)
		} else {
			fmt.Printf("Request %d: BLOCKED\n", i+1)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

func tokenBucketRedisExample() error {
	// Get Redis address from environment variable
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379" // Default Redis address
	}

	// Create Redis backend
	redisConfig := backends.RedisConfig{
		Addr:     redisAddr,
		Password: "", // Assuming no password for local Redis
		DB:       0,  // Default DB
		PoolSize: 10,
	}

	backend, err := backends.NewRedisStorage(redisConfig)
	if err != nil {
		// If Redis is not available, skip this example
		fmt.Println("Redis not available, skipping example")
		return nil
	}

	defer func() {
		if err := backend.Close(); err != nil {
			fmt.Printf("Error closing Redis backend: %v\n", err)
		}
	}()

	// Create token bucket strategy
	strategy := strategies.NewTokenBucket(backend)

	// Test rate limiting
	userID := "user:token_bucket:redis:789"
	config := strategies.TokenBucketConfig{
		RateLimitConfig: strategies.RateLimitConfig{
			Key:   userID,
			Limit: 15,
		},
		BurstSize:  15,
		RefillRate: 3.0, // 3 tokens per second
	}

	for i := range 20 {
		// Reset the bucket for each request to avoid conflicts with previous runs
		if i == 0 {
			err := strategy.Reset(context.Background(), config)
			if err != nil {
				fmt.Printf("Error in Reset: %v\n", err)
				// Continue with the example even if there's an error
				continue
			}
		}

		allowed, err := strategy.Allow(context.Background(), config)
		if err != nil {
			fmt.Printf("Error in Allow: %v\n", err)
			// Continue with the example even if there's an error
			fmt.Printf("Request %d: ERROR\n", i+1)
			continue
		}

		result, err := strategy.GetResult(context.Background(), config)
		if err != nil {
			fmt.Printf("Error in GetResult: %v\n", err)
			// Continue with the example even if there's an error
			fmt.Printf("Request %d: ERROR\n", i+1)
			continue
		}

		if allowed {
			fmt.Printf("Request %d: ALLOWED (remaining: %d)\n", i+1, result.Remaining)
		} else {
			fmt.Printf("Request %d: BLOCKED\n", i+1)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil
}

func statusExample() error {
	// Create memory backend
	backend := backends.NewMemoryStorage()

	// Create token bucket strategy
	strategy := strategies.NewTokenBucket(backend)

	userID := "user:status:789"
	config := strategies.TokenBucketConfig{
		RateLimitConfig: strategies.RateLimitConfig{
			Key:   userID,
			Limit: 10,
		},
		BurstSize:  10,
		RefillRate: 2.0, // 2 tokens per second
	}

	// Reset the bucket to start fresh
	err := strategy.Reset(context.Background(), config)
	if err != nil {
		return err
	}

	// Make some requests to consume tokens
	for i := range 5 {
		allowed, err := strategy.Allow(context.Background(), config)
		if err != nil {
			return err
		}

		result, err := strategy.GetResult(context.Background(), config)
		if err != nil {
			return err
		}

		fmt.Printf("Made request %d: allowed=%v, remaining=%d\n", i+1, allowed, result.Remaining)
		time.Sleep(200 * time.Millisecond)
	}

	// Get current status
	result, err := strategy.GetResult(context.Background(), config)
	if err != nil {
		return err
	}

	fmt.Printf("\nCurrent Status:\n")
	fmt.Printf("  Allowed: %v\n", result.Allowed)
	fmt.Printf("  Remaining: %d\n", result.Remaining)
	fmt.Printf("  Reset Time: %v\n", result.Reset)
	return nil
}

func customCleanupExample() error {
	// Create a rate limiter with a custom cleanup interval
	limiter, err := ratelimit.New(
		ratelimit.WithMemoryBackend(),
		ratelimit.WithFixedWindowStrategy(
			ratelimit.TierConfig{
				Interval: time.Second * 30, // 30 second window
				Limit:    50,               // 50 requests per window
			},
		),
		ratelimit.WithCleanupInterval(time.Minute*5), // Cleanup every 5 minutes
		ratelimit.WithBaseKey("user:custom_cleanup:123"),
	)
	if err != nil {
		return err
	}
	defer func() {
		_ = limiter.Close()
	}()

	fmt.Println("Rate limiter created with 5-minute cleanup interval")
	fmt.Println("Making requests...")

	// Make a few requests
	for i := range 10 {
		allowed, err := limiter.Allow(context.Background())
		if err != nil {
			return err
		}

		stats, err := limiter.GetStats(context.Background())
		if err != nil {
			return err
		}

		if allowed {
			fmt.Printf("Request %d: ALLOWED\n", i+1)
		} else {
			fmt.Printf("Request %d: BLOCKED\n", i+1)
		}

		// Print stats for the first tier
		for tierName, tierStats := range stats {
			fmt.Printf("  Tier %s: remaining=%d, used=%d\n", tierName, tierStats.Remaining, tierStats.Used)
			break // Just show the first tier for brevity
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil
}
